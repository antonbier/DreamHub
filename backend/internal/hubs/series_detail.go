package hubs

import (
	"context"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	"github.com/antonbier/DreamHub/backend/internal/titles"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// seriesDetail is GET /api/v1/series/markers/{id}: one tracked series with
// everything DreamHub can say about it — SerienRecorder's own marker and
// its timered episodes, the series' recordings on disk, and further
// broadcasts in the box's EPG that nothing records yet. Each part comes
// from a different source and fails on its own (an unreachable WebControl
// still leaves the SerienRecorder parts), reported per part rather than
// failing the whole view.
type seriesDetail struct {
	Marker     serienrecorder.Marker            `json:"marker"`
	Upcoming   []serienrecorder.UpcomingEpisode `json:"upcoming"`
	Recordings []seriesRecording                `json:"recordings"`
	Broadcasts []seriesBroadcast                `json:"broadcasts"`
	// Episodes is every episode TVmaze knows, with what's recorded,
	// planned, missing or still to come (series_episodes.go).
	Episodes episodeGuide `json:"episodes"`
	// Unavailable names the parts that couldn't be loaded ("upcoming",
	// "recordings", "broadcasts"), so the screen can say so instead of
	// showing a misleading empty list.
	Unavailable []string `json:"unavailable"`
}

type seriesRecording struct {
	Path        string `json:"path"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ServiceName string `json:"serviceName"`
	Time        int64  `json:"time"` // Unix seconds, UTC
	FileSize    int64  `json:"fileSize"`
	// Season/Episode are parsed from an "S01E02"-style code in the title or
	// description, 0 when there is none — never from the file name.
	Season  int `json:"season"`
	Episode int `json:"episode"`
}

type seriesBroadcast struct {
	webcontrol.EPGEvent
	// HasTimer is true when a timer on the same channel already covers the
	// broadcast (SerienRecorder's own, or any other).
	HasTimer bool `json:"hasTimer"`
}

func (h *Series) detail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "marker id must be a number")
		return
	}
	markers, err := h.SR.Markers(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "serienrecorder_unavailable", "could not reach SerienRecorder")
		return
	}
	var marker *serienrecorder.Marker
	for i := range markers {
		if markers[i].ID == id {
			marker = &markers[i]
			break
		}
	}
	if marker == nil {
		httpx.WriteError(w, r, http.StatusNotFound, "series_not_found", "no tracked series with this id")
		return
	}

	out := seriesDetail{Marker: *marker, Upcoming: []serienrecorder.UpcomingEpisode{}, Recordings: []seriesRecording{}, Broadcasts: []seriesBroadcast{}, Unavailable: []string{}}

	if episodes, err := h.SR.UpcomingEpisodes(r.Context()); err != nil {
		out.Unavailable = append(out.Unavailable, "upcoming")
	} else {
		for _, e := range episodes {
			if e.FSID == marker.FSID || (e.FSID == "" && matchesSeries(e.Series, marker.Name)) {
				out.Upcoming = append(out.Upcoming, e)
			}
		}
	}

	if h.WC == nil {
		out.Unavailable = append(out.Unavailable, "recordings", "broadcasts")
	} else {
		if recs, err := h.seriesRecordings(r.Context(), *marker); err != nil {
			out.Unavailable = append(out.Unavailable, "recordings")
		} else {
			out.Recordings = recs
		}
		if casts, err := h.seriesBroadcasts(r.Context(), marker.Name); err != nil {
			out.Unavailable = append(out.Unavailable, "broadcasts")
		} else {
			out.Broadcasts = casts
		}
	}
	out.Episodes = h.episodeGuide(r.Context(), *marker, out.Recordings, out.Upcoming)
	httpx.WriteJSON(w, http.StatusOK, out)
}

// seriesRecordings finds the series' recordings: in the marker's record
// folder (or every recording folder, when SerienRecorder doesn't name one)
// and in a direct subfolder named after the series — SerienRecorder's
// usual per-series folder. A recording counts when its title names the
// series, or when it sits in that series folder. One folder level only, so
// this stays a handful of /web/movielist calls rather than a disk walk.
func (h *Series) seriesRecordings(ctx context.Context, m serienrecorder.Marker) ([]seriesRecording, error) {
	roots := []string{}
	if m.RecordFolder != "" {
		roots = append(roots, m.RecordFolder)
	} else {
		locs, err := h.WC.Locations(ctx)
		if err != nil {
			return nil, err
		}
		roots = locs
	}

	out := []seriesRecording{}
	seen := map[string]bool{}
	add := func(mv webcontrol.Movie) {
		if mv.Filename == "" || seen[mv.Filename] {
			return
		}
		seen[mv.Filename] = true
		season, episode := parseEpisodeCode(mv.Title + " " + mv.Description)
		out = append(out, seriesRecording{
			Path: mv.Filename, Title: mv.Title, Description: mv.Description, ServiceName: mv.ServiceName,
			Time: int64(mv.Time), FileSize: int64(mv.FileSize), Season: season, Episode: episode,
		})
	}

	for _, root := range roots {
		folder, err := h.WC.MovieDir(ctx, root)
		if err != nil {
			return nil, err
		}
		for _, mv := range folder.Movies {
			if matchesSeries(mv.Title, m.Name) {
				add(mv)
			}
		}
		for _, sub := range folder.Subfolders {
			if normalizeTitle(lastSegment(sub)) != normalizeTitle(m.Name) {
				continue
			}
			inner, err := h.WC.MovieDir(ctx, sub)
			if err != nil {
				return nil, err
			}
			for _, mv := range inner.Movies {
				add(mv)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time > out[j].Time })
	return out, nil
}

// seriesBroadcasts searches the box's EPG for the series' name and keeps
// the broadcasts that haven't ended yet and really are this series (the
// box's search is a substring match: "Tatort" also finds "Tatort-Reiniger
// Spezial"), flagging the ones a timer already covers.
func (h *Series) seriesBroadcasts(ctx context.Context, name string) ([]seriesBroadcast, error) {
	events, err := h.WC.EPGSearch(ctx, name)
	if err != nil {
		return nil, err
	}
	timers, err := h.WC.TimerList(ctx)
	if err != nil {
		timers = nil // no timer flags is better than no broadcasts
	}
	now := h.now().Unix()
	out := []seriesBroadcast{}
	for _, e := range events {
		if e.Start+e.Duration <= now || !matchesSeries(e.Title, name) {
			continue
		}
		b := seriesBroadcast{EPGEvent: e}
		for _, t := range timers {
			if t.ServiceReference == e.ServiceReference && t.Begin <= e.Start && t.End >= e.Start+e.Duration {
				b.HasTimer = true
				break
			}
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out, nil
}

func matchesSeries(title, series string) bool { return titles.Matches(title, series) }

func normalizeTitle(s string) string { return titles.Normalize(s) }

var episodeCode = regexp.MustCompile(`(?i)\bS(\d{1,4})\s?E(\d{1,4})\b`)

// parseEpisodeCode finds an "S01E02" code (as SerienRecorder writes into
// its timers' descriptions) and returns season and episode, 0/0 if none.
func parseEpisodeCode(s string) (int, int) {
	m := episodeCode.FindStringSubmatch(s)
	if m == nil {
		return 0, 0
	}
	season, _ := strconv.Atoi(m[1])
	episode, _ := strconv.Atoi(m[2])
	return season, episode
}

func lastSegment(path string) string {
	p := strings.TrimSuffix(path, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

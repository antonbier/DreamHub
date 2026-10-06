package hubs

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
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
	// Info is SerienRecorder's own description of the series (German
	// text, genres, cast, years) — shown before anything from TMDB, since
	// it's what the box already keeps.
	Info *serienrecorder.SeriesInfo `json:"info,omitempty"`
	// Cover: a folder.jpg in the series' folder on disk
	// (GET /api/v1/series/markers/{id}/cover).
	Cover bool `json:"cover"`
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
	// AlsoOn names the other channels showing the same broadcast at the
	// same time (regional variants: RTL sends one programme on seven
	// channels — "RTL Bayern", "RTL HH SH", …, on the reference box).
	AlsoOn []string `json:"alsoOn,omitempty"`
	// Season/Episode: which episode this is, when known — from the EPG
	// itself or DreamMediaInfo (CodeSource); EpisodeStatus whether it's
	// recorded, planned or missing (series_broadcast_episodes.go).
	Season        int    `json:"season,omitempty"`
	Episode       int    `json:"episode,omitempty"`
	CodeSource    string `json:"codeSource,omitempty"`
	EpisodeStatus string `json:"episodeStatus,omitempty"`
}

func (h *Series) detail(w http.ResponseWriter, r *http.Request) {
	m, ok := h.markerByID(w, r)
	if !ok {
		return
	}
	marker := &m

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
	out.Info = h.seriesInfo(r.Context(), *marker)
	startYear := 0
	if out.Info != nil {
		startYear = out.Info.StartYear()
	}
	out.Episodes = h.episodeGuide(r.Context(), *marker, startYear, out.Recordings, out.Upcoming)
	h.annotateBroadcasts(r.Context(), *marker, out.Broadcasts, out.Recordings, out.Upcoming, &out.Episodes)
	out.Cover = h.WC != nil && h.seriesCover(r.Context(), *marker) != ""
	httpx.WriteJSON(w, http.StatusOK, out)
}

// seriesRecordings finds the series' recordings. Confirmed on the
// reference box (ARCHITECTURE.md, "How SerienRecorder files recordings"):
// SerienRecorder either gives a series its own record folder
// ("serien/FBI Most Wanted", with "Season 5" subfolders below), or files it
// into a recording folder, often in a subfolder named after the series,
// sometimes with its start year ("Gute Zeiten, schlechte Zeiten (1992)").
// So a recording belongs to the series when it's
//
//   - anywhere below the marker's own record folder, when that isn't one
//     of the box's recording folders (it's the series' folder), or
//   - anywhere below a subfolder named after the series, or
//   - directly in a recording folder with a title that is the series.
//
// On the box the folders are read straight from the file system
// (recfs.Walk); elsewhere through /web/movielist, two folder levels deep.
func (h *Series) seriesRecordings(ctx context.Context, m serienrecorder.Marker) ([]seriesRecording, error) {
	dirs, err := h.seriesDirs(ctx, m)
	if err != nil {
		return nil, err
	}
	onBox, scopes, dedicated := dirs.onBox, dirs.scopes, dirs.dedicated

	out := []seriesRecording{}
	seen := map[string]bool{}
	add := func(p, title, desc, service string, t, size int64) {
		p = path.Clean("/" + strings.TrimLeft(p, "/"))
		if seen[p] {
			return
		}
		seen[p] = true
		season, episode := parseEpisodeCode(title + " " + desc)
		out = append(out, seriesRecording{Path: p, Title: title, Description: desc, ServiceName: service, Time: t, FileSize: size, Season: season, Episode: episode})
	}

	if onBox {
		addAll := func(e recfs.Entry) { add(e.Path, e.Title, e.Description, "", e.Time, e.Size) }
		if dedicated != "" {
			for _, e := range recfs.Walk(h.FS, []string{dedicated}, 4, 5000) {
				addAll(e)
			}
		}
		for _, scope := range scopes {
			for _, e := range recfs.Walk(h.FS, []string{scope}, 0, 5000) { // this folder only
				if matchesSeries(e.Title, m.Name) {
					addAll(e)
				}
			}
			entries, _ := h.FS.ReadDir(scope)
			for _, d := range entries {
				if d.IsDir() && seriesFolder(d.Name(), m.Name) {
					for _, e := range recfs.Walk(h.FS, []string{path.Join(scope, d.Name())}, 4, 5000) {
						addAll(e)
					}
				}
			}
		}
	} else {
		// Off the box: /web/movielist, folder by folder, two levels deep
		// (a series folder and its season folders).
		var list func(dir string, depth int, all bool) error
		list = func(dir string, depth int, all bool) error {
			folder, err := h.WC.MovieDir(ctx, strings.TrimSuffix(dir, "/")+"/")
			if err != nil {
				return err
			}
			for _, mv := range folder.Movies {
				if all || matchesSeries(mv.Title, m.Name) {
					add(mv.Filename, mv.Title, mv.Description, mv.ServiceName, int64(mv.Time), int64(mv.FileSize))
				}
			}
			if depth == 0 {
				return nil
			}
			for _, sub := range folder.Subfolders {
				if all || seriesFolder(lastSegment(sub), m.Name) {
					if err := list(sub, depth-1, true); err != nil {
						return err
					}
				}
			}
			return nil
		}
		if dedicated != "" {
			if err := list(dedicated, 1, true); err != nil {
				return nil, err
			}
		}
		for _, scope := range scopes {
			if err := list(scope, 2, false); err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time > out[j].Time })
	return out, nil
}

type seriesDirsResult struct {
	onBox     bool
	scopes    []string // recording folders to look in for title matches and series folders
	dedicated string   // the marker's own record folder, when it's the series' folder
}

// seriesDirs works out where a series' recordings can be (see
// seriesRecordings).
func (h *Series) seriesDirs(ctx context.Context, m serienrecorder.Marker) (seriesDirsResult, error) {
	locs, err := h.WC.Locations(ctx)
	if err != nil {
		return seriesDirsResult{}, err
	}
	isRoot := func(dir string) bool {
		for _, l := range locs {
			if path.Clean(l) == path.Clean(dir) {
				return true
			}
		}
		return false
	}
	r := seriesDirsResult{onBox: h.FS != nil && recfs.Available(h.FS, locs), scopes: locs}
	if m.RecordFolder != "" {
		rf := path.Clean(m.RecordFolder)
		switch {
		case isRoot(rf):
			r.scopes = []string{rf}
		case !r.onBox:
			r.dedicated = rf // /web/movielist answers an empty list for a missing folder
		default:
			// On the box: only a real folder inside the recording folders
			// (a stale marker may point at one that no longer exists).
			if _, err := recfs.CleanIn(h.FS, rf+"/x", locs); err == nil {
				if fi, err := h.FS.Lstat(rf); err == nil && fi.IsDir() {
					r.dedicated = rf
				}
			}
		}
	}
	return r, nil
}

// seriesCover is the series' own cover art on disk: "folder.jpg" in its
// folder (found on the reference box in SerienRecorder's series folders),
// "" when there's none or DreamHub isn't on the box.
func (h *Series) seriesCover(ctx context.Context, m serienrecorder.Marker) string {
	dirs, err := h.seriesDirs(ctx, m)
	if err != nil || !dirs.onBox {
		return ""
	}
	var candidates []string
	if dirs.dedicated != "" {
		candidates = append(candidates, dirs.dedicated)
	}
	for _, scope := range dirs.scopes {
		entries, _ := h.FS.ReadDir(scope)
		for _, d := range entries {
			if d.IsDir() && seriesFolder(d.Name(), m.Name) {
				candidates = append(candidates, path.Join(scope, d.Name()))
			}
		}
	}
	for _, dir := range candidates {
		for _, name := range []string{"folder.jpg", "folder.png", "poster.jpg"} {
			p := path.Join(dir, name)
			if fi, err := h.FS.Lstat(p); err == nil && fi.Mode().IsRegular() && fi.Size() > 0 && fi.Size() < 8<<20 {
				return p
			}
		}
	}
	return ""
}

// cover serves the series' folder cover (seriesCover), read-only.
func (h *Series) cover(w http.ResponseWriter, r *http.Request) {
	m, ok := h.markerByID(w, r)
	if !ok {
		return
	}
	p := h.seriesCover(r.Context(), m)
	if p == "" {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no cover art here")
		return
	}
	b, err := h.FS.ReadFile(p)
	ct := http.DetectContentType(b)
	if err != nil || (ct != "image/jpeg" && ct != "image/png") {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no cover art here")
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(b)
}

// markerByID answers the request itself when the id is bad or unknown.
func (h *Series) markerByID(w http.ResponseWriter, r *http.Request) (serienrecorder.Marker, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "marker id must be a number")
		return serienrecorder.Marker{}, false
	}
	markers, err := h.SR.Markers(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "serienrecorder_unavailable", "could not reach SerienRecorder")
		return serienrecorder.Marker{}, false
	}
	for _, m := range markers {
		if m.ID == id {
			return m, true
		}
	}
	httpx.WriteError(w, r, http.StatusNotFound, "series_not_found", "no tracked series with this id")
	return serienrecorder.Marker{}, false
}

// seriesInfo is SerienRecorder's own information about the series,
// cached for a week (it's fetched from SerienRecorder's server); nil when
// there's none.
func (h *Series) seriesInfo(ctx context.Context, m serienrecorder.Marker) *serienrecorder.SeriesInfo {
	key := "sr|info|" + m.WLID
	if h.Store != nil {
		if raw, at, ok, err := h.Store.Enrichment(ctx, key); err == nil && ok && h.now().Sub(time.Unix(at, 0)) < 7*24*time.Hour {
			if raw == "" {
				return nil
			}
			var info serienrecorder.SeriesInfo
			if json.Unmarshal([]byte(raw), &info) == nil {
				return &info
			}
		}
	}
	info, err := h.SR.SeriesInfo(ctx, m.WLID, m.FSID)
	if err != nil {
		return nil // not cached: asked again next time
	}
	if h.Store != nil {
		raw := ""
		if info != nil {
			b, _ := json.Marshal(info)
			raw = string(b)
		}
		_ = h.Store.PutEnrichment(ctx, key, raw, h.now().Unix())
	}
	return info
}

// yearSuffix is a folder name's trailing start year: "GZSZ (1992)".
var yearSuffix = regexp.MustCompile(`\s*\(\d{4}\)\s*$`)

// seriesFolder reports whether a folder is named after the series, with or
// without its start year.
func seriesFolder(name, series string) bool {
	return normalizeTitle(yearSuffix.ReplaceAllString(name, "")) == normalizeTitle(series)
}

// seriesBroadcasts searches the box's EPG for the series' name and keeps
// the broadcasts that haven't ended yet and really are this series (the
// box's search is a substring match: "Tatort" also finds "Tatort-Reiniger
// Spezial"), flagging the ones a timer already covers.
func (h *Series) seriesBroadcasts(ctx context.Context, name string) ([]seriesBroadcast, error) {
	if len([]rune(strings.TrimSpace(name))) < webcontrol.MinEPGSearchQuery {
		return []seriesBroadcast{}, nil // see Guide.Search: a short query stalls the box
	}
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
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return mergeSimulcasts(out, seriesChannels(timers, name)), nil
}

// mergeSimulcasts folds broadcasts that are the same programme at the same
// time on several channels into one, listing the others in AlsoOn. The one
// kept is the one a timer covers, else one on a channel the series already
// records from (so "Record" picks the user's usual channel), else the
// first in the box's order.
func mergeSimulcasts(bs []seriesBroadcast, usual map[string]bool) []seriesBroadcast {
	type key struct {
		start, duration int64
		title, desc     string
	}
	rank := func(b seriesBroadcast) int {
		switch {
		case b.HasTimer:
			return 2
		case usual[b.ServiceReference]:
			return 1
		}
		return 0
	}
	at := map[key]int{}
	out := make([]seriesBroadcast, 0, len(bs))
	for _, b := range bs {
		k := key{b.Start, b.Duration, titles.Normalize(b.Title), titles.Normalize(b.Description)}
		i, seen := at[k]
		if !seen {
			at[k] = len(out)
			out = append(out, b)
			continue
		}
		kept := &out[i]
		if rank(b) > rank(*kept) {
			b.AlsoOn = append(append(kept.AlsoOn, kept.ServiceName), b.AlsoOn...)
			b.HasTimer = b.HasTimer || kept.HasTimer
			*kept = b
			continue
		}
		kept.AlsoOn = append(kept.AlsoOn, b.ServiceName)
	}
	return out
}

// seriesChannels are the channels the series' timers record from.
func seriesChannels(timers []webcontrol.Timer, series string) map[string]bool {
	out := map[string]bool{}
	for _, t := range timers {
		if matchesSeries(t.Name, series) {
			out[t.ServiceReference] = true
		}
	}
	return out
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

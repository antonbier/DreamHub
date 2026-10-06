package hubs

import (
	"context"
	"strconv"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	"github.com/antonbier/DreamHub/backend/internal/titles"
)

// Which episode a broadcast in "In the EPG" is, and whether it's one the
// user is missing: the "this episode is missing, and it airs on Thursday"
// case. The code comes from the EPG itself when a channel writes it
// ("S05E14" in the title or description), else from DreamMediaInfo's
// match_cache (internal/dmi, read-only), which has already matched EPG
// episode titles against TMDB's episode lists.
//
// Like the episode guide, "missing" is only claimed when it can be told:
// the series' recordings carry season codes (season 1 or later) at all.
// SerienRecorder numbers some series absolutely (S00E8615, a daily soap),
// TMDB by season — then codes don't line up and nothing is claimed, unless
// the episode guide found the absolute numbering to agree with TVmaze's
// (absoluteNumbering): then a broadcast is matched by its episode title —
// against SerienRecorder's timers, the recordings and TVmaze's list — since
// TMDB's season codes would be the wrong numbering.

// Broadcast episode states.
const (
	BroadcastRecorded  = "recorded"  // a recording carries this code
	BroadcastPlanned   = "planned"   // SerienRecorder has timered it, or a timer covers it
	BroadcastMissing   = "missing"   // aired before, not recorded: worth recording now
	BroadcastUnmatched = "unmatched" // code known, but recordings can't be matched
)

// annotateBroadcasts fills in Season/Episode/EpisodeStatus on bs. guide
// is the series' episode guide, nil when there is none.
func (h *Series) annotateBroadcasts(ctx context.Context, m serienrecorder.Marker, bs []seriesBroadcast, recs []seriesRecording, upcoming []serienrecorder.UpcomingEpisode, guide *episodeGuide) {
	if len(bs) == 0 {
		return
	}
	if guide != nil && guide.Absolute {
		annotateAbsolute(m, bs, recs, upcoming, guide)
		return
	}
	type se struct{ s, e int }
	recorded := map[se]bool{}
	matchable := false
	for _, r := range recs {
		if r.Episode > 0 {
			recorded[se{r.Season, r.Episode}] = true
			if r.Season > 0 {
				matchable = true
			}
		}
	}
	planned := map[se]bool{}
	for _, u := range upcoming {
		s, err1 := strconv.Atoi(u.Season)
		e, err2 := strconv.Atoi(u.Episode)
		if err1 == nil && err2 == nil && e > 0 {
			planned[se{s, e}] = true
		}
	}

	var tmdbID int64 // looked up once, only when a broadcast needs it
	looked := false
	for i := range bs {
		b := &bs[i]
		s, e := parseEpisodeCode(b.Title + " " + b.Description)
		if e > 0 {
			b.CodeSource = "epg"
		} else if h.DMI != nil {
			if !looked {
				looked = true
				if entry, ok := h.DMI.Lookup(ctx, m.Name, "tv"); ok {
					tmdbID = entry.TMDBID
				}
			}
			if tmdbID != 0 {
				if ds, de, ok := h.DMI.EpisodeCode(ctx, tmdbID, episodeTitleCandidates(b.Title, b.Description, m.Name)...); ok {
					s, e, b.CodeSource = ds, de, "dreammediainfo"
				}
			}
		}
		if e == 0 {
			continue
		}
		b.Season, b.Episode = s, e
		switch k := (se{s, e}); {
		case recorded[k]:
			b.EpisodeStatus = BroadcastRecorded
		case planned[k] || b.HasTimer:
			b.EpisodeStatus = BroadcastPlanned
		case matchable && s > 0:
			b.EpisodeStatus = BroadcastMissing
		default:
			b.EpisodeStatus = BroadcastUnmatched
		}
	}
}

// annotateAbsolute is annotateBroadcasts for an absolutely numbered series:
// a broadcast's number comes from its episode title.
func annotateAbsolute(m serienrecorder.Marker, bs []seriesBroadcast, recs []seriesRecording, upcoming []serienrecorder.UpcomingEpisode, guide *episodeGuide) {
	type source struct {
		number int
		from   string
	}
	byName := map[string]source{}
	for n, number := range guide.absNames {
		byName[n] = source{number, "tvmaze"}
	}
	recorded := map[int]bool{}
	for _, r := range recs {
		if r.Season == 0 && r.Episode > 0 {
			recorded[r.Episode] = true
			if n := titles.Normalize(recordingEpisodeTitle(r)); n != "" {
				byName[n] = source{r.Episode, "recording"}
			}
		}
	}
	planned := map[int]bool{}
	for _, u := range upcoming {
		if e, err := strconv.Atoi(u.Episode); err == nil && e > 0 && strings.TrimSpace(u.Season) == "0" {
			planned[e] = true
			if n := titles.Normalize(u.Title); n != "" {
				byName[n] = source{e, "serienrecorder"}
			}
		}
	}
	for i := range bs {
		b := &bs[i]
		var src source
		for _, c := range episodeTitleCandidates(b.Title, b.Description, m.Name) {
			if s, ok := byName[titles.Normalize(c)]; ok {
				src = s
				break
			}
		}
		if src.number == 0 {
			continue
		}
		b.Season, b.Episode, b.CodeSource = 0, src.number, src.from
		switch {
		case recorded[src.number]:
			b.EpisodeStatus = BroadcastRecorded
		case planned[src.number] || b.HasTimer:
			b.EpisodeStatus = BroadcastPlanned
		case src.number >= guide.absFrom:
			b.EpisodeStatus = BroadcastMissing
		default:
			b.EpisodeStatus = BroadcastUnmatched
		}
	}
}

// episodeTitleCandidates are the strings an EPG event may carry its
// episode title in: the short description's first line (German EPGs put
// it there), the part of the title after the series name ("Tatort:
// Borowski und …"), and the description up to the first sentence.
func episodeTitleCandidates(title, description, series string) []string {
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(strings.Trim(strings.TrimSpace(s), `"„“`))
		if s == "" || titles.Normalize(s) == titles.Normalize(series) {
			return
		}
		for _, o := range out {
			if o == s {
				return
			}
		}
		out = append(out, s)
	}
	first, _, _ := strings.Cut(description, "\n")
	add(first)
	if rest, ok := afterSeriesName(title, series); ok {
		add(rest)
	}
	if i := strings.IndexAny(first, ".("); i > 0 {
		add(first[:i])
	}
	return out
}

// afterSeriesName is what follows the series name in an EPG title
// ("FBI: Most Wanted - Gefangenenübergabe" → "Gefangenenübergabe").
func afterSeriesName(title, series string) (string, bool) {
	for _, sep := range []string{": ", " - ", " – "} {
		head, rest, ok := strings.Cut(title, sep)
		for ok {
			if titles.Normalize(head) == titles.Normalize(series) {
				return rest, true
			}
			var more string
			more, rest, ok = strings.Cut(rest, sep)
			head += sep + more
		}
	}
	return "", false
}

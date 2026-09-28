package hubs

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	"github.com/antonbier/DreamHub/backend/internal/titles"
	"github.com/antonbier/DreamHub/backend/internal/tvmaze"
)

// The episode guide on a series' detail page: every episode TVmaze knows,
// each marked recorded (a recording carries its S01E02 code), planned
// (SerienRecorder has timered it), still to come, or missing. TVmaze is
// asked through its connector only while it's switched on, and its
// answers are cached in the store (the enrichment table), so the page
// doesn't call TVmaze on every visit and still works while it's down.
//
// "Missing" is only claimed when the series' recordings carry episode codes
// at all — without them nothing can be matched, and every aired episode
// would look missing. Then the guide just says "aired".

// Episode states.
const (
	EpisodeRecorded = "recorded"
	EpisodePlanned  = "planned"
	EpisodeUpcoming = "upcoming"
	EpisodeMissing  = "missing"
	EpisodeAired    = "aired" // aired, but recordings can't be matched
)

// Guide states.
const (
	GuideOK      = "ok"
	GuideOff     = "off"     // TVmaze switched off (or not wired)
	GuideNoMatch = "noMatch" // TVmaze doesn't know a show of exactly this name
	GuideFailed  = "failed"  // TVmaze unreachable, nothing cached
)

type episodeGuide struct {
	State     string        `json:"state"`
	Source    string        `json:"source,omitempty"` // "tvmaze"
	ShowName  string        `json:"showName,omitempty"`
	Matchable bool          `json:"matchable"`
	Seasons   []guideSeason `json:"seasons"`
}

type guideSeason struct {
	Season   int            `json:"season"`
	Episodes []guideEpisode `json:"episodes"`
	Recorded int            `json:"recorded"`
	Planned  int            `json:"planned"`
	Missing  int            `json:"missing"`
	Upcoming int            `json:"upcoming"`
}

type guideEpisode struct {
	Season   int    `json:"season"`
	Number   int    `json:"number"`
	Name     string `json:"name"`
	Airdate  string `json:"airdate,omitempty"`
	Airstamp int64  `json:"airstamp,omitempty"`
	Status   string `json:"status"`
}

const (
	showCacheTTL     = 30 * 24 * time.Hour
	showMissTTL      = 3 * 24 * time.Hour
	episodesCacheTTL = 12 * time.Hour // new episodes get announced
)

func (h *Series) episodeGuide(ctx context.Context, m serienrecorder.Marker, recs []seriesRecording, upcoming []serienrecorder.UpcomingEpisode) episodeGuide {
	g := episodeGuide{State: GuideOff, Seasons: []guideSeason{}}
	if h.TVmaze == nil || h.Store == nil || h.Settings == nil {
		return g
	}
	user, _ := httpx.User(ctx)
	v, err := h.Settings.Values(ctx, user)
	if err != nil || !v.Bool(connector.KeyTVmazeEnabled) {
		return g
	}

	show, ok, err := h.findShow(ctx, m.Name)
	switch {
	case err != nil:
		g.State = GuideFailed
		return g
	case !ok:
		g.State = GuideNoMatch
		return g
	}
	eps, err := h.showEpisodes(ctx, show.ID)
	if err != nil {
		g.State = GuideFailed
		return g
	}
	g.State, g.Source, g.ShowName = GuideOK, "tvmaze", show.Name

	type se struct{ s, e int }
	recorded := map[se]bool{}
	for _, r := range recs {
		if r.Episode > 0 {
			recorded[se{r.Season, r.Episode}] = true
		}
	}
	g.Matchable = len(recorded) > 0
	planned := map[se]bool{}
	for _, u := range upcoming {
		s, err1 := strconv.Atoi(u.Season)
		e, err2 := strconv.Atoi(u.Episode)
		if err1 == nil && err2 == nil && e > 0 {
			planned[se{s, e}] = true
		}
	}

	now := h.now()
	today := now.Format("2006-01-02")
	bySeason := map[int]*guideSeason{}
	for _, e := range eps {
		ge := guideEpisode{Season: e.Season, Number: e.Number, Name: e.Name, Airdate: e.Airdate, Airstamp: e.Airstamp}
		aired := (e.Airstamp > 0 && e.Airstamp <= now.Unix()) || (e.Airstamp == 0 && e.Airdate != "" && e.Airdate <= today)
		switch k := (se{e.Season, e.Number}); {
		case recorded[k]:
			ge.Status = EpisodeRecorded
		case planned[k]:
			ge.Status = EpisodePlanned
		case !aired:
			ge.Status = EpisodeUpcoming
		case g.Matchable:
			ge.Status = EpisodeMissing
		default:
			ge.Status = EpisodeAired
		}
		s := bySeason[e.Season]
		if s == nil {
			s = &guideSeason{Season: e.Season}
			bySeason[e.Season] = s
		}
		s.Episodes = append(s.Episodes, ge)
		switch ge.Status {
		case EpisodeRecorded:
			s.Recorded++
		case EpisodePlanned:
			s.Planned++
		case EpisodeMissing:
			s.Missing++
		case EpisodeUpcoming:
			s.Upcoming++
		}
	}
	for _, s := range bySeason {
		sort.Slice(s.Episodes, func(i, j int) bool { return s.Episodes[i].Number < s.Episodes[j].Number })
		g.Seasons = append(g.Seasons, *s)
	}
	sort.Slice(g.Seasons, func(i, j int) bool { return g.Seasons[i].Season > g.Seasons[j].Season }) // newest first
	return g
}

// findShow looks the series up on TVmaze by name — an exact (normalized)
// name match only, the best-scored if several; a near-miss like
// "Tatort-Reiniger" for "Tatort" never counts. Cached, misses included.
func (h *Series) findShow(ctx context.Context, name string) (tvmaze.Show, bool, error) {
	key := "tvmaze|show|" + titles.Normalize(name)
	if raw, at, ok, err := h.Store.Enrichment(ctx, key); err == nil && ok {
		age := h.now().Sub(time.Unix(at, 0))
		if raw == "" && age < showMissTTL {
			return tvmaze.Show{}, false, nil
		}
		var s tvmaze.Show
		if raw != "" && age < showCacheTTL && json.Unmarshal([]byte(raw), &s) == nil {
			return s, true, nil
		}
	}
	shows, err := h.TVmaze.SearchShows(ctx, name)
	if err != nil {
		return tvmaze.Show{}, false, err
	}
	want := titles.Normalize(name)
	var best *tvmaze.Show
	for i := range shows {
		if titles.Normalize(shows[i].Name) == want && (best == nil || shows[i].Score > best.Score) {
			best = &shows[i]
		}
	}
	raw := ""
	if best != nil {
		b, _ := json.Marshal(best)
		raw = string(b)
	}
	_ = h.Store.PutEnrichment(ctx, key, raw, h.now().Unix())
	if best == nil {
		return tvmaze.Show{}, false, nil
	}
	return *best, true, nil
}

// showEpisodes is a show's episode list, cached for episodesCacheTTL; a
// stale cached list is still used when TVmaze can't be reached.
func (h *Series) showEpisodes(ctx context.Context, id int64) ([]tvmaze.Episode, error) {
	key := "tvmaze|eps|" + strconv.FormatInt(id, 10)
	var cached []tvmaze.Episode
	haveCached := false
	if raw, at, ok, err := h.Store.Enrichment(ctx, key); err == nil && ok && json.Unmarshal([]byte(raw), &cached) == nil {
		haveCached = true
		if h.now().Sub(time.Unix(at, 0)) < episodesCacheTTL {
			return cached, nil
		}
	}
	eps, err := h.TVmaze.Episodes(ctx, id)
	if err != nil {
		if haveCached {
			return cached, nil
		}
		return nil, err
	}
	if b, err := json.Marshal(eps); err == nil {
		_ = h.Store.PutEnrichment(ctx, key, string(b), h.now().Unix())
	}
	return eps, nil
}

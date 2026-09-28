package hubs

import (
	"context"
	"net/http"
	"sort"

	"github.com/antonbier/DreamHub/backend/internal/ai"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/titles"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// The taste profile DiscoveryHub can send along with a recommendation
// request: what the user already records, plans and follows, so the AI
// leans toward the user's taste instead of "generally good TV". Titles
// only, deduplicated, most recent first. Every source is optional — an
// unreachable box or a missing plugin just leaves its part empty, never
// fails the request.

// tasteLimit caps each list; the AI client caps again before sending.
const tasteLimit = 40

func (h *Discovery) taste(ctx context.Context) ai.Taste {
	var t ai.Taste

	if movies, err := h.WC.MovieList(ctx); err == nil {
		sort.SliceStable(movies, func(i, j int) bool { return movies[i].Time > movies[j].Time })
		titles := make([]string, 0, len(movies))
		for _, m := range movies {
			titles = append(titles, m.Title)
		}
		t.Recordings = dedupeTitles(titles, true)
	}

	if timers, err := h.WC.TimerList(ctx); err == nil {
		now := h.Now().Unix()
		sort.SliceStable(timers, func(i, j int) bool { return timers[i].Begin < timers[j].Begin })
		titles := []string{}
		for _, tm := range timers {
			if tm.End > now && !tm.JustPlay {
				titles = append(titles, tm.Name)
			}
		}
		t.Timers = dedupeTitles(titles, true)
	}

	if h.SR != nil {
		if markers, err := h.SR.Markers(ctx); err == nil {
			names := []string{}
			for _, m := range markers {
				if m.Active {
					names = append(names, m.Name)
				}
			}
			t.Series = dedupeTitles(names, false)
		}
	}

	if h.AT != nil {
		if list, err := h.AT.List(ctx); err == nil {
			terms := []string{}
			for _, a := range list {
				if !a.Enabled {
					continue
				}
				if a.Match != "" {
					terms = append(terms, a.Match)
				} else {
					terms = append(terms, a.Name)
				}
			}
			t.AutoTimers = dedupeTitles(terms, false)
		}
	}
	return t
}

// dedupeTitles drops empty and repeated titles and caps the list. With
// episodes, broadcast titles are cut to their series part first ("Tatort:
// X" and "Tatort: Y" count once, as "Tatort": the series is the signal,
// not each episode) — not for names that already are a series name
// ("Die Verräter - Vertraue Niemandem!").
func dedupeTitles(list []string, episodes bool) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, title := range list {
		name := title
		if episodes {
			name = titles.SeriesPart(title)
		}
		key := normalizeTitle(name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
		if len(out) == tasteLimit {
			break
		}
	}
	return out
}

func coveredByTimer(e webcontrol.EPGEvent, timers []webcontrol.Timer) bool {
	for _, t := range timers {
		if t.ServiceReference == e.ServiceReference && t.Begin <= e.Start && t.End >= e.Start+e.Duration {
			return true
		}
	}
	return false
}

type tasteCountsView struct {
	Recordings int `json:"recordings"`
	Series     int `json:"series"`
	Timers     int `json:"timers"`
	AutoTimers int `json:"autoTimers"`
}

func tasteCounts(t ai.Taste) tasteCountsView {
	return tasteCountsView{len(t.Recordings), len(t.Series), len(t.Timers), len(t.AutoTimers)}
}

// tasteView is ai.Taste for the UI: every list present, even when empty
// (ai.Taste omits empty lists to keep the prompt short).
type tasteView struct {
	Recordings []string `json:"recordings"`
	Series     []string `json:"series"`
	Timers     []string `json:"timers"`
	AutoTimers []string `json:"autoTimers"`
}

// getTaste shows the user exactly what "use my taste" would send to the AI
// provider — nothing is sent by this call.
func (h *Discovery) getTaste(w http.ResponseWriter, r *http.Request) {
	t := h.taste(r.Context())
	nonNil := func(l []string) []string {
		if l == nil {
			return []string{}
		}
		return l
	}
	httpx.WriteJSON(w, http.StatusOK, tasteView{nonNil(t.Recordings), nonNil(t.Series), nonNil(t.Timers), nonNil(t.AutoTimers)})
}

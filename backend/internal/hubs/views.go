package hubs

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/hub"
)

// View sources for custom dashboard tiles (the hub contract's views,
// internal/hub/view.go). Each hub returns its candidate items; filtering,
// sorting and the limit happen in the registry, the same for every source.

// Source IDs.
const (
	SourceRecordings     = "media.recordings"
	SourceTimers         = "timers.upcoming"
	SourceGuide          = "guide.broadcasts"
	SourceSeriesUpcoming = "series.upcoming"
)

// maxViewDays caps how far ahead a view fetches EPG: a whole bouquet's
// schedule for a week is a lot for the box to serialize for one tile.
const maxViewDays = 3

// ViewSources implements hub.ViewProvider. Recordings come from the top
// recording folder only, like MediaHub's search — walking every folder
// through WebControl takes ~40 s on the reference box and blocks enigma2
// meanwhile (ARCHITECTURE.md), so that waits for a cached index.
func (h *Media) ViewSources() []hub.ViewSource {
	return []hub.ViewSource{{
		ID:      SourceRecordings,
		Label:   "dashboard.source.recordings",
		Filters: []string{hub.FilterText, hub.FilterOlderThanDays, hub.FilterNewerThanDays, hub.FilterMinSizeGB},
		Sorts:   []string{hub.SortNewest, hub.SortOldest, hub.SortLargest},
	}}
}

func (h *Media) View(ctx context.Context, sourceID string, q hub.ViewQuery, now time.Time) ([]hub.ViewItem, error) {
	movies, err := h.WC.MovieList(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]hub.ViewItem, 0, len(movies))
	for _, m := range movies {
		out = append(out, hub.ViewItem{Title: m.Title, Subtitle: m.ServiceName, Time: int64(m.Time), Size: int64(m.FileSize), Path: "/media"})
	}
	return out, nil
}

// ViewSources implements hub.ViewProvider: timers still to come, flagged
// Warn when they likely clash (the same estimate TimerHub's list shows).
func (h *Timer) ViewSources() []hub.ViewSource {
	return []hub.ViewSource{{
		ID:      SourceTimers,
		Label:   "dashboard.source.timers",
		Filters: []string{hub.FilterText, hub.FilterWithinDays, hub.FilterFromTime, hub.FilterWarnOnly},
		Sorts:   []string{hub.SortSoonest},
	}}
}

func (h *Timer) View(ctx context.Context, sourceID string, q hub.ViewQuery, now time.Time) ([]hub.ViewItem, error) {
	timers, err := h.WC.TimerList(ctx)
	if err != nil {
		return nil, err
	}
	info, _ := h.WC.DeviceInfo(ctx)
	out := []hub.ViewItem{}
	for _, t := range timerViews(timers, info) {
		if t.End <= now.Unix() || t.Disabled {
			continue
		}
		out = append(out, hub.ViewItem{Title: t.Name, Subtitle: t.ServiceName, Time: t.Begin, End: t.End, Warn: t.Conflict, Path: "/timers"})
	}
	return out, nil
}

// ViewSources implements hub.ViewProvider: broadcasts of one bouquet from
// now on ("movies tonight from 20:15": fromTime plus a minimum duration —
// the box's EPG carries no genre DreamHub could filter on).
func (h *Guide) ViewSources() []hub.ViewSource {
	return []hub.ViewSource{{
		ID:      SourceGuide,
		Label:   "dashboard.source.guide",
		Filters: []string{hub.FilterBouquet, hub.FilterText, hub.FilterWithinDays, hub.FilterFromTime, hub.FilterMinDuration},
		Sorts:   []string{hub.SortSoonest},
	}}
}

func (h *Guide) View(ctx context.Context, sourceID string, q hub.ViewQuery, now time.Time) ([]hub.ViewItem, error) {
	bouquet := q.Bouquet
	if bouquet == "" {
		bouquets, err := h.WC.Bouquets(ctx)
		if err != nil {
			return nil, err
		}
		if len(bouquets) == 0 {
			return []hub.ViewItem{}, nil
		}
		bouquet = bouquets[0].ServiceReference
	}
	days := q.WithinDays
	if days <= 0 || days > maxViewDays {
		days = 1
	}
	start := now.Unix()
	events, err := h.WC.EPGMulti(ctx, bouquet, start, start+int64(days)*86400)
	if err != nil {
		return nil, fmt.Errorf("guide view: %w", err)
	}
	out := []hub.ViewItem{}
	for _, e := range events {
		if e.Start+e.Duration <= start {
			continue
		}
		out = append(out, hub.ViewItem{
			Title: e.Title, Subtitle: e.ServiceName, Time: e.Start, End: e.Start + e.Duration,
			Path: "/guide?at=" + strconv.FormatInt(e.Start, 10),
		})
	}
	return out, nil
}

// ViewSources implements hub.ViewProvider: episodes SerienRecorder has
// timered, each opening its series' detail page.
func (h *Series) ViewSources() []hub.ViewSource {
	return []hub.ViewSource{{
		ID:      SourceSeriesUpcoming,
		Label:   "dashboard.source.seriesUpcoming",
		Filters: []string{hub.FilterText, hub.FilterWithinDays, hub.FilterFromTime},
		Sorts:   []string{hub.SortSoonest},
	}}
}

func (h *Series) View(ctx context.Context, sourceID string, q hub.ViewQuery, now time.Time) ([]hub.ViewItem, error) {
	episodes, err := h.SR.UpcomingEpisodes(ctx)
	if err != nil {
		return nil, err
	}
	ids := map[string]int{}
	if markers, err := h.SR.Markers(ctx); err == nil {
		for _, m := range markers {
			ids[m.FSID] = m.ID
		}
	}
	out := []hub.ViewItem{}
	for _, e := range episodes {
		if e.StartTime < now.Unix()-3600 {
			continue // an episode that started over an hour ago is past
		}
		path := "/series"
		if id, ok := ids[e.FSID]; ok {
			path = "/series/" + strconv.Itoa(id)
		}
		out = append(out, hub.ViewItem{Title: e.Series, Subtitle: e.Title + " · " + e.Channel, Time: e.StartTime, Path: path})
	}
	return out, nil
}

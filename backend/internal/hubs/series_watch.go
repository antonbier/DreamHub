package hubs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/events"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// Watching for missing episodes that air soon — the push "FBI S05E07 is
// missing and airs tonight at 21:00" (internal/push). Every few hours
// SeriesHub runs the same check its detail page shows (series_broadcast_
// episodes.go) over every active followed series and publishes
// "series.missingAiring" on the core event bus for each broadcast of a
// missing episode in the next day that no timer covers yet — once per
// broadcast, also across restarts (the bus's own feed is the memory).
// Detection, not automation: it never sets a timer itself.

// EventMissingAiring is the event key.
const EventMissingAiring = events.MissingAiring

const (
	missingWatchInterval = 3 * time.Hour
	missingWatchFirst    = 2 * time.Minute // let the box settle after a restart
	missingWatchAhead    = 24 * time.Hour
)

// Publisher is the core event bus as SeriesHub needs it.
type Publisher interface {
	Publish(ctx context.Context, key string, params any) (store.Notification, error)
}

// WatchMissing runs until ctx is done; meant for its own goroutine.
func (h *Series) WatchMissing(ctx context.Context, bus Publisher, logger *slog.Logger) {
	seen := h.announcedMissing(ctx)
	timer := time.NewTimer(missingWatchFirst)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			n, err := h.checkMissing(ctx, bus, seen)
			if logger != nil {
				if err != nil {
					logger.Warn("missing-episode check failed", "error", err)
				} else {
					logger.Debug("missing-episode check", "announced", n)
				}
			}
			timer.Reset(missingWatchInterval)
		}
	}
}

// announcedMissing reads which broadcasts were already announced from the
// bus's persisted feed, so a restart doesn't announce them again.
func (h *Series) announcedMissing(ctx context.Context) map[string]bool {
	seen := map[string]bool{}
	if h.Store == nil {
		return seen
	}
	list, err := h.Store.Notifications(ctx, 500, false)
	if err != nil {
		return seen
	}
	for _, n := range list {
		if n.Key != EventMissingAiring {
			continue
		}
		var p struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(n.Params, &p) == nil && p.ID != "" {
			seen[p.ID] = true
		}
	}
	return seen
}

// checkMissing runs one pass; it returns how many it announced.
func (h *Series) checkMissing(ctx context.Context, bus Publisher, seen map[string]bool) (int, error) {
	if h.SR == nil || h.WC == nil {
		return 0, nil
	}
	markers, err := h.SR.Markers(ctx)
	if err != nil {
		return 0, err
	}
	upcoming, _ := h.SR.UpcomingEpisodes(ctx)
	now := h.now()
	count := 0
	for _, m := range markers {
		if !m.Active || ctx.Err() != nil {
			continue
		}
		casts, err := h.seriesBroadcasts(ctx, m.Name)
		if err != nil || len(casts) == 0 {
			continue
		}
		recs, err := h.seriesRecordings(ctx, m)
		if err != nil {
			continue
		}
		h.annotateBroadcasts(ctx, m, casts, recs, upcoming, nil)
		for _, b := range casts {
			if b.EpisodeStatus != BroadcastMissing || b.HasTimer || b.Start < now.Unix() || b.Start > now.Add(missingWatchAhead).Unix() {
				continue
			}
			code := fmt.Sprintf("S%02dE%02d", b.Season, b.Episode)
			id := fmt.Sprintf("%d|%s|%s|%d", m.ID, code, b.ServiceReference, b.Start)
			if seen[id] {
				continue
			}
			title := ""
			if c := episodeTitleCandidates(b.Title, b.Description, m.Name); len(c) > 0 {
				title = trimTitle(c[0])
			}
			if _, err := bus.Publish(ctx, EventMissingAiring, map[string]any{
				"id": id, "series": m.Name, "markerId": m.ID, "code": code, "title": title,
				"service": b.ServiceName, "start": b.Start,
			}); err == nil {
				seen[id] = true
				count++
			}
		}
	}
	return count, nil
}

// trimTitle keeps an episode title short for a notification line.
func trimTitle(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 60 {
		return string(r[:59]) + "…"
	}
	return s
}

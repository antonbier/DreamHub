package hubs

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/hub"
	"github.com/antonbier/DreamHub/backend/internal/plex"
)

// Plex rows for the start page (docs/archive/PROFILES_PLAN.md, P4): every shown
// library is a view source for custom tiles — "recently added", "not seen
// yet", shown as a row of posters. From the cache, like Search, so a row
// stays filled while the server is off.

// plexSourcePrefix + a library ID is that library's source.
const plexSourcePrefix = "plex.library."

// ViewSources implements hub.ViewProvider: one source per shown library,
// named by the library's own title.
func (h *Plex) ViewSources() []hub.ViewSource {
	ctx := context.Background()
	c, err := h.config(ctx)
	if err != nil || !c.configured() {
		return nil
	}
	var libs []plex.Library
	h.cacheGet(ctx, "libraries", &libs)
	out := []hub.ViewSource{}
	for _, l := range libs {
		if !shown(l, c) {
			continue
		}
		out = append(out, hub.ViewSource{
			ID: plexSourcePrefix + l.ID, Label: "dashboard.source.plexLibrary", Name: l.Title,
			Filters: []string{hub.FilterText, hub.FilterNewerThanDays, hub.FilterUnwatched},
			Sorts:   []string{hub.SortNewest, hub.SortOldest},
		})
	}
	return out
}

// View implements hub.ViewProvider: a library's films or series, Time
// being when they were added to Plex.
func (h *Plex) View(ctx context.Context, sourceID string, _ hub.ViewQuery, _ time.Time) ([]hub.ViewItem, error) {
	id, ok := strings.CutPrefix(sourceID, plexSourcePrefix)
	if !ok {
		return nil, hub.ErrUnknownSource
	}
	var items []plex.Item
	h.cacheGet(ctx, "items:"+id, &items)
	out := make([]hub.ViewItem, 0, len(items))
	for _, it := range items {
		sub := ""
		if it.Year > 0 {
			sub = strconv.Itoa(it.Year)
		}
		out = append(out, hub.ViewItem{
			Title: it.Title, Subtitle: sub, Time: it.AddedAt, Path: "/plex?item=" + it.RatingKey,
			Image: it.Thumb, Unwatched: !it.Watched(),
		})
	}
	return out, nil
}

package hubs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/plex"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// Watched state lives in Plex: DreamHub sets it there and nowhere else.
//
// One film or episode is marked directly, with undo in the UI (the inverse
// call) and an entry in the change log — it changes nothing on the box and
// is undone with one click (PRINCIPLES.md names this exception). A whole
// series or season touches many episodes at once, so it goes through
// preview and confirm, listing every episode that changes.
//
// Only while the server answers: there is no queue that would send it
// later on its own.

type watchedBody struct {
	Watched bool `json:"watched"`
}

func (h *Plex) setWatched(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireConfig(w, r)
	if !ok {
		return
	}
	var body watchedBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"watched": true|false}`)
		return
	}
	key := r.PathValue("key")
	it, err := h.Client.Item(r.Context(), c.conn, key)
	h.note(err)
	if err != nil {
		h.writeWriteError(w, r, err)
		return
	}
	if it.Type != "movie" && it.Type != "episode" {
		httpx.WriteError(w, r, http.StatusBadRequest, "plex_needs_preview", "a series or season is marked through …/watched/plan")
		return
	}
	if err := h.Client.SetWatched(r.Context(), c.conn, key, body.Watched); err != nil {
		h.note(err)
		h.writeWriteError(w, r, err)
		return
	}
	user, _ := httpx.User(r.Context())
	_ = h.Store.AppendChange(r.Context(), store.ChangeEntry{UserID: user.ID, Action: "plex.watched", Summary: watchedSummary(it, body.Watched)})
	updated := h.refreshAfterWatched(r.Context(), c, it)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"item": itemView{Item: updated, Watched: updated.Watched()}})
}

func (h *Plex) writeWriteError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, plex.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, CodePlexNotFound, "no such item on the Plex server")
	case errors.Is(err, plex.ErrUnauthorized):
		httpx.WriteError(w, r, http.StatusBadGateway, CodePlexUnauthorized, "the Plex server rejected the token")
	case errors.Is(err, plex.ErrUnreachable):
		httpx.WriteError(w, r, http.StatusConflict, CodePlexOffline, "the Plex server doesn't answer; nothing was changed")
	default:
		httpx.WriteError(w, r, http.StatusBadGateway, CodePlexBadAnswer, "the Plex server's answer couldn't be used: "+err.Error())
	}
}

func watchedSummary(it plex.Item, watched bool) string {
	what := "Marked as not watched in Plex: "
	if watched {
		what = "Marked as watched in Plex: "
	}
	return what + displayTitle(it)
}

// displayTitle is "Dark S01E02 · Title" for an episode, the title otherwise.
func displayTitle(it plex.Item) string {
	switch it.Type {
	case "episode":
		return fmt.Sprintf("%s S%02dE%02d · %s", it.ShowTitle, it.ParentIndex, it.Index, it.Title)
	case "season":
		return it.ShowTitle + " · " + it.Title
	}
	return it.Title
}

// planWatched previews marking a series or season (or, for symmetry, one
// item) — every episode whose state would change.
func (h *Plex) planWatched(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireConfig(w, r)
	if !ok {
		return
	}
	var body watchedBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"watched": true|false}`)
		return
	}
	key := r.PathValue("key")
	it, err := h.Client.Item(r.Context(), c.conn, key)
	h.note(err)
	if err != nil {
		h.writeWriteError(w, r, err)
		return
	}
	leaves, err := h.leaves(r.Context(), c, it)
	h.note(err)
	if err != nil {
		h.writeWriteError(w, r, err)
		return
	}
	var changes []plan.Change
	for _, ep := range leaves {
		if ep.Watched() == body.Watched {
			continue
		}
		changes = append(changes, plan.Change{Type: "plex.watched",
			Before: map[string]any{"title": displayTitle(ep), "watched": !body.Watched},
			After:  map[string]any{"title": displayTitle(ep), "watched": body.Watched}})
	}
	if len(changes) == 0 {
		httpx.WriteError(w, r, http.StatusConflict, "plex_nothing_to_change", "every episode is already marked so")
		return
	}
	fingerprint := func(ctx context.Context) (string, error) {
		fresh, err := h.Client.Item(ctx, c.conn, key)
		if err != nil {
			return "", err
		}
		return strconv.Itoa(fresh.ViewedLeafCount) + "/" + strconv.Itoa(fresh.ViewCount), nil
	}
	fp, _ := fingerprint(r.Context())
	user, _ := httpx.User(r.Context())
	p := h.Plans.Create(user, plan.Draft{
		Action:      "plex.watched",
		Changes:     changes,
		Summary:     fmt.Sprintf("%s (%d episodes)", watchedSummary(it, body.Watched), len(changes)),
		Fingerprint: fp,
		Current:     fingerprint,
		Execute: func(ctx context.Context) error {
			if err := h.Client.SetWatched(ctx, c.conn, key, body.Watched); err != nil {
				h.note(err)
				if errors.Is(err, plex.ErrUnreachable) {
					return &plan.Failure{Status: http.StatusConflict, Code: CodePlexOffline, Message: "the Plex server doesn't answer; nothing was changed", Err: err}
				}
				return err
			}
			h.refreshAfterWatched(context.WithoutCancel(ctx), c, it)
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": p})
}

// leaves are the films/episodes under an item: itself, or every episode
// of a season or series.
func (h *Plex) leaves(ctx context.Context, c plexConfig, it plex.Item) ([]plex.Item, error) {
	if it.Type == "movie" || it.Type == "episode" {
		return []plex.Item{it}, nil
	}
	children, err := h.Client.Children(ctx, c.conn, it.RatingKey)
	if err != nil {
		return nil, err
	}
	var out []plex.Item
	for _, ch := range children {
		sub, err := h.leaves(ctx, c, ch)
		if err != nil {
			return nil, err
		}
		out = append(out, sub...)
	}
	return out, nil
}

// refreshAfterWatched brings the cache up to date after a change: the item
// itself, the lists it appears in (its season's episodes, its series'
// seasons, the library's films/series) and "continue watching". Returns
// the item as the server now has it. Best effort — a failure only leaves
// the cache a sync behind.
func (h *Plex) refreshAfterWatched(ctx context.Context, c plexConfig, it plex.Item) plex.Item {
	updated := it
	if fresh, err := h.Client.Item(ctx, c.conn, it.RatingKey); err == nil {
		updated = fresh
		h.cachePut(ctx, "item:"+it.RatingKey, fresh)
	}
	refresh := func(key string) {
		if children, err := h.Client.Children(ctx, c.conn, key); err == nil {
			h.cachePut(ctx, "children:"+key, children)
		}
		if fresh, err := h.Client.Item(ctx, c.conn, key); err == nil {
			h.cachePut(ctx, "item:"+key, fresh)
			h.patchLibraries(ctx, fresh)
		}
	}
	switch it.Type {
	case "movie", "show":
		h.patchLibraries(ctx, updated)
		if it.Type == "show" {
			refresh(it.RatingKey)
		}
	case "season":
		refresh(it.RatingKey)
		refresh(it.ShowKey)
	case "episode":
		refresh(it.SeasonKey)
		refresh(it.ShowKey)
	}
	// Seasons of a series marked as a whole.
	if it.Type == "show" {
		var seasons []plex.Item
		if _, ok := h.cacheGet(ctx, "children:"+it.RatingKey, &seasons); ok {
			for _, s := range seasons {
				if children, err := h.Client.Children(ctx, c.conn, s.RatingKey); err == nil {
					h.cachePut(ctx, "children:"+s.RatingKey, children)
				}
			}
		}
	}
	if deck, err := h.Client.OnDeck(ctx, c.conn); err == nil {
		h.cachePut(ctx, "ondeck", deck)
	}
	return updated
}

// patchLibraries replaces a film or series in every cached library list.
func (h *Plex) patchLibraries(ctx context.Context, it plex.Item) {
	if it.Type != "movie" && it.Type != "show" {
		return
	}
	var libs []plex.Library
	h.cacheGet(ctx, "libraries", &libs)
	for _, l := range libs {
		var items []plex.Item
		at, ok := h.cacheGet(ctx, "items:"+l.ID, &items)
		if !ok {
			continue
		}
		for i := range items {
			if items[i].RatingKey == it.RatingKey {
				items[i] = it
				if b, err := jsonMarshal(items); err == nil {
					_ = h.Store.PutPlexCache(ctx, "items:"+l.ID, b, at)
				}
				break
			}
		}
	}
}

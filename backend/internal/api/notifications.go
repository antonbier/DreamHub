package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// notificationView is the core event bus's feed, as sent to a client — both
// from GET /api/v1/notifications and pushed live over the SSE stream. key
// is an i18n key, params its arguments; never finished text, translated
// client-side the same way status.Problem is everywhere else.
type notificationView struct {
	ID     int64  `json:"id"`
	At     string `json:"at"` // RFC 3339, UTC
	Key    string `json:"key"`
	Params any    `json:"params"`
	Read   bool   `json:"read"`
	// Box names the box the event is about; only with more than one box,
	// so a household with one never sees it.
	Box *notificationBox `json:"box,omitempty"`
}

type notificationBox struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

func toNotificationView(n store.Notification) notificationView {
	return notificationView{ID: n.ID, At: n.At.UTC().Format(time.RFC3339), Key: n.Key, Params: json.RawMessage(n.Params), Read: n.ReadAt != nil}
}

// viewFor builds what a profile sees of an event: nothing (false) for a box
// the profile may not use, the box's name once there are several boxes.
func viewFor(d Deps, user store.User, n store.Notification) (notificationView, bool) {
	v := toNotificationView(n)
	known := false
	var b store.Box
	if d.Boxes != nil {
		b, known = d.Boxes.EventBox(n.Params)
	}
	if known && !user.MayUseBox(b.ID) {
		return v, false
	}
	if known && len(d.Boxes.List()) > 1 {
		v.Box = &notificationBox{ID: b.ID, Name: b.Name, Color: b.Color}
	}
	return v, true
}

// handleNotifications answers the bell's inbox (docs/archive/V1_PLAN.md, WP7:
// "Persisted notifications... inbox in the bell").
func handleNotifications(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 500 {
				httpx.WriteError(w, r, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 500")
				return
			}
			limit = n
		}
		unreadOnly := r.URL.Query().Get("unreadOnly") == "1"
		user := current(r)
		// A profile restricted to some boxes sees only their events, so the
		// list is cut after the filter, not before.
		fetch := limit
		restricted := len(user.Boxes) > 0
		if restricted {
			fetch = 500
		}
		entries, err := d.Store.Notifications(r.Context(), fetch, unreadOnly)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not load notifications")
			return
		}
		unread, err := d.Store.UnreadNotifications(r.Context())
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not count unread notifications")
			return
		}
		out := make([]notificationView, 0, len(entries))
		visibleUnread := 0
		for _, e := range entries {
			v, ok := viewFor(d, user, e)
			if !ok {
				continue
			}
			if !v.Read {
				visibleUnread++
			}
			if len(out) < limit {
				out = append(out, v)
			}
		}
		if restricted {
			unread = visibleUnread
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"notifications": out, "unread": unread})
	}
}

func handleMarkNotificationsRead(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := d.Store.MarkNotificationsRead(r.Context()); err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not mark notifications read")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"unread": 0})
	}
}

// handleEventsStream is the core event bus's live push (docs/archive/V1_PLAN.md,
// WP7: "WebSocket + a minimal event bus") — implemented as Server-Sent
// Events rather than a real WebSocket: every event here only ever flows
// server → client (a new notification, a log line), so SSE gets the same
// user-facing result (no polling) over plain HTTP, with automatic
// reconnection built into the format and no new dependency for a
// hand-rolled WebSocket handshake. The frontend still can't use the
// browser's native EventSource, though — DreamHub's own Basic
// Auth/token headers (internal/auth) aren't something EventSource can send,
// so it reads this the same way a normal authenticated fetch does, parsing
// the stream itself (see frontend's useEventStream.ts).
func handleEventsStream(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "streaming not supported")
			return
		}
		ch, cancel := d.Events.Subscribe()
		defer cancel()
		// Background jobs' changes ride the same stream ("event: job"),
		// only the ones this profile may see.
		var jobCh <-chan store.Job
		if d.Jobs != nil {
			var cancelJobs func()
			jobCh, cancelJobs = d.Jobs.Subscribe()
			defer cancelJobs()
		}
		me := current(r)
		seeAll := perm.For(me).Has(perm.System)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		ctx := r.Context()
		ping := time.NewTicker(20 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case n := <-ch:
				view, ok := viewFor(d, me, n)
				if !ok {
					continue
				}
				data, err := json.Marshal(view)
				if err != nil {
					continue
				}
				fmt.Fprintf(w, "event: notification\ndata: %s\n\n", data)
				flusher.Flush()
			case j := <-jobCh:
				if !seeAll && j.OwnerID != me.ID {
					continue
				}
				// A profile restricted to some boxes hears nothing of the others.
				if j.BoxID != "" && !me.MayUseBox(j.BoxID) {
					continue
				}
				data, err := json.Marshal(toJobView(j))
				if err != nil {
					continue
				}
				fmt.Fprintf(w, "event: job\ndata: %s\n\n", data)
				flusher.Flush()
			case <-ping.C:
				fmt.Fprint(w, ": ping\n\n") // keeps intermediaries/the client's read from timing out on a quiet box
				flusher.Flush()
			}
		}
	}
}

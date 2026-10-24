package hubs

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
)

// The stream test (docs/NETCAST_PLAN.md, N1): on request, the box opens the
// chosen streams for a moment and says which answer, which want a login and
// which are gone. Never in the background, never more than a few at a time,
// and nothing is changed by it.

const (
	iptvProbeMax      = 100 // channels per request; the screen asks in batches
	iptvProbeParallel = 6
	iptvProbeEach     = 8 * time.Second
	iptvProbeAll      = 70 * time.Second
)

type iptvProbeResult struct {
	ID     int    `json:"id"`
	Result string `json:"result"` // iptv.Probe* or "unknown" (not reached before the time ran out)
	Status int    `json:"status,omitempty"`
}

func (h *Channels) probeIPTV(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
		IDs   []int  `json:"ids"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"token", "ids": [...]}`)
		return
	}
	ctx := r.Context()
	user, _ := httpx.User(ctx)
	sess, found := h.iptvSession(req.Token, user.ID)
	if !found {
		httpx.WriteError(w, r, http.StatusGone, CodeIPTVSessionGone, "the list was read too long ago; read it again")
		return
	}
	if len(req.IDs) == 0 || len(req.IDs) > iptvProbeMax {
		httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "nothing was tested", map[string]httpx.FieldError{"ids": {Code: "invalid"}})
		return
	}
	for _, id := range req.IDs {
		if id < 0 || id >= len(sess.pl.Entries) {
			httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "nothing was tested", map[string]httpx.FieldError{"ids": {Code: "invalid"}})
			return
		}
	}
	f := h.fetcher()
	ctx, cancel := context.WithTimeout(ctx, iptvProbeAll)
	defer cancel()
	out := make([]iptvProbeResult, len(req.IDs))
	sem := make(chan struct{}, iptvProbeParallel)
	var wg sync.WaitGroup
	for i, id := range req.IDs {
		out[i] = iptvProbeResult{ID: id, Result: "unknown"}
		if ctx.Err() != nil {
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(i, id int) {
			defer func() { <-sem; wg.Done() }()
			c, cancel := context.WithTimeout(ctx, iptvProbeEach)
			defer cancel()
			res := f.Probe(c, sess.pl.Entries[id].URL)
			out[i].Result, out[i].Status = res.Kind, res.Status
		}(i, id)
	}
	wg.Wait()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": out})
}

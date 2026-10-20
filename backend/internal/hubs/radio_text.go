package hubs

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/radiotext"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Radio text — the "now playing" line of a station (#28). WebControl
// doesn't expose it; it is in the station's audio stream (internal/
// radiotext), so DreamHub listens in on the box's own stream of the
// station for a few seconds and reads it out. The stream proxy never
// switches the TV and needs a tuner that is free or already on the
// station's transponder: asking for the station the box is playing always
// works, another one only when a tuner is free.
//
// A message is 10 bytes per audio frame, so a whole line takes up to ~15 s
// to arrive. The answer is remembered for a while, one sample runs at a
// time per box, and the screen asks again only every half minute or so.

const (
	radioTextWait    = 25 * time.Second // how long to listen for one line
	radioTextMaxRead = 2 << 20
	radioTextKeep    = 30 * time.Second // an answer is reused for this long
)

type radioTextEntry struct {
	text string
	at   time.Time
}

// radioTextCache and its lock belong to the hub.
type radioTextState struct {
	mu      sync.Mutex
	cache   map[string]radioTextEntry // box + ref
	running map[string]bool           // per box
}

func (h *Radio) radioTextNow() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// text answers GET /api/v1/radio/text?ref=.
func (h *Radio) text(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	if !ValidLiveRef(ref) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_query", "ref must be a broadcast service reference")
		return
	}
	ctx := r.Context()
	key := box.Key(ctx)
	k := key + "|" + ref

	h.rt.mu.Lock()
	if e, ok := h.rt.cache[k]; ok && h.radioTextNow().Sub(e.at) < radioTextKeep {
		h.rt.mu.Unlock()
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ref": ref, "text": e.text})
		return
	}
	if h.rt.running[key] {
		h.rt.mu.Unlock()
		httpx.WriteError(w, r, http.StatusTooManyRequests, "radio_text_busy", "another station's text is being read")
		return
	}
	if h.rt.running == nil {
		h.rt.running, h.rt.cache = map[string]bool{}, map[string]radioTextEntry{}
	}
	h.rt.running[key] = true
	h.rt.mu.Unlock()
	defer func() {
		h.rt.mu.Lock()
		delete(h.rt.running, key)
		h.rt.mu.Unlock()
	}()

	listen, cancel := context.WithTimeout(ctx, radioTextWait)
	defer cancel()
	body, err := h.WC.LiveStream(listen, ref)
	if err != nil {
		if errors.Is(err, webcontrol.ErrNotFound) {
			httpx.WriteError(w, r, http.StatusServiceUnavailable, CodeStreamNoTuner, "the box doesn't stream this station")
			return
		}
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach the box's stream")
		return
	}
	defer body.Close()
	text, err := radiotext.Read(body, radioTextMaxRead)
	if err != nil && listen.Err() == nil && text == "" {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not read the box's stream")
		return
	}
	h.rt.mu.Lock()
	h.rt.cache[k] = radioTextEntry{text: text, at: h.radioTextNow()}
	h.rt.mu.Unlock()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ref": ref, "text": text})
}

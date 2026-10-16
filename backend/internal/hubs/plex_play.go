package hubs

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plex"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Playing a Plex film on the box: Enigma2's own media player (servicemp4,
// gstreamer) plays the server's file over HTTP, untouched — no Plex client
// on the box, no transcoding. The box asks DreamHub on the same machine
// (StreamBase, 127.0.0.1), which passes the request on to Plex with the
// token, Range header included (seeking): so the token never ends up in a
// service reference the box keeps in its history.
//
// Whether a file plays depends on the box's decoders (container, video and
// audio codec); the frontend shows the file's formats next to the button.
// Confirmed on the reference box with 0.1.7: most files play, with
// seeking; a few report an unknown codec (ROADMAP.md, PlexHub).

// ticket lets the box fetch one file for a while, without credentials. The
// duration and size, when known, let the stream proxy turn the box's own
// Range requests into an approximate playback position for Timeline
// (progress.go, #30) — only an approximation (it assumes a roughly constant
// bitrate), but better than reporting nothing while Plex is off a sync.
type ticket struct {
	conn       plex.Conn
	partKey    string
	expires    time.Time
	ratingKey  string
	durationMs int64
	size       int64
	// lastReportAt throttles Timeline calls for this ticket (progress.go).
	lastReportAt time.Time
}

// ticketLife covers a long film with pauses.
const ticketLife = 12 * time.Hour

// maxTickets bounds the table; the oldest go first.
const maxTickets = 20

// resumeMargin: close to the start or the end isn't worth resuming for.
const resumeMargin = 30_000 // ms

// Error codes.
const (
	CodePlexPlayUnavailable = "plex_play_unavailable"
	CodePlexNoFile          = "plex_no_file"
)

// playRequest picks which of an item's media versions and parts to play
// (#31: a film in several parts or versions) — the first of each when
// omitted, as before.
type playRequest struct {
	Media int `json:"media"`
	Part  int `json:"part"`
}

func (h *Plex) play(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireConfig(w, r)
	if !ok {
		return
	}
	base := h.streamBase(r.Context())
	if !c.playOnBox || base == "" {
		httpx.WriteError(w, r, http.StatusConflict, CodePlexPlayUnavailable, "playing on the box needs DreamHub on the box and Settings → Plex → play on the box")
		return
	}
	var req playRequest
	if r.ContentLength > 0 {
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {} or {"media": <index>, "part": <index>}`)
			return
		}
	}
	key := r.PathValue("key")
	it, err := h.Client.Item(r.Context(), c.conn, key)
	h.note(err)
	if err != nil {
		h.writeWriteError(w, r, err)
		return
	}
	if it.Type != "movie" && it.Type != "episode" {
		httpx.WriteError(w, r, http.StatusConflict, CodePlexNoFile, "nothing playable: not a film or episode")
		return
	}
	if req.Media < 0 || req.Media >= len(it.Media) {
		httpx.WriteError(w, r, http.StatusConflict, CodePlexNoFile, "no such version of this item")
		return
	}
	media := it.Media[req.Media]
	if req.Part < 0 || req.Part >= len(media.Parts) {
		httpx.WriteError(w, r, http.StatusConflict, CodePlexNoFile, "no such part of this item")
		return
	}
	part := media.Parts[req.Part]
	// Resume where Plex left off, the file's first part only — Plex keeps
	// one ViewOffset per item, not per part, so it can't say which part it
	// was in; a later part always starts at its own beginning.
	var startMs int64
	if req.Part == 0 && it.ViewOffset > resumeMargin && it.ViewOffset < it.Duration-resumeMargin {
		startMs = it.ViewOffset
	}
	id := h.newTicket(ticket{conn: c.conn, partKey: part.Key, expires: h.Now().Add(ticketLife),
		ratingKey: it.RatingKey, durationMs: it.Duration, size: part.Size})
	ext := part.Container
	if ext == "" {
		ext = "mkv"
	}
	streamURL := base + "/api/v1/plex/stream/" + id + "/file." + ext
	// The movie player (seeking, pause), through DreamHub's TV plugin; a
	// plugin from before 0.1.7 doesn't have it, then a plain zap still
	// plays the film, just without the movie player's controls (and without
	// resume: the seek-on-open parameter needs the plugin too).
	ref := serviceRefForURL(streamURL, displayTitle(it))
	err = h.WC.PlayMovie(r.Context(), ref, displayTitle(it), startMs)
	if errors.Is(err, webcontrol.ErrNotFound) {
		err = h.WC.Zap(r.Context(), ref)
	}
	if err != nil {
		writeControlError(w, r, "play", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"playing": displayTitle(it)})
}

// serviceRefForURL is Enigma2's reference for playing a URL with its
// gstreamer player (4097): the URL's colons escaped, then the name shown
// on screen (colons there would end the reference early).
func serviceRefForURL(u, name string) string {
	name = strings.NewReplacer(":", " ", "\n", " ").Replace(name)
	return "4097:0:1:0:0:0:0:0:0:0:" + strings.ReplaceAll(u, ":", "%3a") + ":" + name
}

func (h *Plex) newTicket(t ticket) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.tickets == nil {
		h.tickets = map[string]ticket{}
	}
	now := h.Now()
	for id, old := range h.tickets {
		if now.After(old.expires) {
			delete(h.tickets, id)
		}
	}
	for len(h.tickets) >= maxTickets {
		var oldest string
		for id, old := range h.tickets {
			if oldest == "" || old.expires.Before(h.tickets[oldest].expires) {
				oldest = id
			}
		}
		delete(h.tickets, oldest)
	}
	id := randomHex(24)
	h.tickets[id] = t
	return id
}

// progressThrottle: how often one ticket's Timeline update is sent — far
// less often than the box's own Range requests while it buffers or seeks.
const progressThrottle = 10 * time.Second

// streamStallTimeout: a plain io.Copy from Plex to the box has no limit of
// its own — if Plex (or the LAN between them) stalls mid-film, the copy
// just hangs forever. A film's video bitrate is many times its audio
// bitrate, so a fixed-size player buffer holds far less *time* of video
// than of audio: a stall shows up on screen as the picture freezing while
// the sound keeps playing from its still-full buffer, well before the box
// gives up on its own. Closing the connection here once nothing has
// arrived for a while lets the box's own player notice and reconnect with
// a fresh Range request from where it left off, instead of hanging.
const streamStallTimeout = 15 * time.Second

// Stream passes one ticket's file on to the box's player. Public route
// (no DreamHub login: the box's player has none) but only for requests
// from the machine itself, with a valid ticket.
func (h *Plex) Stream(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "only the box itself plays from here")
		return
	}
	id := r.PathValue("ticket")
	h.mu.Lock()
	t, ok := h.tickets[id]
	report := ok && h.Now().Sub(t.lastReportAt) > progressThrottle
	if report {
		t.lastReportAt = h.Now()
		h.tickets[id] = t
	}
	h.mu.Unlock()
	if !ok || h.Now().After(t.expires) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "unknown or expired ticket")
		return
	}
	if report {
		h.reportProgress(t, r.Header.Get("Range"))
	}
	resp, err := h.Client.Open(r.Context(), t.conn, t.partKey, r.Header.Get("Range"))
	if err != nil {
		h.note(err)
		httpx.WriteError(w, r, http.StatusBadGateway, CodePlexOffline, "the Plex server doesn't answer")
		return
	}
	defer resp.Body.Close()
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag"} {
		if v := resp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		copyUntilStall(w, resp.Body, streamStallTimeout)
	}
}

// copyUntilStall is io.Copy, but gives up once src has produced nothing for
// idle: it closes src (unblocking its Read, which runs in the background
// and would otherwise leak) and returns. The caller's response ends there;
// nothing else here needs to react, since the box's own reconnect (if any)
// comes back as a brand new request.
func copyUntilStall(dst io.Writer, src io.ReadCloser, idle time.Duration) {
	buf := make([]byte, 32*1024)
	type result struct {
		n   int
		err error
	}
	for {
		ch := make(chan result, 1)
		go func() {
			n, err := src.Read(buf)
			ch <- result{n, err}
		}()
		select {
		case res := <-ch:
			if res.n > 0 {
				if _, err := dst.Write(buf[:res.n]); err != nil {
					return
				}
			}
			if res.err != nil {
				return
			}
		case <-time.After(idle):
			src.Close()
			return
		}
	}
}

// reportProgress turns the byte offset the box just asked for into an
// approximate playback position (constant-bitrate assumption — the best
// DreamHub can do without decoding the file) and reports it to Plex as
// "playing" (#30). Fire-and-forget: a Plex server that's gone offline mid-
// film must not slow down or break the stream itself; it just means
// "continue watching" falls a sync behind, same as everything else cached.
func (h *Plex) reportProgress(t ticket, rangeHeader string) {
	if t.size <= 0 || t.durationMs <= 0 || t.ratingKey == "" {
		return
	}
	start := rangeStart(rangeHeader)
	if start < 0 {
		start = 0 // no Range: the box opened the file from the very start
	}
	timeMs := int64(float64(start) / float64(t.size) * float64(t.durationMs))
	if timeMs > t.durationMs {
		timeMs = t.durationMs
	}
	conn, ratingKey, durationMs := t.conn, t.ratingKey, t.durationMs
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = h.Client.Timeline(ctx, conn, ratingKey, "playing", timeMs, durationMs)
	}()
}

// rangeStart reads the first byte of a "Range: bytes=N-" request header, or
// -1 when there's no usable Range (a plain GET from the start, or a
// multi-range request DreamHub doesn't need to support here).
func rangeStart(header string) int64 {
	const prefix = "bytes="
	if !strings.HasPrefix(header, prefix) {
		return -1
	}
	first := strings.SplitN(strings.TrimPrefix(header, prefix), ",", 2)[0]
	n, err := strconv.ParseInt(strings.SplitN(first, "-", 2)[0], 10, 64)
	if err != nil {
		return -1
	}
	return n
}

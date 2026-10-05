package hubs

import (
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plex"
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
// Experimental until confirmed on the reference box (ROADMAP.md).

// ticket lets the box fetch one file for a while, without credentials.
type ticket struct {
	conn    plex.Conn
	partKey string
	expires time.Time
}

// ticketLife covers a long film with pauses.
const ticketLife = 12 * time.Hour

// maxTickets bounds the table; the oldest go first.
const maxTickets = 20

// Error codes.
const (
	CodePlexPlayUnavailable = "plex_play_unavailable"
	CodePlexNoFile          = "plex_no_file"
)

func (h *Plex) play(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireConfig(w, r)
	if !ok {
		return
	}
	if !c.playOnBox || h.StreamBase == "" {
		httpx.WriteError(w, r, http.StatusConflict, CodePlexPlayUnavailable, "playing on the box needs DreamHub on the box and Settings → Plex → play on the box")
		return
	}
	key := r.PathValue("key")
	it, err := h.Client.Item(r.Context(), c.conn, key)
	h.note(err)
	if err != nil {
		h.writeWriteError(w, r, err)
		return
	}
	if (it.Type != "movie" && it.Type != "episode") || len(it.Media) == 0 || len(it.Media[0].Parts) == 0 {
		httpx.WriteError(w, r, http.StatusConflict, CodePlexNoFile, "nothing playable: not a film or episode, or no file")
		return
	}
	part := it.Media[0].Parts[0]
	id := h.newTicket(ticket{conn: c.conn, partKey: part.Key, expires: h.Now().Add(ticketLife)})
	ext := part.Container
	if ext == "" {
		ext = "mkv"
	}
	streamURL := h.StreamBase + "/api/v1/plex/stream/" + id + "/file." + ext
	if err := h.WC.Zap(r.Context(), serviceRefForURL(streamURL, displayTitle(it))); err != nil {
		writeControlError(w, r, "zap", err)
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

// Stream passes one ticket's file on to the box's player. Public route
// (no DreamHub login: the box's player has none) but only for requests
// from the machine itself, with a valid ticket.
func (h *Plex) Stream(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "only the box itself plays from here")
		return
	}
	h.mu.Lock()
	t, ok := h.tickets[r.PathValue("ticket")]
	h.mu.Unlock()
	if !ok || h.Now().After(t.expires) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "unknown or expired ticket")
		return
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
		io.Copy(w, resp.Body)
	}
}

package hubs

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/boxes"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Watching a recording of box A on box B (docs/MULTIDEVICE_PLAN.md, M7).
// B's movie player (DreamHub's TV plugin, like for a Plex film) fetches the
// recording from this DreamHub, which reads it from A — in place when A is
// the machine DreamHub runs on, else through A's WebControl. A therefore
// needs no DreamHub, B must be paired (or be DreamHub's own box) and must
// be able to reach this DreamHub (the same StreamBase as for Plex).
//
// B fetches without a login (its player has none): a ticket in the path is
// the credential, and the request must come from B's own address.

// RelayBoxes is what the relay needs to know about the other boxes.
type RelayBoxes interface {
	Get(id string) (boxes.Entry, bool)
	// PlayBase is how that box reaches this DreamHub; "" if it can't play.
	PlayBase(id string) string
	// Admits: a request from ip is that box's own player.
	Admits(id string, ip net.IP) bool
}

// Error codes.
const (
	CodeRelayUnavailable = "relay_unavailable" // the target box can't play from here
	CodeRelaySameBox     = "relay_same_box"
)

// relayTicket lets one box fetch one recording of another for a while.
type relayTicket struct {
	source, target, path string
	remote               bool // the source is not the machine DreamHub runs on
	expires              time.Time
}

const maxRelays = 20

func (h *Stream) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

type playOnRequest struct {
	Target string `json:"target"`
	Path   string `json:"path"`
	Title  string `json:"title"`
}

// playOn starts the recording at path (on the box of the request) in the
// movie player of another box.
func (h *Stream) playOn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req playOnRequest
	if h.Boxes == nil || httpx.DecodeJSON(r, &req) != nil || req.Target == "" || req.Path == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"target": <box id>, "path": <recording file>}`)
		return
	}
	target, ok := h.Boxes.Get(req.Target)
	if !ok {
		httpx.WriteError(w, r, http.StatusNotFound, "box_not_found", "no such box")
		return
	}
	if user, _ := httpx.User(ctx); !user.MayUseBox(target.ID) {
		httpx.WriteError(w, r, http.StatusForbidden, "box_forbidden", "this profile may not use that box")
		return
	}
	if target.ID == box.Key(ctx) {
		httpx.WriteError(w, r, http.StatusConflict, CodeRelaySameBox, "this recording is on that box already")
		return
	}
	base := h.Boxes.PlayBase(target.ID)
	if base == "" {
		httpx.WriteError(w, r, http.StatusConflict, CodeRelayUnavailable, "that box can't play from here: it needs DreamHub on it, paired with this one")
		return
	}
	p := path.Clean("/" + req.Path)
	if !strings.HasSuffix(p, ".ts") {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such recording")
		return
	}
	// The recording must exist: ask for its first byte.
	roots, err := h.WC.Locations(ctx)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	if !h.recordingExists(ctx, p, roots) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such recording")
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = strings.TrimSuffix(path.Base(p), ".ts")
	}
	id := h.newRelay(relayTicket{source: box.Key(ctx), target: target.ID, path: p, remote: box.Remote(ctx),
		expires: h.now().Add(ticketLife)})
	ref := serviceRefForURL(base+"/api/v1/stream/relay/"+id+"/file.ts", title)
	// The target's own client: the request's box is the source.
	err = target.WC.PlayMovie(ctx, ref, title, 0)
	if errors.Is(err, webcontrol.ErrNotFound) {
		err = target.WC.Zap(ctx, ref) // a plugin from before the movie player
	}
	if err != nil {
		writeControlError(w, r, "play", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"playing": title, "target": target.ID})
}

// recordingExists: a regular .ts below one of the source's folders, read the
// way serveRecording will read it.
func (h *Stream) recordingExists(ctx context.Context, p string, roots []string) bool {
	if onBox(ctx, h.FS, roots) {
		cp, err := recfs.CleanIn(h.FS, p, roots)
		if err != nil {
			return false
		}
		fi, err := h.FS.Lstat(cp)
		return err == nil && fi.Mode().IsRegular()
	}
	if !underAny(p, roots) {
		return false
	}
	f, err := h.WC.RecordingFile(ctx, p, "bytes=0-0")
	if err != nil {
		return false
	}
	f.Body.Close()
	return true
}

func (h *Stream) newRelay(t relayTicket) string {
	h.relayMu.Lock()
	defer h.relayMu.Unlock()
	if h.relays == nil {
		h.relays = map[string]relayTicket{}
	}
	now := h.now()
	for id, old := range h.relays {
		if now.After(old.expires) {
			delete(h.relays, id)
		}
	}
	for len(h.relays) >= maxRelays {
		var oldest string
		for id, old := range h.relays {
			if oldest == "" || old.expires.Before(h.relays[oldest].expires) {
				oldest = id
			}
		}
		delete(h.relays, oldest)
	}
	id := randomHex(24)
	h.relays[id] = t
	return id
}

// Relay is the public route the target box's player fetches from.
func (h *Stream) Relay(w http.ResponseWriter, r *http.Request) {
	h.relayMu.Lock()
	t, ok := h.relays[r.PathValue("ticket")]
	h.relayMu.Unlock()
	if !ok || h.now().After(t.expires) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "unknown or expired ticket")
		return
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || h.Boxes == nil || !h.Boxes.Admits(t.target, net.ParseIP(host)) {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "only the box this was started for plays from here")
		return
	}
	ctx := box.With(r.Context(), t.source)
	if t.remote {
		ctx = box.WithRemote(ctx)
	}
	h.serveRecording(w, r, ctx, t.path)
}

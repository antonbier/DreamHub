package hubs

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Control is ControlHub (CONCEPT.md: "Web remote, message to TV,
// screenshot, power"). Every action here is a direct, immediate call, not
// preview-and-confirm (PRINCIPLES.md): that principle is about durable
// data mutations with real consequences — deleting a recording, a timer —
// and a virtual remote is inherently a live, instantly-reversible input
// device, the same way WebControl's own native remote control page (which
// DreamHub deliberately doesn't reimplement, PRINCIPLES.md) works.
// Confirmed live 2026-09-25 (ARCHITECTURE.md, "Real screenshot and
// remote-control endpoints").
type Control struct {
	WC webcontrol.Client
}

// Perms are ControlHub's route permissions beyond the hub's own (internal/perm).
func (h *Control) Perms() map[string]perm.Perm {
	return map[string]perm.Perm{
		"POST /api/v1/control/key":     perm.Control,
		"POST /api/v1/control/message": perm.Control,
		"POST /api/v1/control/zap":     perm.Control,
	}
}

// Mount registers ControlHub's routes.
func (h *Control) Mount(mux httpx.Mux) {
	mux.HandleFunc("GET /api/v1/control/screenshot", h.screenshot)
	mux.HandleFunc("GET /api/v1/control/power", h.getPower)
	mux.HandleFunc("GET /api/v1/control/current", h.current)
	mux.HandleFunc("POST /api/v1/control/key", h.sendKey)
	mux.HandleFunc("POST /api/v1/control/zap", h.zap)
	mux.HandleFunc("POST /api/v1/control/message", h.message)
}

func (h *Control) screenshot(w http.ResponseWriter, r *http.Request) {
	img, err := h.WC.Screenshot(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store") // it's a live view, never cache a stale frame
	w.Write(img)
}

func (h *Control) getPower(w http.ResponseWriter, r *http.Request) {
	standby, err := h.WC.InStandby(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"inStandby": standby})
}

// current is what the box shows right now (the dashboard's "Now on the
// box" tile): the service, its kind — "tv", "radio" or "recording" (a file
// being played back) — now/next, volume, and whether it's in standby,
// where the service is just the one it will wake up to.
func (h *Control) current(w http.ResponseWriter, r *http.Request) {
	cur, err := h.WC.Current(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
		return
	}
	kind := "tv"
	switch {
	case webcontrol.IsRecordingReference(cur.ServiceReference):
		kind = "recording"
	case webcontrol.IsRadioService(cur.ServiceReference):
		kind = "radio"
	}
	// Not knowing the power state isn't worth failing the whole answer.
	standby, _ := h.WC.InStandby(r.Context())
	httpx.WriteJSON(w, http.StatusOK, struct {
		*webcontrol.CurrentService
		Kind      string `json:"kind"`
		InStandby bool   `json:"inStandby"`
	}{cur, kind, standby})
}

// keyBody is {"code": <Linux input-event code>} — the exact codes
// ControlHub's own remote pad sends (ARCHITECTURE.md has the confirmed
// table: KEY_OK 352, KEY_EXIT 174, arrows, colors, KEY_POWER 116, ...).
// Not restricted to a fixed list: a code the box's current screen doesn't
// handle is simply ignored by the box itself (a brief on-screen icon, not
// an error), exactly like an unmapped physical remote button.
type keyBody struct {
	Code int `json:"code"`
}

func (h *Control) sendKey(w http.ResponseWriter, r *http.Request) {
	var body keyBody
	if err := httpx.DecodeJSON(r, &body); err != nil || body.Code <= 0 || body.Code > 767 {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"code": <positive Linux input-event code>}`)
		return
	}
	if err := h.WC.SendKey(r.Context(), body.Code); err != nil {
		writeControlError(w, r, "key", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"sent": body.Code})
}

type zapBody struct {
	ServiceReference string `json:"serviceReference"`
}

// zap switches the live channel (ControlHub's remote, and GuideHub's
// "switch now" on a currently-running programme).
func (h *Control) zap(w http.ResponseWriter, r *http.Request) {
	var body zapBody
	if err := httpx.DecodeJSON(r, &body); err != nil || body.ServiceReference == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"serviceReference"}`)
		return
	}
	if err := h.WC.Zap(r.Context(), body.ServiceReference); err != nil {
		writeControlError(w, r, "zap", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"zapped": body.ServiceReference})
}

type messageBody struct {
	Text    string `json:"text"`
	Kind    string `json:"kind"`    // "info" (default), "warning", "error"
	Timeout int    `json:"timeout"` // seconds, 0 = until closed; default 10 when omitted is the UI's choice
}

// maxMessageRunes keeps a message to what fits a TV message box.
const maxMessageRunes = 300

// message shows a text on the TV ("dinner's ready"). Direct, like the
// remote: it's a transient on-screen box, not a durable change.
func (h *Control) message(w http.ResponseWriter, r *http.Request) {
	var body messageBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"text", "kind", "timeout"}`)
		return
	}
	text := strings.TrimSpace(body.Text)
	kinds := map[string]int{"": webcontrol.MessageInfo, "info": webcontrol.MessageInfo, "warning": webcontrol.MessageWarning, "error": webcontrol.MessageError}
	kind, ok := kinds[body.Kind]
	if text == "" || utf8.RuneCountInString(text) > maxMessageRunes || !ok || body.Timeout < 0 || body.Timeout > 600 {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_message", "text 1-300 characters, kind info/warning/error, timeout 0-600 s")
		return
	}
	if err := h.WC.Message(r.Context(), text, kind, body.Timeout); err != nil {
		writeControlError(w, r, "message", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"sent": true})
}

func writeControlError(w http.ResponseWriter, r *http.Request, op string, err error) {
	var rej *webcontrol.RejectedError
	switch {
	case err == webcontrol.ErrWritesDisabled:
		httpx.WriteError(w, r, http.StatusConflict, "writes_disabled", "changes to the box are disabled (DREAMHUB_ENABLE_DEVICE_WRITES=0)")
	case err == webcontrol.ErrUnauthorized:
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unauthorized", "WebControl asked for credentials; set them in Settings → WebControl")
	case errors.As(err, &rej):
		httpx.WriteErrorDetails(w, r, http.StatusConflict, CodeBoxRejected, "the box refused the "+op, map[string]any{"boxText": rej.Text})
	default:
		httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
	}
}

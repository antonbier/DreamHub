package hubs

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// AdminHub's terminal (docs/ADMINHUB_PLAN.md, A4, #36; CONCEPT.md,
// "DeviceHub"): not a DreamHub-built terminal, a proxy in front of the
// box's existing shellinaboxd (127.0.0.1:4200, the same one WebAdmin
// already uses at /webadmin/terminal/, ARCHITECTURE.md). Off by default;
// enabling it needs auth turned on globally and the password typed again
// right now (not just an already-authenticated session — PRINCIPLES.md's
// one explicit exception to preview-and-confirm, since there's nothing to
// preview about opening a shell). ShellInABox itself still asks for an OS
// login once inside — this gate is DreamHub's, that one is the box's,
// independently.

// terminalProxyPrefix is stripped before forwarding to shellinaboxd, which
// (confirmed live, ARCHITECTURE.md) already serves correctly from a
// non-root path behind WebControl's own gateway at /webadmin/terminal/.
const terminalProxyPrefix = "/api/v1/admin/terminal/proxy"

// defaultTerminalTarget is shellinaboxd's address on the box.
const defaultTerminalTarget = "http://127.0.0.1:4200"

type terminalState struct {
	mu      sync.Mutex
	enabled bool
}

func (t *terminalState) get() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.enabled
}

func (t *terminalState) set(v bool) {
	t.mu.Lock()
	t.enabled = v
	t.mu.Unlock()
}

// terminalOn: the terminal is enabled and password protection is still on —
// switching protection off later must not leave a shell open to the LAN.
func (h *Admin) terminalOn() bool {
	return h.terminal.get() && h.Auth != nil && h.Auth.AuthConfig().Enabled
}

func (h *Admin) target() string {
	if h.TerminalTarget != "" {
		return h.TerminalTarget
	}
	return defaultTerminalTarget
}

func (h *Admin) terminalStatus(w http.ResponseWriter, r *http.Request) {
	available := h.Auth != nil
	authOn := available && h.Auth.AuthConfig().Enabled
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"available": available,
		"authOn":    authOn,
		"enabled":   h.terminalOn(),
	})
}

func (h *Admin) enableTerminal(w http.ResponseWriter, r *http.Request) {
	if h.Auth == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "terminal_unavailable", "the terminal isn't available on this system")
		return
	}
	cfg := h.Auth.AuthConfig()
	if !cfg.Enabled {
		httpx.WriteError(w, r, http.StatusConflict, "auth_required", "turn on DreamHub's own password protection first (Settings)")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil || body.Password == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"password": "..."}`)
		return
	}
	if !cfg.VerifyPassword(body.Password) {
		httpx.WriteError(w, r, http.StatusUnauthorized, "wrong_password", "wrong password")
		return
	}
	h.terminal.set(true)
	user, _ := httpx.User(r.Context())
	_ = h.Store.AppendChange(r.Context(), store.ChangeEntry{UserID: user.ID, Action: "admin.terminal.enable", Summary: "Enabled the terminal"})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"enabled": true})
}

func (h *Admin) disableTerminal(w http.ResponseWriter, r *http.Request) {
	h.terminal.set(false)
	user, _ := httpx.User(r.Context())
	_ = h.Store.AppendChange(r.Context(), store.ChangeEntry{UserID: user.ID, Action: "admin.terminal.disable", Summary: "Disabled the terminal"})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"enabled": false})
}

// terminalProxy forwards everything under terminalProxyPrefix to
// shellinaboxd, only while the terminal is enabled. Each individual
// request isn't logged (ShellInABox polls constantly while open); enabling
// and disabling already bracket the session in the change log.
func (h *Admin) terminalProxy(w http.ResponseWriter, r *http.Request) {
	if !h.terminalOn() {
		httpx.WriteError(w, r, http.StatusConflict, "terminal_disabled", "the terminal isn't enabled")
		return
	}
	target, err := url.Parse(h.target())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "bad terminal target")
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	r.URL.Path = strings.TrimPrefix(r.URL.Path, terminalProxyPrefix)
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}
	proxy.ServeHTTP(w, r)
}

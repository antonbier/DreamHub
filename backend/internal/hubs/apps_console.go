package hubs

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/wsutil"
)

// This file is AppsHub's App5, the logs and console slice
// (docs/APPSHUB_PLAN.md): per-container `docker logs` (the daemon's own
// journald-backed health already reuses DeviceHub's existing log viewer
// unchanged, by linking there with ?logUnit=docker — nothing new needed
// backend-side for that tier) and a per-container console with
// Terminal's exact safety posture (A4, #36): off by default, needs
// DreamHub's own auth turned on and the password typed again right now,
// a standing warning, admin-only.

func (h *Apps) containerLogs(w http.ResponseWriter, r *http.Request) {
	if h.Docker == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_unavailable", "the Docker helper isn't available")
		return
	}
	tail := 200
	if v := r.URL.Query().Get("tail"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			tail = n
		}
	}
	out, err := h.Docker.Logs(r.Context(), r.PathValue("id"), tail)
	if err == dockerd.ErrNotFound {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such installed app")
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "docker_failed", "the logs couldn't be read")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"text": string(out)})
}

// consoleState is the exact same shape admin_terminal.go's terminalState
// uses — off by default, enabled only by an explicit, freshly
// re-authenticated call, with no persistence across a restart (the
// plan's own bar: "not a lighter version just because it's inside a
// container").
type consoleState struct {
	mu      sync.Mutex
	enabled bool
}

func (c *consoleState) get() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enabled
}

func (c *consoleState) set(v bool) {
	c.mu.Lock()
	c.enabled = v
	c.mu.Unlock()
}

func (h *Apps) consoleStatus(w http.ResponseWriter, r *http.Request) {
	available := h.Auth != nil && h.Docker != nil
	authOn := available && h.Auth.AuthConfig().Enabled
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"available": available,
		"authOn":    authOn,
		"enabled":   h.console.get(),
	})
}

func (h *Apps) enableConsole(w http.ResponseWriter, r *http.Request) {
	if h.Auth == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "console_unavailable", "the console isn't available on this system")
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
	h.console.set(true)
	user, _ := httpx.User(r.Context())
	_ = h.Store.AppendChange(r.Context(), store.ChangeEntry{UserID: user.ID, Action: "apps.console.enable", Summary: "Enabled the Docker app console"})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"enabled": true})
}

func (h *Apps) disableConsole(w http.ResponseWriter, r *http.Request) {
	h.console.set(false)
	user, _ := httpx.User(r.Context())
	_ = h.Store.AppendChange(r.Context(), store.ChangeEntry{UserID: user.ID, Action: "apps.console.disable", Summary: "Disabled the Docker app console"})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"enabled": false})
}

// containerConsole upgrades to a WebSocket and bridges it to a PTY
// session inside the container (dockerd.Client.Attach) — binary frames
// carry raw bytes in both directions, nothing is parsed or
// interpreted here beyond framing. Each individual byte isn't logged
// (the same reasoning admin_terminal.go's own proxy gives for not
// logging every polled request); enabling and disabling the console
// already bracket the capability in the change log.
func (h *Apps) containerConsole(w http.ResponseWriter, r *http.Request) {
	if !h.console.get() {
		httpx.WriteError(w, r, http.StatusConflict, "console_disabled", "the console isn't enabled")
		return
	}
	if h.Docker == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_unavailable", "the Docker helper isn't available")
		return
	}
	upstream, err := h.Docker.Attach(r.Context(), r.PathValue("id"))
	if err == dockerd.ErrNotFound {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such installed app")
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "docker_failed", "could not attach to the container")
		return
	}
	defer upstream.Close()

	ws, err := wsutil.Upgrade(w, r)
	if err != nil {
		return // wsutil.Upgrade already wrote the HTTP error itself
	}
	defer ws.Close()

	errCh := make(chan struct{}, 2)
	go func() {
		defer func() { errCh <- struct{}{} }()
		buf := make([]byte, 4096)
		for {
			n, err := upstream.Read(buf)
			if n > 0 {
				if werr := ws.WriteMessage(wsutil.OpBinary, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		defer func() { errCh <- struct{}{} }()
		for {
			op, data, err := ws.ReadMessage()
			if err != nil || op == wsutil.OpClose {
				return
			}
			if op != wsutil.OpBinary && op != wsutil.OpText {
				continue
			}
			if _, werr := upstream.Write(data); werr != nil {
				return
			}
		}
	}()
	<-errCh // either direction ending is enough to tear the whole session down
}

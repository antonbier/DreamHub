package hubs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plex"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// Signing in to Plex the way TV apps do: DreamHub asks plex.tv for a short
// code, the user enters it at plex.tv/link (signed in there as usual),
// DreamHub picks up the account's token and lists its servers with their
// addresses; the user chooses one (or types another address, e.g. one
// behind their own reverse proxy) and DreamHub saves address and token to
// the Plex settings. Nothing about any particular server is built in.
//
// The account token stays in memory only for the few minutes this takes;
// what's saved is the chosen server's own access token.

type pinState struct {
	pin       plex.Pin
	token     string
	resources []plex.Resource
}

// ensureClientID returns DreamHub's Plex client identifier, making and
// saving one the first time. Plex lists it under the account's devices.
func (h *Plex) ensureClientID(ctx context.Context) (string, error) {
	h.mu.Lock()
	id := h.clientID
	h.mu.Unlock()
	if id != "" {
		return id, nil
	}
	v, err := h.Settings.Values(ctx, h.User)
	if err != nil {
		return "", err
	}
	id = v.String(connector.KeyPlexClientID)
	if id == "" {
		id = "dreamhub-" + randomHex(12)
		raw, _ := json.Marshal(id)
		if err := h.Settings.Update(ctx, h.User, map[string]json.RawMessage{connector.KeyPlexClientID: raw}); err != nil {
			return "", err
		}
	}
	if c, ok := h.Client.(*plex.HTTPClient); ok {
		c.ClientID = id
	}
	h.mu.Lock()
	h.clientID = id
	h.mu.Unlock()
	return id, nil
}

func (h *Plex) loginStart(w http.ResponseWriter, r *http.Request) {
	clientID, err := h.ensureClientID(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not save the Plex client identifier")
		return
	}
	pin, err := h.Client.CreatePin(r.Context(), clientID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "plex_tv_unavailable", "plex.tv doesn't answer")
		return
	}
	h.mu.Lock()
	if h.pins == nil {
		h.pins = map[int64]*pinState{}
	}
	for id, p := range h.pins {
		if h.Now().After(p.pin.ExpiresAt) {
			delete(h.pins, id)
		}
	}
	h.pins[pin.ID] = &pinState{pin: pin}
	h.mu.Unlock()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": pin.ID, "code": pin.Code, "expiresAt": pin.ExpiresAt, "linkUrl": "https://plex.tv/link"})
}

func (h *Plex) pinFor(r *http.Request) (*pinState, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	p, ok := h.pins[id]
	return p, ok
}

// serverView is one of the account's servers as the sign-in lists it.
type serverView struct {
	Name        string            `json:"name"`
	ID          string            `json:"id"`
	Owned       bool              `json:"owned"`
	Connections []plex.Connection `json:"connections"`
}

// loginCheck: "waiting" until the code was entered, then "linked" with the
// account's servers; "expired" once the code is no longer valid.
func (h *Plex) loginCheck(w http.ResponseWriter, r *http.Request) {
	p, ok := h.pinFor(r)
	if !ok {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"state": "expired"})
		return
	}
	clientID, _ := h.ensureClientID(r.Context())
	if p.token == "" {
		token, err := h.Client.CheckPin(r.Context(), clientID, p.pin.ID)
		switch {
		case errors.Is(err, plex.ErrPinExpired):
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"state": "expired"})
			return
		case err != nil:
			httpx.WriteError(w, r, http.StatusBadGateway, "plex_tv_unavailable", "plex.tv doesn't answer")
			return
		case token == "":
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"state": "waiting"})
			return
		}
		res, err := h.Client.Resources(r.Context(), clientID, token)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadGateway, "plex_tv_unavailable", "plex.tv doesn't answer")
			return
		}
		h.mu.Lock()
		p.token, p.resources = token, res
		h.mu.Unlock()
	}
	servers := []serverView{}
	for _, res := range p.resources {
		conns := []plex.Connection{}
		for _, c := range res.Connections {
			if !c.Relay {
				conns = append(conns, c)
			}
		}
		servers = append(servers, serverView{Name: res.Name, ID: res.ClientIdentifier, Owned: res.Owned, Connections: conns})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"state": "linked", "servers": servers})
}

// loginUse saves the chosen server: {serverId, url}. The URL is one of the
// server's listed addresses or one the user typed; it must answer as that
// server before anything is saved.
func (h *Plex) loginUse(w http.ResponseWriter, r *http.Request) {
	p, ok := h.pinFor(r)
	if !ok || p.token == "" {
		httpx.WriteError(w, r, http.StatusConflict, "plex_login_expired", "sign in again")
		return
	}
	var body struct {
		ServerID string `json:"serverId"`
		URL      string `json:"url"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"serverId", "url"}`)
		return
	}
	u := strings.TrimRight(strings.TrimSpace(body.URL), "/")
	if parsed, err := url.Parse(u); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_url", "expected an http(s) address")
		return
	}
	var res *plex.Resource
	for i := range p.resources {
		if p.resources[i].ClientIdentifier == body.ServerID {
			res = &p.resources[i]
		}
	}
	if res == nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "plex_unknown_server", "not one of the account's servers")
		return
	}
	token := res.AccessToken
	if token == "" {
		token = p.token
	}
	conn := plex.Conn{URL: u, Token: token}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	id, err := h.Client.Identity(ctx, conn)
	if err == nil && id.MachineIdentifier != res.ClientIdentifier {
		httpx.WriteError(w, r, http.StatusConflict, "plex_wrong_server", "another Plex server answers at this address")
		return
	}
	if err == nil {
		_, err = h.Client.Server(ctx, conn)
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusConflict, "plex_unreachable_at", "the server doesn't answer at this address")
		return
	}
	user, _ := httpx.User(r.Context())
	rawURL, _ := json.Marshal(u)
	rawToken, _ := json.Marshal(token)
	rawFallbacks, _ := json.Marshal(strings.Join(fallbackAddresses(res.Connections, u), ","))
	if err := h.Settings.Update(r.Context(), user, map[string]json.RawMessage{connector.KeyPlexURL: rawURL, connector.KeyPlexToken: rawToken, connector.KeyPlexFallbackURLs: rawFallbacks}); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	_ = h.Store.AppendChange(r.Context(), store.ChangeEntry{UserID: user.ID, Action: "plex.connect", Summary: "Connected Plex server " + res.Name + " at " + u})
	h.mu.Lock()
	delete(h.pins, p.pin.ID)
	h.lastErr, h.lastTry, h.active = nil, time.Time{}, ""
	h.mu.Unlock()
	go h.Sync(context.WithoutCancel(r.Context()))
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"connected": res.Name, "url": u})
}

// fallbackAddresses are the server's other addresses plex.tv lists (no
// relay: slow and capped), the LAN ones first — tried when the chosen one
// doesn't answer.
func fallbackAddresses(conns []plex.Connection, chosen string) []string {
	var local, remote []string
	for _, c := range conns {
		u := strings.TrimRight(c.URI, "/")
		if c.Relay || u == "" || u == chosen || contains(local, u) || contains(remote, u) {
			continue
		}
		if c.Local {
			local = append(local, u)
		} else {
			remote = append(remote, u)
		}
	}
	return append(local, remote...)
}

package hubs

import (
	"context"
	"net"
	"net/http"
	neturl "net/url"

	"github.com/antonbier/DreamHub/backend/internal/extension"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/status"
)

// Extensions is Extension Hub (docs/EXTENSIONHUB_PLAN.md, E1, #46): one
// nav entry listing every registered extension's own card, each a few
// status/meter/list/action widgets an extension's separate process
// describes — never frontend code of its own (PRINCIPLES.md: "preview and
// confirm, always" stays intact because an action widget's button goes
// through the same plan.Service every built-in hub's own actions do).
type Extensions struct {
	Registry *extension.Registry
	Plans    *plan.Service
	ReadOnly bool
}

// Perms declares the one non-GET route that isn't admin-only by default
// (guardMux/identity.go): previewing an action is gated per-extension,
// inside the handler, by the extension's own declared Admin flag — not
// globally admin-only like an unannotated changing route would default to.
func (h *Extensions) Perms() map[string]perm.Perm {
	return map[string]perm.Perm{"POST /api/v1/extensions/{id}/actions/{action}/plan": perm.Watch}
}

// Mount registers Extension Hub's own routes (behind DreamHub's login).
// Register (the handshake) is mounted separately as a public route — see
// app.go — since an extension's process has no DreamHub login of its own.
func (h *Extensions) Mount(mux httpx.Mux) {
	mux.HandleFunc("GET /api/v1/extensions", h.list)
	mux.HandleFunc("POST /api/v1/extensions/{id}/actions/{action}/plan", h.planAction)
}

// cardView is one extension's card, with its own status alongside the
// core's existing available/degraded/unavailable vocabulary
// (status.Level) — the same "always explain what's missing" rule every
// hub already follows, just evaluated per card instead of per hub.
type cardView struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Icon     string             `json:"icon"`
	Admin    bool               `json:"admin"`
	Level    status.Level       `json:"status"`
	Problems []status.Problem   `json:"problems"`
	Widgets  []extension.Widget `json:"widgets"`
}

// Problem codes for a card's own status — local to this hub, like
// admin_services.go's restart warnings, rather than status.go's shared
// list: these describe something specific to an extension, not a
// connector or capability.
const (
	codeExtensionUnsupported = "extension.unsupportedVersion"
	codeExtensionUnreachable = "extension.unreachable"
)

func (h *Extensions) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := httpx.User(ctx)
	perms := perm.For(user)
	entries := h.Registry.List()
	cards := make([]cardView, 0, len(entries))
	for _, e := range entries {
		if e.Admin && !perms.Has(perm.System) {
			continue
		}
		card := cardView{ID: e.ID, Name: e.Name, Icon: e.Icon, Admin: e.Admin, Level: status.Available, Widgets: []extension.Widget{}, Problems: []status.Problem{}}
		switch {
		case e.SchemaVersion > extension.SchemaVersion:
			card.Level = status.Unavailable
			card.Problems = append(card.Problems, status.Problem{Code: codeExtensionUnsupported, Params: map[string]any{"name": e.Name}})
		default:
			widgets, err := e.Client.Widgets(ctx)
			if err != nil {
				card.Level = status.Unavailable
				card.Problems = append(card.Problems, status.Problem{Code: codeExtensionUnreachable, Params: map[string]any{"name": e.Name}})
			} else {
				card.Widgets = widgets
			}
		}
		cards = append(cards, card)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"extensions": cards})
}

// actionFingerprint is PlanAction's own fingerprint, scoped to this
// extension+action so a stale plan for one can't be confused with
// another's.
func actionFingerprint(id, action, fp string) string { return id + "|" + action + "|" + fp }

func (h *Extensions) planAction(w http.ResponseWriter, r *http.Request) {
	id, action := r.PathValue("id"), r.PathValue("action")
	e, ok := h.Registry.Get(id)
	if !ok {
		httpx.WriteError(w, r, http.StatusNotFound, "extension_not_found", "no such extension")
		return
	}
	user, _ := httpx.User(r.Context())
	if e.Admin && !perm.For(user).Has(perm.System) {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "this profile may not do this")
		return
	}
	ctx := r.Context()
	preview, err := e.Client.PlanAction(ctx, action)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "extension_unavailable", "could not reach the extension")
		return
	}
	pl := h.Plans.Create(user, plan.Draft{
		Action:      "extension." + id + "." + action,
		Warnings:    preview.Warnings,
		Summary:     preview.Summary,
		Detail:      map[string]string{"extension": id, "action": action},
		Fingerprint: actionFingerprint(id, action, preview.Fingerprint),
		Current: func(ctx context.Context) (string, error) {
			p, err := e.Client.PlanAction(ctx, action)
			if err != nil {
				return "", err
			}
			return actionFingerprint(id, action, p.Fingerprint), nil
		},
		Execute: func(ctx context.Context) error {
			if h.ReadOnly {
				return &plan.Failure{Status: http.StatusConflict, Code: "writes_disabled", Message: "device writes are disabled"}
			}
			if err := e.Client.Apply(ctx, action, preview.Fingerprint); err != nil {
				return &plan.Failure{Status: http.StatusBadGateway, Code: "extension_apply_failed", Message: "the extension could not apply this", Err: err}
			}
			return nil
		},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": pl})
}

// --- registration handshake -------------------------------------------------

// registerRequest is what an extension's process posts to register
// itself (docs/EXTENSIONHUB_PLAN.md, "The contract").
type registerRequest struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Icon          string `json:"icon"`
	Admin         bool   `json:"admin"`
	SchemaVersion int    `json:"schemaVersion"`
	// BaseURL is where DreamHub's backend calls the extension back — must
	// be a loopback address (ARCHITECTURE.md's systemd notes, E3's
	// isolation: the extension binds only to localhost, nothing
	// extension-related is reachable except through DreamHub itself).
	BaseURL string `json:"baseUrl"`
}

// Register answers the loopback-only handshake (POST
// /api/v1/extensions/register, mounted as a public route in app.go: an
// extension's process has no DreamHub login). Modeled on Plex.Stream's
// own loopback check (hubs/plex_play.go) — the same reasoning: this
// box's own processes only, nothing reachable over the network.
func (h *Extensions) Register(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
		httpx.WriteError(w, r, http.StatusForbidden, "forbidden", "only a process on this device may register")
		return
	}
	var req registerRequest
	if err := httpx.DecodeJSON(r, &req); err != nil || req.ID == "" || req.Name == "" || req.BaseURL == "" || req.SchemaVersion < 1 {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"id","name","schemaVersion" (>= 1),"baseUrl",...}`)
		return
	}
	baseHost, _, err := net.SplitHostPort(mustHost(req.BaseURL))
	if ip := net.ParseIP(baseHost); err != nil || ip == nil || !ip.IsLoopback() {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", "baseUrl must be a loopback address")
		return
	}
	h.Registry.Register(extension.Entry{
		Registration: extension.Registration{ID: req.ID, Name: req.Name, Icon: req.Icon, Admin: req.Admin, SchemaVersion: req.SchemaVersion},
		Client:       extension.HTTPClient{BaseURL: req.BaseURL},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"registered": true, "coreSchemaVersion": extension.SchemaVersion})
}

// mustHost strips a URL down to its host:port for the loopback check,
// tolerating a bare host:port without a scheme too.
func mustHost(baseURL string) string {
	u, err := neturl.Parse(baseURL)
	if err == nil && u.Host != "" {
		if _, _, err := net.SplitHostPort(u.Host); err == nil {
			return u.Host
		}
		return u.Host + ":80"
	}
	return baseURL
}

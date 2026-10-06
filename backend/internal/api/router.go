// Package api implements DreamHub's core REST API and assembles the hubs'
// routes. Per PRINCIPLES.md's "API-first" principle, every route lives under
// /api/v1 and is the same API any client (the web UI, a mobile app) would
// call — no web-UI-only backend logic.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/apitoken"
	"github.com/antonbier/DreamHub/backend/internal/auth"
	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/dashboard"
	"github.com/antonbier/DreamHub/backend/internal/enrich"
	"github.com/antonbier/DreamHub/backend/internal/events"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/hub"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/push"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/skip2/go-qrcode"
)

// Version is DreamHub's own version, following SemVer per PRINCIPLES.md.
// Reported by /api/v1/health so the Enigma2 plugin can detect a mismatch.
const Version = "0.1.8"

// Mount is a hub's set of routes. The core serves them under the given
// path prefixes, behind auth and the hub's enable switch.
type Mount struct {
	HubID    string
	Prefixes []string
	Register func(*http.ServeMux)
}

// PublicRoute is a hub route outside DreamHub's login — only for a client
// that can't have one (the box's own media player); its handler does its
// own checks. Still answers 404 hub_disabled when the hub is off.
type PublicRoute struct {
	HubID   string
	Pattern string
	Handler http.Handler
}

// Deps are the router's dependencies.
type Deps struct {
	Public     []PublicRoute
	WebControl webcontrol.Client
	Verifier   auth.Verifier
	Settings   *settings.Service
	Connectors *connector.Registry
	Hubs       *hub.Registry
	Plans      *plan.Service
	Dashboard  *dashboard.Service
	Tokens     *apitoken.Service
	// Watcher notices a hub's status actually changing between requests and
	// Store records that as a change-log event (hubStatuses) — see
	// internal/hub/watcher.go's doc comment for what this is and isn't.
	Watcher *hub.StatusWatcher
	Store   *store.Store
	// Events is the core event bus (internal/events, docs/V1_PLAN.md WP7):
	// the persisted notification feed and its live SSE stream.
	Events *events.Bus
	// Enrich adds posters, ratings and overviews to titles
	// (internal/enrich); nil turns /api/v1/enrich off.
	Enrich *enrich.Service
	// Push (nil: not wired) sends the test notification.
	Push   *push.Notifier
	Mounts []Mount
	// User is the current user. v1 has exactly one user (an admin); once
	// profiles exist this comes from the authenticated request instead.
	User   store.User
	Logger *slog.Logger
	Now    func() time.Time
}

// NewRouter builds the full /api/v1 handler tree.
func NewRouter(d Deps) http.Handler {
	if d.Now == nil {
		d.Now = time.Now
	}

	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/v1/whoami", handleWhoami(d))
	protected.HandleFunc("GET /api/v1/deviceinfo", handleDeviceInfo(d))
	protected.HandleFunc("GET /api/v1/settings", handleGetSettings(d))
	protected.HandleFunc("PATCH /api/v1/settings", handlePatchSettings(d))
	protected.HandleFunc("GET /api/v1/navigation", handleNavigation(d))
	protected.HandleFunc("GET /api/v1/hubs", handleHubs(d))
	protected.HandleFunc("GET /api/v1/connectors", handleConnectors(d))
	protected.HandleFunc("GET /api/v1/dashboard", handleGetDashboard(d))
	protected.HandleFunc("PUT /api/v1/dashboard", handlePutDashboard(d))
	protected.HandleFunc("POST /api/v1/enrich/lookup", handleEnrichLookup(d))
	protected.HandleFunc("GET /api/v1/enrich/image/{size}/{file}", handleEnrichImage(d))
	protected.HandleFunc("GET /api/v1/enrich/extras", handleEnrichExtras(d))
	protected.HandleFunc("GET /api/v1/enrich/logo", handleEnrichLogo(d))
	protected.HandleFunc("POST /api/v1/push/test", handlePushTest(d))
	protected.HandleFunc("GET /api/v1/dashboard/sources", handleViewSources(d))
	protected.HandleFunc("POST /api/v1/dashboard/view", handleView(d))
	protected.HandleFunc("GET /api/v1/search", handleSearch(d))
	protected.HandleFunc("GET /api/v1/tokens", handleListTokens(d))
	protected.HandleFunc("POST /api/v1/tokens", handleCreateToken(d))
	protected.HandleFunc("DELETE /api/v1/tokens/{id}", handleDeleteToken(d))
	protected.HandleFunc("POST /api/v1/plans/{id}/apply", handleApplyPlan(d))
	protected.HandleFunc("GET /api/v1/notifications", handleNotifications(d))
	protected.HandleFunc("POST /api/v1/notifications/read", handleMarkNotificationsRead(d))
	protected.HandleFunc("GET /api/v1/events/stream", handleEventsStream(d))
	for _, m := range d.Mounts {
		sub := http.NewServeMux()
		m.Register(sub)
		guarded := hubEnabledGuard(d, m.HubID, sub)
		for _, p := range m.Prefixes {
			protected.Handle(p, guarded)
			protected.Handle(p+"/", guarded)
		}
	}

	mux := http.NewServeMux()
	// Public on purpose: the Enigma2 plugin checks the backend version before
	// it has any credentials, and must keep working when auth is enabled.
	mux.HandleFunc("GET /api/v1/health", handleHealth(d))
	// Public for the same reason health is: the Enigma2 launcher plugin
	// (docs/V1_PLAN.md, WP8) shows this before any credentials exist, and
	// it encodes nothing secret — just the URL to open on a phone.
	mux.HandleFunc("GET /api/v1/qr.png", handleQR(d))
	for _, p := range d.Public {
		mux.Handle(p.Pattern, hubEnabledGuard(d, p.HubID, p.Handler))
	}
	mux.Handle("/api/v1/", auth.Middleware(d.Verifier, protected))

	allowedHosts := func(r *http.Request) []string {
		v, err := d.Settings.Values(r.Context(), d.User)
		if err != nil {
			return nil
		}
		return v.AllowedHosts()
	}
	withUser := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(httpx.WithUser(r.Context(), d.User)))
	})
	return httpx.RequestIDAndLogging(d.Logger, d.Now,
		httpx.HostCheck(allowedHosts,
			httpx.CSRF(withUser)))
}

// hubEnabledGuard answers 404 hub_disabled for a hub switched off in
// settings, so its routes behave as if it weren't installed.
func hubEnabledGuard(d Deps, hubID string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, err := d.Settings.Values(r.Context(), d.User)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not read settings")
			return
		}
		if !v.Bool(hub.EnabledKey(hubID)) {
			httpx.WriteError(w, r, http.StatusNotFound, "hub_disabled", "this hub is disabled in settings")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// serverTimeZone reports the box's own configured IANA zone (e.g.
// "Europe/Berlin"), for the per-user "box" display-zone mode (ROADMAP.md,
// "Time zones") — WebControl's API has no timezone field (confirmed,
// ARCHITECTURE.md), but /etc/localtime is a symlink to
// .../zoneinfo/<Area>/<City> on every Linux distribution (and macOS, useful
// for local dev) — that symlink target is the one place this is actually
// derivable without SSH. Empty on any error; callers fall back to the
// browser's own zone.
func serverTimeZone() string {
	target, err := os.Readlink("/etc/localtime")
	if err != nil {
		return ""
	}
	const marker = "zoneinfo/"
	i := strings.LastIndex(target, marker)
	if i < 0 {
		return ""
	}
	return target[i+len(marker):]
}

// handleQR renders a QR code for the current origin as a PNG — used by the
// Enigma2 launcher plugin (docs/V1_PLAN.md, WP8) so it needs no QR library
// and holds no logic of its own, and by the web UI's onboarding wizard.
// r.Host is exactly what a browser or the plugin already used to reach
// this server, so the code always points somewhere that actually works —
// unlike guessing an IP from the box's network interfaces, which could
// pick the wrong one on a multi-homed box (LAN + ZeroTier, see
// ARCHITECTURE.md).
func handleQR(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		png, err := qrcode.Encode(scheme+"://"+r.Host, qrcode.Medium, 256)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not generate the QR code")
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store") // the box's address can change (DHCP); never cache a stale one
		w.Write(png)
	}
}

func handleHealth(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": Version,
			// Server time in UTC, so clients can warn when the box clock is off
			// (CONCEPT.md, "Time zones").
			"time":           d.Now().UTC().Format(time.RFC3339),
			"serverTimeZone": serverTimeZone(),
		})
	}
}

// handleWhoami reports the current user. Protected (behind auth), so it
// doubles as a credentials check: the web UI's login screen submits a
// username/password as a Basic Auth header here and treats 200 as success,
// 401 as a wrong password (see frontend/src/composables/useAuth.ts). v1 has
// exactly one user, so this doesn't vary by who actually authenticated.
func handleWhoami(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{
			"username": d.User.Name,
			"role":     string(d.User.Role),
		})
	}
}

func handleDeviceInfo(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info, err := d.WebControl.DeviceInfo(r.Context())
		if err != nil {
			d.Logger.Error("deviceinfo failed", "requestId", httpx.RequestID(r.Context()), "error", err)
			httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unavailable", "could not reach WebControl")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, info)
	}
}

func handleGetSettings(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		view, err := d.Settings.View(r.Context(), d.User)
		if err != nil {
			d.Logger.Error("settings view failed", "requestId", httpx.RequestID(r.Context()), "error", err)
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not load settings")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"sections": view})
	}
}

func handlePatchSettings(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Values map[string]json.RawMessage `json:"values"`
		}
		if err := httpx.DecodeJSON(r, &body); err != nil || body.Values == nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"values": {"<key>": <value>, ...}}`)
			return
		}
		if err := d.Settings.Update(r.Context(), d.User, body.Values); err != nil {
			if verr, ok := settings.IsValidation(err); ok {
				httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "one or more settings are invalid; nothing was saved", verr.Fields)
				return
			}
			d.Logger.Error("settings update failed", "requestId", httpx.RequestID(r.Context()), "error", err)
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not save settings")
			return
		}
		view, err := d.Settings.View(r.Context(), d.User)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "saved, but could not reload settings")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"sections": view})
	}
}

// hubStatuses evaluates every hub for the current user.
func hubStatuses(d Deps, r *http.Request) ([]hub.Status, settings.Values, error) {
	v, err := d.Settings.Values(r.Context(), d.User)
	if err != nil {
		return nil, nil, err
	}
	reports := d.Connectors.Reports(r.Context(), v, d.Settings.Version())
	statuses := d.Hubs.Statuses(v, reports)
	recordStatusTransitions(d, r, statuses)
	return statuses, v, nil
}

// recordStatusTransitions writes any hub status change (internal/hub's
// StatusWatcher) to the change log, with no owning user — that's how a
// system-detected event is told apart from one a user made (see
// internal/store's ChangeEntry). Best-effort: a logging failure here must
// never break the request that happened to notice the transition.
func recordStatusTransitions(d Deps, r *http.Request, statuses []hub.Status) {
	if d.Watcher == nil || d.Store == nil {
		return
	}
	for _, t := range d.Watcher.Diff(statuses) {
		detail, _ := json.Marshal(map[string]any{"hub": t.HubID, "from": t.From, "to": t.To})
		if err := d.Store.AppendChange(r.Context(), store.ChangeEntry{
			Action: "hub.statusChanged", Summary: t.Summary(), Detail: detail,
		}); err != nil {
			d.Logger.Error("recording hub status transition failed", "requestId", httpx.RequestID(r.Context()), "error", err)
		}
	}
}

func handleNavigation(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		statuses, v, err := hubStatuses(d, r)
		if err != nil {
			d.Logger.Error("navigation failed", "requestId", httpx.RequestID(r.Context()), "error", err)
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not build navigation")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, d.Hubs.Navigation(statuses, d.User.Role, v.String(settings.KeyNavLabels)))
	}
}

// handleHubs is the diagnostics view: every hub the user may see, including
// disabled ones, with status, problems and requirements.
func handleHubs(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		statuses, _, err := hubStatuses(d, r)
		if err != nil {
			d.Logger.Error("hub status failed", "requestId", httpx.RequestID(r.Context()), "error", err)
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not evaluate hubs")
			return
		}
		visible := make([]hub.Status, 0, len(statuses))
		for _, s := range statuses {
			if hub.Visible(s.Role, d.User.Role) {
				visible = append(visible, s)
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"hubs": visible})
	}
}

func handleConnectors(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := d.Settings.Values(r.Context(), d.User)
		if err != nil {
			d.Logger.Error("connector status failed", "requestId", httpx.RequestID(r.Context()), "error", err)
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not evaluate connectors")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"connectors": d.Connectors.SortedReports(r.Context(), v, d.Settings.Version())})
	}
}

// handleGetDashboard returns the current user's dashboard layout,
// reconciled against the built-in tile catalog (internal/dashboard).
func handleGetDashboard(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		layout, err := d.Dashboard.Get(r.Context(), d.User)
		if err != nil {
			d.Logger.Error("dashboard get failed", "requestId", httpx.RequestID(r.Context()), "error", err)
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not load the dashboard")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, layout)
	}
}

// handlePutDashboard saves the dashboard layout directly — no
// preview-and-confirm step, same exception as DreamHub's own settings
// (PRINCIPLES.md): this changes the app's own state, not the device.
func handlePutDashboard(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body dashboard.Layout
		if err := httpx.DecodeJSON(r, &body); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"tiles": [{"type", "width", "hidden"}, ...]}`)
			return
		}
		layout, err := d.Dashboard.Save(r.Context(), d.User, body)
		if err != nil {
			if errors.Is(err, dashboard.ErrInvalidTile) {
				httpx.WriteError(w, r, http.StatusBadRequest, "invalid_tile", err.Error())
				return
			}
			d.Logger.Error("dashboard save failed", "requestId", httpx.RequestID(r.Context()), "error", err)
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not save the dashboard")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, layout)
	}
}

// handleSearch queries every registered hub search provider (internal/hub,
// "The hub contract") for hubs the current user can currently see —
// CONCEPT.md, "Global search". An empty or missing q returns no results
// rather than erroring.
func handleSearch(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": []hub.SearchResult{}})
			return
		}
		v, err := d.Settings.Values(r.Context(), d.User)
		if err != nil {
			d.Logger.Error("search failed", "requestId", httpx.RequestID(r.Context()), "error", err)
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not search")
			return
		}
		reports := d.Connectors.Reports(r.Context(), v, d.Settings.Version())
		results := d.Hubs.Search(r.Context(), v, reports, d.User.Role, q)
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": results})
	}
}

// handleViewSources lists the view sources custom dashboard tiles can use
// right now (internal/hub/view.go) — only hubs the user can currently see.
func handleViewSources(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := d.Settings.Values(r.Context(), d.User)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not load settings")
			return
		}
		reports := d.Connectors.Reports(r.Context(), v, d.Settings.Version())
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"sources": d.Hubs.ViewSources(v, reports, d.User.Role)})
	}
}

// viewBody is a custom tile's criteria, evaluated for display (a saved
// tile) or for the builder's live preview. Read-only despite the POST: the
// criteria are structured, not a query string.
type viewBody struct {
	Source string        `json:"source"`
	Query  hub.ViewQuery `json:"query"`
	// TimeZone is the zone "from 20:15" means (the user's display zone);
	// UTC when empty or unknown.
	TimeZone string `json:"timeZone"`
}

func handleView(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body viewBody
		if err := httpx.DecodeJSON(r, &body); err != nil || body.Source == "" {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"source", "query", "timeZone"}`)
			return
		}
		loc := time.UTC
		if body.TimeZone != "" {
			if l, err := time.LoadLocation(body.TimeZone); err == nil {
				loc = l
			}
		}
		v, err := d.Settings.Values(r.Context(), d.User)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not load settings")
			return
		}
		reports := d.Connectors.Reports(r.Context(), v, d.Settings.Version())
		res, err := d.Hubs.View(r.Context(), v, reports, d.User.Role, body.Source, body.Query, d.Now(), loc)
		switch {
		case errors.Is(err, hub.ErrUnknownSource):
			httpx.WriteError(w, r, http.StatusNotFound, "unknown_source", "no hub offers this source")
		case errors.Is(err, hub.ErrSourceUnavailable):
			httpx.WriteError(w, r, http.StatusConflict, "source_unavailable", "this source's hub is disabled or unavailable")
		case err != nil:
			httpx.WriteError(w, r, http.StatusBadGateway, "source_failed", "the source could not be read")
		default:
			httpx.WriteJSON(w, http.StatusOK, res)
		}
	}
}

// handleListTokens lists the current user's API tokens — never the secret
// itself (internal/apitoken).
func handleListTokens(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokens, err := d.Tokens.List(r.Context(), d.User)
		if err != nil {
			d.Logger.Error("listing tokens failed", "requestId", httpx.RequestID(r.Context()), "error", err)
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not list tokens")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"tokens": tokens})
	}
}

// handleCreateToken generates a new API token and returns its secret once
// (PRINCIPLES.md: a separate, revocable credential for app/API clients).
func handleCreateToken(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if err := httpx.DecodeJSON(r, &body); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"name": "..."}`)
			return
		}
		created, err := d.Tokens.Create(r.Context(), d.User, body.Name)
		if err != nil {
			if errors.Is(err, apitoken.ErrNameRequired) {
				httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "a name is required",
					map[string]httpx.FieldError{"name": {Code: "required"}})
				return
			}
			d.Logger.Error("creating token failed", "requestId", httpx.RequestID(r.Context()), "error", err)
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not create the token")
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, created)
	}
}

// handleDeleteToken revokes a token; 404 if it doesn't exist or belongs to
// another user (never distinguished in the response, so one user can't
// probe another's token IDs).
func handleDeleteToken(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "id must be a number")
			return
		}
		if err := d.Tokens.Revoke(r.Context(), d.User, id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				httpx.WriteError(w, r, http.StatusNotFound, "token_not_found", "no such token")
				return
			}
			d.Logger.Error("revoking token failed", "requestId", httpx.RequestID(r.Context()), "error", err)
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not revoke the token")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleApplyPlan executes a previewed plan — the second of the two steps of
// every change (PRINCIPLES.md, "Preview and confirm").
func handleApplyPlan(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := d.Plans.Apply(r.Context(), d.User, r.PathValue("id"))
		var failure *plan.Failure
		switch {
		case err == nil:
			httpx.WriteJSON(w, http.StatusOK, res)
		case errors.Is(err, plan.ErrNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "plan_not_found", "plan not found, expired or already used; preview again")
		case errors.Is(err, plan.ErrNotOwner):
			httpx.WriteError(w, r, http.StatusForbidden, "plan_forbidden", "this plan belongs to another user")
		case errors.Is(err, plan.ErrStale):
			httpx.WriteError(w, r, http.StatusConflict, "plan_stale", "the state changed since the preview; nothing was done, preview again")
		case errors.Is(err, webcontrol.ErrWritesDisabled):
			httpx.WriteError(w, r, http.StatusConflict, "writes_disabled", "changes to the box are disabled (DREAMHUB_ENABLE_DEVICE_WRITES=0)")
		case errors.As(err, &failure):
			d.Logger.Warn("plan apply refused", "requestId", httpx.RequestID(r.Context()), "code", failure.Code, "error", err)
			httpx.WriteErrorDetails(w, r, failure.Status, failure.Code, failure.Message, failure.Details)
		case errors.Is(err, webcontrol.ErrUnauthorized):
			httpx.WriteError(w, r, http.StatusBadGateway, "webcontrol_unauthorized", "WebControl asked for credentials; set them in Settings → WebControl")
		default:
			d.Logger.Error("plan apply failed", "requestId", httpx.RequestID(r.Context()), "error", err)
			httpx.WriteError(w, r, http.StatusBadGateway, "apply_failed", "the change could not be applied")
		}
	}
}

package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/profile"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// ProfileHeader carries a device's profile session token
// (docs/PROFILES_PLAN.md: a header like everything else the frontend
// sends — no cookies, so no new CSRF surface).
const ProfileHeader = "X-DreamHub-Profile"

// Who a request is, after the door (internal/auth) let it in.
var (
	errProfileRequired = errors.New("profile required")
	errProfileSession  = errors.New("profile session invalid")
)

// resolveUser decides whose request this is: an API token's owner, the
// device's chosen profile, or — while the household has a single profile
// without a PIN — that profile.
func resolveUser(d Deps, r *http.Request) (store.User, error) {
	ctx := r.Context()
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") && d.Tokens != nil {
		if id, ok := d.Tokens.Owner(ctx, strings.TrimPrefix(a, "Bearer ")); ok {
			return d.Store.User(ctx, id)
		}
	}
	if token := r.Header.Get(ProfileHeader); token != "" {
		u, err := d.Profiles.Resolve(ctx, token)
		if err != nil {
			return store.User{}, errProfileSession
		}
		return u, nil
	}
	u, ok, err := d.Profiles.Single(ctx)
	if err != nil {
		return store.User{}, err
	}
	if !ok {
		return store.User{}, errProfileRequired
	}
	return u, nil
}

// identify puts the request's profile into its context, or answers 401
// with a code the frontend turns into the profile picker.
func identify(d Deps, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := resolveUser(d, r)
		switch {
		case errors.Is(err, errProfileRequired):
			httpx.WriteError(w, r, http.StatusUnauthorized, "profile_required", "choose a profile first")
		case errors.Is(err, errProfileSession):
			httpx.WriteError(w, r, http.StatusUnauthorized, "profile_session_invalid", "this device's profile session ended; choose a profile again")
		case err != nil:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not identify the profile")
		default:
			next.ServeHTTP(w, r.WithContext(httpx.WithUser(r.Context(), u)))
		}
	})
}

// current returns the request's profile; identify guarantees there is one
// on every protected route.
func current(r *http.Request) store.User {
	u, _ := httpx.User(r.Context())
	return u
}

// perms returns the request's profile's permissions.
func perms(r *http.Request) perm.Set { return perm.For(current(r)) }

// requirePerm answers 403 unless the request's profile has every need.
func requirePerm(next http.Handler, needs ...perm.Perm) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		have := perms(r)
		for _, p := range needs {
			if !have.Has(p) {
				httpx.WriteErrorDetails(w, r, http.StatusForbidden, "forbidden", "this profile may not do this",
					map[string]httpx.FieldError{"permission": {Code: string(p)}})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Route is one registered route and the permissions it needs.
type Route struct {
	Pattern string
	Needs   []perm.Perm
	// Declared: the route's permission was stated, not defaulted.
	Declared bool
}

// guardMux registers routes behind their permission check: the hub's own
// permission, plus the route's declared one. A route that changes
// something and declares nothing is admin-only (perm.System) — so a new
// hub can't accidentally hand viewers a delete button.
type guardMux struct {
	mux    *http.ServeMux
	hub    perm.Perm
	routes map[string]perm.Perm
	seen   *[]Route
}

func (g guardMux) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	g.Handle(pattern, http.HandlerFunc(h))
}

func (g guardMux) Handle(pattern string, h http.Handler) {
	needs := []perm.Perm{g.hub}
	p, declared := g.routes[pattern]
	switch {
	case declared:
		needs = append(needs, p)
	case mutatingPattern(pattern):
		needs = append(needs, perm.System)
	}
	*g.seen = append(*g.seen, Route{Pattern: pattern, Needs: needs, Declared: declared})
	g.mux.Handle(pattern, requirePerm(h, needs...))
}

// mutatingPattern reports whether a pattern's method changes something.
func mutatingPattern(pattern string) bool {
	method, _, ok := strings.Cut(pattern, " ")
	if !ok {
		return true // any method
	}
	return method != http.MethodGet && method != http.MethodHead
}

// checkDeclared panics on a declared permission for a route that was never
// registered — a typo would otherwise silently fall back to the default.
func checkDeclared(declared map[string]perm.Perm, seen []Route) {
	for pattern := range declared {
		found := false
		for _, r := range seen {
			if r.Pattern == pattern {
				found = true
			}
		}
		if !found {
			panic("api: permission declared for unregistered route " + pattern)
		}
	}
}

// --- profiles ---------------------------------------------------------------

// handleListProfiles is the picker: every profile (no PIN hashes), which
// one this device uses if any, and whether a picker is needed at all.
// Behind the door but not behind a profile.
func handleListProfiles(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := d.Profiles.List(r.Context())
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not list profiles")
			return
		}
		_, single, _ := d.Profiles.Single(r.Context())
		var currentID int64
		if u, err := resolveUser(d, r); err == nil {
			currentID = u.ID
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"profiles": list, "pickerNeeded": !single, "current": currentID, "colors": profile.Colors,
		})
	}
}

// handleStartSession picks a profile on this device.
func handleStartSession(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "id must be a number")
			return
		}
		var body struct {
			Pin string `json:"pin"`
		}
		if r.ContentLength > 0 {
			if err := httpx.DecodeJSON(r, &body); err != nil {
				httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"pin": "..."}`)
				return
			}
		}
		token, u, err := d.Profiles.StartSession(r.Context(), id, body.Pin)
		var locked *profile.LockedError
		switch {
		case errors.As(err, &locked):
			w.Header().Set("Retry-After", strconv.Itoa(int(locked.RetryAfter.Seconds())+1))
			httpx.WriteError(w, r, http.StatusTooManyRequests, "pin_locked", "too many wrong PINs; wait and try again")
		case errors.Is(err, profile.ErrWrongPin):
			httpx.WriteError(w, r, http.StatusForbidden, "pin_wrong", "wrong PIN")
		case errors.Is(err, profile.ErrNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "profile_not_found", "no such profile")
		case err != nil:
			httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not start the session")
		default:
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"token": token, "profile": profile.View(u)})
		}
	}
}

// handleEndSession signs this device out of its profile.
func handleEndSession(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token := r.Header.Get(ProfileHeader); token != "" {
			_ = d.Profiles.EndSession(r.Context(), token)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleCreateProfile(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in profile.Input
		if err := httpx.DecodeJSON(r, &in); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"name", "role", "color", "pin", "overrides"}`)
			return
		}
		p, err := d.Profiles.Create(r.Context(), current(r), in)
		if writeProfileError(w, r, err) {
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, p)
	}
}

func handleUpdateProfile(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "id must be a number")
			return
		}
		var in profile.Input
		if err := httpx.DecodeJSON(r, &in); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"name", "role", "color", "pin", "overrides"}`)
			return
		}
		p, err := d.Profiles.Update(r.Context(), current(r), id, in)
		if writeProfileError(w, r, err) {
			return
		}
		httpx.WriteJSON(w, http.StatusOK, p)
	}
}

func handleDeleteProfile(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_id", "id must be a number")
			return
		}
		if writeProfileError(w, r, d.Profiles.Delete(r.Context(), current(r), id)) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleUpdateMyPin lets every profile change or remove its own PIN —
// the one profile change that isn't the admin's.
func handleUpdateMyPin(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Pin *string `json:"pin"`
		}
		if err := httpx.DecodeJSON(r, &body); err != nil || body.Pin == nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"pin": "..."}`)
			return
		}
		me := current(r)
		p, err := d.Profiles.Update(r.Context(), me, me.ID, profile.Input{Pin: body.Pin})
		if writeProfileError(w, r, err) {
			return
		}
		httpx.WriteJSON(w, http.StatusOK, p)
	}
}

// writeProfileError answers a profile service error; false when err is nil.
func writeProfileError(w http.ResponseWriter, r *http.Request, err error) bool {
	var verr *profile.ValidationError
	switch {
	case err == nil:
		return false
	case errors.As(err, &verr):
		fields := map[string]httpx.FieldError{}
		for k, code := range verr.Fields {
			fields[k] = httpx.FieldError{Code: code}
		}
		httpx.WriteErrorDetails(w, r, http.StatusBadRequest, "validation_failed", "one or more fields are invalid", fields)
	case errors.Is(err, profile.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "profile_not_found", "no such profile")
	case errors.Is(err, profile.ErrLastAdmin):
		httpx.WriteError(w, r, http.StatusConflict, "profile_last_admin", "at least one admin profile must remain")
	case errors.Is(err, profile.ErrAdminPinRequired):
		httpx.WriteError(w, r, http.StatusConflict, "profile_admin_pin_required", "admin profiles need a PIN once there's more than one profile")
	case errors.Is(err, profile.ErrSelf):
		httpx.WriteError(w, r, http.StatusConflict, "profile_self", "you can't delete your own profile")
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "could not save the profile")
	}
	return true
}

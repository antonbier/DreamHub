// Package auth implements DreamHub's auth model: Basic Auth for the web UI,
// a separate bearer API token for app/API clients, both optional and off by
// default. See PRINCIPLES.md, "Authentication: simple by default, pluggable
// for later" — OIDC is explicitly not a v1 requirement, but Verifier is an
// interface so a different implementation can replace this one later
// without touching callers.
package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
)

// Verifier checks whether a request is authenticated. Implementations must
// be safe for concurrent use.
type Verifier interface {
	// Verify returns true if the request carries valid credentials, or if
	// auth is disabled entirely.
	Verify(r *http.Request) bool
}

// Credentials holds the two independent auth mechanisms: a Basic Auth
// username/password for the web UI, and API tokens for app/API clients.
// Either can be used interchangeably against any endpoint.
type Credentials struct {
	Enabled  bool
	Username string
	Password string
	// Tokens are valid bearer tokens, e.g. generated from the Settings UI.
	// A map (not a single token) because more than one client — the web
	// UI's own session and a future mobile app — may hold independent,
	// individually revocable tokens.
	Tokens map[string]bool
}

// Verify implements Verifier.
func (c Credentials) Verify(r *http.Request) bool {
	if !c.Enabled {
		return true
	}

	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		return c.Tokens[token]
	}

	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(c.Username)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(c.Password)) == 1
	return userMatch && passMatch
}

// Middleware wraps an http.Handler, rejecting unauthenticated requests with
// 401 when v reports the request isn't verified.
func Middleware(v Verifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v.Verify(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="DreamHub"`)
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "valid credentials required")
	})
}

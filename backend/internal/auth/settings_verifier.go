package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Config is the auth configuration as stored in settings.
type Config struct {
	Enabled      bool
	Username     string
	PasswordHash string // bcrypt
	// Version changes whenever the configuration changes, so cached
	// successful logins become invalid immediately.
	Version uint64
}

// ConfigSource provides the current auth configuration (implemented by the
// settings service).
type ConfigSource interface {
	AuthConfig() Config
}

// VerifyPassword reports whether password matches this config's stored
// hash. Unlike Verify (which checks a request, and caches a hit for
// cacheTTL), this always does a fresh bcrypt compare — for the rare case
// where a sensitive action needs the user to type their password again
// right now, not just prove they have an authenticated session (AdminHub's
// terminal, PRINCIPLES.md's one exception to preview-and-confirm).
func (c Config) VerifyPassword(password string) bool {
	if c.PasswordHash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(c.PasswordHash), []byte(password)) == nil
}

// cacheTTL is how long a successful Basic Auth check is remembered. The
// browser sends the password with every request, and bcrypt is deliberately
// slow — tens of milliseconds per check on the box's ARM cores — so checking
// it on every request would noticeably slow the UI down.
const cacheTTL = 10 * time.Minute

// Failed logins are counted per remote address: after maxFailures wrong
// credentials within failWindow the address gets no further password or
// token checks for blockFor, so a device on the LAN can't guess at will. A
// login that is already cached keeps working (the owner's own browser), and
// the block applies per address only — behind a reverse proxy that address
// is the proxy's, which is why the numbers are generous.
const (
	maxFailures = 10
	failWindow  = 5 * time.Minute
	blockFor    = 5 * time.Minute
)

type failState struct {
	count int
	first time.Time
	until time.Time
}

// TokenVerifier checks a bearer API token (implemented by
// internal/apitoken.Service).
type TokenVerifier interface {
	Verify(ctx context.Context, token string) bool
}

// SettingsVerifier verifies Basic Auth against the credentials configured in
// settings, or a bearer token against tokens (internal/apitoken) — either
// is accepted for any endpoint (PRINCIPLES.md).
type SettingsVerifier struct {
	src    ConfigSource
	tokens TokenVerifier
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]time.Time
	fails map[string]*failState
}

// NewSettingsVerifier returns a verifier backed by src and tokens.
func NewSettingsVerifier(src ConfigSource, tokens TokenVerifier) *SettingsVerifier {
	return &SettingsVerifier{src: src, tokens: tokens, now: time.Now, cache: map[string]time.Time{}, fails: map[string]*failState{}}
}

// Verify implements Verifier.
func (v *SettingsVerifier) Verify(r *http.Request) bool {
	c := v.src.AuthConfig()
	if !c.Enabled {
		return true
	}
	addr := remoteHost(r)
	if tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		if v.blocked(addr) {
			return false
		}
		if v.tokens != nil && v.tokens.Verify(r.Context(), tok) {
			return true
		}
		v.fail(addr)
		return false
	}
	user, pass, ok := r.BasicAuth()
	if !ok || c.PasswordHash == "" {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(user), []byte(c.Username)) != 1 {
		if !v.blocked(addr) {
			v.fail(addr)
		}
		return false
	}

	sum := sha256.Sum256([]byte(strconv.FormatUint(c.Version, 10) + "\x00" + user + "\x00" + pass))
	key := hex.EncodeToString(sum[:])
	now := v.now()

	v.mu.Lock()
	exp, hit := v.cache[key]
	v.mu.Unlock()
	if hit && now.Before(exp) {
		return true
	}

	if v.blocked(addr) {
		return false
	}
	if bcrypt.CompareHashAndPassword([]byte(c.PasswordHash), []byte(pass)) != nil {
		v.fail(addr)
		return false
	}
	v.mu.Lock()
	delete(v.fails, addr)
	for k, e := range v.cache { // drop expired entries; the map stays tiny
		if !now.Before(e) {
			delete(v.cache, k)
		}
	}
	v.cache[key] = now.Add(cacheTTL)
	v.mu.Unlock()
	return true
}

func remoteHost(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}

func (v *SettingsVerifier) blocked(addr string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	f := v.fails[addr]
	return f != nil && v.now().Before(f.until)
}

func (v *SettingsVerifier) fail(addr string) {
	now := v.now()
	v.mu.Lock()
	defer v.mu.Unlock()
	for a, f := range v.fails { // forget old ones; the map stays small
		if now.After(f.until) && now.Sub(f.first) > failWindow {
			delete(v.fails, a)
		}
	}
	f := v.fails[addr]
	if f == nil || now.Sub(f.first) > failWindow {
		f = &failState{first: now}
		v.fails[addr] = f
	}
	f.count++
	if f.count >= maxFailures {
		f.until = now.Add(blockFor)
		f.count = 0
		f.first = now
	}
}

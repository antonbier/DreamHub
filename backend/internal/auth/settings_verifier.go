package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
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

// cacheTTL is how long a successful Basic Auth check is remembered. The
// browser sends the password with every request, and bcrypt is deliberately
// slow — tens of milliseconds per check on the box's ARM cores — so checking
// it on every request would noticeably slow the UI down.
const cacheTTL = 10 * time.Minute

// SettingsVerifier verifies Basic Auth against the credentials configured in
// settings. API tokens are not implemented yet; bearer requests are rejected
// while auth is enabled.
type SettingsVerifier struct {
	src ConfigSource
	now func() time.Time

	mu    sync.Mutex
	cache map[string]time.Time
}

// NewSettingsVerifier returns a verifier backed by src.
func NewSettingsVerifier(src ConfigSource) *SettingsVerifier {
	return &SettingsVerifier{src: src, now: time.Now, cache: map[string]time.Time{}}
}

// Verify implements Verifier.
func (v *SettingsVerifier) Verify(r *http.Request) bool {
	c := v.src.AuthConfig()
	if !c.Enabled {
		return true
	}
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return false
	}
	user, pass, ok := r.BasicAuth()
	if !ok || c.PasswordHash == "" {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(user), []byte(c.Username)) != 1 {
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

	if bcrypt.CompareHashAndPassword([]byte(c.PasswordHash), []byte(pass)) != nil {
		return false
	}
	v.mu.Lock()
	for k, e := range v.cache { // drop expired entries; the map stays tiny
		if !now.Before(e) {
			delete(v.cache, k)
		}
	}
	v.cache[key] = now.Add(cacheTTL)
	v.mu.Unlock()
	return true
}

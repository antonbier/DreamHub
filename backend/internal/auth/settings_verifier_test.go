package auth

import (
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type staticSource struct{ c Config }

func (s *staticSource) AuthConfig() Config { return s.c }

func TestSettingsVerifier(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct horse"), bcrypt.MinCost)
	src := &staticSource{c: Config{Enabled: false, Username: "armin", PasswordHash: string(hash), Version: 1}}
	v := NewSettingsVerifier(src)

	req := func(user, pass string) bool {
		r := httptest.NewRequest("GET", "/", nil)
		if user != "" {
			r.SetBasicAuth(user, pass)
		}
		return v.Verify(r)
	}

	if !req("", "") {
		t.Fatal("disabled auth must let everything through")
	}

	src.c.Enabled = true
	if req("", "") || req("armin", "wrong") || req("someone", "correct horse") {
		t.Fatal("missing or wrong credentials must be rejected")
	}
	if !req("armin", "correct horse") {
		t.Fatal("correct credentials must be accepted")
	}

	// Cached success is dropped as soon as the config version changes.
	src.c.Version = 2
	src.c.PasswordHash = "$2a$04$invalidinvalidinvalidinvalidinvalidinvalidinvalidinv"
	if req("armin", "correct horse") {
		t.Fatal("a changed password must invalidate cached logins immediately")
	}
}

func TestSettingsVerifier_CacheExpires(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("pw-12345"), bcrypt.MinCost)
	src := &staticSource{c: Config{Enabled: true, Username: "a", PasswordHash: string(hash), Version: 1}}
	v := NewSettingsVerifier(src)
	now := time.Date(2026, 9, 23, 18, 0, 0, 0, time.UTC)
	v.now = func() time.Time { return now }

	r := httptest.NewRequest("GET", "/", nil)
	r.SetBasicAuth("a", "pw-12345")
	if !v.Verify(r) {
		t.Fatal("expected success")
	}
	if len(v.cache) != 1 {
		t.Fatalf("expected one cache entry, got %d", len(v.cache))
	}
	now = now.Add(cacheTTL + time.Second)
	if !v.Verify(r) {
		t.Fatal("expired cache must fall back to bcrypt, still succeeding")
	}
	if len(v.cache) != 1 {
		t.Errorf("expired entries must be pruned, got %d", len(v.cache))
	}
}

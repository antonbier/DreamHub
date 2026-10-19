package auth

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type staticSource struct{ c Config }

func (s *staticSource) AuthConfig() Config { return s.c }

// staticTokens accepts exactly the tokens in the set.
type staticTokens map[string]bool

func (s staticTokens) Verify(_ context.Context, token string) bool { return s[token] }

func TestSettingsVerifier(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct horse"), bcrypt.MinCost)
	src := &staticSource{c: Config{Enabled: false, Username: "armin", PasswordHash: string(hash), Version: 1}}
	v := NewSettingsVerifier(src, staticTokens{"good-token": true})

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

	bearer := func(token string) bool {
		r := httptest.NewRequest("GET", "/", nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		return v.Verify(r)
	}
	if !bearer("good-token") {
		t.Fatal("a valid API token must be accepted")
	}
	if bearer("bad-token") {
		t.Fatal("an unknown API token must be rejected")
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
	v := NewSettingsVerifier(src, nil)
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

func TestSettingsVerifier_BlocksGuessing(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("right-pw"), bcrypt.MinCost)
	src := &staticSource{c: Config{Enabled: true, Username: "a", PasswordHash: string(hash), Version: 1}}
	v := NewSettingsVerifier(src, staticTokens{"tok": true})
	now := time.Unix(1_000_000, 0)
	v.now = func() time.Time { return now }

	req := func(addr, user, pass string) bool {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = addr + ":5555"
		r.SetBasicAuth(user, pass)
		return v.Verify(r)
	}
	for i := 0; i < maxFailures; i++ {
		if req("10.0.0.9", "a", "guess") {
			t.Fatal("a wrong password was accepted")
		}
	}
	if req("10.0.0.9", "a", "right-pw") {
		t.Fatal("the guessing address must be blocked, even with the right password")
	}
	if !req("10.0.0.2", "a", "right-pw") {
		t.Fatal("another address must not be affected")
	}
	now = now.Add(blockFor + time.Second)
	if !req("10.0.0.9", "a", "right-pw") {
		t.Fatal("the block must end")
	}
}

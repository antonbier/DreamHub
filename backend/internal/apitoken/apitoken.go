// Package apitoken implements bearer API tokens: a way for app/API clients
// to authenticate that's separate from the Basic Auth password and
// individually revocable (PRINCIPLES.md, "Authentication"). Settings →
// Authentication generates and revokes them in the web UI.
package apitoken

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/store"
)

// Token is a token as shown to a client — never the secret itself once
// created.
type Token struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// Created is the one-time response to a successful creation: the token
// plus its plaintext secret, which is never retrievable again.
type Created struct {
	Token
	Secret string `json:"token"`
}

// ErrNameRequired is returned by Create for an empty name.
var ErrNameRequired = errors.New("apitoken: name is required")

// tokenPrefix makes a token recognizable at a glance (in logs, in a
// password manager) without weakening it — the random part still carries
// all the entropy.
const tokenPrefix = "dh_"

// Service creates, lists, revokes and verifies tokens.
type Service struct {
	store *store.Store
	now   func() time.Time
}

// NewService returns a Service. now overrides the clock in tests; nil means
// time.Now.
func NewService(st *store.Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: st, now: now}
}

func hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// List returns a user's tokens, newest first.
func (s *Service) List(ctx context.Context, u store.User) ([]Token, error) {
	stored, err := s.store.APITokens(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	out := make([]Token, len(stored))
	for i, t := range stored {
		out[i] = Token{ID: t.ID, Name: t.Name, CreatedAt: t.CreatedAt}
	}
	return out, nil
}

// Create generates a new token, stores its hash and returns the plaintext
// secret once. The change is recorded in the change log (never the secret).
func (s *Service) Create(ctx context.Context, u store.User, name string) (Created, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Created{}, ErrNameRequired
	}

	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return Created{}, fmt.Errorf("apitoken: generating token: %w", err)
	}
	secret := tokenPrefix + hex.EncodeToString(raw)

	stored, err := s.store.CreateAPIToken(ctx, u.ID, name, hash(secret))
	if err != nil {
		return Created{}, err
	}

	detail, _ := json.Marshal(map[string]any{"name": name})
	_ = s.store.AppendChange(ctx, store.ChangeEntry{
		UserID: u.ID, Action: "apitoken.create", Summary: fmt.Sprintf("Created API token %q", name), Detail: detail,
	})

	return Created{Token: Token{ID: stored.ID, Name: stored.Name, CreatedAt: stored.CreatedAt}, Secret: secret}, nil
}

// Revoke deletes a token. store.ErrNotFound if it doesn't exist or belongs
// to another user.
func (s *Service) Revoke(ctx context.Context, u store.User, id int64) error {
	tokens, err := s.store.APITokens(ctx, u.ID)
	if err != nil {
		return err
	}
	var name string
	for _, t := range tokens {
		if t.ID == id {
			name = t.Name
		}
	}
	if err := s.store.DeleteAPIToken(ctx, u.ID, id); err != nil {
		return err
	}
	detail, _ := json.Marshal(map[string]any{"name": name})
	_ = s.store.AppendChange(ctx, store.ChangeEntry{
		UserID: u.ID, Action: "apitoken.revoke", Summary: fmt.Sprintf("Revoked API token %q", name), Detail: detail,
	})
	return nil
}

// Verify reports whether secret is a currently valid token. Safe to call on
// every request — a single indexed lookup by hash, no per-request password
// hashing (PRINCIPLES.md's bcrypt caching exists for the opposite reason:
// user-chosen passwords are deliberately slow to check; a 24-byte random
// token doesn't need that).
func (s *Service) Verify(ctx context.Context, secret string) bool {
	if secret == "" {
		return false
	}
	_, err := s.store.APITokenByHash(ctx, hash(secret))
	return err == nil
}

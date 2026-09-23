package apitoken

import (
	"context"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/store"
)

func newTestService(t *testing.T) (*Service, store.User) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	u, err := st.EnsureDefaultUser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return NewService(st, nil), u
}

func TestCreate_ReturnsASecretThatThenVerifies(t *testing.T) {
	s, u := newTestService(t)
	ctx := context.Background()

	created, err := s.Create(ctx, u, "My Phone")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Secret, "dh_") || len(created.Secret) < 20 {
		t.Errorf("unexpected secret shape: %q", created.Secret)
	}
	if created.Name != "My Phone" || created.ID == 0 {
		t.Errorf("unexpected token: %+v", created.Token)
	}
	if !s.Verify(ctx, created.Secret) {
		t.Error("the freshly created secret must verify")
	}
	if s.Verify(ctx, created.Secret+"x") {
		t.Error("a tampered secret must not verify")
	}
	if s.Verify(ctx, "") {
		t.Error("an empty secret must never verify")
	}
}

func TestCreate_RejectsAnEmptyName(t *testing.T) {
	s, u := newTestService(t)
	if _, err := s.Create(context.Background(), u, "   "); err != ErrNameRequired {
		t.Fatalf("got %v, want ErrNameRequired", err)
	}
}

func TestList_NeverIncludesTheSecret(t *testing.T) {
	s, u := newTestService(t)
	ctx := context.Background()
	created, _ := s.Create(ctx, u, "CLI")

	tokens, err := s.List(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 1 || tokens[0].ID != created.ID || tokens[0].Name != "CLI" {
		t.Fatalf("got %+v", tokens)
	}
}

func TestRevoke_TokenNoLongerVerifies(t *testing.T) {
	s, u := newTestService(t)
	ctx := context.Background()
	created, _ := s.Create(ctx, u, "CLI")

	if err := s.Revoke(ctx, u, created.ID); err != nil {
		t.Fatal(err)
	}
	if s.Verify(ctx, created.Secret) {
		t.Error("a revoked token must not verify anymore")
	}
	if err := s.Revoke(ctx, u, created.ID); err != store.ErrNotFound {
		t.Fatalf("revoking again: got %v, want ErrNotFound", err)
	}
}

func TestRevoke_CannotRevokeAnotherUsersToken(t *testing.T) {
	s, u := newTestService(t)
	ctx := context.Background()
	created, _ := s.Create(ctx, u, "CLI")

	other := store.User{ID: u.ID + 1, Name: "someone-else", Role: store.RoleUser}
	if err := s.Revoke(ctx, other, created.ID); err != store.ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
	if !s.Verify(ctx, created.Secret) {
		t.Error("the token must still be valid — the revoke above must not have gone through")
	}
}

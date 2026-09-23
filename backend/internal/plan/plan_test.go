package plan

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/store"
)

func setup(t *testing.T) (*Service, *store.Store, store.User, *time.Time) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	u, _ := st.EnsureDefaultUser(context.Background())
	now := time.Date(2026, 9, 23, 18, 0, 0, 0, time.UTC)
	return NewService(st, func() time.Time { return now }), st, u, &now
}

func draft(executed *int, fp string, current *string) Draft {
	return Draft{
		Action: "test.change", Summary: "did it",
		Fingerprint: fp,
		Current:     func(context.Context) (string, error) { return *current, nil },
		Execute:     func(context.Context) error { *executed++; return nil },
	}
}

func TestApply_OnceAndLogged(t *testing.T) {
	s, st, u, _ := setup(t)
	ctx := context.Background()
	n, cur := 0, "v1"
	p := s.Create(u, draft(&n, "v1", &cur))

	if _, err := s.Apply(ctx, u, p.ID); err != nil || n != 1 {
		t.Fatalf("expected one execution, got %d (%v)", n, err)
	}
	if _, err := s.Apply(ctx, u, p.ID); !errors.Is(err, ErrNotFound) || n != 1 {
		t.Fatalf("second apply must fail with ErrNotFound and not execute, got %v, %d executions", err, n)
	}
	changes, _ := st.Changes(ctx, 1)
	if len(changes) != 1 || changes[0].Action != "test.change" {
		t.Errorf("applied plan must be logged, got %+v", changes)
	}
}

func TestApply_Expired(t *testing.T) {
	s, _, u, now := setup(t)
	n, cur := 0, "v1"
	p := s.Create(u, draft(&n, "v1", &cur))
	*now = now.Add(TTL)
	if _, err := s.Apply(context.Background(), u, p.ID); !errors.Is(err, ErrNotFound) || n != 0 {
		t.Fatalf("expired plan must not execute, got %v", err)
	}
}

func TestApply_OtherUser(t *testing.T) {
	s, _, u, _ := setup(t)
	n, cur := 0, "v1"
	p := s.Create(u, draft(&n, "v1", &cur))
	other := store.User{ID: u.ID + 1, Role: store.RoleAdmin}
	if _, err := s.Apply(context.Background(), other, p.ID); !errors.Is(err, ErrNotOwner) || n != 0 {
		t.Fatalf("another user must not apply the plan, got %v", err)
	}
	if _, err := s.Apply(context.Background(), u, p.ID); err != nil {
		t.Fatalf("the owner must still be able to apply it: %v", err)
	}
}

func TestApply_Stale(t *testing.T) {
	s, st, u, _ := setup(t)
	n, cur := 0, "v1"
	p := s.Create(u, draft(&n, "v1", &cur))
	cur = "v2"
	if _, err := s.Apply(context.Background(), u, p.ID); !errors.Is(err, ErrStale) || n != 0 {
		t.Fatalf("stale plan must not execute, got %v", err)
	}
	if changes, _ := st.Changes(context.Background(), 1); len(changes) != 0 {
		t.Error("a stale plan must not be logged")
	}
}

func TestApply_ExecuteErrorNotLogged(t *testing.T) {
	s, st, u, _ := setup(t)
	d := Draft{Action: "x", Execute: func(context.Context) error { return errors.New("box said no") }}
	p := s.Create(u, d)
	if _, err := s.Apply(context.Background(), u, p.ID); err == nil {
		t.Fatal("expected the execution error")
	}
	if changes, _ := st.Changes(context.Background(), 1); len(changes) != 0 {
		t.Error("a failed change must not be logged as applied")
	}
}

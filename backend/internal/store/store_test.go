package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dir
}

func TestOpen_MigratesAndIsIdempotent(t *testing.T) {
	s, dir := openTemp(t)
	ctx := context.Background()
	v, err := s.SchemaVersion(ctx)
	if err != nil || v != len(migrations) {
		t.Fatalf("expected schema version %d, got %d (%v)", len(migrations), v, err)
	}
	s.Close()

	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopening must not fail: %v", err)
	}
	defer s2.Close()
	if v2, _ := s2.SchemaVersion(ctx); v2 != v {
		t.Errorf("reopening changed schema version: %d -> %d", v, v2)
	}
}

func TestOpen_RestrictsFilePermissions(t *testing.T) {
	_, dir := openTemp(t)
	info, err := os.Stat(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("database must not be readable by group/others, got %o", perm)
	}
}

func TestEnsureDefaultUser_CreatesAdminWithOneDashboardOnce(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	u1, err := s.EnsureDefaultUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	u2, err := s.EnsureDefaultUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u1.ID != u2.ID || u1.Role != RoleAdmin {
		t.Fatalf("expected the same admin user twice, got %+v and %+v", u1, u2)
	}
	ds, err := s.Dashboards(ctx, u1.ID)
	if err != nil || len(ds) != 1 {
		t.Fatalf("expected exactly one default dashboard, got %d (%v)", len(ds), err)
	}
}

func TestSettings_WriteReadDeleteAndLog(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	u, _ := s.EnsureDefaultUser(ctx)

	err := s.WriteSettings(ctx, []SettingWrite{
		{UserID: u.ID, Key: "general.language", Value: json.RawMessage(`"de"`)},
		{UserID: GlobalScope, Key: "auth.enabled", Value: json.RawMessage(`false`)},
	}, ChangeEntry{UserID: u.ID, Action: "settings.update", Summary: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Setting(ctx, u.ID, "general.language"); string(v) != `"de"` {
		t.Errorf("expected per-user value, got %s", v)
	}
	if _, err := s.Setting(ctx, GlobalScope, "general.language"); err != ErrNotFound {
		t.Errorf("user setting must not leak into global scope")
	}

	if err := s.WriteSettings(ctx, []SettingWrite{{UserID: u.ID, Key: "general.language"}},
		ChangeEntry{UserID: u.ID, Action: "settings.update", Summary: "reset"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Setting(ctx, u.ID, "general.language"); err != ErrNotFound {
		t.Errorf("nil value must delete the setting")
	}

	changes, err := s.Changes(ctx, 10)
	if err != nil || len(changes) != 2 || changes[0].Summary != "reset" {
		t.Fatalf("expected 2 log entries, newest first, got %+v (%v)", changes, err)
	}
}

func TestSaveDashboardLayout(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	u, _ := s.EnsureDefaultUser(ctx)
	ds, _ := s.Dashboards(ctx, u.ID)

	if err := s.SaveDashboardLayout(ctx, u.ID, ds[0].ID, json.RawMessage(`{bad`)); err != ErrInvalidLayout {
		t.Errorf("expected ErrInvalidLayout, got %v", err)
	}
	if err := s.SaveDashboardLayout(ctx, u.ID+1, ds[0].ID, json.RawMessage(`[]`)); err != ErrNotFound {
		t.Errorf("another user's dashboard must not be writable, got %v", err)
	}
	layout := json.RawMessage(`[{"tile":"storage","w":1}]`)
	if err := s.SaveDashboardLayout(ctx, u.ID, ds[0].ID, layout); err != nil {
		t.Fatal(err)
	}
	ds, _ = s.Dashboards(ctx, u.ID)
	if string(ds[0].Layout) != string(layout) {
		t.Errorf("layout not saved: %s", ds[0].Layout)
	}
}

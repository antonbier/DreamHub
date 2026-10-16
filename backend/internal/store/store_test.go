package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/box"
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

// The multi-box migration (docs/MULTIDEVICE_PLAN.md, M1) gives everything
// that existed to the first box, "main", and keeps a copy of the database
// as it was.
func TestMigration_BoxesKeepsEarlierDataOnTheFirstBox(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	// A database at the schema before multi-box support.
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	last := len(migrations) - 1
	for i := 0; i < last; i++ {
		if _, err := db.Exec(migrations[i]); err != nil {
			t.Fatalf("migration %d: %v", i+1, err)
		}
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", last)); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, name, role, created_at) VALUES (7, 'Anna', 'admin', 'x')`,
		`INSERT INTO timer_owners (service_ref, begin_at, user_id, created_at) VALUES ('1:0:1:AA', 1000, 7, 'x')`,
		`INSERT INTO channel_snapshot VALUES ('1:0:1:AA', 'ARD', 'ARD', 'tv', 1, 2, 1, 1)`,
		`INSERT INTO channel_backups VALUES ('bk1', 'x', 'before', 7)`,
		`INSERT INTO adscan_logos VALUES ('1:0:1:AA', '{}', 'x', '')`,
		`INSERT INTO adscan_results VALUES ('/media/hdd/movie/a.ts', '1:0:1:AA', '{}', 'x')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	db.Close()

	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	main, other := box.With(ctx, "main"), box.With(ctx, "bedroom")

	if bs, err := s.Boxes(ctx); err != nil || len(bs) != 1 || bs[0].ID != "main" {
		t.Fatalf("boxes: %v %v", bs, err)
	}
	if id, err := s.TimerOwner(main, "1:0:1:AA", 1000); err != nil || id != 7 {
		t.Errorf("timer owner on main: %d %v", id, err)
	}
	if _, err := s.TimerOwner(other, "1:0:1:AA", 1000); !errors.Is(err, ErrNotFound) {
		t.Errorf("timer owner on another box: %v", err)
	}
	if ch, _, err := s.SeenChannels(main); err != nil || len(ch) != 1 {
		t.Errorf("channels on main: %v %v", ch, err)
	}
	if ch, _, _ := s.SeenChannels(other); len(ch) != 0 {
		t.Errorf("channels on another box: %v", ch)
	}
	if bk, _ := s.ChannelBackups(main); len(bk) != 1 {
		t.Errorf("backups on main: %v", bk)
	}
	if _, ok, _ := s.AdScanLogo(main, "1:0:1:AA"); !ok {
		t.Error("logo on main")
	}
	if _, ok, _ := s.AdScanResult(main, "/media/hdd/movie/a.ts"); !ok {
		t.Error("result on main")
	}
	if _, ok, _ := s.AdScanResult(other, "/media/hdd/movie/a.ts"); ok {
		t.Error("result on another box")
	}
	// The copy of the database as it was holds the old schema.
	backup := fmt.Sprintf("%s.pre-migration-%d", path, last)
	old, err := sql.Open("sqlite", "file:"+backup)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	var n int
	if err := old.QueryRow(`SELECT COUNT(*) FROM timer_owners`).Scan(&n); err != nil || n != 1 {
		t.Errorf("backup: %d rows, %v", n, err)
	}
	if _, err := old.Exec(`SELECT box_id FROM timer_owners`); err == nil {
		t.Error("the backup should still have the old schema")
	}
}

func TestOpen_FreshDatabaseNeedsNoBackup(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	matches, _ := filepath.Glob(filepath.Join(dir, "*.pre-migration-*"))
	if len(matches) != 0 {
		t.Errorf("unexpected backup of an empty database: %v", matches)
	}
}

func TestBoxes_SaveAndDeleteRemovesTheirData(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.SaveBox(ctx, Box{ID: "bedroom", Name: "Bedroom", URL: "http://192.168.1.50"}); err != nil {
		t.Fatal(err)
	}
	bs, _ := s.Boxes(ctx)
	if len(bs) != 2 || bs[1].ID != "bedroom" || bs[1].Position != 1 {
		t.Fatalf("boxes: %+v", bs)
	}
	s.SaveAdScanResult(box.With(ctx, "bedroom"), "/a.ts", "", []byte(`{}`))
	if err := s.DeleteBox(ctx, "bedroom"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.AdScanResult(box.With(ctx, "bedroom"), "/a.ts"); ok {
		t.Error("a removed box's data must go with it")
	}
	if err := s.DeleteBox(ctx, "bedroom"); !errors.Is(err, ErrBoxNotFound) {
		t.Errorf("deleting twice: %v", err)
	}
}

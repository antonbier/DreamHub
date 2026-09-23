// Package store is DreamHub's persistent state: one SQLite database under
// the data directory (see PRINCIPLES.md, "Data storage: one SQLite file, on
// /data"). It uses a pure-Go SQLite driver so the binary stays statically
// cross-compilable with CGO_ENABLED=0.
//
// All timestamps are stored as UTC RFC 3339 strings (PRINCIPLES.md, "UTC
// inside, local time only on screen").
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// FileName is the database file inside the data directory.
const FileName = "dreamhub.db"

// Store wraps the SQLite database.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open creates the data directory if needed, opens (or creates) the
// database, applies pending migrations and restricts file permissions so
// secrets stored in settings aren't world-readable.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("store: creating data dir: %w", err)
	}
	path := filepath.Join(dataDir, FileName)
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: opening database: %w", err)
	}
	// One connection: a household appliance has little concurrency, and it
	// sidesteps SQLite "database is locked" errors entirely.
	db.SetMaxOpenConns(1)

	s := &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
		db.Close()
		return nil, fmt.Errorf("store: restricting database permissions: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) timestamp() string { return s.now().Format(time.RFC3339Nano) }

func parseTime(v string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, v)
	return t
}

// migrations are applied in order; the index+1 is stored in PRAGMA
// user_version. Never edit an applied migration — append a new one.
var migrations = []string{
	`
	CREATE TABLE users (
		id         INTEGER PRIMARY KEY,
		name       TEXT NOT NULL UNIQUE,
		role       TEXT NOT NULL CHECK (role IN ('admin', 'user')),
		created_at TEXT NOT NULL
	);

	-- user_id 0 means a global setting; otherwise the owning user.
	CREATE TABLE settings (
		user_id    INTEGER NOT NULL,
		key        TEXT NOT NULL,
		value      TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (user_id, key)
	);

	-- Multiple dashboards per user are supported by the model; v1 shows one.
	CREATE TABLE dashboards (
		id         INTEGER PRIMARY KEY,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name       TEXT NOT NULL,
		position   INTEGER NOT NULL DEFAULT 0,
		layout     TEXT NOT NULL DEFAULT '[]',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);

	CREATE TABLE change_log (
		id      INTEGER PRIMARY KEY,
		at      TEXT NOT NULL,
		user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
		action  TEXT NOT NULL,
		summary TEXT NOT NULL,
		detail  TEXT NOT NULL DEFAULT '{}'
	);
	CREATE INDEX change_log_at ON change_log (at);
	`,
}

func (s *Store) migrate(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("store: reading schema version: %w", err)
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("store: migration %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: migration %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: migration %d: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("store: migration %d: %w", i+1, err)
		}
	}
	return nil
}

// SchemaVersion reports the applied migration count.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v)
	return v, err
}

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
	"io"
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
	if err := s.backupBeforeMigration(context.Background(), path); err != nil {
		db.Close()
		return nil, err
	}
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
	`
	-- API tokens (PRINCIPLES.md: "a separate, user-generatable API token,
	-- not tied to the Basic Auth password"). Only the SHA-256 hash is
	-- stored; the plaintext token is shown once, at creation.
	CREATE TABLE api_tokens (
		id         INTEGER PRIMARY KEY,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name       TEXT NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL
	);
	`,
	`
	-- DiscoveryHub's daily AI call budget (ROADMAP.md: "ai.dailyLimit" is a
	-- cost guard) — one row per UTC calendar date, incremented before each
	-- real AI call so a hung/slow response can't be retried past the limit.
	CREATE TABLE ai_usage (
		date  TEXT PRIMARY KEY,
		count INTEGER NOT NULL DEFAULT 0
	);
	`,
	`
	-- The core event bus's persisted feed (docs/archive/V1_PLAN.md, WP7):
	-- detection, not automation — recording.started/finished,
	-- disk.aboveThreshold, hub.statusChanged. No user_id: v1 has exactly
	-- one user and every event here is system-detected, never a user
	-- action (those already go to change_log). key+params, never finished
	-- text, translated client-side like status.Problem everywhere else.
	CREATE TABLE notifications (
		id      INTEGER PRIMARY KEY,
		at      TEXT NOT NULL,
		key     TEXT NOT NULL,
		params  TEXT NOT NULL DEFAULT '{}',
		read_at TEXT
	);
	CREATE INDEX notifications_at ON notifications (at);
	`,
	`
	-- Enrichment cache (internal/enrich): what an external database (TMDB
	-- first) said about a title, so lists don't ask it again on every view
	-- and still show pictures and ratings while it's unreachable. key is
	-- source, language, kind and normalized title; info is the JSON result,
	-- '' for "known not to match" (cached too, for a shorter time).
	CREATE TABLE enrichment (
		key        TEXT PRIMARY KEY,
		info       TEXT NOT NULL,
		fetched_at INTEGER NOT NULL
	);
	`,
	`
	-- DiscoveryHub's last recommendations per user (internal/hubs/
	-- discovery_saved.go): kept so the day's picks are still there after
	-- leaving the page, a reload, or on another device — generating costs
	-- an AI call, so it shouldn't have to be repeated. data is the JSON
	-- answer; replaced on every new generation.
	CREATE TABLE discovery_picks (
		user_id      INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		generated_at INTEGER NOT NULL,
		data         TEXT NOT NULL
	);
	`,
	`
	-- PlexHub's copy of what the Plex server last said (internal/hubs/
	-- plex.go): the library list, each library's films or series, a
	-- series' seasons and episodes — so the library is still there while
	-- the server is switched off. key names the list ("libraries",
	-- "items:<library>", "children:<ratingKey>"), data is its JSON.
	CREATE TABLE plex_cache (
		key       TEXT PRIMARY KEY,
		data      TEXT NOT NULL,
		synced_at INTEGER NOT NULL
	);
	`,
	`
	-- Profiles (docs/archive/PROFILES_PLAN.md): an optional PIN (bcrypt), a color
	-- for the picker, and the admin's per-profile permission switches as a
	-- JSON object {"<perm>": true|false}. role 'user' is the viewer role.
	ALTER TABLE users ADD COLUMN pin_hash TEXT NOT NULL DEFAULT '';
	ALTER TABLE users ADD COLUMN color TEXT NOT NULL DEFAULT '';
	ALTER TABLE users ADD COLUMN overrides TEXT NOT NULL DEFAULT '{}';

	-- A device's chosen profile: the frontend sends the token, only its
	-- SHA-256 is stored. Remembered until signed out, the profile is
	-- deleted, or its PIN changes.
	CREATE TABLE profile_sessions (
		id           INTEGER PRIMARY KEY,
		user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		token_hash   TEXT NOT NULL UNIQUE,
		created_at   TEXT NOT NULL,
		last_used_at TEXT NOT NULL
	);

	-- Which profile made a timer through DreamHub, so a viewer can delete
	-- their own timers but nobody else's. The box's timers have no owner;
	-- one is identified by channel and start time, like everywhere else.
	CREATE TABLE timer_owners (
		service_ref TEXT NOT NULL,
		begin_at    INTEGER NOT NULL,
		user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at  TEXT NOT NULL,
		PRIMARY KEY (service_ref, begin_at)
	);
	`,
	`
	-- The start page an admin designs for a role (docs/archive/PROFILES_PLAN.md,
	-- P4): a profile of that role that hasn't customized its own sees it.
	CREATE TABLE dashboard_templates (
		role       TEXT PRIMARY KEY,
		layout     TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	`,
	`
	-- ChannelHub's snapshot of every channel the box knows
	-- (docs/CHANNELHUB_PLAN.md): the box doesn't record when a channel
	-- appeared, so DreamHub does. Times in Unix seconds; *_look count
	-- DreamHub's looks at the list (1, 2, …), so "new" and "gone" don't
	-- depend on two looks being seconds apart.
	CREATE TABLE channel_snapshot (
		service_ref TEXT PRIMARY KEY,
		name        TEXT NOT NULL,
		provider    TEXT NOT NULL,
		kind        TEXT NOT NULL CHECK (kind IN ('tv', 'radio')),
		first_seen  INTEGER NOT NULL,
		last_seen   INTEGER NOT NULL,
		first_look  INTEGER NOT NULL,
		last_look   INTEGER NOT NULL
	);

	-- The bouquets a profile sees (docs/CHANNELHUB_PLAN.md, C5) as a JSON
	-- list of bouquet references; empty means all. Prepared, no interface
	-- sets it yet.
	ALTER TABLE users ADD COLUMN bouquets TEXT NOT NULL DEFAULT '[]';

	-- The channel-list backups DreamHub made before each change (through
	-- the BouquetEditor plugin, which keeps the files on the box).
	CREATE TABLE channel_backups (
		filename   TEXT PRIMARY KEY,
		created_at TEXT NOT NULL,
		summary    TEXT NOT NULL,
		user_id    INTEGER
	);
	`,
	`
	-- Background jobs (internal/jobs, #41): long work such as ad detection
	-- that outlives a request. message/params are an i18n key and its
	-- parameters; checkpoint is the job kind's own resume state.
	CREATE TABLE jobs (
		id          INTEGER PRIMARY KEY,
		kind        TEXT NOT NULL,
		key         TEXT NOT NULL DEFAULT '',
		owner_id    INTEGER NOT NULL,
		params      TEXT NOT NULL DEFAULT '{}',
		state       TEXT NOT NULL,
		done        INTEGER NOT NULL DEFAULT 0,
		total       INTEGER NOT NULL DEFAULT 0,
		message     TEXT NOT NULL DEFAULT '',
		msg_params  TEXT NOT NULL DEFAULT '{}',
		error       TEXT NOT NULL DEFAULT '',
		checkpoint  TEXT NOT NULL DEFAULT '',
		created_at  TEXT NOT NULL,
		started_at  TEXT NOT NULL DEFAULT '',
		finished_at TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX jobs_state ON jobs (state);
	`,
	`
	-- Ad detection (docs/ADSCAN_PLAN.md, #44): a channel's learned logo
	-- (internal/adscan.Mask as JSON) and each recording's last analysis.
	CREATE TABLE adscan_logos (
		service_ref TEXT PRIMARY KEY,
		mask        TEXT NOT NULL,
		learned_at  TEXT NOT NULL,
		learned_from TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE adscan_results (
		path        TEXT PRIMARY KEY,
		service_ref TEXT NOT NULL DEFAULT '',
		result      TEXT NOT NULL,
		analyzed_at TEXT NOT NULL
	);
	`,
	`
	-- AdminHub's script library (docs/ADMINHUB_PLAN.md, A3, #47): scripts an
	-- admin uploaded and may run by hand, with a confirm step and a record
	-- of every run (AutomationHub, later, triggers the same stored scripts
	-- rather than keeping its own copies).
	CREATE TABLE admin_scripts (
		id          INTEGER PRIMARY KEY,
		name        TEXT NOT NULL,
		content     TEXT NOT NULL,
		uploaded_at TEXT NOT NULL,
		uploaded_by INTEGER NOT NULL
	);
	CREATE TABLE admin_script_runs (
		id          INTEGER PRIMARY KEY,
		script_id   INTEGER NOT NULL REFERENCES admin_scripts(id) ON DELETE CASCADE,
		started_at  TEXT NOT NULL,
		finished_at TEXT NOT NULL,
		exit_code   INTEGER NOT NULL,
		output      TEXT NOT NULL DEFAULT '',
		user_id     INTEGER NOT NULL
	);
	CREATE INDEX admin_script_runs_script ON admin_script_runs (script_id);
	`,
	`
	-- Multi-box (docs/MULTIDEVICE_PLAN.md, M1): the boxes DreamHub controls,
	-- and everything that belongs to one box is keyed by it. All data so
	-- far belongs to the first box, "main". The first box's address and
	-- login stay in the configuration and the WebControl settings (empty
	-- here); further boxes keep theirs in their row.
	CREATE TABLE boxes (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL,
		color      TEXT NOT NULL DEFAULT '',
		url        TEXT NOT NULL DEFAULT '',
		username   TEXT NOT NULL DEFAULT '',
		password   TEXT NOT NULL DEFAULT '',
		position   INTEGER NOT NULL,
		created_at TEXT NOT NULL
	);
	INSERT INTO boxes (id, name, position, created_at) VALUES ('main', 'Dreambox', 0, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

	ALTER TABLE timer_owners RENAME TO timer_owners_old;
	CREATE TABLE timer_owners (
		box_id      TEXT NOT NULL DEFAULT 'main',
		service_ref TEXT NOT NULL,
		begin_at    INTEGER NOT NULL,
		user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at  TEXT NOT NULL,
		PRIMARY KEY (box_id, service_ref, begin_at)
	);
	INSERT INTO timer_owners (service_ref, begin_at, user_id, created_at) SELECT service_ref, begin_at, user_id, created_at FROM timer_owners_old;
	DROP TABLE timer_owners_old;

	ALTER TABLE channel_snapshot RENAME TO channel_snapshot_old;
	CREATE TABLE channel_snapshot (
		box_id      TEXT NOT NULL DEFAULT 'main',
		service_ref TEXT NOT NULL,
		name        TEXT NOT NULL,
		provider    TEXT NOT NULL,
		kind        TEXT NOT NULL CHECK (kind IN ('tv', 'radio')),
		first_seen  INTEGER NOT NULL,
		last_seen   INTEGER NOT NULL,
		first_look  INTEGER NOT NULL,
		last_look   INTEGER NOT NULL,
		PRIMARY KEY (box_id, service_ref)
	);
	INSERT INTO channel_snapshot (service_ref, name, provider, kind, first_seen, last_seen, first_look, last_look)
		SELECT service_ref, name, provider, kind, first_seen, last_seen, first_look, last_look FROM channel_snapshot_old;
	DROP TABLE channel_snapshot_old;

	ALTER TABLE channel_backups RENAME TO channel_backups_old;
	CREATE TABLE channel_backups (
		box_id     TEXT NOT NULL DEFAULT 'main',
		filename   TEXT NOT NULL,
		created_at TEXT NOT NULL,
		summary    TEXT NOT NULL,
		user_id    INTEGER,
		PRIMARY KEY (box_id, filename)
	);
	INSERT INTO channel_backups (filename, created_at, summary, user_id) SELECT filename, created_at, summary, user_id FROM channel_backups_old;
	DROP TABLE channel_backups_old;

	ALTER TABLE adscan_logos RENAME TO adscan_logos_old;
	CREATE TABLE adscan_logos (
		box_id       TEXT NOT NULL DEFAULT 'main',
		service_ref  TEXT NOT NULL,
		mask         TEXT NOT NULL,
		learned_at   TEXT NOT NULL,
		learned_from TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (box_id, service_ref)
	);
	INSERT INTO adscan_logos (service_ref, mask, learned_at, learned_from) SELECT service_ref, mask, learned_at, learned_from FROM adscan_logos_old;
	DROP TABLE adscan_logos_old;

	ALTER TABLE adscan_results RENAME TO adscan_results_old;
	CREATE TABLE adscan_results (
		box_id      TEXT NOT NULL DEFAULT 'main',
		path        TEXT NOT NULL,
		service_ref TEXT NOT NULL DEFAULT '',
		result      TEXT NOT NULL,
		analyzed_at TEXT NOT NULL,
		PRIMARY KEY (box_id, path)
	);
	INSERT INTO adscan_results (path, service_ref, result, analyzed_at) SELECT path, service_ref, result, analyzed_at FROM adscan_results_old;
	DROP TABLE adscan_results_old;

	-- A job runs against the box it was queued for.
	ALTER TABLE jobs ADD COLUMN box_id TEXT NOT NULL DEFAULT 'main';
	`,
	`
	-- The boxes a profile may use (docs/MULTIDEVICE_PLAN.md, M3) as a JSON
	-- list of box IDs; empty means all boxes.
	ALTER TABLE users ADD COLUMN boxes TEXT NOT NULL DEFAULT '[]';
	`,
	`
	-- Pairing to the DreamHub on a box and how that box reaches this hub
	-- (docs/MULTIDEVICE_PLAN.md, M6).
	ALTER TABLE boxes ADD COLUMN peer_url TEXT NOT NULL DEFAULT '';
	ALTER TABLE boxes ADD COLUMN peer_token TEXT NOT NULL DEFAULT '';
	ALTER TABLE boxes ADD COLUMN stream_base TEXT NOT NULL DEFAULT '';
	`,
	`
	-- How a box that is off is switched on (docs/MULTIDEVICE_PLAN.md, M9).
	ALTER TABLE boxes ADD COLUMN wake_method TEXT NOT NULL DEFAULT '';
	ALTER TABLE boxes ADD COLUMN wake_mac TEXT NOT NULL DEFAULT '';
	ALTER TABLE boxes ADD COLUMN wake_ext TEXT NOT NULL DEFAULT '';
	ALTER TABLE boxes ADD COLUMN wake_action TEXT NOT NULL DEFAULT '';
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

// backupBeforeMigration copies the database next to itself when an
// existing one is about to be migrated ("dreamhub.db.pre-migration-<from>"):
// the multi-box migration re-keys several tables, and a user must be able
// to go back to the version they came from. A fresh database has nothing
// worth saving; the previous backup of the same step is replaced.
func (s *Store) backupBeforeMigration(ctx context.Context, path string) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("store: reading schema version: %w", err)
	}
	if version == 0 || version >= len(migrations) {
		return nil
	}
	// Fold the WAL into the main file so the copy is complete.
	if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("store: checkpointing before migration: %w", err)
	}
	in, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("store: backing up before migration: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(fmt.Sprintf("%s.pre-migration-%d", path, version), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("store: backing up before migration: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("store: backing up before migration: %w", err)
	}
	return out.Close()
}

// SchemaVersion reports the applied migration count.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v)
	return v, err
}

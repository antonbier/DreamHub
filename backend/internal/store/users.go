package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Role is a profile's role. What a role may do is defined in internal/perm,
// not here (docs/PROFILES_PLAN.md: "Check permissions, never roles").
type Role string

const (
	RoleAdmin Role = "admin"
	// RoleUser is the viewer role ("Zuschauer"); the stored name predates
	// profiles and is kept so the database needs no table rebuild.
	RoleUser Role = "user"
)

// User is a DreamHub profile.
type User struct {
	ID        int64
	Name      string
	Role      Role
	CreatedAt time.Time
	// PinHash is the bcrypt hash of the profile's PIN, "" for none.
	PinHash string
	// Color is the picker's color token, "" for the default.
	Color string
	// Overrides are the admin's per-profile permission switches
	// (perm.Switchable): permission name → on/off.
	Overrides map[string]bool
}

// HasPin reports whether the profile is protected by a PIN.
func (u User) HasPin() bool { return u.PinHash != "" }

// ErrNotFound is returned when a requested row doesn't exist.
var ErrNotFound = errors.New("store: not found")

// ErrNameTaken is returned when another profile already has the name.
var ErrNameTaken = errors.New("store: name taken")

// DefaultAdminName is the name of the user created on first start.
const DefaultAdminName = "admin"

// EnsureDefaultUser returns the first admin, creating one (with one default
// dashboard) on first start. Safe to call on every startup.
func (s *Store) EnsureDefaultUser(ctx context.Context) (User, error) {
	u, err := s.scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE role = ? ORDER BY id LIMIT 1`, string(RoleAdmin)))
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return User{}, err
	}
	return s.CreateUser(ctx, User{Name: DefaultAdminName, Role: RoleAdmin})
}

// CreateUser adds a profile with an empty default dashboard.
func (s *Store) CreateUser(ctx context.Context, u User) (User, error) {
	return s.CreateUserLike(ctx, u, 0)
}

// CreateUserLike adds a profile that starts with the personal settings of
// the profile likeID (language, appearance, display time zone — whoever
// sets up a profile for someone in the household usually shares them);
// likeID 0 starts from the defaults.
func (s *Store) CreateUserLike(ctx context.Context, u User, likeID int64) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	overrides, _ := json.Marshal(nonNil(u.Overrides))
	now := s.timestamp()
	res, err := tx.ExecContext(ctx, `INSERT INTO users (name, role, created_at, pin_hash, color, overrides) VALUES (?, ?, ?, ?, ?, ?)`,
		u.Name, string(u.Role), now, u.PinHash, u.Color, string(overrides))
	if err != nil {
		if s.nameTaken(ctx, tx, u.Name, 0) {
			return User{}, ErrNameTaken
		}
		return User{}, fmt.Errorf("store: creating user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO dashboards (user_id, name, position, layout, created_at, updated_at) VALUES (?, ?, 0, '[]', ?, ?)`,
		id, "Dashboard", now, now); err != nil {
		return User{}, fmt.Errorf("store: creating default dashboard: %w", err)
	}
	if likeID > 0 {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO settings (user_id, key, value, updated_at) SELECT ?, key, value, ? FROM settings WHERE user_id = ?`,
			id, now, likeID); err != nil {
			return User{}, fmt.Errorf("store: copying personal settings: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return s.User(ctx, id)
}

// UpdateUser saves a profile's name, role, PIN hash, color and switches.
// A changed PIN hash signs the profile out everywhere.
func (s *Store) UpdateUser(ctx context.Context, u User) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldPin string
	if err := tx.QueryRowContext(ctx, `SELECT pin_hash FROM users WHERE id = ?`, u.ID).Scan(&oldPin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if s.nameTaken(ctx, tx, u.Name, u.ID) {
		return ErrNameTaken
	}
	overrides, _ := json.Marshal(nonNil(u.Overrides))
	if _, err := tx.ExecContext(ctx, `UPDATE users SET name = ?, role = ?, pin_hash = ?, color = ?, overrides = ? WHERE id = ?`,
		u.Name, string(u.Role), u.PinHash, u.Color, string(overrides), u.ID); err != nil {
		return err
	}
	if oldPin != u.PinHash {
		if _, err := tx.ExecContext(ctx, `DELETE FROM profile_sessions WHERE user_id = ?`, u.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteUser removes a profile and everything that's only theirs: their
// settings, dashboards, tokens, sessions and timer ownership.
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// settings.user_id has no foreign key (0 means global).
	if _, err := tx.ExecContext(ctx, `DELETE FROM settings WHERE user_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) nameTaken(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, name string, exceptID int64) bool {
	var n int
	_ = q.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE name = ? COLLATE NOCASE AND id != ?`, name, exceptID).Scan(&n)
	return n > 0
}

func nonNil(m map[string]bool) map[string]bool {
	if m == nil {
		return map[string]bool{}
	}
	return m
}

const userCols = `id, name, role, created_at, pin_hash, color, overrides`

// User returns a user by ID.
func (s *Store) User(ctx context.Context, id int64) (User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

// UserByName returns a user by name.
func (s *Store) UserByName(ctx context.Context, name string) (User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE name = ?`, name))
}

// Users returns every profile, oldest first.
func (s *Store) Users(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUserRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) scanUser(row *sql.Row) (User, error) {
	u, err := scanUserRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func scanUserRow(row scannable) (User, error) {
	var u User
	var role, created, overrides string
	if err := row.Scan(&u.ID, &u.Name, &role, &created, &u.PinHash, &u.Color, &overrides); err != nil {
		return User{}, err
	}
	u.Role = Role(role)
	u.CreatedAt = parseTime(created)
	u.Overrides = map[string]bool{}
	_ = json.Unmarshal([]byte(overrides), &u.Overrides)
	return u, nil
}

// --- profile sessions -----------------------------------------------------

// CreateProfileSession stores a session for a profile by its token hash.
func (s *Store) CreateProfileSession(ctx context.Context, userID int64, tokenHash string) error {
	now := s.timestamp()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO profile_sessions (user_id, token_hash, created_at, last_used_at) VALUES (?, ?, ?, ?)`,
		userID, tokenHash, now, now)
	return err
}

// ProfileSessionUser returns the profile a session belongs to and when the
// session was last used.
func (s *Store) ProfileSessionUser(ctx context.Context, tokenHash string) (int64, time.Time, error) {
	var id int64
	var last string
	err := s.db.QueryRowContext(ctx, `SELECT user_id, last_used_at FROM profile_sessions WHERE token_hash = ?`, tokenHash).Scan(&id, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, time.Time{}, ErrNotFound
	}
	return id, parseTime(last), err
}

// TouchProfileSession records that a session was used now.
func (s *Store) TouchProfileSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE profile_sessions SET last_used_at = ? WHERE token_hash = ?`, s.timestamp(), tokenHash)
	return err
}

// DeleteProfileSession ends one session.
func (s *Store) DeleteProfileSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM profile_sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// DeleteProfileSessionsBefore drops sessions unused since before t.
func (s *Store) DeleteProfileSessionsBefore(ctx context.Context, t time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM profile_sessions WHERE last_used_at < ?`, t.UTC().Format(time.RFC3339Nano))
	return err
}

// --- timer ownership --------------------------------------------------------

// SetTimerOwner records that a profile made the timer on a channel at a
// start time.
func (s *Store) SetTimerOwner(ctx context.Context, serviceRef string, begin, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO timer_owners (service_ref, begin_at, user_id, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (service_ref, begin_at) DO UPDATE SET user_id = excluded.user_id`,
		serviceRef, begin, userID, s.timestamp())
	return err
}

// TimerOwner returns who made a timer through DreamHub, ErrNotFound when
// nobody did (made on the TV, by AutoTimer or SerienRecorder).
func (s *Store) TimerOwner(ctx context.Context, serviceRef string, begin int64) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM timer_owners WHERE service_ref = ? AND begin_at = ?`, serviceRef, begin).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// DeleteTimerOwnersBefore forgets the owners of timers that began before
// t — they've long been recorded or deleted.
func (s *Store) DeleteTimerOwnersBefore(ctx context.Context, t int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM timer_owners WHERE begin_at < ?`, t)
	return err
}

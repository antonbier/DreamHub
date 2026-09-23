package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Role is a user's permission level. Profiles and roles are prepared in the
// data model from day one; v1 has a single admin user (PRINCIPLES.md,
// "Profiles, roles and dashboards: ready, not built").
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// User is a DreamHub user.
type User struct {
	ID        int64
	Name      string
	Role      Role
	CreatedAt time.Time
}

// ErrNotFound is returned when a requested row doesn't exist.
var ErrNotFound = errors.New("store: not found")

// DefaultAdminName is the name of the user created on first start.
const DefaultAdminName = "admin"

// EnsureDefaultUser returns the single v1 user, creating it (as admin, with
// one default dashboard) on first start. Safe to call on every startup.
func (s *Store) EnsureDefaultUser(ctx context.Context) (User, error) {
	u, err := s.UserByName(ctx, DefaultAdminName)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return User{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	now := s.timestamp()
	res, err := tx.ExecContext(ctx, `INSERT INTO users (name, role, created_at) VALUES (?, ?, ?)`,
		DefaultAdminName, string(RoleAdmin), now)
	if err != nil {
		return User{}, fmt.Errorf("store: creating default user: %w", err)
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
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return s.User(ctx, id)
}

// User returns a user by ID.
func (s *Store) User(ctx context.Context, id int64) (User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx, `SELECT id, name, role, created_at FROM users WHERE id = ?`, id))
}

// UserByName returns a user by name.
func (s *Store) UserByName(ctx context.Context, name string) (User, error) {
	return s.scanUser(s.db.QueryRowContext(ctx, `SELECT id, name, role, created_at FROM users WHERE name = ?`, name))
}

func (s *Store) scanUser(row *sql.Row) (User, error) {
	var u User
	var role, created string
	if err := row.Scan(&u.ID, &u.Name, &role, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	u.Role = Role(role)
	u.CreatedAt = parseTime(created)
	return u, nil
}

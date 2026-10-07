package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Dashboard is one of a user's dashboards. Layout is an opaque JSON document
// owned by the frontend (tile IDs, order, widths, custom tile definitions).
type Dashboard struct {
	ID        int64
	UserID    int64
	Name      string
	Position  int
	Layout    json.RawMessage
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Dashboards returns a user's dashboards in display order.
func (s *Store) Dashboards(ctx context.Context, userID int64) ([]Dashboard, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, name, position, layout, created_at, updated_at FROM dashboards WHERE user_id = ? ORDER BY position, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dashboard
	for rows.Next() {
		var d Dashboard
		var layout, created, updated string
		if err := rows.Scan(&d.ID, &d.UserID, &d.Name, &d.Position, &layout, &created, &updated); err != nil {
			return nil, err
		}
		d.Layout = json.RawMessage(layout)
		d.CreatedAt, d.UpdatedAt = parseTime(created), parseTime(updated)
		out = append(out, d)
	}
	return out, rows.Err()
}

// SaveDashboardLayout replaces a dashboard's layout. The layout must be
// valid JSON; the caller owns its structure.
func (s *Store) SaveDashboardLayout(ctx context.Context, userID, dashboardID int64, layout json.RawMessage) error {
	if !json.Valid(layout) {
		return ErrInvalidLayout
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE dashboards SET layout = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		string(layout), s.timestamp(), dashboardID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ErrInvalidLayout is returned when a dashboard layout isn't valid JSON.
var ErrInvalidLayout = errors.New("store: layout is not valid JSON")

// DashboardTemplate returns the start page designed for a role; ok is
// false when none was saved.
func (s *Store) DashboardTemplate(ctx context.Context, role Role) (json.RawMessage, bool, error) {
	var layout string
	err := s.db.QueryRowContext(ctx, `SELECT layout FROM dashboard_templates WHERE role = ?`, string(role)).Scan(&layout)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return json.RawMessage(layout), true, nil
}

// SaveDashboardTemplate replaces the start page designed for a role.
func (s *Store) SaveDashboardTemplate(ctx context.Context, role Role, layout json.RawMessage) error {
	if !json.Valid(layout) {
		return ErrInvalidLayout
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO dashboard_templates (role, layout, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT (role) DO UPDATE SET layout = excluded.layout, updated_at = excluded.updated_at`,
		string(role), string(layout), s.timestamp())
	return err
}

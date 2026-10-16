package store

import (
	"context"
	"errors"
)

// Box is one Dreambox DreamHub controls (docs/MULTIDEVICE_PLAN.md).
// URL, Username and Password are empty for the first box: its address
// comes from the configuration and its login from the WebControl
// settings, as before multi-box support.
type Box struct {
	ID       string
	Name     string
	Color    string
	URL      string
	Username string
	Password string
	Position int
}

// ErrBoxNotFound is returned for an unknown box ID.
var ErrBoxNotFound = errors.New("box not found")

// Boxes lists the boxes in their order.
func (s *Store) Boxes(ctx context.Context) ([]Box, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, color, url, username, password, position FROM boxes ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Box
	for rows.Next() {
		var b Box
		if err := rows.Scan(&b.ID, &b.Name, &b.Color, &b.URL, &b.Username, &b.Password, &b.Position); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// SaveBox adds a box or replaces the one with this ID; a new box goes last.
func (s *Store) SaveBox(ctx context.Context, b Box) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO boxes (id, name, color, url, username, password, position, created_at)
		VALUES (?, ?, ?, ?, ?, ?, (SELECT COALESCE(MAX(position), -1) + 1 FROM boxes), ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name, color = excluded.color, url = excluded.url,
			username = excluded.username, password = excluded.password`,
		b.ID, b.Name, b.Color, b.URL, b.Username, b.Password, s.timestamp())
	return err
}

// DeleteBox removes a box's row and everything keyed by it.
func (s *Store) DeleteBox(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM boxes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrBoxNotFound
	}
	for _, t := range []string{"timer_owners", "channel_snapshot", "channel_backups", "adscan_logos", "adscan_results", "jobs"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+t+` WHERE box_id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

package store

import (
	"context"
	"encoding/json"
	"time"
)

// Notification is one persisted event from the core event bus
// (docs/archive/V1_PLAN.md, WP7): "detection, not automation" — a recording
// started/finished, disk crossed a threshold, a hub's status changed.
// Key+params, never finished text — translated client-side the same way
// status.Problem is everywhere else.
type Notification struct {
	ID     int64
	At     time.Time
	Key    string
	Params json.RawMessage
	ReadAt *time.Time
}

// AppendNotification records a new event.
func (s *Store) AppendNotification(ctx context.Context, key string, params json.RawMessage) (Notification, error) {
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	at := s.timestamp()
	res, err := s.db.ExecContext(ctx, `INSERT INTO notifications (at, key, params) VALUES (?, ?, ?)`, at, key, string(params))
	if err != nil {
		return Notification{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Notification{}, err
	}
	return Notification{ID: id, At: parseTime(at), Key: key, Params: params}, nil
}

// Notifications returns the most recent events, newest first.
func (s *Store) Notifications(ctx context.Context, limit int, unreadOnly bool) ([]Notification, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT id, at, key, params, read_at FROM notifications`
	if unreadOnly {
		q += ` WHERE read_at IS NULL`
	}
	q += ` ORDER BY id DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		var n Notification
		var at, params string
		var readAt *string
		if err := rows.Scan(&n.ID, &at, &n.Key, &params, &readAt); err != nil {
			return nil, err
		}
		n.At = parseTime(at)
		n.Params = json.RawMessage(params)
		if readAt != nil {
			t := parseTime(*readAt)
			n.ReadAt = &t
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UnreadNotifications counts events not yet marked read.
func (s *Store) UnreadNotifications(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notifications WHERE read_at IS NULL`).Scan(&n)
	return n, err
}

// MarkNotificationsRead marks every currently-unread event read (v1 has one
// user and one inbox, so "read" is global rather than per-user).
func (s *Store) MarkNotificationsRead(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notifications SET read_at = ? WHERE read_at IS NULL`, s.timestamp())
	return err
}

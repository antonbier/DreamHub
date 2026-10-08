package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// ChangeEntry is one applied change, recorded for the change log in
// SystemHub and as the basis for undo (PRINCIPLES.md, "Preview and
// confirm"). Detail must never contain secret values.
type ChangeEntry struct {
	ID      int64
	At      time.Time
	UserID  int64
	Action  string
	Summary string
	Detail  json.RawMessage
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (s *Store) appendChange(ctx context.Context, ex execer, e ChangeEntry) error {
	detail := e.Detail
	if len(detail) == 0 {
		detail = json.RawMessage(`{}`)
	}
	var user any
	if e.UserID != 0 {
		user = e.UserID
	}
	_, err := ex.ExecContext(ctx,
		`INSERT INTO change_log (at, user_id, action, summary, detail) VALUES (?, ?, ?, ?, ?)`,
		s.timestamp(), user, e.Action, e.Summary, string(detail))
	return err
}

// AppendChange records a change on its own (outside another transaction).
func (s *Store) AppendChange(ctx context.Context, e ChangeEntry) error {
	return s.appendChange(ctx, s.db, e)
}

// Changes returns the most recent change log entries, newest first.
func (s *Store) Changes(ctx context.Context, limit int) ([]ChangeEntry, error) {
	return s.ChangesBy(ctx, limit, -1)
}

// ChangesBy returns the most recent entries one profile made, newest
// first: userID 0 means the system's own (no profile), -1 everyone's.
func (s *Store) ChangesBy(ctx context.Context, limit int, userID int64) ([]ChangeEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, at, COALESCE(user_id, 0), action, summary, detail FROM change_log
		 WHERE ? < 0 OR COALESCE(user_id, 0) = ? ORDER BY id DESC LIMIT ?`, userID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChangeEntry
	for rows.Next() {
		var e ChangeEntry
		var at, detail string
		if err := rows.Scan(&e.ID, &at, &e.UserID, &e.Action, &e.Summary, &detail); err != nil {
			return nil, err
		}
		e.At = parseTime(at)
		e.Detail = json.RawMessage(detail)
		out = append(out, e)
	}
	return out, rows.Err()
}

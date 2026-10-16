package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// GlobalScope is the user ID used for settings that apply to everyone.
const GlobalScope int64 = 0

// SettingValue is a raw stored setting: the JSON-encoded value.
type SettingValue = json.RawMessage

// Setting returns one stored setting, or ErrNotFound if it was never set.
func (s *Store) Setting(ctx context.Context, userID int64, key string) (SettingValue, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE user_id = ? AND key = ?`, userID, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return SettingValue(v), nil
}

// Settings returns all stored settings for a scope (GlobalScope or a user ID).
func (s *Store) Settings(ctx context.Context, userID int64) (map[string]SettingValue, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]SettingValue{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = SettingValue(v)
	}
	return out, rows.Err()
}

// SettingWrite is one setting to store; a nil Value deletes the setting
// (falling back to its default).
type SettingWrite struct {
	UserID int64
	Key    string
	Value  SettingValue
}

// WriteSettings stores several settings and appends a change log entry, all
// in one transaction — either everything is saved and logged, or nothing.
func (s *Store) WriteSettings(ctx context.Context, writes []SettingWrite, entry ChangeEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := s.timestamp()
	for _, w := range writes {
		if w.Value == nil {
			if _, err := tx.ExecContext(ctx, `DELETE FROM settings WHERE user_id = ? AND key = ?`, w.UserID, w.Key); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO settings (user_id, key, value, updated_at) VALUES (?, ?, ?, ?)
			 ON CONFLICT (user_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			w.UserID, w.Key, string(w.Value), now); err != nil {
			return err
		}
	}
	// No action: nothing worth a change-log line (hidden bookkeeping keys only).
	if entry.Action != "" {
		if err := s.appendChange(ctx, tx, entry); err != nil {
			return err
		}
	}
	return tx.Commit()
}

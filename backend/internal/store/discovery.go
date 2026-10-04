package store

import (
	"context"
	"database/sql"
	"errors"
)

// DiscoveryPicks returns a user's last saved recommendations (JSON) and
// when they were generated (Unix seconds); ok=false when there are none.
func (s *Store) DiscoveryPicks(ctx context.Context, userID int64) (data string, generatedAt int64, ok bool, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT data, generated_at FROM discovery_picks WHERE user_id = ?`, userID).Scan(&data, &generatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, false, nil
	}
	return data, generatedAt, err == nil, err
}

// PutDiscoveryPicks replaces a user's saved recommendations.
func (s *Store) PutDiscoveryPicks(ctx context.Context, userID int64, data string, generatedAt int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO discovery_picks (user_id, generated_at, data) VALUES (?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET generated_at = excluded.generated_at, data = excluded.data`,
		userID, generatedAt, data)
	return err
}

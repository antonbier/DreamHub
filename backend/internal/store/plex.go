package store

import (
	"context"
	"database/sql"
	"errors"
)

// PlexCache returns PlexHub's cached list for key, when it was synced (Unix
// seconds) and whether there is one.
func (s *Store) PlexCache(ctx context.Context, key string) (data string, syncedAt int64, ok bool, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT data, synced_at FROM plex_cache WHERE key = ?`, key).Scan(&data, &syncedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, false, nil
	}
	return data, syncedAt, err == nil, err
}

// PutPlexCache stores or replaces PlexHub's cached list for key.
func (s *Store) PutPlexCache(ctx context.Context, key, data string, syncedAt int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO plex_cache (key, data, synced_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET data = excluded.data, synced_at = excluded.synced_at`,
		key, data, syncedAt)
	return err
}

// ClearPlexCache forgets everything cached from Plex — another server was
// set up, so the old one's library must not show under its name.
func (s *Store) ClearPlexCache(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM plex_cache`)
	return err
}

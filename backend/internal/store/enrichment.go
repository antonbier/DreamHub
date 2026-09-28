package store

import (
	"context"
	"database/sql"
	"errors"
)

// Enrichment returns the cached enrichment result for key: its JSON (''
// for a cached miss), when it was fetched (Unix seconds), and whether
// there is an entry at all.
func (s *Store) Enrichment(ctx context.Context, key string) (info string, fetchedAt int64, ok bool, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT info, fetched_at FROM enrichment WHERE key = ?`, key).Scan(&info, &fetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, false, nil
	}
	return info, fetchedAt, err == nil, err
}

// PutEnrichment stores or replaces the enrichment result for key.
func (s *Store) PutEnrichment(ctx context.Context, key, info string, fetchedAt int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO enrichment (key, info, fetched_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET info = excluded.info, fetched_at = excluded.fetched_at`,
		key, info, fetchedAt)
	return err
}

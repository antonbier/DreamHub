package store

import (
	"context"
	"database/sql"
	"errors"
)

// IncrementAIUsage records one AI call for the given UTC calendar date
// ("2026-09-24") and returns the day's total so far, including this call —
// DiscoveryHub checks this against the configured daily limit before
// spending a real API call.
func (s *Store) IncrementAIUsage(ctx context.Context, date string) (int, error) {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO ai_usage (date, count) VALUES (?, 1)
		 ON CONFLICT (date) DO UPDATE SET count = count + 1`, date); err != nil {
		return 0, err
	}
	return s.AIUsageToday(ctx, date)
}

// ReserveAIUsage counts one more AI call for date if fewer than limit were
// made — check and count in one statement, so two requests at the same
// moment can't both slip under the limit.
func (s *Store) ReserveAIUsage(ctx context.Context, date string, limit int64) (bool, error) {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO ai_usage (date, count) VALUES (?, 0) ON CONFLICT (date) DO NOTHING`, date); err != nil {
		return false, err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE ai_usage SET count = count + 1 WHERE date = ? AND count < ?`, date, limit)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// AIUsageToday returns the given date's count without incrementing it.
func (s *Store) AIUsageToday(ctx context.Context, date string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT count FROM ai_usage WHERE date = ?`, date).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return count, err
}

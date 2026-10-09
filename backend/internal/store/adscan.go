package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// AdScanLogo is a channel's learned logo mask (internal/adscan.Mask as
// JSON), keyed by service reference.
type AdScanLogo struct {
	ServiceRef  string
	Mask        json.RawMessage
	LearnedAt   time.Time
	LearnedFrom string // the recording it was learned from
}

// AdScanLogo returns a channel's mask; ok false when none was learned yet.
func (s *Store) AdScanLogo(ctx context.Context, serviceRef string) (AdScanLogo, bool, error) {
	var l AdScanLogo
	var mask, at string
	err := s.db.QueryRowContext(ctx, `SELECT service_ref, mask, learned_at, learned_from FROM adscan_logos WHERE service_ref = ?`, serviceRef).
		Scan(&l.ServiceRef, &mask, &at, &l.LearnedFrom)
	if errors.Is(err, sql.ErrNoRows) {
		return AdScanLogo{}, false, nil
	}
	if err != nil {
		return AdScanLogo{}, false, err
	}
	l.Mask, l.LearnedAt = json.RawMessage(mask), parseTime(at)
	return l, true, nil
}

// SaveAdScanLogo stores (or replaces) a channel's mask.
func (s *Store) SaveAdScanLogo(ctx context.Context, serviceRef string, mask json.RawMessage, from string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO adscan_logos (service_ref, mask, learned_at, learned_from) VALUES (?, ?, ?, ?)
		ON CONFLICT (service_ref) DO UPDATE SET mask = excluded.mask, learned_at = excluded.learned_at, learned_from = excluded.learned_from`,
		serviceRef, string(mask), s.timestamp(), from)
	return err
}

// AdScanResult is a recording's last analysis (internal/adscan.Result as
// JSON).
type AdScanResult struct {
	Path       string
	ServiceRef string
	Result     json.RawMessage
	AnalyzedAt time.Time
}

// AdScanResult returns a recording's analysis; ok false when it has none.
func (s *Store) AdScanResult(ctx context.Context, path string) (AdScanResult, bool, error) {
	var r AdScanResult
	var res, at string
	err := s.db.QueryRowContext(ctx, `SELECT path, service_ref, result, analyzed_at FROM adscan_results WHERE path = ?`, path).
		Scan(&r.Path, &r.ServiceRef, &res, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return AdScanResult{}, false, nil
	}
	if err != nil {
		return AdScanResult{}, false, err
	}
	r.Result, r.AnalyzedAt = json.RawMessage(res), parseTime(at)
	return r, true, nil
}

// SaveAdScanResult stores (or replaces) a recording's analysis.
func (s *Store) SaveAdScanResult(ctx context.Context, path, serviceRef string, result json.RawMessage) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO adscan_results (path, service_ref, result, analyzed_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (path) DO UPDATE SET service_ref = excluded.service_ref, result = excluded.result, analyzed_at = excluded.analyzed_at`,
		path, serviceRef, string(result), s.timestamp())
	return err
}

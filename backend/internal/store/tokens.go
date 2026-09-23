package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// APIToken is a bearer token for app/API clients (PRINCIPLES.md: separate
// from the Basic Auth password, individually revocable). TokenHash is the
// hex-encoded SHA-256 of the plaintext token, which is never stored.
type APIToken struct {
	ID        int64
	UserID    int64
	Name      string
	TokenHash string
	CreatedAt time.Time
}

// CreateAPIToken stores a new token.
func (s *Store) CreateAPIToken(ctx context.Context, userID int64, name, tokenHash string) (APIToken, error) {
	now := s.timestamp()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO api_tokens (user_id, name, token_hash, created_at) VALUES (?, ?, ?, ?)`,
		userID, name, tokenHash, now)
	if err != nil {
		return APIToken{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return APIToken{}, err
	}
	return APIToken{ID: id, UserID: userID, Name: name, TokenHash: tokenHash, CreatedAt: parseTime(now)}, nil
}

// APITokens returns a user's tokens, newest first.
func (s *Store) APITokens(ctx context.Context, userID int64) ([]APIToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, name, token_hash, created_at FROM api_tokens WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIToken
	for rows.Next() {
		t, err := scanAPIToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// APITokenByHash looks up a token by its hash, for verifying a bearer
// request — across all users, since the request doesn't say who it's from.
func (s *Store) APITokenByHash(ctx context.Context, tokenHash string) (APIToken, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, token_hash, created_at FROM api_tokens WHERE token_hash = ?`, tokenHash)
	var t APIToken
	var created string
	if err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return APIToken{}, ErrNotFound
		}
		return APIToken{}, err
	}
	t.CreatedAt = parseTime(created)
	return t, nil
}

// DeleteAPIToken revokes a token, scoped to its owner so one user can't
// revoke another's.
func (s *Store) DeleteAPIToken(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanAPIToken(row scannable) (APIToken, error) {
	var t APIToken
	var created string
	if err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &created); err != nil {
		return APIToken{}, err
	}
	t.CreatedAt = parseTime(created)
	return t, nil
}

package store

import "context"

// SeenChannel is one channel of ChannelHub's snapshot.
type SeenChannel struct {
	ServiceRef string
	Name       string
	Provider   string
	Kind       string // "tv" or "radio"
	FirstSeen  int64  // Unix seconds
	LastSeen   int64
	// FirstLook and LastLook number DreamHub's looks at the box's list:
	// FirstLook > 1 means it appeared after the first look ("new"),
	// LastLook below the newest look means the box no longer lists it.
	FirstLook int64
	LastLook  int64
}

// RecordChannels stores what the box lists now as the next look: new
// channels get first seen = at, known ones keep theirs; all get last seen
// = at, and name and provider are updated.
func (s *Store) RecordChannels(ctx context.Context, channels []SeenChannel, at int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var look int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(last_look), 0) + 1 FROM channel_snapshot`).Scan(&look); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO channel_snapshot (service_ref, name, provider, kind, first_seen, last_seen, first_look, last_look)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (service_ref) DO UPDATE SET name = excluded.name, provider = excluded.provider, kind = excluded.kind,
			last_seen = excluded.last_seen, last_look = excluded.last_look`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, c := range channels {
		if _, err := stmt.ExecContext(ctx, c.ServiceRef, c.Name, c.Provider, c.Kind, at, at, look, look); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SeenChannels returns the whole snapshot, including channels the box no
// longer lists, and the newest look's number (0 before the first).
func (s *Store) SeenChannels(ctx context.Context) ([]SeenChannel, int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT service_ref, name, provider, kind, first_seen, last_seen, first_look, last_look FROM channel_snapshot ORDER BY name`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []SeenChannel
	var latest int64
	for rows.Next() {
		var c SeenChannel
		if err := rows.Scan(&c.ServiceRef, &c.Name, &c.Provider, &c.Kind, &c.FirstSeen, &c.LastSeen, &c.FirstLook, &c.LastLook); err != nil {
			return nil, 0, err
		}
		if c.LastLook > latest {
			latest = c.LastLook
		}
		out = append(out, c)
	}
	return out, latest, rows.Err()
}

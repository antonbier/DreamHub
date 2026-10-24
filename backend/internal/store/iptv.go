package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"sort"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/box"
)

// ErrIPTVSourceNotFound: no remembered playlist with that ID on this box.
var ErrIPTVSourceNotFound = errors.New("store: no such playlist")

// IPTVSource is a playlist remembered for a later comparison
// (docs/NETCAST_PLAN.md, N1).
type IPTVSource struct {
	ID          int64
	Name        string
	URL         string // may hold a login; never sent to the browser
	DisplayURL  string // the address without login, query and fragment
	BouquetRef  string
	BouquetName string
	Kind        string // "tv" or "radio"
	CreatedAt   time.Time
	ComparedAt  time.Time // zero before the first comparison
	// Seen holds a key for every channel the list had at the last look.
	Seen []uint64
}

// IPTVMember is a channel that was put into the source's bouquet from the
// list: URLHash is a hash of its stream address (never the address itself,
// which may hold a login), Ident the key of the channel in the list (its
// tvg-id, else its name), Name the name it had.
type IPTVMember struct {
	URLHash string
	Ident   string
	Name    string
}

func encodeSeen(keys []uint64) []byte {
	sorted := append([]uint64(nil), keys...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	out := make([]byte, 0, len(sorted)*8)
	var prev uint64
	for i, k := range sorted {
		if i > 0 && k == prev {
			continue
		}
		out = binary.BigEndian.AppendUint64(out, k)
		prev = k
	}
	return out
}

func decodeSeen(b []byte) []uint64 {
	out := make([]uint64, 0, len(b)/8)
	for i := 0; i+8 <= len(b); i += 8 {
		out = append(out, binary.BigEndian.Uint64(b[i:]))
	}
	return out
}

// SaveIPTVSource remembers a playlist for the box of ctx. Putting more
// channels of the same list into the same bouquet again updates the source
// instead of adding a second one. members are added to the ones known.
func (s *Store) SaveIPTVSource(ctx context.Context, src IPTVSource, members []IPTVMember) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	boxID := box.Key(ctx)
	now := s.timestamp()
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM iptv_sources WHERE box_id = ? AND url = ? AND bouquet_ref = ?`, boxID, src.URL, src.BouquetRef).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		res, err := tx.ExecContext(ctx, `INSERT INTO iptv_sources (box_id, name, url, display_url, bouquet_ref, bouquet_name, kind, created_at, compared_at, seen)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			boxID, src.Name, src.URL, src.DisplayURL, src.BouquetRef, src.BouquetName, src.Kind, now, now, encodeSeen(src.Seen))
		if err != nil {
			return 0, err
		}
		if id, err = res.LastInsertId(); err != nil {
			return 0, err
		}
	case err != nil:
		return 0, err
	default:
		if _, err := tx.ExecContext(ctx, `UPDATE iptv_sources SET name = ?, bouquet_name = ?, compared_at = ?, seen = ? WHERE id = ?`,
			src.Name, src.BouquetName, now, encodeSeen(src.Seen), id); err != nil {
			return 0, err
		}
	}
	if err := putMembers(ctx, tx, id, members); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func putMembers(ctx context.Context, tx *sql.Tx, id int64, members []IPTVMember) error {
	for _, m := range members {
		if _, err := tx.ExecContext(ctx, `INSERT INTO iptv_source_entries (source_id, url_hash, ident, name) VALUES (?, ?, ?, ?)
			ON CONFLICT (source_id, url_hash) DO UPDATE SET ident = excluded.ident, name = excluded.name`, id, m.URLHash, m.Ident, m.Name); err != nil {
			return err
		}
	}
	return nil
}

// IPTVSources lists the remembered playlists of the box of ctx, without the
// address and the seen keys.
func (s *Store) IPTVSources(ctx context.Context) ([]IPTVSource, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, display_url, bouquet_ref, bouquet_name, kind, created_at, compared_at
		FROM iptv_sources WHERE box_id = ? ORDER BY name, id`, box.Key(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IPTVSource
	for rows.Next() {
		var x IPTVSource
		var created, compared string
		if err := rows.Scan(&x.ID, &x.Name, &x.DisplayURL, &x.BouquetRef, &x.BouquetName, &x.Kind, &created, &compared); err != nil {
			return nil, err
		}
		x.CreatedAt, x.ComparedAt = parseTime(created), parseTime(compared)
		out = append(out, x)
	}
	return out, rows.Err()
}

// IPTVSource returns one remembered playlist in full.
func (s *Store) IPTVSource(ctx context.Context, id int64) (IPTVSource, error) {
	var x IPTVSource
	var created, compared string
	var seen []byte
	err := s.db.QueryRowContext(ctx, `SELECT id, name, url, display_url, bouquet_ref, bouquet_name, kind, created_at, compared_at, seen
		FROM iptv_sources WHERE id = ? AND box_id = ?`, id, box.Key(ctx)).
		Scan(&x.ID, &x.Name, &x.URL, &x.DisplayURL, &x.BouquetRef, &x.BouquetName, &x.Kind, &created, &compared, &seen)
	if errors.Is(err, sql.ErrNoRows) {
		return IPTVSource{}, ErrIPTVSourceNotFound
	}
	if err != nil {
		return IPTVSource{}, err
	}
	x.CreatedAt, x.ComparedAt, x.Seen = parseTime(created), parseTime(compared), decodeSeen(seen)
	return x, nil
}

// IPTVMembers returns the channels put into the source's bouquet from it.
func (s *Store) IPTVMembers(ctx context.Context, id int64) ([]IPTVMember, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.url_hash, m.ident, m.name FROM iptv_source_entries m
		JOIN iptv_sources s ON s.id = m.source_id WHERE m.source_id = ? AND s.box_id = ? ORDER BY m.rowid`, id, box.Key(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IPTVMember
	for rows.Next() {
		var m IPTVMember
		if err := rows.Scan(&m.URLHash, &m.Ident, &m.Name); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// IPTVChange is what a comparison, once confirmed, changes in what DreamHub
// remembers: members that left (by URL hash) and came, the new seen keys,
// the bouquet's name now.
type IPTVChange struct {
	RemoveHashes []string
	Add          []IPTVMember
	Seen         []uint64
	BouquetName  string
}

// UpdateIPTVSource applies a comparison to the remembered playlist.
func (s *Store) UpdateIPTVSource(ctx context.Context, id int64, c IPTVChange) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE iptv_sources SET bouquet_name = ?, compared_at = ?, seen = ? WHERE id = ? AND box_id = ?`,
		c.BouquetName, s.timestamp(), encodeSeen(c.Seen), id, box.Key(ctx))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrIPTVSourceNotFound
	}
	for _, h := range c.RemoveHashes {
		if _, err := tx.ExecContext(ctx, `DELETE FROM iptv_source_entries WHERE source_id = ? AND url_hash = ?`, id, h); err != nil {
			return err
		}
	}
	if err := putMembers(ctx, tx, id, c.Add); err != nil {
		return err
	}
	return tx.Commit()
}

// MarkIPTVCompared notes that a comparison was looked at.
func (s *Store) MarkIPTVCompared(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE iptv_sources SET compared_at = ? WHERE id = ? AND box_id = ?`, s.timestamp(), id, box.Key(ctx))
	return err
}

// DeleteIPTVSource makes DreamHub forget a playlist; the bouquet on the box
// is not touched.
func (s *Store) DeleteIPTVSource(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM iptv_sources WHERE id = ? AND box_id = ?`, id, box.Key(ctx))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrIPTVSourceNotFound
	}
	return nil
}

// IPTVTwin is the box channel whose guide an internet channel shows.
type IPTVTwin struct {
	ServiceRef  string
	ServiceName string
}

// IPTVTwins returns the remembered guide matches of the box of ctx, by the
// channel's key in its list.
func (s *Store) IPTVTwins(ctx context.Context) (map[string]IPTVTwin, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ident, service_ref, service_name FROM iptv_twins WHERE box_id = ?`, box.Key(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]IPTVTwin{}
	for rows.Next() {
		var ident string
		var t IPTVTwin
		if err := rows.Scan(&ident, &t.ServiceRef, &t.ServiceName); err != nil {
			return nil, err
		}
		out[ident] = t
	}
	return out, rows.Err()
}

// SaveIPTVTwins remembers guide matches, replacing earlier ones of the same
// channels.
func (s *Store) SaveIPTVTwins(ctx context.Context, twins map[string]IPTVTwin) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for ident, t := range twins {
		if _, err := tx.ExecContext(ctx, `INSERT INTO iptv_twins (box_id, ident, service_ref, service_name) VALUES (?, ?, ?, ?)
			ON CONFLICT (box_id, ident) DO UPDATE SET service_ref = excluded.service_ref, service_name = excluded.service_name`,
			box.Key(ctx), ident, t.ServiceRef, t.ServiceName); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ForgetIPTVTwin removes one remembered match.
func (s *Store) ForgetIPTVTwin(ctx context.Context, ident string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM iptv_twins WHERE box_id = ? AND ident = ?`, box.Key(ctx), ident)
	return err
}

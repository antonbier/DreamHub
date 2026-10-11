package store

import (
	"context"
	"database/sql"
	"time"
)

// Script is one script an admin uploaded to AdminHub's library
// (docs/ADMINHUB_PLAN.md, A3) — run by hand, with confirmation, never
// automatically (that's AutomationHub's job, later, reusing the same row).
type Script struct {
	ID         int64
	Name       string
	Content    string
	UploadedAt time.Time
	UploadedBy int64
}

// AddScript stores a new script and returns its ID.
func (s *Store) AddScript(ctx context.Context, name, content string, userID int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO admin_scripts (name, content, uploaded_at, uploaded_by) VALUES (?, ?, ?, ?)`,
		name, content, s.timestamp(), userID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Scripts lists every stored script, newest first.
func (s *Store) Scripts(ctx context.Context) ([]Script, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, content, uploaded_at, uploaded_by FROM admin_scripts ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Script{}
	for rows.Next() {
		sc, err := scanScript(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// Script returns one script by ID.
func (s *Store) Script(ctx context.Context, id int64) (Script, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, content, uploaded_at, uploaded_by FROM admin_scripts WHERE id = ?`, id)
	sc, err := scanScript(row)
	if err == sql.ErrNoRows {
		return Script{}, false, nil
	}
	if err != nil {
		return Script{}, false, err
	}
	return sc, true, nil
}

// DeleteScript removes a script and its run history (ON DELETE CASCADE).
func (s *Store) DeleteScript(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM admin_scripts WHERE id = ?`, id)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanScript(r rowScanner) (Script, error) {
	var sc Script
	var uploadedAt string
	if err := r.Scan(&sc.ID, &sc.Name, &sc.Content, &uploadedAt, &sc.UploadedBy); err != nil {
		return Script{}, err
	}
	sc.UploadedAt = parseTime(uploadedAt)
	return sc, nil
}

// ScriptRun is one run of a script, kept for the admin to review (output
// truncated by the runner itself, not here).
type ScriptRun struct {
	ID         int64
	ScriptID   int64
	StartedAt  time.Time
	FinishedAt time.Time
	ExitCode   int
	Output     string
	UserID     int64
}

// AddScriptRun records a finished run.
func (s *Store) AddScriptRun(ctx context.Context, run ScriptRun) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO admin_script_runs (script_id, started_at, finished_at, exit_code, output, user_id)
		VALUES (?, ?, ?, ?, ?, ?)`,
		run.ScriptID, run.StartedAt.UTC().Format(time.RFC3339Nano), run.FinishedAt.UTC().Format(time.RFC3339Nano), run.ExitCode, run.Output, run.UserID)
	return err
}

// ScriptRuns lists a script's runs, newest first, capped at limit.
func (s *Store) ScriptRuns(ctx context.Context, scriptID int64, limit int) ([]ScriptRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, script_id, started_at, finished_at, exit_code, output, user_id
		FROM admin_script_runs WHERE script_id = ? ORDER BY id DESC LIMIT ?`, scriptID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScriptRun{}
	for rows.Next() {
		var run ScriptRun
		var started, finished string
		if err := rows.Scan(&run.ID, &run.ScriptID, &started, &finished, &run.ExitCode, &run.Output, &run.UserID); err != nil {
			return nil, err
		}
		run.StartedAt, run.FinishedAt = parseTime(started), parseTime(finished)
		out = append(out, run)
	}
	return out, rows.Err()
}

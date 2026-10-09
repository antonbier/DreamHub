package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Job states (internal/jobs).
const (
	JobQueued   = "queued"   // waiting for a worker
	JobRunning  = "running"  // a worker runs it
	JobWaiting  = "waiting"  // started, held by a blocker (a recording runs)
	JobDone     = "done"     // finished successfully
	JobFailed   = "failed"   // finished with an error
	JobCanceled = "canceled" // stopped by a user
)

// JobActive reports whether a job in state s is still to finish.
func JobActive(s string) bool { return s == JobQueued || s == JobRunning || s == JobWaiting }

// Job is one background job's persisted state.
type Job struct {
	ID      int64
	Kind    string
	Key     string // what it works on, e.g. a recording's path; "" for none
	OwnerID int64
	Params  json.RawMessage
	State   string
	Done    int64
	Total   int64
	// Message is an i18n key for what the job is doing right now (or why
	// it waits), MessageParams its parameters.
	Message       string
	MessageParams json.RawMessage
	// Error is an error code when State is failed.
	Error string
	// Checkpoint is the job kind's own state for resuming after a restart.
	Checkpoint json.RawMessage
	CreatedAt  time.Time
	StartedAt  time.Time // zero until it first runs
	FinishedAt time.Time // zero until it ends
}

const jobColumns = `id, kind, key, owner_id, params, state, done, total, message, msg_params, error, checkpoint, created_at, started_at, finished_at`

func rawOr(v json.RawMessage, def string) string {
	if len(v) == 0 {
		return def
	}
	return string(v)
}

func timeOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// InsertJob stores a new job; ID and CreatedAt are filled in.
func (s *Store) InsertJob(ctx context.Context, j Job) (Job, error) {
	j.CreatedAt = parseTime(s.timestamp())
	res, err := s.db.ExecContext(ctx, `INSERT INTO jobs (kind, key, owner_id, params, state, done, total, message, msg_params, error, checkpoint, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.Kind, j.Key, j.OwnerID, rawOr(j.Params, "{}"), j.State, j.Done, j.Total, j.Message, rawOr(j.MessageParams, "{}"), j.Error, rawOr(j.Checkpoint, ""), timeOrEmpty(j.CreatedAt))
	if err != nil {
		return Job{}, err
	}
	j.ID, err = res.LastInsertId()
	return j, err
}

// UpdateJob writes a job's changing fields (state, progress, message,
// error, checkpoint, start and end).
func (s *Store) UpdateJob(ctx context.Context, j Job) error {
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET state = ?, done = ?, total = ?, message = ?, msg_params = ?, error = ?, checkpoint = ?, started_at = ?, finished_at = ? WHERE id = ?`,
		j.State, j.Done, j.Total, j.Message, rawOr(j.MessageParams, "{}"), j.Error, rawOr(j.Checkpoint, ""), timeOrEmpty(j.StartedAt), timeOrEmpty(j.FinishedAt), j.ID)
	return err
}

func scanJob(row interface{ Scan(...any) error }) (Job, error) {
	var j Job
	var params, msgParams, checkpoint, created, started, finished string
	if err := row.Scan(&j.ID, &j.Kind, &j.Key, &j.OwnerID, &params, &j.State, &j.Done, &j.Total, &j.Message, &msgParams, &j.Error, &checkpoint, &created, &started, &finished); err != nil {
		return Job{}, err
	}
	j.Params, j.MessageParams = json.RawMessage(params), json.RawMessage(msgParams)
	if checkpoint != "" {
		j.Checkpoint = json.RawMessage(checkpoint)
	}
	j.CreatedAt = parseTime(created)
	if started != "" {
		j.StartedAt = parseTime(started)
	}
	if finished != "" {
		j.FinishedAt = parseTime(finished)
	}
	return j, nil
}

// ErrJobNotFound is returned for an unknown job ID.
var ErrJobNotFound = errors.New("job not found")

// JobByID returns one job.
func (s *Store) JobByID(ctx context.Context, id int64) (Job, error) {
	j, err := scanJob(s.db.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrJobNotFound
	}
	return j, err
}

// Jobs returns jobs newest first: all of them for ownerID 0, else one
// owner's. Active jobs come first regardless of age.
func (s *Store) Jobs(ctx context.Context, ownerID int64, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `SELECT ` + jobColumns + ` FROM jobs`
	args := []any{}
	if ownerID != 0 {
		q += ` WHERE owner_id = ?`
		args = append(args, ownerID)
	}
	q += ` ORDER BY state IN ('queued', 'running', 'waiting') DESC, id DESC LIMIT ?`
	args = append(args, limit)
	return s.queryJobs(ctx, q, args...)
}

// ActiveJobs returns every job still to finish, oldest first — the queue.
func (s *Store) ActiveJobs(ctx context.Context) ([]Job, error) {
	return s.queryJobs(ctx, `SELECT `+jobColumns+` FROM jobs WHERE state IN ('queued', 'running', 'waiting') ORDER BY id`)
}

func (s *Store) queryJobs(ctx context.Context, q string, args ...any) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// PruneJobs deletes finished jobs that ended before cutoff, and beyond the
// newest keep finished ones.
func (s *Store) PruneJobs(ctx context.Context, cutoff time.Time, keep int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE state NOT IN ('queued', 'running', 'waiting') AND (finished_at < ? OR id NOT IN (
		SELECT id FROM jobs WHERE state NOT IN ('queued', 'running', 'waiting') ORDER BY id DESC LIMIT ?))`, timeOrEmpty(cutoff), keep)
	return err
}

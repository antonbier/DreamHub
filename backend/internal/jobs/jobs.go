// Package jobs is DreamHub's background queue (#41): work that takes longer
// than a request, such as ad detection over a batch of recordings
// (docs/ADSCAN_PLAN.md). A hub registers a Kind and enqueues jobs of it;
// the queue persists them, runs a few at a time, reports progress live,
// holds them while a blocker says so (a recording is running — the single
// tuner and the disk belong to it), and lets their owner cancel them.
//
// The queue changes nothing by itself. A job that changes something on the
// box is enqueued only by applying a plan the user confirmed; analysis jobs
// only produce suggestions, which go through preview and confirm later.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/store"
)

// Kind is one sort of job, registered by the hub that owns it.
type Kind struct {
	// Name is the job's kind, "<hub>.<what>", e.g. "media.adscan".
	Name string
	// Run does the work. It reports progress through r, calls r.Wait at
	// safe points (between two recordings, every few samples) so a
	// blocker can hold it, and returns soon after ctx is done.
	Run func(ctx context.Context, r *Run) error
	// Resumable jobs are queued again after a restart, with their last
	// checkpoint; others are marked failed ("interrupted").
	Resumable bool
	// Abandon cleans up after a job a restart interrupted and that won't
	// resume (a half-written file); optional.
	Abandon func(ctx context.Context, j store.Job)
}

// Blocker reports a reason to hold the queue right now ("recording"), or
// "" when jobs may run. Reasons become i18n keys: "jobs.blocked.<reason>".
type Blocker func(ctx context.Context) string

// Error is a job failure with an error code the UI translates.
type Error struct {
	Code string
	Err  error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return e.Code
	}
	return e.Code + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

// Fail wraps err as a job failure with code.
func Fail(code string, err error) error { return &Error{Code: code, Err: err} }

var (
	ErrUnknownKind = errors.New("unknown job kind")
	ErrNotActive   = errors.New("job already finished")
)

// Service is the queue.
type Service struct {
	Store  *store.Store
	Logger *slog.Logger
	Now    func() time.Time
	// Workers is how many jobs run at once; 1 when unset. The box has six
	// cores, but its disk and the enigma2 process come first.
	Workers int
	// Blockers hold the queue: no job starts, and running jobs wait at
	// their next r.Wait.
	Blockers []Blocker
	// Recheck is how often a held queue asks its blockers again.
	Recheck time.Duration
	// Keep is how many finished jobs stay listed, at most KeepFor long.
	Keep    int
	KeepFor time.Duration

	mu        sync.Mutex
	enqMu     sync.Mutex // makes Enqueue's check-then-insert atomic
	kinds     map[string]Kind
	running   map[int64]*Run
	blockedBy string
	wake      chan struct{}
	subs      map[chan store.Job]struct{}
}

func (s *Service) init() {
	if s.kinds == nil {
		s.kinds = map[string]Kind{}
		s.running = map[int64]*Run{}
		s.wake = make(chan struct{}, 1)
		s.subs = map[chan store.Job]struct{}{}
	}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) recheck() time.Duration {
	if s.Recheck > 0 {
		return s.Recheck
	}
	return 15 * time.Second
}

// Register adds a job kind. Call it before Run.
func (s *Service) Register(k Kind) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	s.kinds[k.Name] = k
}

// Enqueue adds a job. With a key, a job of the same kind and key that is
// still to finish is returned instead of adding a second one (existing is
// then true) — analysing a recording twice at once makes no sense.
func (s *Service) Enqueue(ctx context.Context, kind, key string, ownerID int64, params any) (job store.Job, existing bool, err error) {
	s.mu.Lock()
	s.init()
	_, known := s.kinds[kind]
	s.mu.Unlock()
	if !known {
		return store.Job{}, false, ErrUnknownKind
	}
	raw, err := json.Marshal(params)
	if err != nil || params == nil {
		raw = json.RawMessage(`{}`)
	}
	s.enqMu.Lock()
	defer s.enqMu.Unlock()
	if key != "" {
		active, err := s.Store.ActiveJobs(ctx)
		if err != nil {
			return store.Job{}, false, err
		}
		for _, j := range active {
			if j.Kind == kind && j.Key == key {
				return j, true, nil
			}
		}
	}
	job, err = s.Store.InsertJob(ctx, store.Job{Kind: kind, Key: key, OwnerID: ownerID, Params: raw, State: store.JobQueued})
	if err != nil {
		return store.Job{}, false, err
	}
	s.publish(job)
	s.poke()
	return job, false, nil
}

// Cancel stops a job: a queued one at once, a running one as soon as its
// work notices.
func (s *Service) Cancel(ctx context.Context, id int64) (store.Job, error) {
	s.mu.Lock()
	s.init()
	r := s.running[id]
	s.mu.Unlock()
	if r != nil {
		r.cancel(true)
		return r.snapshot(), nil
	}
	j, err := s.Store.JobByID(ctx, id)
	if err != nil {
		return store.Job{}, err
	}
	if !store.JobActive(j.State) {
		return j, ErrNotActive
	}
	j.State, j.FinishedAt = store.JobCanceled, s.now()
	if err := s.Store.UpdateJob(ctx, j); err != nil {
		return store.Job{}, err
	}
	s.publish(j)
	return j, nil
}

// BlockedBy is the reason the queue is held right now, or "".
func (s *Service) BlockedBy() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blockedBy
}

// Subscribe registers a live listener for job changes. Progress arrives at
// most about once a second per job. Call cancel when done.
func (s *Service) Subscribe() (<-chan store.Job, func()) {
	ch := make(chan store.Job, 32)
	s.mu.Lock()
	s.init()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}
}

func (s *Service) publish(j store.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- j:
		default: // a slow listener misses this one; GET /api/v1/jobs has it
		}
	}
}

func (s *Service) poke() {
	s.mu.Lock()
	s.init()
	wake := s.wake
	s.mu.Unlock()
	select {
	case wake <- struct{}{}:
	default:
	}
}

// blocked asks the blockers and remembers the answer for BlockedBy.
func (s *Service) blocked(ctx context.Context) string {
	reason := ""
	for _, b := range s.Blockers {
		if reason = b(ctx); reason != "" {
			break
		}
	}
	s.mu.Lock()
	changed := s.blockedBy != reason
	s.blockedBy = reason
	s.mu.Unlock()
	if changed && s.Logger != nil {
		if reason != "" {
			s.Logger.Info("job queue held", "reason", reason)
		} else {
			s.Logger.Info("job queue free again")
		}
	}
	return reason
}

// Run recovers jobs a restart interrupted and then runs the queue until
// ctx is done. Meant for its own goroutine for the process's lifetime.
func (s *Service) Run(ctx context.Context) {
	s.mu.Lock()
	s.init()
	wake := s.wake
	s.mu.Unlock()
	s.recover(ctx)
	s.prune(ctx)
	tick := time.NewTicker(s.recheck())
	defer tick.Stop()
	for {
		s.dispatch(ctx)
		select {
		case <-ctx.Done():
			s.mu.Lock()
			for _, r := range s.running {
				r.cancel(false)
			}
			s.mu.Unlock()
			return
		case <-wake:
		case <-tick.C:
		}
	}
}

// recover requeues resumable jobs that were running when DreamHub stopped,
// and fails the rest.
func (s *Service) recover(ctx context.Context) {
	active, err := s.Store.ActiveJobs(ctx)
	if err != nil {
		s.logErr("loading jobs", err)
		return
	}
	for _, j := range active {
		if j.State == store.JobQueued {
			continue
		}
		s.mu.Lock()
		k, ok := s.kinds[j.Kind]
		s.mu.Unlock()
		if ok && k.Resumable {
			j.State, j.Message, j.MessageParams = store.JobQueued, "", nil
		} else {
			j.State, j.Error, j.FinishedAt = store.JobFailed, "interrupted", s.now()
			if ok && k.Abandon != nil {
				k.Abandon(ctx, j)
			}
		}
		if err := s.Store.UpdateJob(ctx, j); err != nil {
			s.logErr("recovering a job", err)
		}
	}
}

func (s *Service) dispatch(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	workers := max(s.Workers, 1)
	s.mu.Lock()
	free := workers - len(s.running)
	s.mu.Unlock()
	if free <= 0 {
		return
	}
	active, err := s.Store.ActiveJobs(ctx)
	if err != nil {
		s.logErr("loading jobs", err)
		return
	}
	var queued []store.Job
	for _, j := range active {
		if j.State == store.JobQueued {
			queued = append(queued, j)
		}
	}
	if len(queued) == 0 {
		return
	}
	if s.blocked(ctx) != "" {
		return
	}
	for _, j := range queued {
		if free == 0 {
			break
		}
		s.mu.Lock()
		k, ok := s.kinds[j.Kind]
		s.mu.Unlock()
		if !ok {
			j.State, j.Error, j.FinishedAt = store.JobFailed, "unknown_kind", s.now()
			_ = s.Store.UpdateJob(ctx, j)
			s.publish(j)
			continue
		}
		s.start(ctx, k, j)
		free--
	}
}

func (s *Service) start(parent context.Context, k Kind, j store.Job) {
	ctx, cancel := context.WithCancel(parent)
	r := &Run{svc: s, job: j, stop: cancel}
	r.job.State = store.JobRunning
	if r.job.StartedAt.IsZero() {
		r.job.StartedAt = s.now()
	}
	s.mu.Lock()
	s.running[j.ID] = r
	s.mu.Unlock()
	r.save(true)

	go func() {
		defer cancel()
		err := safeRun(ctx, k, r)
		r.mu.Lock()
		canceled := r.canceled
		r.mu.Unlock()
		if parent.Err() != nil && !canceled {
			// DreamHub is stopping: leave it active, recover decides on
			// the next start.
			s.mu.Lock()
			delete(s.running, j.ID)
			s.mu.Unlock()
			return
		}
		r.mu.Lock()
		switch {
		case canceled:
			r.job.State = store.JobCanceled
		case err != nil:
			r.job.State, r.job.Error = store.JobFailed, "job_failed"
			var je *Error
			if errors.As(err, &je) {
				r.job.Error = je.Code
			}
		default:
			r.job.State = store.JobDone
			if r.job.Total > 0 {
				r.job.Done = r.job.Total
			}
		}
		r.job.FinishedAt = s.now()
		r.mu.Unlock()
		if err != nil && !canceled {
			s.logErr("job "+k.Name+" failed", err)
		}
		r.save(true)
		s.mu.Lock()
		delete(s.running, j.ID)
		s.mu.Unlock()
		s.prune(context.WithoutCancel(parent))
		s.poke()
	}()
}

// safeRun turns a panicking job into a failed one instead of taking
// DreamHub down.
func safeRun(ctx context.Context, k Kind, r *Run) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = Fail("job_panicked", errors.New("panic"))
		}
	}()
	return k.Run(ctx, r)
}

func (s *Service) prune(ctx context.Context) {
	keep, keepFor := s.Keep, s.KeepFor
	if keep <= 0 {
		keep = 200
	}
	if keepFor <= 0 {
		keepFor = 30 * 24 * time.Hour
	}
	if err := s.Store.PruneJobs(ctx, s.now().Add(-keepFor), keep); err != nil {
		s.logErr("pruning jobs", err)
	}
}

func (s *Service) logErr(msg string, err error) {
	if s.Logger != nil {
		s.Logger.Warn(msg, "err", err)
	}
}

// Run is a job while it runs: the handle its Kind's Run reports through.
type Run struct {
	svc  *Service
	stop context.CancelFunc

	mu       sync.Mutex
	job      store.Job
	canceled bool
	saved    time.Time
}

// ID is the job's ID.
func (r *Run) ID() int64 { return r.snapshot().ID }

// OwnerID is the user who started the job.
func (r *Run) OwnerID() int64 { return r.snapshot().OwnerID }

// Params decodes the job's parameters into v.
func (r *Run) Params(v any) error { return json.Unmarshal(r.snapshot().Params, v) }

// Checkpoint decodes the last saved checkpoint into v; false when there is
// none (a fresh start).
func (r *Run) Checkpoint(v any) (bool, error) {
	cp := r.snapshot().Checkpoint
	if len(cp) == 0 {
		return false, nil
	}
	return true, json.Unmarshal(cp, v)
}

// SaveCheckpoint stores v as the state to resume from after a restart.
func (r *Run) SaveCheckpoint(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.job.Checkpoint = raw
	r.mu.Unlock()
	r.save(true)
	return nil
}

// Progress reports done of total steps and what is happening (an i18n key
// with params). Cheap to call often: it is stored and sent at most about
// once a second.
func (r *Run) Progress(done, total int64, message string, params any) {
	raw, err := json.Marshal(params)
	if err != nil || params == nil {
		raw = json.RawMessage(`{}`)
	}
	r.mu.Lock()
	r.job.Done, r.job.Total, r.job.Message, r.job.MessageParams = done, total, message, raw
	r.mu.Unlock()
	r.save(false)
}

// Wait returns at once when nothing holds the queue; else it marks the job
// waiting and returns when the blockers let go (nil) or ctx ends (its
// error). Call it at safe points.
func (r *Run) Wait(ctx context.Context) error {
	reason := r.svc.blocked(ctx)
	if reason == "" {
		return ctx.Err()
	}
	r.mu.Lock()
	message, params := r.job.Message, r.job.MessageParams
	r.job.State, r.job.Message, r.job.MessageParams = store.JobWaiting, "jobs.blocked."+reason, json.RawMessage(`{}`)
	r.mu.Unlock()
	r.save(true)
	for reason != "" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(r.svc.recheck()):
		}
		reason = r.svc.blocked(ctx)
	}
	r.mu.Lock()
	r.job.State, r.job.Message, r.job.MessageParams = store.JobRunning, message, params
	r.mu.Unlock()
	r.save(true)
	return nil
}

func (r *Run) snapshot() store.Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.job
}

func (r *Run) cancel(byUser bool) {
	r.mu.Lock()
	if byUser {
		r.canceled = true
	}
	r.mu.Unlock()
	r.stop()
}

// save persists and publishes the job; unless force, at most once a second.
func (r *Run) save(force bool) {
	now := r.svc.now()
	r.mu.Lock()
	if !force && now.Sub(r.saved) < time.Second {
		r.mu.Unlock()
		return
	}
	r.saved = now
	j := r.job
	r.mu.Unlock()
	if err := r.svc.Store.UpdateJob(context.Background(), j); err != nil {
		r.svc.logErr("saving a job", err)
	}
	r.svc.publish(j)
}

package jobs

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/store"
)

func newService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return &Service{Store: st, Recheck: 10 * time.Millisecond}, st
}

func start(t *testing.T, s *Service) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	stop := func() { cancel(); <-done }
	t.Cleanup(stop)
	return cancel
}

// waitFor polls the store until job id reaches state.
func waitFor(t *testing.T, st *store.Store, id int64, state string) store.Job {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		j, err := st.JobByID(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if j.State == state {
			return j
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %d: state %q, want %q (error %q)", id, j.State, state, j.Error)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRunsJobsWithProgress(t *testing.T) {
	s, st := newService(t)
	s.Register(Kind{Name: "test.count", Run: func(ctx context.Context, r *Run) error {
		var p struct{ N int64 }
		if err := r.Params(&p); err != nil {
			return err
		}
		for i := int64(1); i <= p.N; i++ {
			if err := r.Wait(ctx); err != nil {
				return err
			}
			r.Progress(i, p.N, "test.counting", map[string]int64{"i": i})
		}
		return nil
	}})
	start(t, s)
	j, existing, err := s.Enqueue(context.Background(), "test.count", "", 1, map[string]int{"N": 5})
	if err != nil || existing {
		t.Fatal(err, existing)
	}
	got := waitFor(t, st, j.ID, store.JobDone)
	if got.Done != 5 || got.Total != 5 || got.StartedAt.IsZero() || got.FinishedAt.IsZero() {
		t.Fatalf("finished job: %+v", got)
	}
}

func TestEnqueueSameKeyTwiceReturnsTheFirst(t *testing.T) {
	s, _ := newService(t)
	s.Register(Kind{Name: "test.noop", Run: func(context.Context, *Run) error { return nil }})
	// Not running the queue: both calls see the first job still queued.
	a, _, err := s.Enqueue(context.Background(), "test.noop", "/media/hdd/movie/a.ts", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, existing, err := s.Enqueue(context.Background(), "test.noop", "/media/hdd/movie/a.ts", 2, nil)
	if err != nil || !existing || b.ID != a.ID {
		t.Fatalf("second enqueue: id %d (first %d), existing %v, err %v", b.ID, a.ID, existing, err)
	}
	if _, _, err := s.Enqueue(context.Background(), "test.unknown", "", 1, nil); !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("unknown kind: %v", err)
	}
}

func TestBlockerHoldsQueueAndRunningJobs(t *testing.T) {
	s, st := newService(t)
	var recording atomic.Bool
	s.Blockers = []Blocker{func(context.Context) string {
		if recording.Load() {
			return "recording"
		}
		return ""
	}}
	step := make(chan struct{})
	s.Register(Kind{Name: "test.steps", Run: func(ctx context.Context, r *Run) error {
		for i := 0; i < 2; i++ {
			<-step
			if err := r.Wait(ctx); err != nil {
				return err
			}
		}
		return nil
	}})
	start(t, s)

	j, _, _ := s.Enqueue(context.Background(), "test.steps", "", 1, nil)
	waitFor(t, st, j.ID, store.JobRunning)
	recording.Store(true)
	step <- struct{}{}
	held := waitFor(t, st, j.ID, store.JobWaiting)
	if held.Message != "jobs.blocked.recording" {
		t.Fatalf("waiting message %q", held.Message)
	}
	// A new job doesn't start while held.
	j2, _, _ := s.Enqueue(context.Background(), "test.steps", "", 1, nil)
	time.Sleep(50 * time.Millisecond)
	if got, _ := st.JobByID(context.Background(), j2.ID); got.State != store.JobQueued {
		t.Fatalf("second job started while held: %s", got.State)
	}
	if s.BlockedBy() != "recording" {
		t.Fatalf("BlockedBy %q", s.BlockedBy())
	}
	recording.Store(false)
	waitFor(t, st, j.ID, store.JobRunning)
	step <- struct{}{}
	waitFor(t, st, j.ID, store.JobDone)
	step <- struct{}{}
	step <- struct{}{}
	waitFor(t, st, j2.ID, store.JobDone)
}

func TestCancelQueuedAndRunning(t *testing.T) {
	s, st := newService(t)
	s.Register(Kind{Name: "test.forever", Run: func(ctx context.Context, r *Run) error {
		<-ctx.Done()
		return ctx.Err()
	}})
	start(t, s)
	running, _, _ := s.Enqueue(context.Background(), "test.forever", "", 1, nil)
	queued, _, _ := s.Enqueue(context.Background(), "test.forever", "", 1, nil)
	waitFor(t, st, running.ID, store.JobRunning)

	if _, err := s.Cancel(context.Background(), queued.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, st, queued.ID, store.JobCanceled)
	if _, err := s.Cancel(context.Background(), running.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, st, running.ID, store.JobCanceled)
	if _, err := s.Cancel(context.Background(), running.ID); !errors.Is(err, ErrNotActive) {
		t.Fatalf("cancelling a finished job: %v", err)
	}
}

func TestFailureCodesAndPanics(t *testing.T) {
	s, st := newService(t)
	s.Register(Kind{Name: "test.fail", Run: func(context.Context, *Run) error {
		return Fail("ffmpeg_missing", errors.New("exec: not found"))
	}})
	s.Register(Kind{Name: "test.panic", Run: func(context.Context, *Run) error { panic("boom") }})
	start(t, s)
	a, _, _ := s.Enqueue(context.Background(), "test.fail", "", 1, nil)
	b, _, _ := s.Enqueue(context.Background(), "test.panic", "", 1, nil)
	if got := waitFor(t, st, a.ID, store.JobFailed); got.Error != "ffmpeg_missing" {
		t.Fatalf("error code %q", got.Error)
	}
	if got := waitFor(t, st, b.ID, store.JobFailed); got.Error != "job_panicked" {
		t.Fatalf("error code %q", got.Error)
	}
}

func TestRestartResumesFromCheckpointOrFails(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	// First process: two jobs get to work, save a checkpoint, then
	// DreamHub stops.
	reached := make(chan struct{}, 2)
	first := &Service{Store: st, Workers: 2, Recheck: 10 * time.Millisecond}
	block := func(ctx context.Context, r *Run) error {
		if err := r.SaveCheckpoint(map[string]int{"next": 7}); err != nil {
			return err
		}
		reached <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	}
	first.Register(Kind{Name: "test.resumable", Run: block, Resumable: true})
	first.Register(Kind{Name: "test.oneshot", Run: block})
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { first.Run(runCtx); close(done) }()
	res, _, _ := first.Enqueue(ctx, "test.resumable", "", 1, nil)
	one, _, _ := first.Enqueue(ctx, "test.oneshot", "", 1, nil)
	<-reached
	<-reached
	stop()
	<-done
	time.Sleep(20 * time.Millisecond) // the job goroutines return after Run

	// Second process.
	var resumedAt sync.Map
	second := &Service{Store: st, Recheck: 10 * time.Millisecond}
	second.Register(Kind{Name: "test.resumable", Resumable: true, Run: func(ctx context.Context, r *Run) error {
		var cp struct{ Next int }
		ok, err := r.Checkpoint(&cp)
		resumedAt.Store("next", cp.Next)
		resumedAt.Store("ok", ok)
		return err
	}})
	var abandoned atomic.Int64
	second.Register(Kind{Name: "test.oneshot", Run: func(context.Context, *Run) error { return nil },
		Abandon: func(_ context.Context, j store.Job) { abandoned.Store(j.ID) }})
	start(t, second)
	waitFor(t, st, res.ID, store.JobDone)
	if got := waitFor(t, st, one.ID, store.JobFailed); got.Error != "interrupted" {
		t.Fatalf("one-shot job after restart: %q", got.Error)
	}
	if abandoned.Load() != one.ID {
		t.Fatalf("Abandon wasn't called for the interrupted one-shot job")
	}
	if next, _ := resumedAt.Load("next"); next != 7 {
		t.Fatalf("resumed from checkpoint %v, want 7", next)
	}
}

func TestSubscribeSeesStateChanges(t *testing.T) {
	s, _ := newService(t)
	s.Register(Kind{Name: "test.noop", Run: func(context.Context, *Run) error { return nil }})
	ch, cancel := s.Subscribe()
	defer cancel()
	start(t, s)
	j, _, _ := s.Enqueue(context.Background(), "test.noop", "", 1, nil)
	seen := map[string]bool{}
	timeout := time.After(3 * time.Second)
	for !seen[store.JobDone] {
		select {
		case got := <-ch:
			if got.ID == j.ID {
				seen[got.State] = true
			}
		case <-timeout:
			t.Fatalf("saw %v, never done", seen)
		}
	}
	if !seen[store.JobQueued] || !seen[store.JobRunning] {
		t.Fatalf("saw %v", seen)
	}
}

func TestPruneKeepsActiveJobs(t *testing.T) {
	s, st := newService(t)
	s.Keep = 1
	s.Register(Kind{Name: "test.noop", Run: func(context.Context, *Run) error { return nil }})
	ctx := context.Background()
	var ids []int64
	for i := 0; i < 3; i++ {
		j, _, _ := s.Enqueue(ctx, "test.noop", "", 1, nil)
		ids = append(ids, j.ID)
	}
	start(t, s)
	waitFor(t, st, ids[2], store.JobDone)
	queued, _, _ := s.Enqueue(ctx, "test.noop", "", 1, nil)
	waitFor(t, st, queued.ID, store.JobDone)
	all, err := st.Jobs(ctx, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != queued.ID {
		t.Fatalf("after pruning to 1: %d jobs, first %+v", len(all), all)
	}
}

func TestEnqueueSameKeyConcurrentCreatesOne(t *testing.T) {
	s, _ := newService(t)
	s.Register(Kind{Name: "test.noop", Run: func(context.Context, *Run) error { return nil }})
	const n = 16
	var wg sync.WaitGroup
	var fresh atomic.Int32
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, existing, err := s.Enqueue(context.Background(), "test.noop", "k", 1, nil)
			if err != nil {
				t.Error(err)
			}
			if !existing {
				fresh.Add(1)
			}
		}()
	}
	wg.Wait()
	if fresh.Load() != 1 {
		t.Fatalf("%d jobs created for one key, want 1", fresh.Load())
	}
}

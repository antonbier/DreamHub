package events

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

// fakeClient overrides TimerList and DeviceInfo with test-controlled data;
// every other method is the real mock's, unused by the poller.
type fakeClient struct {
	webcontrol.Client
	timers []webcontrol.Timer
	hdds   []webcontrol.HDD
	// frontends is the tuner list; infoErr makes DeviceInfo fail.
	frontends []webcontrol.Frontend
	infoErr   error
}

func (f *fakeClient) TimerList(context.Context) ([]webcontrol.Timer, error) { return f.timers, nil }
func (f *fakeClient) DeviceInfo(context.Context) (*webcontrol.DeviceInfo, error) {
	if f.infoErr != nil {
		return nil, f.infoErr
	}
	return &webcontrol.DeviceInfo{HDDs: f.hdds, Frontends: f.frontends}, nil
}

func newTestBus(t *testing.T) (*Bus, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return NewBus(st, nil), st
}

func keys(t *testing.T, st *store.Store) []string {
	t.Helper()
	entries, err := st.Notifications(context.Background(), 50, false)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		out[len(entries)-1-i] = e.Key // oldest first, matching publish order
	}
	return out
}

func TestPoller_RecordingStartedAndFinished(t *testing.T) {
	bus, st := newTestBus(t)
	fc := &fakeClient{Client: mock.New()}
	p := &Poller{WC: fc, Bus: bus}
	ctx := context.Background()

	timer := webcontrol.Timer{ServiceReference: "1:0:1:1:1:1:1:0:0:0:", Name: "Example", Begin: 100, End: 200}

	fc.timers = []webcontrol.Timer{timer} // state 0 (waiting)
	p.tick(ctx)
	if got := keys(t, st); len(got) != 0 {
		t.Fatalf("a waiting timer must not publish anything, got %v", got)
	}

	timer.State = webcontrol.TimerRunning
	fc.timers = []webcontrol.Timer{timer}
	p.tick(ctx)
	if got := keys(t, st); len(got) != 1 || got[0] != "recording.started" {
		t.Fatalf("expected exactly one recording.started, got %v", got)
	}

	// Still running: no repeat event.
	p.tick(ctx)
	if got := keys(t, st); len(got) != 1 {
		t.Fatalf("a still-running timer must not re-publish, got %v", got)
	}

	timer.State = webcontrol.TimerEnded
	fc.timers = []webcontrol.Timer{timer}
	p.tick(ctx)
	got := keys(t, st)
	if len(got) != 2 || got[1] != "recording.finished" {
		t.Fatalf("expected recording.started then recording.finished, got %v", got)
	}
}

func TestPoller_RecordingFinishedWhenTimerDisappears(t *testing.T) {
	bus, st := newTestBus(t)
	fc := &fakeClient{Client: mock.New()}
	p := &Poller{WC: fc, Bus: bus}
	ctx := context.Background()

	timer := webcontrol.Timer{ServiceReference: "1:0:1:1:1:1:1:0:0:0:", Name: "Example", Begin: 100, End: 200}
	fc.timers = []webcontrol.Timer{timer}
	p.tick(ctx) // baseline: waiting
	timer.State = webcontrol.TimerRunning
	fc.timers = []webcontrol.Timer{timer}
	p.tick(ctx)

	fc.timers = nil // deleted mid-recording
	p.tick(ctx)

	got := keys(t, st)
	if len(got) != 2 || got[0] != "recording.started" || got[1] != "recording.finished" {
		t.Fatalf("a deleted running timer must still publish recording.finished, got %v", got)
	}
	entries, _ := st.Notifications(ctx, 1, false)
	if !strings.Contains(string(entries[0].Params), "Example") {
		t.Fatalf("the finished event lost the recording's name: %s", entries[0].Params)
	}
}

// A restart mid-recording takes the running recording as the baseline:
// no "started" for something that started before.
func TestPoller_RestartDoesNotReannounce(t *testing.T) {
	bus, st := newTestBus(t)
	fc := &fakeClient{Client: mock.New()}
	p := &Poller{WC: fc, Bus: bus}
	ctx := context.Background()
	running := webcontrol.Timer{ServiceReference: "1:0:1:1:1:1:1:0:0:0:", Name: "Example", Begin: 100, End: 200, State: webcontrol.TimerRunning}
	fc.timers = []webcontrol.Timer{running}
	p.tick(ctx)
	p.tick(ctx)
	if got := keys(t, st); len(got) != 0 {
		t.Fatalf("got %v after a restart mid-recording", got)
	}
	// Extending the running recording isn't a new recording.
	running.End = 500
	fc.timers = []webcontrol.Timer{running}
	p.tick(ctx)
	if got := keys(t, st); len(got) != 0 {
		t.Fatalf("got %v after extending the end", got)
	}
}

// A zap timer records nothing.
func TestPoller_ZapTimersAreNotRecordings(t *testing.T) {
	bus, st := newTestBus(t)
	fc := &fakeClient{Client: mock.New()}
	p := &Poller{WC: fc, Bus: bus}
	ctx := context.Background()
	zap := webcontrol.Timer{ServiceReference: "1:0:1:1:1:1:1:0:0:0:", Begin: 100, End: 200, JustPlay: true}
	fc.timers = []webcontrol.Timer{zap}
	p.tick(ctx)
	zap.State = webcontrol.TimerRunning
	fc.timers = []webcontrol.Timer{zap}
	p.tick(ctx)
	if got := keys(t, st); len(got) != 0 {
		t.Fatalf("a zap timer published %v", got)
	}
}

func TestPoller_DiskAboveThresholdWithHysteresis(t *testing.T) {
	bus, st := newTestBus(t)
	fc := &fakeClient{Client: mock.New()}
	p := &Poller{WC: fc, Bus: bus}
	ctx := context.Background()

	fc.hdds = []webcontrol.HDD{{Model: "Samsung", Capacity: "1 TB", Free: "50 GB"}} // 95%
	p.tick(ctx)
	if got := keys(t, st); len(got) != 1 || got[0] != "disk.aboveThreshold" {
		t.Fatalf("expected one disk.aboveThreshold at 95%%, got %v", got)
	}

	// Still above 90%: no repeat.
	p.tick(ctx)
	if got := keys(t, st); len(got) != 1 {
		t.Fatalf("must not re-publish while still above threshold, got %v", got)
	}

	// Drops to 87% — between the reset (85%) and the threshold (90%): no
	// new event yet, but the flag isn't reset either.
	fc.hdds = []webcontrol.HDD{{Model: "Samsung", Capacity: "1 TB", Free: "130 GB"}}
	p.tick(ctx)
	if got := keys(t, st); len(got) != 1 {
		t.Fatalf("87%% must not re-publish (still above the 85%% reset line), got %v", got)
	}

	// Drops below 85%, then back above 90%: fires again.
	fc.hdds = []webcontrol.HDD{{Model: "Samsung", Capacity: "1 TB", Free: "200 GB"}} // 80%
	p.tick(ctx)
	fc.hdds = []webcontrol.HDD{{Model: "Samsung", Capacity: "1 TB", Free: "50 GB"}} // 95% again
	p.tick(ctx)
	if got := keys(t, st); len(got) != 2 {
		t.Fatalf("expected a second disk.aboveThreshold after dropping below the reset line, got %v", got)
	}
}

func TestPoller_NewTimerConflict(t *testing.T) {
	bus, st := newTestBus(t)
	fc := &fakeClient{Client: mock.New()}
	p := &Poller{WC: fc, Bus: bus, Now: func() time.Time { return time.Unix(1000, 0) }}
	ctx := context.Background()

	// Two different transponders, overlapping, one tuner: a conflict that
	// already exists when DreamHub starts is the baseline, not news.
	a := webcontrol.Timer{ServiceReference: "1:0:19:2EE3:441:1:C00000:0:0:0:", Name: "Bauer sucht Frau", Begin: 2000, End: 5000}
	b := webcontrol.Timer{ServiceReference: "1:0:19:2B7A:3F3:1:C00000:0:0:0:", Name: "The Rookie", Begin: 4000, End: 7000}
	fc.timers = []webcontrol.Timer{a, b}
	p.tick(ctx)
	if got := keys(t, st); len(got) != 0 {
		t.Fatalf("existing conflicts must not be announced on the first tick, got %v", got)
	}

	// SerienRecorder adds a third, clashing timer in the background.
	c := webcontrol.Timer{ServiceReference: "1:0:19:283D:3FB:1:C00000:0:0:0:", Name: "Tatort", Begin: 6000, End: 9000}
	fc.timers = []webcontrol.Timer{a, b, c}
	p.tick(ctx)
	p.tick(ctx) // no repeat while it stays in conflict
	if got := keys(t, st); len(got) != 1 || got[0] != "timer.conflict" {
		t.Fatalf("expected exactly one timer.conflict, got %v", got)
	}

	// Past timers don't count, and a disabled one resolves nothing to announce.
	fc.timers = []webcontrol.Timer{{Name: "old", ServiceReference: a.ServiceReference, Begin: 100, End: 900}, {Name: "old2", ServiceReference: b.ServiceReference, Begin: 200, End: 800}}
	p.tick(ctx)
	if got := keys(t, st); len(got) != 1 {
		t.Errorf("past timers must not raise conflicts, got %v", got)
	}
}

// A failed device-info read must not make the poller assume one tuner and
// announce conflicts a two-tuner box doesn't have.
func TestPoller_NoConflictFromAMissingTunerList(t *testing.T) {
	bus, st := newTestBus(t)
	fc := &fakeClient{Client: mock.New(), frontends: []webcontrol.Frontend{{Name: "Tuner A", Model: "DVB-S2"}, {Name: "Tuner B", Model: "DVB-S2"}}}
	p := &Poller{WC: fc, Bus: bus, Now: func() time.Time { return time.Unix(1000, 0) }}
	ctx := context.Background()
	a := webcontrol.Timer{ServiceReference: "1:0:19:2EE3:441:1:C00000:0:0:0:", Name: "A", Begin: 2000, End: 5000}
	b := webcontrol.Timer{ServiceReference: "1:0:19:2B7A:3F3:1:C00000:0:0:0:", Name: "B", Begin: 4000, End: 7000}
	fc.timers = []webcontrol.Timer{a, b}
	p.tick(ctx) // baseline: two tuners, no clash
	fc.infoErr = errors.New("device info unavailable")
	p.tick(ctx)
	if got := keys(t, st); len(got) != 0 {
		t.Fatalf("announced %v without knowing the tuners", got)
	}
}

func TestPoller_TagsEventsWithTheBoxOnlyWhenAsked(t *testing.T) {
	bus, st := newTestBus(t)
	fc := &fakeClient{Client: mock.New()}
	tag := false
	p := &Poller{WC: fc, Bus: bus, BoxID: "bedroom", Tag: func() bool { return tag }}
	ctx := context.Background()
	timer := webcontrol.Timer{ServiceReference: "1:0:1:1:1:1:1:0:0:0:", Name: "Example", Begin: 100, End: 200}
	p.tick(ctx)
	timer.State = webcontrol.TimerRunning
	fc.timers = []webcontrol.Timer{timer}
	p.tick(ctx)
	timer.State = webcontrol.TimerEnded
	fc.timers = []webcontrol.Timer{timer}
	tag = true
	p.tick(ctx)
	entries, _ := st.Notifications(ctx, 10, false)
	if len(entries) != 2 {
		t.Fatalf("expected two events, got %d", len(entries))
	}
	for _, e := range entries { // newest first
		hasBox := strings.Contains(string(e.Params), `"box":"bedroom"`)
		if (e.Key == "recording.finished") != hasBox {
			t.Errorf("%s: params %s", e.Key, e.Params)
		}
	}
}

type deadClient struct{ webcontrol.Client }

func (deadClient) TimerList(context.Context) ([]webcontrol.Timer, error) {
	return nil, errors.New("no route to host")
}

func TestPoller_BacksOffWhileTheBoxDoesNotAnswer(t *testing.T) {
	bus, _ := newTestBus(t)
	p := &Poller{WC: deadClient{mock.New()}, Bus: bus, Interval: 30 * time.Second}
	if p.wait() != 30*time.Second {
		t.Fatalf("a box that answered is asked on time, got %v", p.wait())
	}
	for i := 0; i < 3; i++ {
		p.tick(context.Background())
	}
	if p.wait() != 8*30*time.Second {
		t.Errorf("three failures double the pause three times, got %v", p.wait())
	}
	for i := 0; i < 10; i++ {
		p.tick(context.Background())
	}
	if p.wait() != maxBackoff {
		t.Errorf("the pause stops growing at %v, got %v", maxBackoff, p.wait())
	}
}

func TestFleet_StartsOnePollerPerBoxIncludingLaterOnes(t *testing.T) {
	var mu sync.Mutex
	ids := []string{"main"}
	started := map[string]int{}
	f := &Fleet{Check: 5 * time.Millisecond,
		IDs: func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), ids...) },
		New: func(id string) *Poller {
			mu.Lock()
			started[id]++
			mu.Unlock()
			return &Poller{WC: deadClient{mock.New()}, Bus: nil, Interval: time.Hour}
		}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go f.Run(ctx)
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	ids = append(ids, "bedroom")
	mu.Unlock()
	time.Sleep(40 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if started["main"] != 1 || started["bedroom"] != 1 {
		t.Errorf("each box gets exactly one poller: %v", started)
	}
}

func TestFleet_StopsThePollerOfARemovedBox(t *testing.T) {
	var mu sync.Mutex
	ids := []string{"main", "bedroom"}
	stopped := map[string]bool{}
	f := &Fleet{Check: 5 * time.Millisecond,
		IDs: func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), ids...) },
		// A poller is stopped through its context: the client notes the cancel.
		New: func(id string) *Poller {
			return &Poller{WC: ctxWatcher{deadClient{mock.New()}, func() { mu.Lock(); stopped[id] = true; mu.Unlock() }}, Interval: time.Hour, BoxID: id}
		}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go f.Run(ctx)
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	ids = []string{"main"}
	mu.Unlock()
	time.Sleep(40 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if !stopped["bedroom"] || stopped["main"] {
		t.Errorf("only the removed box's poller stops: %v", stopped)
	}
}

// ctxWatcher calls onDone once the context of a TimerList call ends.
type ctxWatcher struct {
	webcontrol.Client
	onDone func()
}

func (w ctxWatcher) TimerList(ctx context.Context) ([]webcontrol.Timer, error) {
	go func() { <-ctx.Done(); w.onDone() }()
	return nil, errors.New("no route to host")
}

func TestPoller_ReportsWhetherTheBoxAnswered(t *testing.T) {
	var got []bool
	p := &Poller{WC: deadClient{mock.New()}, Interval: time.Hour, OnReach: func(ok bool) { got = append(got, ok) }}
	p.tick(context.Background())
	q := &Poller{WC: mock.New(), Interval: time.Hour, OnReach: func(ok bool) { got = append(got, ok) }}
	q.tick(context.Background())
	if len(got) != 2 || got[0] || !got[1] {
		t.Errorf("reach: %v", got)
	}
}

func TestPoller_BoostIgnoresTheBackOffForAWhile(t *testing.T) {
	bus, _ := newTestBus(t)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	p := &Poller{WC: deadClient{mock.New()}, Bus: bus, Interval: 30 * time.Second, Now: func() time.Time { return now }}
	for i := 0; i < 5; i++ {
		p.tick(context.Background())
	}
	if p.wait() != 5*time.Minute {
		t.Fatalf("backed off, got %v", p.wait())
	}
	p.Boost()
	if p.wait() != BoostEvery {
		t.Errorf("switched on: asked every %v, got %v", BoostEvery, p.wait())
	}
	select {
	case <-p.nudgeCh():
	default:
		t.Error("a boost looks at the box now")
	}
	now = now.Add(BoostFor + time.Second)
	if p.wait() != 5*time.Minute {
		t.Errorf("the boost ends, got %v", p.wait())
	}
}

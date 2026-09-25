package events

import (
	"context"
	"testing"

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
}

func (f *fakeClient) TimerList(context.Context) ([]webcontrol.Timer, error) { return f.timers, nil }
func (f *fakeClient) DeviceInfo(context.Context) (*webcontrol.DeviceInfo, error) {
	return &webcontrol.DeviceInfo{HDDs: f.hdds}, nil
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

	timer := webcontrol.Timer{ServiceReference: "1:0:1:1:1:1:1:0:0:0:", Begin: 100, End: 200, State: webcontrol.TimerRunning}
	fc.timers = []webcontrol.Timer{timer}
	p.tick(ctx)

	fc.timers = nil // deleted mid-recording
	p.tick(ctx)

	got := keys(t, st)
	if len(got) != 2 || got[0] != "recording.started" || got[1] != "recording.finished" {
		t.Fatalf("a deleted running timer must still publish recording.finished, got %v", got)
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

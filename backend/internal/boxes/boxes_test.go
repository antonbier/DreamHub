package boxes

import (
	"context"
	"errors"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

func twoBoxes(t *testing.T) (*Registry, *mock.Client, *mock.Client) {
	t.Helper()
	a, b := mock.New(), mock.New()
	r := NewRegistry()
	r.Add(store.Box{ID: "main", Name: "Living room"}, Clients{WC: a}, true)
	r.Add(store.Box{ID: "bedroom", Name: "Bedroom"}, Clients{WC: b}, false)
	return r, a, b
}

func TestWebControl_FollowsTheContextsBox(t *testing.T) {
	r, a, b := twoBoxes(t)
	ctx := context.Background()
	timers, _ := b.TimerList(ctx)
	if err := b.DeleteTimer(ctx, webcontrol.TimerRef{ServiceReference: timers[0].ServiceReference, Begin: timers[0].Begin, End: timers[0].End}); err != nil {
		t.Fatal(err)
	}
	wc := r.WebControl()
	inA, _ := wc.TimerList(box.With(ctx, "main"))
	inB, _ := wc.TimerList(box.With(ctx, "bedroom"))
	wantA, _ := a.TimerList(ctx)
	if len(inA) != len(wantA) || len(inB) != len(wantA)-1 {
		t.Fatalf("main %d, bedroom %d, expected %d and %d", len(inA), len(inB), len(wantA), len(wantA)-1)
	}
	// No box named: the first.
	if none, _ := wc.TimerList(ctx); len(none) != len(wantA) {
		t.Errorf("an unaddressed call goes to the first box, got %d timers", len(none))
	}
}

func TestWebControl_UnknownBoxIsAnErrorNotAFallback(t *testing.T) {
	r, _, _ := twoBoxes(t)
	_, err := r.WebControl().TimerList(box.With(context.Background(), "garage"))
	if !errors.Is(err, ErrUnknownBox) {
		t.Fatalf("expected ErrUnknownBox, got %v", err)
	}
}

func TestWebControl_EditorOnlyWhereTheClientHasOne(t *testing.T) {
	r, _, b := twoBoxes(t)
	b.NoBouquetEditor = true
	wc := r.WebControl()
	if !wc.BouquetEditorAvailable(box.With(context.Background(), "main")) || wc.BouquetEditorAvailable(box.With(context.Background(), "bedroom")) {
		t.Error("editor availability must follow the box")
	}
	// A client that has no editor at all is reported as not found.
	r.Add(store.Box{ID: "plain"}, Clients{WC: struct{ webcontrol.Client }{mock.New()}}, false)
	if err := wc.AddBouquet(box.With(context.Background(), "plain"), "x", false); !errors.Is(err, webcontrol.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSeriesAndAutoTimer_AbsentPluginIsNotInstalled(t *testing.T) {
	r, _, _ := twoBoxes(t)
	ctx := box.With(context.Background(), "bedroom")
	if _, err := r.Series().Markers(ctx); !errors.Is(err, serienrecorder.ErrNotInstalled) {
		t.Errorf("series: %v", err)
	}
	if _, err := r.AutoTimer().List(ctx); err == nil {
		t.Error("autotimer: expected an error for a box without the plugin")
	}
}

func TestCapabilities_MeasuredPerBox(t *testing.T) {
	r, _, _ := twoBoxes(t)
	reports := map[string]connector.Report{
		connector.IDWebControl:     {State: connector.OK},
		connector.IDSerienRecorder: {State: connector.NotInstalled},
		connector.IDAutoTimer:      {State: connector.OK},
	}
	has := func(caps []string, c string) bool {
		for _, x := range caps {
			if x == c {
				return true
			}
		}
		return false
	}
	local := r.Capabilities(box.With(context.Background(), "main"), reports)
	remote := r.Capabilities(box.With(context.Background(), "bedroom"), reports)
	if !has(local, CapAutoTimer) || has(local, CapSerienRecorder) {
		t.Errorf("plugins follow the connector reports: %v", local)
	}
	if !has(local, CapDocker) || !has(local, CapRecordingsLocal) || !r.Full(box.With(context.Background(), "main")) {
		t.Errorf("the box DreamHub runs on is Full: %v", local)
	}
	if has(remote, CapDocker) || has(remote, CapPackages) || has(remote, CapRecordingsLocal) || r.Full(box.With(context.Background(), "bedroom")) {
		t.Errorf("another box is Basic and has no machine-level features: %v", remote)
	}
	if !has(remote, CapWebControl) {
		t.Errorf("a Basic box still has its web interface: %v", remote)
	}
	if got := r.Capabilities(box.With(context.Background(), "nowhere"), reports); len(got) != 0 {
		t.Errorf("an unknown box has no capabilities: %v", got)
	}
}

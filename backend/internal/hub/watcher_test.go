package hub

import (
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/status"
)

func TestStatusWatcher_FirstCallReportsNothing(t *testing.T) {
	w := NewStatusWatcher()
	statuses := []Status{{ID: "media", Name: "MediaHub", Level: status.Available}}
	if got := w.Diff(statuses); len(got) != 0 {
		t.Errorf("first call must not report a transition (nothing to compare against), got %+v", got)
	}
}

func TestStatusWatcher_ReportsAnActualChange(t *testing.T) {
	w := NewStatusWatcher()
	w.Diff([]Status{{ID: "media", Name: "MediaHub", Level: status.Available}})

	got := w.Diff([]Status{{ID: "media", Name: "MediaHub", Level: status.Degraded}})
	if len(got) != 1 || got[0].From != status.Available || got[0].To != status.Degraded || got[0].HubName != "MediaHub" {
		t.Fatalf("got %+v", got)
	}
	if got[0].Summary() != "MediaHub became degraded" {
		t.Errorf("summary = %q", got[0].Summary())
	}
}

func TestStatusWatcher_NoChangeReportsNothing(t *testing.T) {
	w := NewStatusWatcher()
	s := []Status{{ID: "media", Name: "MediaHub", Level: status.Available}}
	w.Diff(s)
	if got := w.Diff(s); len(got) != 0 {
		t.Errorf("an unchanged level must not be reported, got %+v", got)
	}
}

func TestStatusWatcher_TracksEachHubIndependently(t *testing.T) {
	w := NewStatusWatcher()
	w.Diff([]Status{
		{ID: "media", Name: "MediaHub", Level: status.Available},
		{ID: "guide", Name: "GuideHub", Level: status.Available},
	})
	got := w.Diff([]Status{
		{ID: "media", Name: "MediaHub", Level: status.Available},
		{ID: "guide", Name: "GuideHub", Level: status.Unavailable},
	})
	if len(got) != 1 || got[0].HubID != "guide" {
		t.Fatalf("only the changed hub must be reported, got %+v", got)
	}
}

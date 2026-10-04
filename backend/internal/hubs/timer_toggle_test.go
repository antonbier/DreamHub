package hubs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

// refusingBox is the mock box, but it refuses to add an enabled timer —
// as the real one does when it finds a conflict.
type refusingBox struct{ *mock.Client }

func (b refusingBox) AddTimer(ctx context.Context, t webcontrol.NewTimer) error {
	if !t.Disabled {
		return &webcontrol.RejectedError{Op: "timeradd", Text: "Conflicting Timer(s) detected!"}
	}
	return b.Client.AddTimer(ctx, t)
}

func TestTimerToggle_RestoresWhenTheBoxRefuses(t *testing.T) {
	box := mock.New()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	user, _ := st.EnsureDefaultUser(context.Background())
	now := func() time.Time { return mock.Evening.Add(18 * time.Hour) }
	plans := plan.NewService(st, now)
	ref := webcontrol.TimerRef{ServiceReference: "1:0:19:2EE3:441:1:C00000:0:0:0:", Begin: mock.Evening.Add(20*time.Hour + 15*time.Minute).Unix(), End: mock.Evening.Add(21*time.Hour + 15*time.Minute).Unix()}

	// First disable it on the plain mock, then try to enable on a refusing box.
	for _, step := range []struct {
		wc      webcontrol.Client
		enabled bool
		wantOK  bool
	}{{box, false, true}, {refusingBox{box}, true, false}} {
		h := &Timer{WC: step.wc, Plans: plans, Now: now}
		body, _ := json.Marshal(map[string]any{"serviceReference": ref.ServiceReference, "begin": ref.Begin, "end": ref.End, "enabled": step.enabled})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/", strings.NewReader(string(body))).WithContext(httpx.WithUser(context.Background(), user))
		h.planToggle(rec, req)
		var out struct {
			Plan struct{ ID string } `json:"plan"`
		}
		json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != http.StatusOK {
			t.Fatalf("plan: %d %s", rec.Code, rec.Body)
		}
		_, err := plans.Apply(context.Background(), user, out.Plan.ID)
		if (err == nil) != step.wantOK {
			t.Fatalf("enabled=%v: apply error %v", step.enabled, err)
		}
	}
	timers, _ := box.TimerList(context.Background())
	got, ok := findTimer(timers, ref)
	if !ok || !got.Disabled || len(timers) != 2 {
		t.Errorf("the refused change must leave the timer as it was (disabled, still there): %+v %v", got, ok)
	}
}

package hubs

import (
	"context"
	"encoding/json"
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

// endedBox lists two more timers: one finished, one switched off (the box
// reports that one as ended too), and records what is deleted.
type endedBox struct {
	*mock.Client
	deleted []int64
}

func (b *endedBox) TimerList(ctx context.Context) ([]webcontrol.Timer, error) {
	ts, err := b.Client.TimerList(ctx)
	ref := "1:0:19:2EE3:441:1:C00000:0:0:0:"
	return append(ts,
		webcontrol.Timer{ServiceReference: ref, Name: "done", Begin: 1000, End: 2000, State: webcontrol.TimerEnded},
		webcontrol.Timer{ServiceReference: ref, Name: "off", Begin: 3000, End: 4000, State: webcontrol.TimerEnded, Disabled: true},
	), err
}

func (b *endedBox) DeleteTimer(_ context.Context, ref webcontrol.TimerRef) error {
	b.deleted = append(b.deleted, ref.Begin)
	return nil
}

func TestTimerCleanup_OnlyFinishedTimers(t *testing.T) {
	box := &endedBox{Client: mock.New()}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	user, _ := st.EnsureDefaultUser(context.Background())
	now := func() time.Time { return mock.Evening }
	plans := plan.NewService(st, now)
	h := &Timer{WC: box, Plans: plans, Now: now}

	rec := httptest.NewRecorder()
	h.planCleanup(rec, httptest.NewRequest("POST", "/", strings.NewReader("{}")).WithContext(httpx.WithUser(context.Background(), user)))
	var out struct {
		Plan  struct{ ID string } `json:"plan"`
		Count int                 `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Count != 1 {
		t.Fatalf("count: %d %s", out.Count, rec.Body)
	}
	if _, err := plans.Apply(context.Background(), user, out.Plan.ID); err != nil {
		t.Fatal(err)
	}
	if len(box.deleted) != 1 || box.deleted[0] != 1000 {
		t.Fatalf("deleted %v, want only the finished timer", box.deleted)
	}
}

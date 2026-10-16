package hubs

import (
	"context"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/box"
)

// Each box has its own live-stream limit: two streams of one box do not
// keep another box from streaming.
func TestStream_LiveLimitIsPerBox(t *testing.T) {
	h := &Stream{}
	a, b := box.With(context.Background(), "main"), box.With(context.Background(), "bedroom")
	var releases []func()
	for i := 0; i < maxLiveStreams; i++ {
		rel, ok := h.acquire(a)
		if !ok {
			t.Fatalf("stream %d of box a refused", i)
		}
		releases = append(releases, rel)
	}
	if _, ok := h.acquire(a); ok {
		t.Error("box a is over its limit")
	}
	rel, ok := h.acquire(b)
	if !ok {
		t.Fatal("box b must not be held up by box a")
	}
	rel()
	releases[0]()
	if _, ok := h.acquire(a); !ok {
		t.Error("a released slot is free again")
	}
}

// Package events is DreamHub's core event bus (docs/V1_PLAN.md, WP7):
// "detection, not automation" — DreamHub notices things happening on the
// box (a recording started or finished, disk crossed a threshold) by
// polling the same WebControl endpoints every hub already uses, not by
// running anything on a timer itself. Every event is persisted
// (internal/store's notifications table) and fanned out live to anyone
// listening (the SSE stream behind GET /api/v1/events/stream).
package events

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/store"
)

// Bus persists events and fans them out to live subscribers (the bell,
// AdminHub's future live views). It never blocks a publisher on a slow
// subscriber — a subscriber that falls behind misses live updates but can
// always catch up from the persisted feed (GET /api/v1/notifications).
type Bus struct {
	store *store.Store
	now   func() time.Time

	mu   sync.Mutex
	subs map[chan store.Notification]struct{}
}

// NewBus returns a Bus persisting through st.
func NewBus(st *store.Store, now func() time.Time) *Bus {
	if now == nil {
		now = time.Now
	}
	return &Bus{store: st, now: now, subs: map[chan store.Notification]struct{}{}}
}

// Publish persists an event and fans it out to current subscribers. params
// is marshaled to JSON; a nil or unmarshalable value becomes `{}`.
func (b *Bus) Publish(ctx context.Context, key string, params any) (store.Notification, error) {
	raw, err := json.Marshal(params)
	if err != nil || params == nil {
		raw = json.RawMessage(`{}`)
	}
	n, err := b.store.AppendNotification(ctx, key, raw)
	if err != nil {
		return store.Notification{}, err
	}
	b.mu.Lock()
	for ch := range b.subs {
		select {
		case ch <- n:
		default: // a subscriber that isn't keeping up loses this one live; it's still in the persisted feed
		}
	}
	b.mu.Unlock()
	return n, nil
}

// Subscribe registers a live listener. Call the returned cancel func when
// done (e.g. when the SSE client disconnects) to stop leaking the channel.
func (b *Bus) Subscribe() (<-chan store.Notification, func()) {
	ch := make(chan store.Notification, 16)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	cancel := func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
	return ch, cancel
}

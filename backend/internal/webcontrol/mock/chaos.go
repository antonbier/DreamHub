package mock

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"
)

// Chaos makes the mock box behave like a real one under load
// (DREAMHUB_MOCK_CHAOS): enigma2 serves WebControl single-threaded, so
// answers take seconds when it's busy, some fail, and the box is away now
// and then (a reboot, Wi-Fi). Most of the bugs found on 2026-10-09 —
// timeouts, coalesced requests, a change interrupted halfway — only show
// against such a box, never against an instant, faultless mock.
type Chaos struct {
	// Each answer takes between MinDelay and MaxDelay.
	MinDelay, MaxDelay time.Duration
	// FailRate is the share of calls that fail outright.
	FailRate float64
	// The box is unreachable for OfflineFor out of every OfflineEvery.
	OfflineEvery, OfflineFor time.Duration

	mu    sync.Mutex
	start time.Time
}

// ErrChaos is a failure Chaos made up.
var ErrChaos = errors.New("mock: the box didn't answer (simulated)")

// Slow is a busy box: every answer takes 0.2–3 s.
func Slow() *Chaos { return &Chaos{MinDelay: 200 * time.Millisecond, MaxDelay: 3 * time.Second} }

// Flaky is a busy box that also fails one call in ten and is away for
// 20 s every 3 minutes.
func Flaky() *Chaos {
	c := Slow()
	c.FailRate, c.OfflineEvery, c.OfflineFor = 0.1, 3*time.Minute, 20*time.Second
	return c
}

// disturb delays the call and may fail it; nil Chaos does nothing.
func (c *Client) disturb(ctx context.Context) error {
	ch := c.Chaos
	if ch == nil {
		return nil
	}
	ch.mu.Lock()
	if ch.start.IsZero() {
		ch.start = time.Now()
	}
	elapsed := time.Since(ch.start)
	ch.mu.Unlock()
	if ch.OfflineEvery > 0 && elapsed%ch.OfflineEvery >= ch.OfflineEvery-ch.OfflineFor {
		// An unreachable box answers nothing: the caller waits until its
		// own deadline, like a real connection timeout.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(ch.OfflineFor):
			return ErrChaos
		}
	}
	delay := ch.MinDelay
	if ch.MaxDelay > ch.MinDelay {
		delay += time.Duration(rand.Int64N(int64(ch.MaxDelay - ch.MinDelay)))
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
	}
	if ch.FailRate > 0 && rand.Float64() < ch.FailRate {
		return ErrChaos
	}
	return nil
}

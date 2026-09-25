package events

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// DefaultInterval is how often the poller checks the box.
const DefaultInterval = 30 * time.Second

// diskThreshold and diskReset give the "above 90%" check hysteresis, so a
// disk hovering right at the line doesn't fire a fresh event every tick —
// it must drop back under diskReset before it can fire again.
const (
	diskThreshold = 90.0
	diskReset     = 85.0
)

// Poller detects "recording.started"/"recording.finished" and
// "disk.aboveThreshold" by diffing successive snapshots of
// /web/timerlist and /web/deviceinfo — the same data TimerHub and
// MediaHub's own screens already read, not a new capability on the box.
// Timer conflicts aren't detected here yet (open, docs/V1_PLAN.md WP7):
// TimerHub's own per-timer conflict marker already covers the interactive
// case; a background "a new conflict appeared" notification needs the same
// tuner-aware estimate (internal/tuner) wired in without duplicating it.
type Poller struct {
	WC       webcontrol.Client
	Bus      *Bus
	Interval time.Duration
	Logger   *slog.Logger

	running   map[string]bool // timer key -> was recording last tick
	diskAbove map[string]bool // disk model -> currently above threshold
}

func key(t webcontrol.Timer) string {
	return t.ServiceReference + "|" + strconv.FormatInt(t.Begin, 10) + "|" + strconv.FormatInt(t.End, 10)
}

// Run polls until ctx is done. Meant to run in its own goroutine for the
// process's lifetime.
func (p *Poller) Run(ctx context.Context) {
	if p.Interval <= 0 {
		p.Interval = DefaultInterval
	}
	p.tick(ctx)
	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.tick(ctx)
		}
	}
}

func (p *Poller) tick(ctx context.Context) {
	if p.running == nil {
		p.running = map[string]bool{}
	}
	if p.diskAbove == nil {
		p.diskAbove = map[string]bool{}
	}
	p.checkRecordings(ctx)
	p.checkDisk(ctx)
}

func (p *Poller) checkRecordings(ctx context.Context) {
	timers, err := p.WC.TimerList(ctx)
	if err != nil {
		if p.Logger != nil {
			p.Logger.Debug("events: poller could not read timers", "error", err)
		}
		return
	}
	seen := make(map[string]bool, len(timers))
	for _, t := range timers {
		k := key(t)
		recording := !t.Disabled && t.State == webcontrol.TimerRunning
		seen[k] = recording
		wasRecording := p.running[k]
		if recording && !wasRecording {
			p.Bus.Publish(ctx, "recording.started", map[string]any{"name": t.Name, "service": t.ServiceName})
		} else if wasRecording && !recording {
			p.Bus.Publish(ctx, "recording.finished", map[string]any{"name": t.Name, "service": t.ServiceName})
		}
	}
	// A timer that disappeared entirely (deleted) while recording also
	// counts as finished — MediaHub's own file is done being written either
	// way.
	for k, was := range p.running {
		if was && !seenContains(seen, k) {
			p.Bus.Publish(ctx, "recording.finished", map[string]any{})
		}
	}
	p.running = seen
}

func seenContains(seen map[string]bool, k string) bool {
	_, ok := seen[k]
	return ok
}

func (p *Poller) checkDisk(ctx context.Context) {
	info, err := p.WC.DeviceInfo(ctx)
	if err != nil {
		if p.Logger != nil {
			p.Logger.Debug("events: poller could not read device info", "error", err)
		}
		return
	}
	for _, hd := range info.HDDs {
		cap, ok := webcontrol.ParseSize(hd.Capacity)
		if !ok || cap <= 0 {
			continue
		}
		free, ok := webcontrol.ParseSize(hd.Free)
		if !ok {
			continue
		}
		pct := 100 * float64(cap-free) / float64(cap)
		switch {
		case pct >= diskThreshold && !p.diskAbove[hd.Model]:
			p.diskAbove[hd.Model] = true
			p.Bus.Publish(ctx, "disk.aboveThreshold", map[string]any{"model": hd.Model, "percent": pct})
		case pct < diskReset:
			p.diskAbove[hd.Model] = false
		}
	}
}

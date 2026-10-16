package events

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/tuner"
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
// It also notices "timer.conflict": a timer still to come that newly
// clashes with another on the box's tuners — the same estimate TimerHub's
// list shows (tuner.TimerConflicts), typically after SerienRecorder or
// AutoTimer added a timer in the background.
type Poller struct {
	WC       webcontrol.Client
	Bus      *Bus
	Interval time.Duration
	Logger   *slog.Logger
	// BoxID is the box this poller watches (internal/box); "" polls the
	// first box. Tag adds the box's ID to every event it publishes — set
	// by Fleet once there is more than one box to tell apart.
	BoxID string
	Tag   func() bool
	// OnReach is told after every tick whether the box answered (for the
	// box selector's offline state).
	OnReach func(online bool)
	// failures counts the ticks in a row the box did not answer, to back
	// off: a box that is off is not asked every 30 seconds.
	failures int

	// running holds the timers that were recording last tick, by channel
	// and start (not end: extending a running recording isn't a new one);
	// nil until the first tick, which only takes the baseline — a restart
	// mid-recording mustn't announce it as just started.
	running   map[string]webcontrol.Timer
	diskAbove map[string]bool // disk model -> currently above threshold
	// conflicting holds the timers that clashed last tick; nil until the
	// first tick, which only takes the baseline — a restart mustn't
	// re-announce every conflict that already existed.
	conflicting map[string]bool
	// Now is the clock ("still to come"); time.Now when nil.
	Now func() time.Time
}

// key identifies a timer across ticks by channel and start: the end can
// move (a recording extended, AutoTimer adjusting) without it becoming
// another timer — the box itself matches duplicates the same way.
func key(t webcontrol.Timer) string {
	return t.ServiceReference + "|" + strconv.FormatInt(t.Begin, 10)
}

// Run polls until ctx is done. Meant to run in its own goroutine for the
// process's lifetime.
func (p *Poller) Run(ctx context.Context) {
	if p.Interval <= 0 {
		p.Interval = DefaultInterval
	}
	for {
		p.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(p.wait()):
		}
	}
}

// maxBackoff is the longest pause between two looks at a box that does not
// answer.
const maxBackoff = 5 * time.Minute

// wait is the pause until the next tick: the interval, doubled for each
// failed tick in a row up to maxBackoff.
func (p *Poller) wait() time.Duration {
	d := p.Interval
	for i := 0; i < p.failures && d < maxBackoff; i++ {
		d *= 2
	}
	return min(d, max(maxBackoff, p.Interval))
}

// publish sends an event, tagged with the box when there are several.
func (p *Poller) publish(ctx context.Context, key string, params map[string]any) {
	if p.BoxID != "" && p.Tag != nil && p.Tag() {
		params["box"] = p.BoxID
	}
	p.Bus.Publish(ctx, key, params)
}

func (p *Poller) tick(ctx context.Context) {
	if p.BoxID != "" {
		ctx = box.With(ctx, p.BoxID)
	}
	if p.diskAbove == nil {
		p.diskAbove = map[string]bool{}
	}
	timers, err := p.WC.TimerList(ctx)
	if err != nil {
		p.failures++
	} else {
		p.failures = 0
	}
	if p.OnReach != nil {
		p.OnReach(err == nil)
	}
	if err != nil {
		if p.Logger != nil {
			p.Logger.Debug("events: poller could not read timers", "error", err)
		}
	}
	info, infoErr := p.WC.DeviceInfo(ctx)
	if infoErr != nil && p.Logger != nil {
		p.Logger.Debug("events: poller could not read device info", "error", infoErr)
	}
	if err == nil {
		p.checkRecordings(ctx, timers)
		// Without the tuner list a clash can't be judged (a missing list
		// would be read as "one tuner"), and the last known state stays.
		if infoErr == nil {
			p.checkConflicts(ctx, timers, info)
		}
	}
	if infoErr == nil {
		p.checkDisk(ctx, info)
	}
}

func (p *Poller) checkConflicts(ctx context.Context, timers []webcontrol.Timer, info *webcontrol.DeviceInfo) {
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	clash := tuner.TimerConflicts(timers, info)
	current := map[string]bool{}
	for i, t := range timers {
		if !clash[i] || t.End <= now.Unix() {
			continue
		}
		k := key(t)
		current[k] = true
		if p.conflicting != nil && !p.conflicting[k] {
			p.publish(ctx, TimerConflict, map[string]any{"name": t.Name, "service": t.ServiceName, "begin": t.Begin})
		}
	}
	p.conflicting = current
}

func (p *Poller) checkRecordings(ctx context.Context, timers []webcontrol.Timer) {
	now := make(map[string]webcontrol.Timer, len(timers))
	for _, t := range timers {
		// A zap timer switches channels; it records nothing.
		if !t.Disabled && !t.JustPlay && t.State == webcontrol.TimerRunning {
			now[key(t)] = t
		}
	}
	if p.running != nil {
		for k, t := range now {
			if _, was := p.running[k]; !was {
				p.publish(ctx, RecordingStarted, map[string]any{"name": t.Name, "service": t.ServiceName})
			}
		}
		// Gone from "recording": finished, or deleted while recording —
		// the file is done being written either way. The name comes from
		// the last tick, the timer may not be listed any more.
		for k, t := range p.running {
			if _, still := now[k]; !still {
				p.publish(ctx, RecordingFinished, map[string]any{"name": t.Name, "service": t.ServiceName})
			}
		}
	}
	p.running = now
}

func (p *Poller) checkDisk(ctx context.Context, info *webcontrol.DeviceInfo) {
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
			p.publish(ctx, DiskAboveLimit, map[string]any{"model": hd.Model, "percent": pct})
		case pct < diskReset:
			p.diskAbove[hd.Model] = false
		}
	}
}

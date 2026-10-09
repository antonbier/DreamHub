package adscan

import (
	"context"
	"errors"
	"math"
)

// Block is a found ad break.
type Block struct {
	Span
	// Uncertain: a boundary sample scored close to the threshold, or the
	// logo mask is weak. The preview asks about these instead of
	// suggesting them.
	Uncertain bool `json:"uncertain,omitempty"`
}

// Result is one recording's analysis.
type Result struct {
	Duration float64 `json:"duration"`
	Blocks   []Block `json:"blocks"`
	// Probes is how many keyframes were decoded for it.
	Probes int `json:"probes"`
	// WeakLogo: the channel's logo gave too small a mask (semi-transparent,
	// animated) — every block is uncertain.
	WeakLogo bool `json:"weakLogo,omitempty"`
	// Learned: the mask was learned from this recording, not reused.
	Learned bool `json:"learned,omitempty"`
	// Trimmed: the breaks were cut out of the recording (MediaHub
	// trimming); Duration is the new length.
	Trimmed bool `json:"trimmed,omitempty"`
}

// Options steer an analysis.
type Options struct {
	Sampling Sampling
	// LearnFrames is how many frames, spread over the recording, a mask is
	// learned from.
	LearnFrames int
	// Wait is called every few probes; a non-nil error stops the analysis
	// (the job's Wait: holds while a recording runs, ends when cancelled).
	Wait func(ctx context.Context) error
	// Progress reports decoded frames of an estimated total.
	Progress func(done, total int)
}

// How close to the threshold a score makes its sample "unclear".
const unclearMargin = 0.1

// Analyze finds the ad breaks of one recording. known is the channel's
// mask from earlier recordings, or nil; the returned mask is the one used
// (learned here when known was nil, weak, or no longer matched — a channel
// can change its logo).
func Analyze(ctx context.Context, src Source, known *Mask, o Options) (Result, Mask, error) {
	if o.Sampling.Step <= 0 {
		o.Sampling = DefaultSampling
	}
	if o.LearnFrames <= 0 {
		o.LearnFrames = 60
	}
	duration, err := src.Duration(ctx)
	if err != nil {
		return Result{}, Mask{}, err
	}
	a := &analysis{ctx: ctx, src: src, o: o, total: o.LearnFrames + int(duration/o.Sampling.Step*1.4)}

	if known != nil && !known.Weak() {
		res, err := a.scan(*known, duration)
		if err != nil {
			return Result{}, Mask{}, err
		}
		// A mask that matches less than half the recording is no longer
		// this channel's logo (or never was): learn again.
		if a.logoShare() >= 0.5 {
			return res, *known, nil
		}
	}
	m, err := a.learn(duration)
	if err != nil {
		return Result{}, Mask{}, err
	}
	res, err := a.scan(m, duration)
	res.Learned = true
	return res, m, err
}

type analysis struct {
	ctx    context.Context
	src    Source
	o      Options
	probes int
	total  int
	scores map[float64]float64
	fatal  error
}

// frame decodes one keyframe, waiting between frames when asked to.
func (a *analysis) frame(t float64) (Frame, error) {
	if a.probes%20 == 0 && a.o.Wait != nil {
		if err := a.o.Wait(a.ctx); err != nil {
			return Frame{}, err
		}
	}
	if err := a.ctx.Err(); err != nil {
		return Frame{}, err
	}
	a.probes++
	if a.o.Progress != nil {
		a.o.Progress(a.probes, max(a.total, a.probes))
	}
	return a.src.Frame(a.ctx, t)
}

func (a *analysis) learn(duration float64) (Mask, error) {
	var frames []Frame
	n := a.o.LearnFrames
	for i := 0; i < n; i++ {
		// Spread evenly, away from the very start and end (a timer's
		// margins often hold the neighbouring programmes).
		f, err := a.frame(duration * (float64(i) + 0.5) / float64(n))
		if errors.Is(err, ErrNoFrame) {
			continue
		}
		if err != nil {
			return Mask{}, err
		}
		frames = append(frames, f)
	}
	if len(frames) < n/2 {
		return Mask{}, ErrNoFrame
	}
	return Learn(frames), nil
}

func (a *analysis) scan(m Mask, duration float64) (Result, error) {
	a.scores = map[float64]float64{}
	s := a.o.Sampling
	probe := func(t float64) (bool, bool) {
		if a.fatal != nil {
			return false, false
		}
		f, err := a.frame(t)
		if err != nil {
			if !errors.Is(err, ErrNoFrame) {
				a.fatal = err
			}
			return false, false
		}
		score := m.Score(f.Pix)
		a.scores[t] = score
		return score >= s.Threshold, true
	}
	gaps := findGaps(probe, duration, s)
	if a.fatal != nil {
		return Result{}, a.fatal
	}
	if len(a.scores) == 0 {
		return Result{}, ErrNoFrame
	}
	res := Result{Duration: duration, Probes: a.probes, WeakLogo: m.Weak(), Blocks: []Block{}}
	for _, g := range gaps {
		res.Blocks = append(res.Blocks, Block{Span: g, Uncertain: m.Weak() || a.unclear(g, s)})
	}
	return res, nil
}

// unclear reports whether a sample inside the gap, or within one step of
// its edges, scored close to the threshold.
func (a *analysis) unclear(g Span, s Sampling) bool {
	for t, score := range a.scores {
		if t >= g.Start-s.Step && t <= g.End+s.Step && math.Abs(score-s.Threshold) < unclearMargin {
			return true
		}
	}
	return false
}

// logoShare is the share of samples of the last scan that saw the logo.
func (a *analysis) logoShare() float64 {
	if len(a.scores) == 0 {
		return 0
	}
	n := 0
	for _, score := range a.scores {
		if score >= a.o.Sampling.Threshold {
			n++
		}
	}
	return float64(n) / float64(len(a.scores))
}

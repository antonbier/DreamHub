package adscan

import "sort"

// Sampling are the detection's knobs (settings later, once tuned on real
// recordings — docs/ADSCAN_PLAN.md, "Open questions").
type Sampling struct {
	Step      float64 // coarse sampling interval, seconds
	Precision float64 // bisect until a boundary is known this closely
	MinBlock  float64 // shorter logo gaps are not ads (a weak frame, a fade)
	MinGap    float64 // shorter logo stretches between two gaps join them (an ident)
	Threshold float64 // a score below this means "no logo"
}

// DefaultSampling: the spike's values (spikes/adscan/README.md). MinBlock
// must stay above Step, or a gap between two "logo" samples goes unseen.
var DefaultSampling = Sampling{Step: 25, Precision: 1, MinBlock: 60, MinGap: 20, Threshold: 0.42}

// Span is a stretch of a recording, in seconds from its start.
type Span struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// probeFunc reports whether the logo is visible at t; ok false when no
// frame could be read there (that sample is skipped).
type probeFunc func(t float64) (logo, ok bool)

// findGaps samples every Step seconds and, wherever two neighbours
// disagree, bisects between them until the change is known to Precision.
// It returns the stretches without logo that last at least MinBlock, after
// joining those separated by less than MinGap of logo.
func findGaps(probe probeFunc, duration float64, s Sampling) []Span {
	type sample struct {
		t    float64
		logo bool
	}
	var samples []sample
	for t := 0.0; t < duration; t += s.Step {
		if logo, ok := probe(t); ok {
			samples = append(samples, sample{t, logo})
		}
	}
	if len(samples) == 0 {
		return nil
	}

	type edge struct {
		at   float64
		logo bool // the state from here on
	}
	var edges []edge
	for i := 1; i < len(samples); i++ {
		lo, hi := samples[i-1], samples[i]
		if lo.logo == hi.logo {
			continue
		}
		a, b := lo.t, hi.t
		for b-a > s.Precision {
			mid := (a + b) / 2
			logo, ok := probe(mid)
			if !ok {
				break // no frame here: keep the coarser boundary
			}
			if logo == lo.logo {
				a = mid
			} else {
				b = mid
			}
		}
		edges = append(edges, edge{b, hi.logo})
	}

	var gaps []Span
	open, start := !samples[0].logo, 0.0
	for _, e := range edges {
		if !e.logo && !open {
			open, start = true, e.at
		} else if e.logo && open {
			open = false
			gaps = append(gaps, Span{start, e.at})
		}
	}
	if open {
		gaps = append(gaps, Span{start, duration})
	}

	sort.Slice(gaps, func(i, j int) bool { return gaps[i].Start < gaps[j].Start })
	var joined []Span
	for _, g := range gaps {
		if n := len(joined); n > 0 && g.Start-joined[n-1].End < s.MinGap {
			joined[n-1].End = g.End
			continue
		}
		joined = append(joined, g)
	}
	var out []Span
	for _, g := range joined {
		if g.End-g.Start >= s.MinBlock {
			out = append(out, g)
		}
	}
	return out
}

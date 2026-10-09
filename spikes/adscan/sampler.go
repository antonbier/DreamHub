package main

import "sort"

// probeFunc reports whether the logo is visible at sec seconds into the
// recording. In the real thing it decodes one keyframe (a slice of the .ts
// found through .ts.ap) and scores it against the channel's mask.
type probeFunc func(sec float64) bool

type sampling struct {
	Step      float64 // coarse sampling interval, seconds
	Precision float64 // bisect until a boundary is known this closely
	MinBlock  float64 // shorter logo gaps are not ads (a weak frame, a fade)
	MinGap    float64 // shorter logo stretches between two gaps join them
}

type span struct{ Start, End float64 }

// findAds samples the logo every Step seconds and, wherever two neighbours
// disagree, bisects between them until the change is known to Precision.
// It returns the stretches without logo that last at least MinBlock, after
// joining those separated by less than MinGap of logo, and the number of
// probes it took.
func findAds(probe probeFunc, duration float64, s sampling) ([]span, int) {
	probes := 0
	cache := map[float64]bool{}
	look := func(t float64) bool {
		if v, ok := cache[t]; ok {
			return v
		}
		probes++
		v := probe(t)
		cache[t] = v
		return v
	}

	var ts []float64
	for t := 0.0; t < duration; t += s.Step {
		ts = append(ts, t)
	}
	if last := duration - 0.001; len(ts) == 0 || ts[len(ts)-1] < last {
		ts = append(ts, last)
	}

	// Boundaries: the moment the logo state flips, with the new state.
	type edge struct {
		at   float64
		logo bool
	}
	var edges []edge
	for i := 1; i < len(ts); i++ {
		lo, hi := ts[i-1], ts[i]
		a, b := look(lo), look(hi)
		if a == b {
			// Equal ends can still hide a short gap; MinBlock < Step would
			// need denser sampling, which the settings must not allow.
			continue
		}
		for hi-lo > s.Precision {
			mid := (lo + hi) / 2
			if look(mid) == a {
				lo = mid
			} else {
				hi = mid
			}
		}
		edges = append(edges, edge{hi, b})
	}

	// Stretches without logo, including one running from the start or to
	// the end of the recording.
	var gaps []span
	open, start := !look(ts[0]), 0.0
	for _, e := range edges {
		if !e.logo && !open {
			open, start = true, e.at
		} else if e.logo && open {
			open = false
			gaps = append(gaps, span{start, e.at})
		}
	}
	if open {
		gaps = append(gaps, span{start, duration})
	}

	sort.Slice(gaps, func(i, j int) bool { return gaps[i].Start < gaps[j].Start })
	var joined []span
	for _, g := range gaps {
		if n := len(joined); n > 0 && g.Start-joined[n-1].End < s.MinGap {
			joined[n-1].End = g.End
			continue
		}
		joined = append(joined, g)
	}
	var ads []span
	for _, g := range joined {
		if g.End-g.Start >= s.MinBlock {
			ads = append(ads, g)
		}
	}
	return ads, probes
}

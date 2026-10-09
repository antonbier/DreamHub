package adscan

import "math"

// Mask is a channel's logo: the pixels in one corner that are an edge with
// the same direction in most frames (spikes/adscan/README.md).
type Mask struct {
	Corner string `json:"corner"`
	X0     int    `json:"x0"`
	Y0     int    `json:"y0"`
	W      int    `json:"w"`
	H      int    `json:"h"`
	// Dirs holds, per pixel (W*H), the logo edge's direction 0–7, or -1.
	Dirs []int8 `json:"dirs"`
	// Count is the number of logo pixels; Frames how many frames it was
	// learned from.
	Count  int `json:"count"`
	Frames int `json:"frames"`
}

const (
	// A pixel counts as an edge above this Sobel magnitude (|gx|+|gy|).
	edgeThreshold = 48 * 4
	// A pixel belongs to the logo if it is an edge in the same direction
	// (one bin either way) in at least this share of the frames. Not
	// higher: commercial channels run up to 12 minutes of ads an hour plus
	// trailers without logo, so the logo is missing from a quarter of the
	// frames or more (spikes/adscan/README.md: 0.8 learned nothing then).
	stableShare = 0.6
	// MinMaskPixels: a smaller mask (a faint, semi-transparent logo) is
	// matched by chance too easily; its results are only "uncertain".
	MinMaskPixels = 60
)

type corner struct {
	name         string
	x0, y0, w, h int
}

// The four corner regions searched for a logo, a quarter of each side.
var corners = []corner{
	{"top-left", 0, 0, FrameW / 4, FrameH / 4},
	{"top-right", FrameW - FrameW/4, 0, FrameW / 4, FrameH / 4},
	{"bottom-left", 0, FrameH - FrameH/4, FrameW / 4, FrameH / 4},
	{"bottom-right", FrameW - FrameW/4, FrameH - FrameH/4, FrameW / 4, FrameH / 4},
}

// edges returns, per pixel, the Sobel edge direction quantized to eight
// bins — the polarity included, so a bright stroke on a dark background
// differs from a dark one — or -1 where there is no edge.
func edges(pix []byte) []int8 {
	e := make([]int8, FrameW*FrameH)
	for i := range e {
		e[i] = -1
	}
	at := func(x, y int) int { return int(pix[y*FrameW+x]) }
	for y := 1; y < FrameH-1; y++ {
		for x := 1; x < FrameW-1; x++ {
			gx := -at(x-1, y-1) - 2*at(x-1, y) - at(x-1, y+1) + at(x+1, y-1) + 2*at(x+1, y) + at(x+1, y+1)
			gy := -at(x-1, y-1) - 2*at(x, y-1) - at(x+1, y-1) + at(x-1, y+1) + 2*at(x, y+1) + at(x+1, y+1)
			if abs(gx)+abs(gy) <= edgeThreshold {
				continue
			}
			a := math.Atan2(float64(gy), float64(gx))
			e[y*FrameW+x] = int8(int(math.Round(a/(math.Pi/4))+8) % 8)
		}
	}
	return e
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func near(a, b int8) bool { return a == b || (a+1)%8 == b || (a+7)%8 == b }

// Learn finds the logo in frames spread over a recording: per corner, the
// pixels that are an edge with the same direction in most frames; the
// corner with the most of them wins. Ads are usually well under half the
// airtime, so the logo wins the vote; a temporary overlay ("New episode")
// over a whole recording doesn't.
func Learn(frames []Frame) Mask {
	hits := make([][8]int32, FrameW*FrameH)
	for _, f := range frames {
		for i, d := range edges(f.Pix) {
			if d >= 0 {
				hits[i][d]++
			}
		}
	}
	need := int32(stableShare * float64(len(frames)))
	var best Mask
	for _, c := range corners {
		m := Mask{Corner: c.name, X0: c.x0, Y0: c.y0, W: c.w, H: c.h, Dirs: make([]int8, c.w*c.h), Frames: len(frames)}
		for y := 0; y < c.h; y++ {
			for x := 0; x < c.w; x++ {
				m.Dirs[y*c.w+x] = -1
				h := hits[(c.y0+y)*FrameW+c.x0+x]
				for d, n := range h {
					// Neighbouring bins count too: a direction near a bin
					// boundary flips between the two with encoding noise.
					l, r := h[(d+7)%8], h[(d+1)%8]
					if n+l+r >= need && need > 0 && n >= l && n >= r {
						m.Dirs[y*c.w+x] = int8(d)
						m.Count++
						break
					}
				}
			}
		}
		if m.Count > best.Count {
			best = m
		}
	}
	if best.Dirs == nil {
		best.Frames = len(frames)
	}
	return best
}

// Weak reports whether the mask is too small to trust on its own.
func (m Mask) Weak() bool { return m.Count < MinMaskPixels }

// Score is the share of the mask's pixels where the frame has an edge in
// the same direction (one bin either way) at the same place: about 1 with
// the logo, low without — a busy background has edges everywhere, but
// rarely in the logo's directions at the logo's places.
func (m Mask) Score(pix []byte) float64 {
	if m.Count == 0 || len(pix) != FrameW*FrameH {
		return 0
	}
	e := edges(pix)
	found := 0
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			want := m.Dirs[y*m.W+x]
			if want < 0 {
				continue
			}
			if d := e[(m.Y0+y)*FrameW+m.X0+x]; d >= 0 && near(d, want) {
				found++
			}
		}
	}
	return float64(found) / float64(m.Count)
}

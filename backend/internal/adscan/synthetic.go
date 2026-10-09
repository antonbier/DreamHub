package adscan

import (
	"context"
	"hash/fnv"
	"math/rand/v2"
)

// Synthetic is mock mode's Opener: every recording becomes an hour with a
// logo in the top-left corner and three ad breaks (positions derived from
// the path, so each recording differs but stays the same between runs),
// over a busy background that changes with every frame. The mock box's
// recordings hold no real video.
type Synthetic struct{}

// Open implements Opener.
func (Synthetic) Open(path string) (Source, error) {
	h := fnv.New64a()
	h.Write([]byte(path))
	seed := h.Sum64()
	r := rand.New(rand.NewPCG(seed, 1))
	s := &synthetic{seed: seed, duration: 3600}
	// Three breaks of 3–7 minutes, one in each third of the hour.
	for i := 0; i < 3; i++ {
		start := float64(i)*1200 + 200 + r.Float64()*600
		s.ads = append(s.ads, Span{start, start + 180 + r.Float64()*240})
	}
	return s, nil
}

type synthetic struct {
	seed     uint64
	duration float64
	ads      []Span
}

// Ads are the breaks the synthetic recording really has (for tests).
func (s *synthetic) Ads() []Span { return s.ads }

func (s *synthetic) Duration(context.Context) (float64, error) { return s.duration, nil }
func (s *synthetic) Close() error                              { return nil }

func (s *synthetic) Frame(ctx context.Context, at float64) (Frame, error) {
	if at < 0 || at >= s.duration {
		return Frame{}, ErrNoFrame
	}
	// Keyframes every 0.5 s, like real recordings.
	at = float64(int(at*2)) / 2
	r := rand.New(rand.NewPCG(s.seed, uint64(at*2)))
	pix := make([]byte, FrameW*FrameH)
	// Background: a flat tone with random rectangles — edges everywhere,
	// in every direction, as in a real picture.
	base := byte(60 + r.IntN(120))
	for i := range pix {
		pix[i] = base
	}
	for n := 0; n < 40; n++ {
		x, y := r.IntN(FrameW), r.IntN(FrameH)
		w, h := 4+r.IntN(60), 4+r.IntN(40)
		fill(pix, x, y, w, h, byte(r.IntN(256)))
	}
	inAd := false
	for _, ad := range s.ads {
		if at >= ad.Start && at < ad.End {
			inAd = true
		}
	}
	if !inAd {
		// The logo: a light box with three dark letter-ish bars.
		fill(pix, 30, 16, 40, 14, 230)
		for i := 0; i < 3; i++ {
			fill(pix, 34+i*12, 19, 6, 8, 40)
		}
	}
	return Frame{At: at, Pix: pix}, nil
}

func fill(pix []byte, x, y, w, h int, v byte) {
	for yy := y; yy < y+h && yy < FrameH; yy++ {
		for xx := x; xx < x+w && xx < FrameW; xx++ {
			pix[yy*FrameW+xx] = v
		}
	}
}

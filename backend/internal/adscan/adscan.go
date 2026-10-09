// Package adscan finds ad breaks in recordings by the channel logo
// (docs/ADSCAN_PLAN.md, #44, phase 1): a channel's logo is learned from
// sparse keyframes of the recording itself, then the recording is sampled
// coarsely and bisected wherever the logo appears or disappears. Nothing is
// per country or channel: a French box learns its own channels' logos.
//
// It only finds; it changes nothing. Writing the result into the
// recording's cut list is a separate, confirmed step (sidecar.go).
//
// Frames come from a Source: FFmpeg on the box (ffmpeg.go), synthetic
// frames in mock mode (synthetic.go).
package adscan

import (
	"context"
	"errors"
)

// Every frame is scaled to one size before anything else, so masks are
// comparable between SD and HD and a frame stays small (83 KB).
const (
	FrameW = 384
	FrameH = 216
)

// Frame is one decoded keyframe, grayscale, FrameW×FrameH.
type Frame struct {
	// At is the frame's time in seconds from the recording's start.
	At  float64
	Pix []byte
}

// Source gives keyframes of one recording.
type Source interface {
	// Duration is the recording's length in seconds.
	Duration(ctx context.Context) (float64, error)
	// Frame decodes the keyframe at or just after at seconds.
	Frame(ctx context.Context, at float64) (Frame, error)
	Close() error
}

// Opener opens a recording (its .ts path) as a Source.
type Opener interface {
	Open(path string) (Source, error)
}

// ErrNoFrame: no decodable keyframe near the asked time (a damaged spot in
// the recording, or its very end).
var ErrNoFrame = errors.New("adscan: no keyframe there")

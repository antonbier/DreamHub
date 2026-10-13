// Package mock is a logosearch.Source with a few demo entries and a drawn
// picture for each, so the logo chooser has something to show in mock
// mode without the internet.
package mock

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"

	"github.com/antonbier/DreamHub/backend/internal/logosearch"
)

type Source struct{}

var _ logosearch.Source = Source{}

var demo = []logosearch.Entry{
	{Path: "countries/germany/rtl-de.png", Name: "rtl-de", Country: "germany"},
	{Path: "countries/austria/atv-at.png", Name: "atv-at", Country: "austria"},
	{Path: "countries/italy/rai-1-it.png", Name: "rai-1-it", Country: "italy"},
}

func (Source) Search(ctx context.Context, name string, limit int) ([]logosearch.Entry, error) {
	return logosearch.Rank(withNorm(demo), name, limit), nil
}

// withNorm lets Rank compute each entry's squashed name itself.
func withNorm(in []logosearch.Entry) []logosearch.Entry { return in }

// Fetch draws a plain tile whose colour depends on the path.
func (Source) Fetch(ctx context.Context, path string) ([]byte, error) {
	if !logosearch.ValidPath(path) {
		return nil, logosearch.ErrInvalidPath
	}
	var sum uint8
	for i := 0; i < len(path); i++ {
		sum += path[i]
	}
	img := image.NewRGBA(image.Rect(0, 0, 120, 72))
	c := color.RGBA{R: 40 + sum%160, G: 90 + sum%120, B: 160, A: 255}
	for y := 0; y < 72; y++ {
		for x := 0; x < 120; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

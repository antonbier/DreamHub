// Package mock implements fanart.Client without network access or a key.
package mock

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"strconv"

	"github.com/antonbier/DreamHub/backend/internal/fanart"
)

type Client struct{}

func New() *Client { return &Client{} }

var _ fanart.Client = (*Client)(nil)

// Logo has one for every title.
func (*Client) Logo(ctx context.Context, apiKey, kind string, id int64, langs []string) (string, error) {
	section, k := "movies", "hdmovielogo"
	if kind == "tv" {
		section, k = "tv", "hdtvlogo"
	}
	return "/fanart/" + section + "/" + strconv.FormatInt(id, 10) + "/" + k + "/mock-logo.png", nil
}

// Image draws a wide, mostly transparent bar, like a logo.
func (*Client) Image(ctx context.Context, path string) ([]byte, string, error) {
	if !fanart.ValidImage(path) {
		return nil, "", fanart.ErrInvalidImage
	}
	img := image.NewNRGBA(image.Rect(0, 0, 400, 155))
	for y := 55; y < 100; y++ {
		for x := 20; x < 380; x++ {
			img.Set(x, y, color.NRGBA{230, 230, 235, 255})
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes(), "image/png", nil
}

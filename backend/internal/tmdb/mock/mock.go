// Package mock implements tmdb.Client without network access or a key.
package mock

import (
	"bytes"
	"context"
	"hash/fnv"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/antonbier/DreamHub/backend/internal/tmdb"
)

// Client knows a handful of titles, matching the mock box's recordings,
// EPG and series, with posters, ratings and overviews; anything else comes
// back as a film of that name without details, so every flow has
// something to show.
type Client struct {
	// Searches counts Search calls, for tests on caching (atomic:
	// enrichment searches concurrently).
	Searches atomic.Int64
}

var _ tmdb.Client = (*Client)(nil)

func New() *Client { return &Client{} }

var known = []struct {
	match string
	res   tmdb.Result
}{
	{"bauer sucht frau", tmdb.Result{Kind: "tv", ID: 1001, Title: "Bauer sucht Frau International", Year: 2012,
		Overview: "Landwirte aus aller Welt suchen die große Liebe.", Rating: 5.1, Votes: 12, GenreIDs: []int{10764},
		PosterPath: "/mock-bauer.png", BackdropPath: "/mock-bauer-bg.png"}},
	{"tatort", tmdb.Result{Kind: "tv", ID: 1002, Title: "Tatort", Year: 1970,
		Overview: "Die langlebigste Krimireihe im deutschsprachigen Fernsehen: jede Folge ein Fall, ermittelt von wechselnden Teams.",
		Rating:   6.9, Votes: 310, GenreIDs: []int{80, 18}, PosterPath: "/mock-tatort.png", BackdropPath: "/mock-tatort-bg.png"}},
	{"the rookie", tmdb.Result{Kind: "tv", ID: 1003, Title: "The Rookie", OriginalTitle: "The Rookie", Year: 2018,
		Overview: "John Nolan fängt mit Mitte vierzig noch einmal als Polizeianfänger in Los Angeles an.",
		Rating:   8.5, Votes: 2400, GenreIDs: []int{80, 18, 35}, PosterPath: "/mock-rookie.png", BackdropPath: "/mock-rookie-bg.png"}},
	{"bones", tmdb.Result{Kind: "tv", ID: 1004, Title: "Bones - Die Knochenjägerin", OriginalTitle: "Bones", Year: 2005,
		Overview: "Die forensische Anthropologin Temperance Brennan hilft dem FBI, Morde anhand von Knochen aufzuklären.",
		Rating:   8.2, Votes: 3100, GenreIDs: []int{80, 18}, PosterPath: "/mock-bones.png"}},
}

func (c *Client) Search(ctx context.Context, apiKey, query, language string) ([]tmdb.Result, error) {
	c.Searches.Add(1)
	q := strings.ToLower(query)
	var out []tmdb.Result
	for _, k := range known {
		if strings.Contains(q, k.match) || strings.Contains(k.match, q) && len(q) > 3 {
			out = append(out, k.res)
		}
	}
	if strings.Contains(q, "tatort") {
		out = append(out, tmdb.Result{Kind: "movie", ID: 2001, Title: "Tatort: Borowski und das Haus am Meer", Year: 2026})
	}
	if len(out) > 0 {
		return out, nil
	}
	return []tmdb.Result{{Kind: "movie", ID: 9999, Title: query, Year: 2020}}, nil
}

func (*Client) EpisodeTitle(ctx context.Context, apiKey string, tvID int64, season, episode int, language string) (string, error) {
	return "Mock-Episode", nil
}

func (*Client) Details(ctx context.Context, apiKey, kind string, id int64, language string) (tmdb.Result, error) {
	for _, k := range known {
		if k.res.ID == id && k.res.Kind == kind {
			return k.res, nil
		}
	}
	return tmdb.Result{Kind: kind, ID: id, Title: "Mock", PosterPath: "/mock-" + strconv.FormatInt(id, 10) + ".png"}, nil
}

func (*Client) Genres(ctx context.Context, apiKey, language string) (map[int]string, error) {
	return map[int]string{18: "Drama", 35: "Komödie", 80: "Krimi", 10764: "Reality"}, nil
}

// Image draws a plain two-tone placeholder poster, colored by path, so
// the UI's image handling is exercised without the network.
func (*Client) Image(ctx context.Context, size, path string) ([]byte, string, error) {
	if !tmdb.ValidImage(size, path) {
		return nil, "", tmdb.ErrInvalidImage
	}
	h := fnv.New32a()
	h.Write([]byte(path))
	v := h.Sum32()
	base := color.RGBA{uint8(40 + v%120), uint8(40 + (v>>8)%120), uint8(60 + (v>>16)%120), 255}
	w, ht := 185, 278
	if strings.HasSuffix(path, "-bg.png") {
		w, ht = 780, 439
	}
	img := image.NewRGBA(image.Rect(0, 0, w, ht))
	draw.Draw(img, img.Bounds(), image.NewUniform(base), image.Point{}, draw.Src)
	dark := color.RGBA{base.R / 2, base.G / 2, base.B / 2, 255}
	draw.Draw(img, image.Rect(0, ht*2/3, w, ht), image.NewUniform(dark), image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "image/png", nil
}

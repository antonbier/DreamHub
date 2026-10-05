// Package mock implements plex.Client without network access: a small
// server with a film library and a series library, watched state that
// changes when set, generated pictures and a sign-in code that is
// "entered" on the second check.
package mock

import (
	"bytes"
	"context"
	"errors"
	"hash/fnv"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/plex"
)

// URL is the mock server's address; Token opens it.
const (
	URL   = "http://plex.mock:32400"
	Token = "mock-plex-token"
)

// Client is the mock server. Offline makes every server call fail as if
// the machine were switched off.
type Client struct {
	mu       sync.Mutex
	Offline  bool
	items    map[string]*plex.Item
	children map[string][]string
	pinCalls int
	// Watched records SetWatched calls, for tests.
	Watched []string
	// Unreadable makes Item answer with something that can't be read, as
	// a server with a newer answer format would — not offline.
	Unreadable bool
}

var _ plex.Client = (*Client)(nil)

// New returns the mock with its fixtures.
func New() *Client {
	c := &Client{items: map[string]*plex.Item{}, children: map[string][]string{}}
	movie := func(key, title string, year int, minutes int64, viewed int, offset int64, genres ...string) {
		c.items[key] = &plex.Item{RatingKey: key, Type: "movie", Title: title, Year: year, Duration: minutes * 60000,
			ViewCount: viewed, ViewOffset: offset, Genres: genres, Rating: 7.4, Thumb: "/library/metadata/" + key + "/thumb/1",
			Art: "/library/metadata/" + key + "/art/1", AddedAt: 1790000000 - int64(len(c.items))*86400,
			Summary: "A film from the mock Plex library.",
			Media: []plex.Media{{Container: "mkv", VideoCodec: "h264", AudioCodec: "ac3", AudioChannels: 6, VideoResolution: "1080", Bitrate: 8000,
				Parts: []plex.Part{{ID: 9000 + int64(len(c.items)), Key: "/library/parts/" + key + "/1/file.mkv", Size: 4_200_000_000, Container: "mkv"}}}}}
		c.children["lib:1"] = append(c.children["lib:1"], key)
	}
	movie("101", "Das Boot", 1981, 149, 1, 0, "Drama", "War")
	movie("102", "Lola rennt", 1998, 81, 0, 1_800_000, "Thriller")
	movie("103", "Good Bye, Lenin!", 2003, 121, 0, 0, "Comedy", "Drama")
	movie("104", "Der Himmel über Berlin", 1987, 128, 2, 0, "Drama", "Fantasy")
	movie("105", "Toni Erdmann", 2016, 162, 0, 0, "Comedy")
	movie("106", "Im Westen nichts Neues", 2022, 148, 0, 0, "War", "Drama")

	// One series: Dark, two seasons of three episodes.
	c.items["201"] = &plex.Item{RatingKey: "201", Type: "show", Title: "Dark", Year: 2017, Genres: []string{"Mystery", "Sci-Fi"},
		Rating: 8.7, Thumb: "/library/metadata/201/thumb/1", Art: "/library/metadata/201/art/1", ChildCount: 2, AddedAt: 1789000000,
		Summary: "A series from the mock Plex library."}
	c.children["lib:2"] = []string{"201"}
	for s := 1; s <= 2; s++ {
		sk := "21" + string(rune('0'+s))
		c.items[sk] = &plex.Item{RatingKey: sk, Type: "season", Title: "Staffel " + string(rune('0'+s)), ShowTitle: "Dark", ShowKey: "201", Index: s,
			Thumb: "/library/metadata/" + sk + "/thumb/1"}
		c.children["201"] = append(c.children["201"], sk)
		for e := 1; e <= 3; e++ {
			ek := sk + string(rune('0'+e))
			viewed := 0
			if s == 1 {
				viewed = 1
			}
			c.items[ek] = &plex.Item{RatingKey: ek, Type: "episode", Title: "Folge " + string(rune('0'+e)), ShowTitle: "Dark", ShowKey: "201", SeasonKey: sk,
				SeasonTitle: "Staffel " + string(rune('0'+s)), Index: e, ParentIndex: s, Duration: 52 * 60000, ViewCount: viewed,
				Thumb: "/library/metadata/" + ek + "/thumb/1",
				Media: []plex.Media{{Container: "mkv", VideoCodec: "hevc", AudioCodec: "eac3", AudioChannels: 6, VideoResolution: "4k", Bitrate: 15000,
					Parts: []plex.Part{{ID: 8000, Key: "/library/parts/" + ek + "/1/file.mkv", Size: 2_100_000_000, Container: "mkv"}}}}}
			c.children[sk] = append(c.children[sk], ek)
		}
	}
	c.recount()
	return c
}

// recount derives the series' and seasons' episode counts.
func (c *Client) recount() {
	for _, it := range c.items {
		if it.Type != "show" && it.Type != "season" {
			continue
		}
		it.LeafCount, it.ViewedLeafCount = 0, 0
		for _, ep := range c.leaves(it.RatingKey) {
			it.LeafCount++
			if ep.ViewCount > 0 {
				it.ViewedLeafCount++
			}
		}
	}
}

func (c *Client) leaves(key string) []*plex.Item {
	it := c.items[key]
	if it == nil {
		return nil
	}
	if it.Type == "movie" || it.Type == "episode" {
		return []*plex.Item{it}
	}
	var out []*plex.Item
	for _, k := range c.children[key] {
		out = append(out, c.leaves(k)...)
	}
	return out
}

func (c *Client) check(conn plex.Conn) error {
	if c.Offline || conn.URL != URL {
		return plex.ErrUnreachable
	}
	if conn.Token != Token {
		return plex.ErrUnauthorized
	}
	return nil
}

func (c *Client) Identity(_ context.Context, conn plex.Conn) (plex.Identity, error) {
	if c.Offline || conn.URL != URL {
		return plex.Identity{}, plex.ErrUnreachable
	}
	return plex.Identity{MachineIdentifier: "mock-machine", Version: "1.43.4"}, nil
}

func (c *Client) Server(_ context.Context, conn plex.Conn) (plex.Server, error) {
	if err := c.check(conn); err != nil {
		return plex.Server{}, err
	}
	return plex.Server{Name: "Unraid", Version: "1.43.4"}, nil
}

func (c *Client) Libraries(_ context.Context, conn plex.Conn) ([]plex.Library, error) {
	if err := c.check(conn); err != nil {
		return nil, err
	}
	return []plex.Library{{ID: "1", Title: "Filme", Type: "movie"}, {ID: "2", Title: "Serien", Type: "show"}, {ID: "3", Title: "Musik", Type: "artist"}}, nil
}

func (c *Client) get(keys []string) []plex.Item {
	out := make([]plex.Item, 0, len(keys))
	for _, k := range keys {
		out = append(out, *c.items[k])
	}
	return out
}

func (c *Client) Items(_ context.Context, conn plex.Conn, libraryID string) ([]plex.Item, error) {
	if err := c.check(conn); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.get(c.children["lib:"+libraryID]), nil
}

func (c *Client) Item(_ context.Context, conn plex.Conn, key string) (plex.Item, error) {
	if err := c.check(conn); err != nil {
		return plex.Item{}, err
	}
	if c.Unreadable {
		return plex.Item{}, errors.New("json: cannot unmarshal array into Go struct field .rating of type float64")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.items[key]
	if !ok {
		return plex.Item{}, plex.ErrNotFound
	}
	return *it, nil
}

func (c *Client) Children(_ context.Context, conn plex.Conn, key string) ([]plex.Item, error) {
	if err := c.check(conn); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; !ok {
		return nil, plex.ErrNotFound
	}
	return c.get(c.children[key]), nil
}

func (c *Client) OnDeck(_ context.Context, conn plex.Conn) ([]plex.Item, error) {
	if err := c.check(conn); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []plex.Item
	for _, it := range c.items {
		if it.Type == "movie" && it.ViewOffset > 0 && it.ViewCount == 0 {
			out = append(out, *it)
		}
	}
	// The next unseen episode of a started series.
	for _, ep := range c.leaves("201") {
		if ep.ViewCount == 0 {
			out = append(out, *ep)
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RatingKey < out[j].RatingKey })
	return out, nil
}

func (c *Client) SetWatched(_ context.Context, conn plex.Conn, key string, watched bool) error {
	if err := c.check(conn); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; !ok {
		return plex.ErrNotFound
	}
	for _, it := range c.leaves(key) {
		if watched {
			it.ViewCount, it.ViewOffset = it.ViewCount+1, 0
		} else {
			it.ViewCount, it.ViewOffset = 0, 0
		}
	}
	c.recount()
	state := "unwatched"
	if watched {
		state = "watched"
	}
	c.Watched = append(c.Watched, key+":"+state)
	return nil
}

// Image draws a plain rectangle in a colour derived from the path.
func (c *Client) Image(_ context.Context, conn plex.Conn, path string, width, height int) ([]byte, string, error) {
	if err := c.check(conn); err != nil {
		return nil, "", err
	}
	if !strings.HasPrefix(path, "/library/metadata/") {
		return nil, "", plex.ErrNotFound
	}
	h := fnv.New32a()
	h.Write([]byte(path))
	v := h.Sum32()
	img := image.NewRGBA(image.Rect(0, 0, max(width/4, 8), max(height/4, 8)))
	col := color.RGBA{uint8(60 + v%120), uint8(60 + (v>>8)%120), uint8(60 + (v>>16)%120), 255}
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			img.Set(x, y, col)
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, nil)
	return buf.Bytes(), "image/jpeg", nil
}

func (c *Client) Open(_ context.Context, conn plex.Conn, partKey, rangeHeader string) (*http.Response, error) {
	if err := c.check(conn); err != nil {
		return nil, err
	}
	body := "mock media " + partKey
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"video/x-matroska"}},
		Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}, nil
}

func (c *Client) CreatePin(context.Context, string) (plex.Pin, error) {
	c.mu.Lock()
	c.pinCalls = 0
	c.mu.Unlock()
	return plex.Pin{ID: 4711, Code: "DH42", ExpiresAt: time.Now().Add(15 * time.Minute)}, nil
}

// CheckPin reports the code as entered from the second check on.
func (c *Client) CheckPin(_ context.Context, _ string, id int64) (string, error) {
	if id != 4711 {
		return "", plex.ErrPinExpired
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pinCalls++
	if c.pinCalls < 2 {
		return "", nil
	}
	return "mock-account-token", nil
}

func (c *Client) Resources(_ context.Context, _ string, token string) ([]plex.Resource, error) {
	if token != "mock-account-token" {
		return nil, plex.ErrUnauthorized
	}
	return []plex.Resource{{Name: "Unraid", ClientIdentifier: "mock-machine", Owned: true, AccessToken: Token,
		Connections: []plex.Connection{{URI: URL, Local: true}, {URI: "https://plex.example.org", Local: false}}}}, nil
}

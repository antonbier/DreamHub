// Package plex is the Plex connector's client: a Plex Media Server's
// libraries, watched state and media files, plus plex.tv's PIN sign-in
// (the "enter this code at plex.tv/link" flow TV apps use) and its list of
// the account's servers.
//
// The server's address is the user's (Settings → Services → Plex): any
// Plex server, reachable by the box. Plex is optional and often not always
// on (CLAUDE.md), so callers (internal/hubs/plex.go) cache what they read
// and degrade to the cache when the server doesn't answer.
package plex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Conn is one server to talk to: its base URL (no trailing slash) and the
// token that opens it.
type Conn struct {
	URL   string
	Token string
}

// Identity is a server's /identity answer (no token needed).
type Identity struct {
	MachineIdentifier string `json:"machineIdentifier"`
	Version           string `json:"version"`
}

// Server is the server's own description (GET / with a token).
type Server struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Library is one library section.
type Library struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Type is Plex's: "movie", "show", "artist", "photo". DreamHub shows
	// films and series only.
	Type string `json:"type"`
}

// Part is one file of a media version.
type Part struct {
	ID        int64  `json:"id"`
	Key       string `json:"key"` // /library/parts/<id>/<ts>/file.mkv — the file itself, Range-capable
	File      string `json:"file,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Container string `json:"container,omitempty"`
}

// Media is one version of an item (a film can have several files).
type Media struct {
	Container       string `json:"container,omitempty"`
	VideoCodec      string `json:"videoCodec,omitempty"`
	AudioCodec      string `json:"audioCodec,omitempty"`
	AudioChannels   int    `json:"audioChannels,omitempty"`
	VideoResolution string `json:"videoResolution,omitempty"`
	Bitrate         int    `json:"bitrate,omitempty"` // kbit/s
	Parts           []Part `json:"parts,omitempty"`
}

// Item is a film, series, season or episode.
type Item struct {
	RatingKey string `json:"ratingKey"`
	Type      string `json:"type"` // movie, show, season, episode
	Title     string `json:"title"`
	// Series and season of an episode; series of a season. ShowKey and
	// SeasonKey are their rating keys.
	ShowKey     string `json:"showKey,omitempty"`
	SeasonKey   string `json:"seasonKey,omitempty"`
	ShowTitle   string `json:"showTitle,omitempty"`
	SeasonTitle string `json:"seasonTitle,omitempty"`
	// Index: an episode's number or a season's; ParentIndex: an episode's
	// season.
	Index       int    `json:"index,omitempty"`
	ParentIndex int    `json:"parentIndex,omitempty"`
	Year        int    `json:"year,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Thumb       string `json:"thumb,omitempty"` // server path, served through DreamHub
	Art         string `json:"art,omitempty"`
	// Logo is the title's transparent logo (Plex's "clearLogo"), when the
	// server has one.
	Logo          string   `json:"logo,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	Rating        float64  `json:"rating,omitempty"` // audience rating 0-10, when the agent has one
	ContentRating string   `json:"contentRating,omitempty"`
	Duration      int64    `json:"duration,omitempty"` // ms
	// Watched state: a film/episode is watched with ViewCount > 0 and in
	// progress with ViewOffset > 0; a series/season counts its episodes.
	ViewCount       int     `json:"viewCount,omitempty"`
	ViewOffset      int64   `json:"viewOffset,omitempty"` // ms
	LastViewedAt    int64   `json:"lastViewedAt,omitempty"`
	LeafCount       int     `json:"leafCount,omitempty"`
	ViewedLeafCount int     `json:"viewedLeafCount,omitempty"`
	ChildCount      int     `json:"childCount,omitempty"`
	AddedAt         int64   `json:"addedAt,omitempty"` // Unix seconds
	Aired           string  `json:"aired,omitempty"`   // YYYY-MM-DD
	Media           []Media `json:"media,omitempty"`
}

// Watched reports whether the item counts as seen: a film or episode
// played to the end at least once, a series or season with every episode
// seen.
func (it Item) Watched() bool {
	switch it.Type {
	case "show", "season":
		return it.LeafCount > 0 && it.ViewedLeafCount >= it.LeafCount
	default:
		return it.ViewCount > 0
	}
}

// Pin is a plex.tv sign-in code: the user enters Code at plex.tv/link,
// then CheckPin returns the account's token.
type Pin struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Resource is one of the account's servers, from plex.tv.
type Resource struct {
	Name             string       `json:"name"`
	ClientIdentifier string       `json:"clientIdentifier"`
	Owned            bool         `json:"owned"`
	AccessToken      string       `json:"-"`
	Connections      []Connection `json:"connections"`
}

// Connection is one address a server can be reached by.
type Connection struct {
	URI   string `json:"uri"`
	Local bool   `json:"local"`
	Relay bool   `json:"relay"`
}

// Client is implemented by HTTPClient and ./mock.
type Client interface {
	Identity(ctx context.Context, c Conn) (Identity, error)
	Server(ctx context.Context, c Conn) (Server, error)
	Libraries(ctx context.Context, c Conn) ([]Library, error)
	// Items lists a library's films or series (not seasons/episodes).
	Items(ctx context.Context, c Conn, libraryID string) ([]Item, error)
	// Item is one item with its media versions.
	Item(ctx context.Context, c Conn, ratingKey string) (Item, error)
	// Children are a series' seasons or a season's episodes.
	Children(ctx context.Context, c Conn, ratingKey string) ([]Item, error)
	// OnDeck is Plex's "continue watching": started films and the next
	// episode of series in progress.
	OnDeck(ctx context.Context, c Conn) ([]Item, error)
	// SetWatched marks an item (a whole series or season too) as seen or
	// not seen.
	SetWatched(ctx context.Context, c Conn, ratingKey string, watched bool) error
	// Image is a picture scaled by the server (thumb/art paths), JPEG.
	Image(ctx context.Context, c Conn, path string, width, height int) ([]byte, string, error)
	// Open requests a file (a Part's Key) for streaming, forwarding the
	// Range header; the caller closes the body.
	Open(ctx context.Context, c Conn, partKey, rangeHeader string) (*http.Response, error)

	CreatePin(ctx context.Context, clientID string) (Pin, error)
	// CheckPin returns the account token once the code was entered, "" before.
	CheckPin(ctx context.Context, clientID string, id int64) (string, error)
	Resources(ctx context.Context, clientID, token string) ([]Resource, error)
}

// Errors.
var (
	ErrUnauthorized = errors.New("plex: token rejected")
	ErrNotFound     = errors.New("plex: not found")
	ErrUnreachable  = errors.New("plex: server not reachable")
	ErrPinExpired   = errors.New("plex: sign-in code expired")
)

// Product is how DreamHub introduces itself to Plex (shown under the
// account's authorized devices).
const Product = "DreamHub"

// HTTPClient talks to a Plex server and plex.tv.
type HTTPClient struct {
	HTTP *http.Client
	// Stream has no overall timeout: a film streams for hours.
	Stream   *http.Client
	PlexTV   string // https://plex.tv, overridable in tests
	ClientID string // X-Plex-Client-Identifier for server requests
	Version  string
}

// NewHTTPClient returns a client with sensible timeouts. A Plex server on
// the home network answers well within them; one that's switched off
// fails fast so the cache takes over.
func NewHTTPClient(version string) *HTTPClient {
	return &HTTPClient{
		HTTP:    &http.Client{Timeout: 15 * time.Second},
		Stream:  &http.Client{},
		PlexTV:  "https://plex.tv",
		Version: version,
	}
}

func (h *HTTPClient) headers(req *http.Request, clientID, token string) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Product", Product)
	req.Header.Set("X-Plex-Version", h.Version)
	req.Header.Set("X-Plex-Platform", "Enigma2")
	req.Header.Set("X-Plex-Device-Name", "DreamHub")
	if clientID != "" {
		req.Header.Set("X-Plex-Client-Identifier", clientID)
	}
	if token != "" {
		req.Header.Set("X-Plex-Token", token)
	}
}

func (h *HTTPClient) do(ctx context.Context, method, rawURL, clientID, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return err
	}
	h.headers(req, clientID, token)
	resp, err := h.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode >= 300:
		return fmt.Errorf("plex: %s answered %d", req.URL.Path, resp.StatusCode)
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(out)
}

func (h *HTTPClient) server(ctx context.Context, c Conn, path string, out any) error {
	return h.do(ctx, http.MethodGet, strings.TrimRight(c.URL, "/")+path, h.ClientID, c.Token, out)
}

func (h *HTTPClient) Identity(ctx context.Context, c Conn) (Identity, error) {
	var res struct {
		MediaContainer Identity `json:"MediaContainer"`
	}
	err := h.server(ctx, Conn{URL: c.URL}, "/identity", &res)
	return res.MediaContainer, err
}

func (h *HTTPClient) Server(ctx context.Context, c Conn) (Server, error) {
	var res struct {
		MediaContainer struct {
			FriendlyName string `json:"friendlyName"`
			Version      string `json:"version"`
		} `json:"MediaContainer"`
	}
	err := h.server(ctx, c, "/", &res)
	return Server{Name: res.MediaContainer.FriendlyName, Version: res.MediaContainer.Version}, err
}

func (h *HTTPClient) Libraries(ctx context.Context, c Conn) ([]Library, error) {
	var res struct {
		MediaContainer struct {
			Directory []struct {
				Key   string `json:"key"`
				Title string `json:"title"`
				Type  string `json:"type"`
			} `json:"Directory"`
		} `json:"MediaContainer"`
	}
	if err := h.server(ctx, c, "/library/sections", &res); err != nil {
		return nil, err
	}
	out := make([]Library, 0, len(res.MediaContainer.Directory))
	for _, d := range res.MediaContainer.Directory {
		out = append(out, Library{ID: d.Key, Title: d.Title, Type: d.Type})
	}
	return out, nil
}

// metadata is Plex's JSON for one item.
type metadata struct {
	RatingKey             string `json:"ratingKey"`
	ParentRatingKey       string `json:"parentRatingKey"`
	GrandparentRatingKey  string `json:"grandparentRatingKey"`
	Type                  string `json:"type"`
	Title                 string `json:"title"`
	ParentTitle           string `json:"parentTitle"`
	GrandparentTitle      string `json:"grandparentTitle"`
	Index                 int    `json:"index"`
	ParentIndex           int    `json:"parentIndex"`
	Year                  int    `json:"year"`
	Summary               string `json:"summary"`
	Thumb                 string `json:"thumb"`
	ParentThumb           string `json:"parentThumb"`
	GrandparentThumb      string `json:"grandparentThumb"`
	Art                   string `json:"art"`
	AudienceRating        number `json:"audienceRating"`
	Rating                number `json:"rating"`
	ContentRating         string `json:"contentRating"`
	Duration              int64  `json:"duration"`
	ViewCount             int    `json:"viewCount"`
	ViewOffset            int64  `json:"viewOffset"`
	LastViewedAt          int64  `json:"lastViewedAt"`
	LeafCount             int    `json:"leafCount"`
	ViewedLeafCount       int    `json:"viewedLeafCount"`
	ChildCount            int    `json:"childCount"`
	AddedAt               int64  `json:"addedAt"`
	OriginallyAvailableAt string `json:"originallyAvailableAt"`
	Genre                 []struct {
		Tag string `json:"tag"`
	} `json:"Genre"`
	Image []struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"Image"`
	Media []struct {
		Container       string `json:"container"`
		VideoCodec      string `json:"videoCodec"`
		AudioCodec      string `json:"audioCodec"`
		AudioChannels   int    `json:"audioChannels"`
		VideoResolution string `json:"videoResolution"`
		Bitrate         int    `json:"bitrate"`
		Part            []Part `json:"Part"`
	} `json:"Media"`
}

// number is a JSON number that ignores anything else under its key.
// Needed because encoding/json matches keys without regard to case: newer
// servers send "rating" (a number) and "Rating" (a list of rating sources
// — IMDb, Rotten Tomatoes …) side by side, and the list would otherwise
// fail the whole answer.
type number float64

func (n *number) UnmarshalJSON(b []byte) error {
	var f float64
	if json.Unmarshal(b, &f) == nil {
		*n = number(f)
	}
	return nil
}

func (m metadata) item() Item {
	it := Item{
		RatingKey: m.RatingKey, Type: m.Type, Title: m.Title, Index: m.Index, ParentIndex: m.ParentIndex,
		Year: m.Year, Summary: m.Summary, Thumb: m.Thumb, Art: m.Art, ContentRating: m.ContentRating,
		Duration: m.Duration, ViewCount: m.ViewCount, ViewOffset: m.ViewOffset, LastViewedAt: m.LastViewedAt,
		LeafCount: m.LeafCount, ViewedLeafCount: m.ViewedLeafCount, ChildCount: m.ChildCount,
		AddedAt: m.AddedAt, Aired: m.OriginallyAvailableAt,
	}
	it.Rating = float64(m.AudienceRating)
	if it.Rating == 0 {
		it.Rating = float64(m.Rating)
	}
	switch m.Type {
	case "episode":
		it.ShowTitle, it.SeasonTitle = m.GrandparentTitle, m.ParentTitle
		it.ShowKey, it.SeasonKey = m.GrandparentRatingKey, m.ParentRatingKey
		// An episode's own thumb is a still; the series poster is what a
		// list shows next to "continue watching".
		if it.Thumb == "" {
			it.Thumb = m.GrandparentThumb
		}
	case "season":
		it.ShowTitle, it.ShowKey = m.ParentTitle, m.ParentRatingKey
		if it.Thumb == "" {
			it.Thumb = m.ParentThumb
		}
	}
	for _, img := range m.Image {
		if img.Type == "clearLogo" {
			it.Logo = img.URL
		}
	}
	for _, g := range m.Genre {
		it.Genres = append(it.Genres, g.Tag)
	}
	for _, md := range m.Media {
		it.Media = append(it.Media, Media{Container: md.Container, VideoCodec: md.VideoCodec, AudioCodec: md.AudioCodec,
			AudioChannels: md.AudioChannels, VideoResolution: md.VideoResolution, Bitrate: md.Bitrate, Parts: md.Part})
	}
	return it
}

func (h *HTTPClient) list(ctx context.Context, c Conn, path string) ([]Item, error) {
	var res struct {
		MediaContainer struct {
			Metadata []metadata `json:"Metadata"`
		} `json:"MediaContainer"`
	}
	if err := h.server(ctx, c, path, &res); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(res.MediaContainer.Metadata))
	for _, m := range res.MediaContainer.Metadata {
		out = append(out, m.item())
	}
	return out, nil
}

func (h *HTTPClient) Items(ctx context.Context, c Conn, libraryID string) ([]Item, error) {
	return h.list(ctx, c, "/library/sections/"+url.PathEscape(libraryID)+"/all")
}

func (h *HTTPClient) Item(ctx context.Context, c Conn, ratingKey string) (Item, error) {
	items, err := h.list(ctx, c, "/library/metadata/"+url.PathEscape(ratingKey))
	if err != nil {
		return Item{}, err
	}
	if len(items) == 0 {
		return Item{}, ErrNotFound
	}
	return items[0], nil
}

func (h *HTTPClient) Children(ctx context.Context, c Conn, ratingKey string) ([]Item, error) {
	return h.list(ctx, c, "/library/metadata/"+url.PathEscape(ratingKey)+"/children")
}

func (h *HTTPClient) OnDeck(ctx context.Context, c Conn) ([]Item, error) {
	return h.list(ctx, c, "/library/onDeck")
}

func (h *HTTPClient) SetWatched(ctx context.Context, c Conn, ratingKey string, watched bool) error {
	op := "/:/unscrobble"
	if watched {
		op = "/:/scrobble"
	}
	q := url.Values{"identifier": {"com.plexapp.plugins.library"}, "key": {ratingKey}}
	return h.server(ctx, c, op+"?"+q.Encode(), nil)
}

func (h *HTTPClient) Image(ctx context.Context, c Conn, path string, width, height int) ([]byte, string, error) {
	q := url.Values{
		"url": {path}, "width": {strconv.Itoa(width)}, "height": {strconv.Itoa(height)},
		"minSize": {"1"}, "upscale": {"1"}, "format": {"jpeg"},
	}
	if strings.Contains(path, "/clearLogo") {
		q.Set("format", "png") // keeps the transparency
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.URL, "/")+"/photo/:/transcode?"+q.Encode(), nil)
	if err != nil {
		return nil, "", err
	}
	h.headers(req, h.ClientID, c.Token)
	req.Header.Set("Accept", "image/jpeg,image/*")
	resp, err := h.HTTP.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, "", ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", ErrNotFound
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return b, resp.Header.Get("Content-Type"), err
}

func (h *HTTPClient) Open(ctx context.Context, c Conn, partKey, rangeHeader string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.URL, "/")+partKey, nil)
	if err != nil {
		return nil, err
	}
	h.headers(req, h.ClientID, c.Token)
	req.Header.Del("Accept")
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	resp, err := h.Stream.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		return nil, ErrUnauthorized
	}
	return resp, nil
}

func (h *HTTPClient) CreatePin(ctx context.Context, clientID string) (Pin, error) {
	var res struct {
		ID        int64     `json:"id"`
		Code      string    `json:"code"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	// strong=false: a short code the user types at plex.tv/link.
	if err := h.do(ctx, http.MethodPost, h.PlexTV+"/api/v2/pins?strong=false", clientID, "", &res); err != nil {
		return Pin{}, err
	}
	return Pin{ID: res.ID, Code: res.Code, ExpiresAt: res.ExpiresAt}, nil
}

func (h *HTTPClient) CheckPin(ctx context.Context, clientID string, id int64) (string, error) {
	var res struct {
		AuthToken *string   `json:"authToken"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	err := h.do(ctx, http.MethodGet, h.PlexTV+"/api/v2/pins/"+strconv.FormatInt(id, 10), clientID, "", &res)
	if errors.Is(err, ErrNotFound) {
		return "", ErrPinExpired
	}
	if err != nil {
		return "", err
	}
	if res.AuthToken == nil {
		return "", nil
	}
	return *res.AuthToken, nil
}

func (h *HTTPClient) Resources(ctx context.Context, clientID, token string) ([]Resource, error) {
	var res []struct {
		Name             string       `json:"name"`
		Provides         string       `json:"provides"`
		ClientIdentifier string       `json:"clientIdentifier"`
		Owned            bool         `json:"owned"`
		AccessToken      string       `json:"accessToken"`
		Connections      []Connection `json:"connections"`
	}
	if err := h.do(ctx, http.MethodGet, h.PlexTV+"/api/v2/resources?includeHttps=1&includeRelay=0", clientID, token, &res); err != nil {
		return nil, err
	}
	var out []Resource
	for _, r := range res {
		if !strings.Contains(r.Provides, "server") {
			continue
		}
		out = append(out, Resource{Name: r.Name, ClientIdentifier: r.ClientIdentifier, Owned: r.Owned, AccessToken: r.AccessToken, Connections: r.Connections})
	}
	return out, nil
}

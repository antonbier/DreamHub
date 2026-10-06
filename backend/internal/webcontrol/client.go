// Package webcontrol talks to a Dreambox's WebControl `/web/*` API — the
// confirmed OpenWebif-compatible XML API documented in ARCHITECTURE.md.
// DreamHub calls this instead of re-parsing recordings/EPG files directly,
// per CLAUDE.md's "don't re-implement what WebControl already exposes".
package webcontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DeviceInfo mirrors the confirmed /web/deviceinfo response shape (see
// ARCHITECTURE.md). Only the fields DreamHub actually uses are mapped;
// encoding/xml ignores anything else in the response.
type DeviceInfo struct {
	XMLName       xml.Name   `xml:"e2deviceinfo" json:"-"`
	EnigmaVersion string     `xml:"e2enigmaversion" json:"enigmaVersion"`
	ImageVersion  string     `xml:"e2imageversion" json:"imageVersion"`
	WebifVersion  string     `xml:"e2webifversion" json:"webifVersion"`
	DeviceName    string     `xml:"e2devicename" json:"deviceName"`
	Frontends     []Frontend `xml:"e2frontends>e2frontend" json:"frontends"`
	HDDs          []HDD      `xml:"e2hdds>e2hdd" json:"hdds"`
}

// Frontend is one tuner.
type Frontend struct {
	Name  string `xml:"e2name" json:"name"`
	Model string `xml:"e2model" json:"model"`
}

// HDD is one storage device from /web/deviceinfo. Capacity and Free are
// human-formatted strings as WebControl reports them (e.g. "2 TB",
// "263.947 GB") — there's no machine-readable byte count in this endpoint
// (confirmed live, ARCHITECTURE.md). See ParseSize for turning these into a
// usage percentage.
type HDD struct {
	Model    string `xml:"e2model" json:"model"`
	Capacity string `xml:"e2capacity" json:"capacity"`
	Free     string `xml:"e2free" json:"free"`
}

// Tuners returns the number of tuners, at least 1.
func (d *DeviceInfo) Tuners() int {
	if d == nil || len(d.Frontends) == 0 {
		return 1
	}
	return len(d.Frontends)
}

// Movie mirrors one <e2movie> entry from /web/movielist (see
// ARCHITECTURE.md for the confirmed live schema).
type Movie struct {
	ServiceReference string `xml:"e2servicereference" json:"serviceReference"`
	Title            string `xml:"e2title" json:"title"`
	Description      string `xml:"e2description" json:"description"`
	DescriptionExt   string `xml:"e2descriptionextended" json:"descriptionExtended"`
	ServiceName      string `xml:"e2servicename" json:"serviceName"`
	Time             Int64  `xml:"e2time" json:"time"`
	Length           string `xml:"e2length" json:"length"`
	Filename         string `xml:"e2filename" json:"filename"`
	FileSize         Int64  `xml:"e2filesize" json:"fileSize"`
}

// Int64 is a number field WebControl sometimes fills with Python's "None"
// — confirmed live 2026-09-24: /web/movielist reports e2filesize "None" for
// a manually copied .mkv. A strict int64 made encoding/xml fail on that
// entry, and the listing silently lost it and everything after it,
// including the folder's subfolders. Anything that isn't a number reads
// as 0.
type Int64 int64

func (n *Int64) UnmarshalText(b []byte) error {
	v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		v = 0
	}
	*n = Int64(v)
	return nil
}

// Service is one bouquet or channel entry from /web/getservices (see
// ARCHITECTURE.md). With no query string it lists bouquets; with
// ?sRef=<bouquet reference> it lists the channels inside that bouquet —
// confirmed live 2026-09-24, same schema either way.
type Service struct {
	ServiceReference string `xml:"e2servicereference" json:"serviceReference"`
	ServiceName      string `xml:"e2servicename" json:"serviceName"`
}

type serviceList struct {
	XMLName  xml.Name  `xml:"e2servicelist"`
	Services []Service `xml:"e2service"`
}

// EPGEvent is one <e2event> from /web/epgmulti (see ARCHITECTURE.md: the
// endpoint takes bRef, a bouquet reference, plus a time/endTime window in
// Unix seconds, and returns every event starting in that window for every
// channel in the bouquet, in chronological order per channel — confirmed
// live 2026-09-24).
type EPGEvent struct {
	ID               int64  `xml:"e2eventid" json:"id"`
	Start            int64  `xml:"e2eventstart" json:"start"` // Unix seconds, UTC
	Duration         int64  `xml:"e2eventduration" json:"duration"`
	Title            string `xml:"e2eventtitle" json:"title"`
	Description      string `xml:"e2eventdescription" json:"description"`
	ServiceReference string `xml:"e2eventservicereference" json:"serviceReference"`
	ServiceName      string `xml:"e2eventservicename" json:"serviceName"`
}

// EPGSearch finds EPG events whose title contains query, via
// /web/epgsearch — the box's own title search across every service it has
// EPG for (past events included; no result cap — confirmed live 2026-09-28:
// a one-letter query answers 41 MB in ~11 s and holds up enigma2 meanwhile,
// so callers must not send short queries, see MinEPGSearchQuery). Events of services
// the box can't name (reference "-1:…", name "<n/a>") are dropped: they
// can't be recorded or shown meaningfully.
// MinEPGSearchQuery is the shortest query worth sending to /web/epgsearch:
// three letters answered in ~0.2 s on the reference box, two in ~2 s, one
// in ~11 s.
const MinEPGSearchQuery = 3

func (c *HTTPClient) EPGSearch(ctx context.Context, query string) ([]EPGEvent, error) {
	body, err := c.do(ctx, http.MethodPost, "/web/epgsearch", url.Values{"search": {query}})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var list epgEventList
	if err := decodeXML(body, "/web/epgsearch", &list); err != nil {
		return nil, err
	}
	out := list.Events[:0]
	for _, e := range list.Events {
		if !strings.HasPrefix(e.ServiceReference, "-1:") {
			out = append(out, e)
		}
	}
	return out, nil
}

type epgEventList struct {
	XMLName xml.Name   `xml:"e2eventlist"`
	Events  []EPGEvent `xml:"e2event"`
}

// Timer mirrors one <e2timer> of /web/timerlist. Field names confirmed
// against a real live response 2026-09-24 (issue #14); see
// backend/testdata/fixtures/timerlist.xml.
type Timer struct {
	ServiceReference string `xml:"e2servicereference" json:"serviceReference"`
	ServiceName      string `xml:"e2servicename" json:"serviceName"`
	Name             string `xml:"e2name" json:"name"`
	Description      string `xml:"e2description" json:"description"`
	Begin            int64  `xml:"e2timebegin" json:"begin"` // Unix seconds, incl. margin
	End              int64  `xml:"e2timeend" json:"end"`     // Unix seconds, incl. margin
	Disabled         bool   `xml:"e2disabled" json:"disabled"`
	// JustPlay marks a zap timer: it switches channel, records nothing.
	JustPlay bool `xml:"e2justplay" json:"justPlay"`
	// EventID links the timer to its EPG event (0 if none).
	EventID int64 `xml:"e2eit" json:"eventId"`
	// Tags are space-separated; SerienRecorder tags its own timers
	// "SerienRecorder" (confirmed in the live fixture).
	Tags string `xml:"e2tags" json:"tags"`
	// Repeated is a weekday bitmask (bit 0 = Monday); 0 for one-off timers.
	Repeated int `xml:"e2repeated" json:"repeated"`
	// AfterEvent ("0" nothing, "1" standby, "2" deep standby, "3" auto)
	// and Location (the folder it records into) are kept as the box
	// reports them, so a timer can be put back exactly (disable/enable).
	AfterEvent string `xml:"e2afterevent" json:"afterEvent,omitempty"`
	Location   string `xml:"e2location" json:"location,omitempty"`
	// Filename is the file a running timer records into (empty before it
	// starts) — how MediaHub knows a recording is still being written.
	Filename string `xml:"e2filename" json:"filename"`
	// State is Enigma2's timer state (timer.py, confirmed on the device):
	// 0 waiting, 1 prepared (about to start), 2 running, 3 ended. A
	// disabled timer is reported as 3.
	State int `xml:"e2state" json:"state"`
}

// Timer states, see Timer.State.
const (
	TimerWaiting  = 0
	TimerPrepared = 1
	TimerRunning  = 2
	TimerEnded    = 3
)

// Active reports whether the timer is about to record or recording right
// now (ARCHITECTURE.md, "Timer states").
func (t Timer) Active() bool {
	return !t.Disabled && (t.State == TimerPrepared || t.State == TimerRunning)
}

// TimerRef identifies an existing timer the way /web/timerdelete matches
// it: the exact service reference string plus begin and end as listed by
// /web/timerlist (ARCHITECTURE.md, "Timer write endpoints").
type TimerRef struct {
	ServiceReference string
	Begin            int64
	End              int64
}

type timerList struct {
	XMLName xml.Name `xml:"e2timerlist"`
	Timers  []Timer  `xml:"e2timer"`
}

// NewTimer is a timer to create. Begin and End already include margins.
type NewTimer struct {
	ServiceReference string
	Name             string
	Description      string
	Begin            int64
	End              int64
	// EventID is the EPG event the timer records, if known (0 otherwise). It
	// lets the box link the timer to its EPG entry.
	EventID int64
	// The rest only matters when putting an existing timer back (TimerHub's
	// disable/enable): zero values give a new timer's defaults.
	Disabled   bool
	JustPlay   bool
	AfterEvent string // "" = "3" (auto)
	Dirname    string // "" = the box's preferred timer path
	Tags       string
	Repeated   int
}

// AsNewTimer is t as /web/timeradd needs it to recreate it unchanged.
func (t Timer) AsNewTimer() NewTimer {
	loc := t.Location
	if loc == "None" {
		loc = ""
	}
	return NewTimer{
		ServiceReference: t.ServiceReference, Name: t.Name, Description: t.Description, Begin: t.Begin, End: t.End,
		EventID: t.EventID, Disabled: t.Disabled, JustPlay: t.JustPlay, AfterEvent: t.AfterEvent, Dirname: loc,
		Tags: t.Tags, Repeated: t.Repeated,
	}
}

// Client is anything that can answer WebControl API calls — implemented by
// both the real HTTP client below and the mock in ./mock, per
// PRINCIPLES.md's "Mock-first development".
type Client interface {
	DeviceInfo(ctx context.Context) (*DeviceInfo, error)
	MovieList(ctx context.Context) ([]Movie, error)
	TimerList(ctx context.Context) ([]Timer, error)
	AddTimer(ctx context.Context, t NewTimer) error
	// DeleteTimer removes a timer. Deleting a running timer stops the
	// recording in progress — the box aborts it (ARCHITECTURE.md).
	DeleteTimer(ctx context.Context, ref TimerRef) error
	// Bouquets lists the box's bouquets (GuideHub's channel groups).
	Bouquets(ctx context.Context) ([]Service, error)
	// Services lists the channels inside a bouquet (bouquetRef, as returned
	// by Bouquets).
	Services(ctx context.Context, bouquetRef string) ([]Service, error)
	// EPGMulti returns every event starting within [start, end) (Unix
	// seconds) for every channel in a bouquet — the data source for
	// GuideHub's timeline grid.
	EPGMulti(ctx context.Context, bouquetRef string, start, end int64) ([]EPGEvent, error)
	// EPGNowNext returns what runs now and next on every channel of a
	// bouquet, in the bouquet's order (/web/epgnownext?bRef=, one call).
	EPGNowNext(ctx context.Context, bouquetRef string) ([]NowNext, error)
	// EPGSearch finds EPG events by title substring (the box's own search).
	EPGSearch(ctx context.Context, query string) ([]EPGEvent, error)
	// Locations lists the box's recording folders (/web/getlocations).
	Locations(ctx context.Context) ([]string, error)
	// MovieDir lists one recording folder: its recordings and its direct
	// subfolders (/web/movielist?dirname=).
	MovieDir(ctx context.Context, dir string) (MovieFolder, error)
	// CurrentServiceReference is what the box is playing right now — a
	// channel, or a recording (its reference then carries the file path).
	CurrentServiceReference(ctx context.Context) (string, error)
	// Zap switches the live channel (ControlHub, GuideHub's "switch now").
	Zap(ctx context.Context, serviceReference string) error
	// PlayMovie plays a reference in Enigma2's movie player — seeking,
	// pause, the movie infobar — through DreamHub's own TV plugin
	// (/dreamhub/play). /web/zap only opens the movie player for a path
	// starting with "/", so a stream URL would play like a channel.
	// ErrNotFound when the plugin on the box is too old to have it.
	PlayMovie(ctx context.Context, serviceReference, title string) error
	// SendKey simulates a remote-control keypress (ControlHub).
	SendKey(ctx context.Context, code int) error
	// Message shows a text on the TV (ControlHub): kind is one of the
	// Message* constants, timeout in seconds (0 = until closed).
	Message(ctx context.Context, text string, kind, timeout int) error
	// Screenshot grabs the box's current screen as a PNG (ControlHub).
	Screenshot(ctx context.Context) ([]byte, error)
	// InStandby reports whether the box is in standby (ControlHub).
	InStandby(ctx context.Context) (bool, error)
	// Current is what the box shows right now, with its now/next and the
	// volume (/web/getcurrent).
	Current(ctx context.Context) (*CurrentService, error)
	// LiveStream opens the box's own, untranscoded stream of a service:
	// enigma2's stream proxy on port 8001 (ARCHITECTURE.md). It tunes
	// only when a tuner is free for it — it never switches the TV. The
	// caller reads and closes it; once data flows there's no timeout.
	LiveStream(ctx context.Context, serviceReference string) (io.ReadCloser, error)
	// RecordingFile opens a recording through WebControl's /file, passing
	// rangeHeader ("bytes=…", or "") on — for StreamHub when DreamHub
	// doesn't run on the box and can't read the file itself.
	RecordingFile(ctx context.Context, path, rangeHeader string) (*FileResponse, error)
}

// FileResponse is a file as the box serves it: 200 with the whole file or
// 206 with the requested range (ContentRange set).
type FileResponse struct {
	Status        int
	ContentLength int64 // -1 when unknown
	ContentRange  string
	Body          io.ReadCloser
}

// StreamPort is enigma2's stream proxy (streamproxy.socket).
const StreamPort = "8001"

// MovieFolder is one folder of recordings as /web/movielist?dirname=
// reports it.
type MovieFolder struct {
	Movies     []Movie  `json:"movies"`
	Subfolders []string `json:"subfolders"` // absolute, with a trailing slash
}

// ErrWritesDisabled is returned by the real client's write operations until
// they're explicitly enabled. No write endpoint was observed during the
// read-only inspection (ARCHITECTURE.md), so DreamHub doesn't send one to a
// real box on the strength of an assumption.
var ErrWritesDisabled = errors.New("webcontrol: write operations are disabled (DREAMHUB_ENABLE_DEVICE_WRITES=0)")

// ErrUnauthorized means WebControl asked for credentials — e.g. the user set
// config.plugins.Webinterface.localauth=true and DreamHub has none (or the
// wrong ones) configured (ARCHITECTURE.md, "WebControl auth for a backend on
// the box itself").
var ErrUnauthorized = errors.New("webcontrol: credentials required or rejected")

// ErrNotFound is a 404 from the box: the path doesn't exist there (an
// endpoint of a plugin that isn't installed, or too old).
var ErrNotFound = errors.New("webcontrol: not found on the box")

// RejectedError is a write the box refused (e2state=False). Text is the
// box's own explanation, translated into the box's OSD language — show it,
// never match on it (ARCHITECTURE.md). Conflicts land here too: the box
// runs its own tuner-aware check on every timeradd.
type RejectedError struct {
	Op   string
	Text string
}

func (e *RejectedError) Error() string {
	return "webcontrol: " + e.Op + " rejected by the box: " + e.Text
}

// Credentials returns the user name and password for WebControl's Basic
// Auth, or empty strings for none. Called per request, so a settings change
// takes effect without a restart.
type Credentials func() (user, password string)

// HTTPClient calls a real device's WebControl API over HTTP.
type HTTPClient struct {
	BaseURL string
	HTTP    *http.Client
	// AllowWrites enables write operations (e.g. AddTimer).
	AllowWrites bool
	// Credentials, if set, supplies Basic Auth for every request.
	Credentials Credentials

	// Streams is the client for streams (LiveStream, RecordingFile): no
	// overall timeout, only one for the answer to start.
	Streams *http.Client

	mu        sync.Mutex
	sessionID string

	inflightMu sync.Mutex
	inflight   map[string]*inflightGet
}

// inflightGet is one GET in progress, shared by every caller asking for the
// same path while it's outstanding — see coalescedGet.
type inflightGet struct {
	wg   sync.WaitGroup
	body []byte
	err  error
}

// NewHTTPClient returns an HTTPClient with a sane default timeout — a
// device on the LAN should answer quickly, and the backend shouldn't hang
// indefinitely if it doesn't (see PRINCIPLES.md, "Resilience when external
// services are unavailable").
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		BaseURL:  baseURL,
		HTTP:     &http.Client{Timeout: 5 * time.Second},
		Streams:  &http.Client{Transport: streamTransport()},
		inflight: map[string]*inflightGet{},
	}
}

func (c *HTTPClient) DeviceInfo(ctx context.Context) (*DeviceInfo, error) {
	var info DeviceInfo
	if err := c.getXML(ctx, "/web/deviceinfo", &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *HTTPClient) MovieList(ctx context.Context) ([]Movie, error) {
	body, err := c.get(ctx, "/web/movielist")
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return decodeMovieList(body), nil
}

// decodeMovieList tolerates a malformed/truncated tail — confirmed live on
// the reference device 2026-09-24 (see testdata/fixtures/movielist_malformed.xml):
// /web/movielist can end mid-response with an internal "Processing Failed"
// HTML fragment instead of a properly closed </e2movielist>, rather than
// returning an error status — apparently triggered by one specific
// recording's own metadata confusing WebControl's own response generation,
// not anything malformed in the request. Decoding each <e2movie> one at a
// time and keeping whatever parsed successfully before the first decode
// error means one broken entry doesn't take down the whole list.
func decodeMovieList(r io.Reader) []Movie {
	dec := xml.NewDecoder(r)
	var movies []Movie
	for {
		tok, err := dec.Token()
		if err != nil {
			return movies
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "e2movie" {
			continue
		}
		var m Movie
		if dec.DecodeElement(&m, &se) != nil {
			return movies
		}
		movies = append(movies, m)
	}
}

// Locations lists the recording folders configured on the box.
func (c *HTTPClient) Locations(ctx context.Context) ([]string, error) {
	var res struct {
		Locations []string `xml:"e2location"`
	}
	if err := c.getXML(ctx, "/web/getlocations", &res); err != nil {
		return nil, err
	}
	var out []string
	for _, l := range res.Locations {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

// MovieDir lists one folder. dirname must always be given explicitly: without
// it the box still lists the default folder's recordings but crashes on the
// subfolder part ("Processing Failed" — pathExists(None) in the plugin,
// confirmed live 2026-09-24, ARCHITECTURE.md), which is why MovieList only
// ever saw the top folder.
func (c *HTTPClient) MovieDir(ctx context.Context, dir string) (MovieFolder, error) {
	body, err := c.do(ctx, http.MethodPost, "/web/movielist", url.Values{"dirname": {dir}})
	if err != nil {
		return MovieFolder{}, err
	}
	defer body.Close()
	return decodeMovieFolder(body), nil
}

// decodeMovieFolder reads <e2movie> entries and <e2location> subfolders,
// keeping whatever parsed before a malformed tail (see decodeMovieList).
func decodeMovieFolder(r io.Reader) MovieFolder {
	dec := xml.NewDecoder(r)
	f := MovieFolder{Movies: []Movie{}, Subfolders: []string{}}
	for {
		tok, err := dec.Token()
		if err != nil {
			return f
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "e2movie":
			var m Movie
			if dec.DecodeElement(&m, &se) != nil {
				return f
			}
			f.Movies = append(f.Movies, m)
		case "e2location":
			var loc string
			if dec.DecodeElement(&loc, &se) != nil {
				return f
			}
			if loc = strings.TrimSpace(loc); loc != "" {
				f.Subfolders = append(f.Subfolders, loc)
			}
		}
	}
}

// CurrentServiceReference reads /web/getcurrent's service reference.
func (c *HTTPClient) CurrentServiceReference(ctx context.Context) (string, error) {
	body, err := c.get(ctx, "/web/getcurrent")
	if err != nil {
		return "", err
	}
	defer body.Close()
	dec := xml.NewDecoder(body)
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("webcontrol: no service in /web/getcurrent: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "e2servicereference" {
			var ref string
			if err := dec.DecodeElement(&ref, &se); err != nil {
				return "", err
			}
			return strings.TrimSpace(ref), nil
		}
	}
}

func (c *HTTPClient) TimerList(ctx context.Context) ([]Timer, error) {
	var list timerList
	if err := c.getXML(ctx, "/web/timerlist", &list); err != nil {
		return nil, err
	}
	return list.Timers, nil
}

func (c *HTTPClient) Bouquets(ctx context.Context) ([]Service, error) {
	var list serviceList
	if err := c.getXML(ctx, "/web/getservices", &list); err != nil {
		return nil, err
	}
	return list.Services, nil
}

func (c *HTTPClient) Services(ctx context.Context, bouquetRef string) ([]Service, error) {
	var list serviceList
	q := url.Values{"sRef": {bouquetRef}}
	if err := c.getXML(ctx, "/web/getservices?"+q.Encode(), &list); err != nil {
		return nil, err
	}
	return list.Services, nil
}

func (c *HTTPClient) EPGMulti(ctx context.Context, bouquetRef string, start, end int64) ([]EPGEvent, error) {
	var list epgEventList
	// Despite its name, endTime is a duration in minutes: the plugin hands
	// it to epgcache.lookupEvent as the (service, type, begin, minutes)
	// tuple's last element. An end timestamp there reads as "millions of
	// minutes" and returns the whole EPG (~25 days, megabytes) — confirmed
	// live 2026-09-24 (ARCHITECTURE.md, "epgmulti's endTime is minutes").
	minutes := (end - start + 59) / 60
	q := url.Values{
		"bRef":    {bouquetRef},
		"time":    {strconv.FormatInt(start, 10)},
		"endTime": {strconv.FormatInt(minutes, 10)},
	}
	if err := c.getXML(ctx, "/web/epgmulti?"+q.Encode(), &list); err != nil {
		return nil, err
	}
	return list.Events, nil
}

// NowNext is one channel's current and next event; nil where the box has
// no EPG for it.
type NowNext struct {
	ServiceReference string    `json:"serviceReference"`
	ServiceName      string    `json:"serviceName"`
	Now              *EPGEvent `json:"now"`
	Next             *EPGEvent `json:"next"`
}

// EPGNowNext reads /web/epgnownext, which takes the bouquet reference as
// bRef, not sRef (ARCHITECTURE.md, "epgnownext takes bRef"): two events
// per channel, now then next, in bouquet order. A channel without EPG
// answers its fields as the literal "None" (confirmed live, fixture
// epgnownext.xml), so they're read as text and converted here.
func (c *HTTPClient) EPGNowNext(ctx context.Context, bouquetRef string) ([]NowNext, error) {
	var list struct {
		Events []struct {
			ID               string `xml:"e2eventid"`
			Start            string `xml:"e2eventstart"`
			Duration         string `xml:"e2eventduration"`
			Title            string `xml:"e2eventtitle"`
			Description      string `xml:"e2eventdescription"`
			ServiceReference string `xml:"e2eventservicereference"`
			ServiceName      string `xml:"e2eventservicename"`
		} `xml:"e2event"`
	}
	if err := c.getXML(ctx, "/web/epgnownext?"+url.Values{"bRef": {bouquetRef}}.Encode(), &list); err != nil {
		return nil, err
	}
	var out []NowNext
	index := map[string]int{}
	for _, e := range list.Events {
		ref := strings.TrimSpace(e.ServiceReference)
		if ref == "" || strings.HasPrefix(ref, "-1:") {
			continue
		}
		i, ok := index[ref]
		if !ok {
			i = len(out)
			index[ref] = i
			out = append(out, NowNext{ServiceReference: ref, ServiceName: strings.TrimSpace(e.ServiceName)})
		}
		id, err1 := strconv.ParseInt(strings.TrimSpace(e.ID), 10, 64)
		start, err2 := strconv.ParseInt(strings.TrimSpace(e.Start), 10, 64)
		dur, err3 := strconv.ParseInt(strings.TrimSpace(e.Duration), 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			continue // "None": no EPG
		}
		ev := &EPGEvent{ID: id, Start: start, Duration: dur, Title: strings.TrimSpace(e.Title), Description: strings.TrimSpace(e.Description),
			ServiceReference: ref, ServiceName: out[i].ServiceName}
		if out[i].Now == nil {
			out[i].Now = ev
		} else if out[i].Next == nil {
			out[i].Next = ev
		}
	}
	return out, nil
}

type simpleResult struct {
	XMLName   xml.Name `xml:"e2simplexmlresult"`
	State     bool     `xml:"e2state"`
	StateText string   `xml:"e2statetext"`
}

// AddTimer creates a recording timer via /web/timeradd — confirmed live
// 2026-09-24 (ARCHITECTURE.md, "Timer write endpoints"). Always a
// form-encoded POST with every parameter the endpoint knows: the timer
// screens refuse GET on boxes with anti_hijack on (the plugin's default),
// and a parameter left out reaches the plugin as None and crashes it
// instead of producing a clean error. The box adds no margins; Begin and
// End already include them.
//
// A nil error doesn't prove the timer exists: the box reports a timer it
// silently dropped as a duplicate as success too. Callers verify against
// TimerList.
func (c *HTTPClient) AddTimer(ctx context.Context, t NewTimer) error {
	if !c.AllowWrites {
		return ErrWritesDisabled
	}
	eit := ""
	if t.EventID > 0 {
		eit = strconv.FormatInt(t.EventID, 10)
	}
	flag := func(b bool) string {
		if b {
			return "1" // only "1" switches it on (ARCHITECTURE.md)
		}
		return "0"
	}
	afterEvent := t.AfterEvent
	if afterEvent == "" {
		afterEvent = "3" // "auto", Enigma2's default
	}
	form := url.Values{
		"sRef":        {t.ServiceReference},
		"begin":       {strconv.FormatInt(t.Begin, 10)},
		"end":         {strconv.FormatInt(t.End, 10)},
		"name":        {t.Name},
		"description": {t.Description},
		"repeated":    {strconv.Itoa(t.Repeated)},
		"dirname":     {t.Dirname}, // "" = the box's own preferred timer path
		"tags":        {t.Tags},
		"eit":         {eit},
		"disabled":    {flag(t.Disabled)},
		"justplay":    {flag(t.JustPlay)},
		"afterevent":  {afterEvent},
	}
	return c.simpleWrite(ctx, "timeradd", form)
}

// DeleteTimer removes a timer via /web/timerdelete (confirmed live
// 2026-09-24). The box matches the service reference string and begin/end
// exactly, so ref must carry the values from TimerList verbatim.
func (c *HTTPClient) DeleteTimer(ctx context.Context, ref TimerRef) error {
	if !c.AllowWrites {
		return ErrWritesDisabled
	}
	form := url.Values{
		"sRef":  {ref.ServiceReference},
		"begin": {strconv.FormatInt(ref.Begin, 10)},
		"end":   {strconv.FormatInt(ref.End, 10)},
	}
	return c.simpleWrite(ctx, "timerdelete", form)
}

func (c *HTTPClient) simpleWrite(ctx context.Context, op string, form url.Values) error {
	body, err := c.do(ctx, http.MethodPost, "/web/"+op, form)
	if err != nil {
		return err
	}
	defer body.Close()
	var res simpleResult
	if err := decodeXML(body, "/web/"+op, &res); err != nil {
		return err
	}
	if !res.State {
		return &RejectedError{Op: op, Text: strings.TrimSpace(res.StateText)}
	}
	return nil
}

// remoteControlResult is /web/remotecontrol's own response shape — a
// different schema from simpleResult (e2result/e2resulttext, not
// e2state/e2statetext), confirmed live 2026-09-25 (ARCHITECTURE.md, "Real
// screenshot and remote-control endpoints").
type remoteControlResult struct {
	XMLName    xml.Name `xml:"e2remotecontrol"`
	Result     bool     `xml:"e2result"`
	ResultText string   `xml:"e2resulttext"`
}

// Zap switches the live channel via /web/zap (confirmed live 2026-09-25).
// Unlike the timer endpoints, zap.xml accepts GET (no anti_hijack
// restriction). Gated by AllowWrites like every other change to the box:
// ControlHub's whole point is controlling the live device, so the same
// read-only-trial switch applies even though switching channels is
// instantly reversible.
func (c *HTTPClient) Zap(ctx context.Context, serviceReference string) error {
	if !c.AllowWrites {
		return ErrWritesDisabled
	}
	body, err := c.get(ctx, "/web/zap?sRef="+url.QueryEscape(serviceReference))
	if err != nil {
		return err
	}
	defer body.Close()
	var res simpleResult
	if err := decodeXML(body, "/web/zap", &res); err != nil {
		return err
	}
	if !res.State {
		return &RejectedError{Op: "zap", Text: strings.TrimSpace(res.StateText)}
	}
	return nil
}

// PlayMovie asks DreamHub's TV plugin to open the movie player with the
// reference (enigma2-plugin/src/webplay.py). A form POST like every other
// write; the plugin answers {"ok": true} or {"ok": false, "error": "…"}.
func (c *HTTPClient) PlayMovie(ctx context.Context, serviceReference, title string) error {
	if !c.AllowWrites {
		return ErrWritesDisabled
	}
	body, err := c.do(ctx, http.MethodPost, "/dreamhub/play", url.Values{"sRef": {serviceReference}, "title": {title}})
	if err != nil {
		return err
	}
	defer body.Close()
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 1<<16)).Decode(&res); err != nil {
		return fmt.Errorf("webcontrol: /dreamhub/play: %w", err)
	}
	if !res.OK {
		return &RejectedError{Op: "play", Text: res.Error}
	}
	return nil
}

// SendKey simulates a remote-control keypress via /web/remotecontrol
// (confirmed live 2026-09-25) — standard Linux input-event codes (KEY_OK =
// 352, KEY_EXIT = 174, KEY_MENU = 139, the arrow/color/power keys — see
// ARCHITECTURE.md for the full confirmed table). A key the box's current
// screen doesn't handle still answers success; the box just shows a brief
// "blocked" icon and ignores it, not an error here. Gated by AllowWrites,
// same reasoning as Zap.
func (c *HTTPClient) SendKey(ctx context.Context, code int) error {
	if !c.AllowWrites {
		return ErrWritesDisabled
	}
	body, err := c.get(ctx, "/web/remotecontrol?command="+strconv.Itoa(code))
	if err != nil {
		return err
	}
	defer body.Close()
	var res remoteControlResult
	if err := decodeXML(body, "/web/remotecontrol", &res); err != nil {
		return err
	}
	if !res.Result {
		return &RejectedError{Op: "remotecontrol", Text: strings.TrimSpace(res.ResultText)}
	}
	return nil
}

// Message kinds, as Enigma2's MessageBox numbers them.
const (
	MessageInfo    = 1
	MessageWarning = 2
	MessageError   = 3
)

// Message shows text on the TV via /web/message (text, type, timeout —
// the classic WebInterface's parameters; not yet confirmed live on the
// reference box, ROADMAP.md). A form-encoded POST like every other write
// (anti_hijack, ARCHITECTURE.md), gated by AllowWrites: it puts something
// on the screen of whoever is watching.
func (c *HTTPClient) Message(ctx context.Context, text string, kind, timeout int) error {
	if !c.AllowWrites {
		return ErrWritesDisabled
	}
	form := url.Values{
		"text":    {text},
		"type":    {strconv.Itoa(kind)},
		"timeout": {strconv.Itoa(timeout)},
	}
	return c.simpleWrite(ctx, "message", form)
}

// Screenshot grabs the box's current screen as a PNG via /screenshot
// (confirmed live 2026-09-25) — a top-level path, not under /web/, served
// natively by ePixmap (no external binary dependency, unlike the
// unrelated /grab, which needs a separate "aio-grab" package that isn't
// installed on the reference box). osd=1&video=1 composites the GUI
// overlay with whatever's playing — what's actually visible on the TV.
// Read-only: not gated by AllowWrites.
func (c *HTTPClient) Screenshot(ctx context.Context) ([]byte, error) {
	body, err := c.get(ctx, "/screenshot?format=png&osd=1&video=1")
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(body)
}

// powerState is /web/powerstate's response shape.
type powerState struct {
	XMLName   xml.Name `xml:"e2powerstate"`
	InStandby bool     `xml:"e2instandby"`
}

// InStandby reports whether the box is currently in standby via
// /web/powerstate (confirmed live 2026-09-25) — ControlHub checks this
// before assuming a blank screenshot means something's wrong: the
// reference box was found in standby mid-session, where every simulated
// key is silently ignored except power.
func (c *HTTPClient) InStandby(ctx context.Context) (bool, error) {
	var res powerState
	if err := c.getXML(ctx, "/web/powerstate", &res); err != nil {
		return false, err
	}
	return res.InStandby, nil
}

var sizeRe = regexp.MustCompile(`^([\d.]+)\s*([KMGT]?B)$`)
var sizeUnits = map[string]float64{"B": 1, "KB": 1 << 10, "MB": 1 << 20, "GB": 1 << 30, "TB": 1 << 40}

// ParseSize turns a human-formatted size like "2 TB" or "263.947 GB" (the
// only form /web/deviceinfo's HDD fields come in — see HDD) into a byte
// count. Precision is limited by the source string's own rounding; good
// enough for a usage percentage, not for exact byte accounting.
func ParseSize(s string) (int64, bool) {
	m := sizeRe.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(s)))
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return int64(n * sizeUnits[m[2]]), true
}

// get issues a GET and returns the response body for the caller to read and
// close — the shared HTTP mechanics behind both getXML and MovieList's own
// tolerant decoding.
func (c *HTTPClient) get(ctx context.Context, path string) (io.ReadCloser, error) {
	body, err := c.coalescedGet(ctx, path)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

// coalescedGet dedupes concurrent identical GETs (docs/V1_PLAN.md, WP7):
// enigma2 serves WebControl single-threaded (ARCHITECTURE.md), so several
// callers asking for the same path at once — dashboard tiles loading
// together, the debounced search palette re-firing — only make the box
// slower without any of them getting fresher data. One real request is
// made per distinct path; every caller waiting on it gets the same result.
// This only ever merges requests that are already happening at the same
// moment and doesn't keep anything cached afterwards, so it can never
// return anything staler than a directly serialized call already would —
// in particular it never undermines a plan's own apply-time freshness
// check (internal/plan's Fingerprint/Current), which always runs after any
// in-flight call this could have coalesced with has long finished.
func (c *HTTPClient) coalescedGet(ctx context.Context, path string) ([]byte, error) {
	c.inflightMu.Lock()
	if call, ok := c.inflight[path]; ok {
		c.inflightMu.Unlock()
		call.wg.Wait()
		return call.body, call.err
	}
	call := &inflightGet{}
	call.wg.Add(1)
	c.inflight[path] = call
	c.inflightMu.Unlock()

	rc, err := c.do(ctx, http.MethodGet, path, nil)
	var body []byte
	if err == nil {
		body, err = io.ReadAll(rc)
		rc.Close()
	}
	call.body, call.err = body, err

	c.inflightMu.Lock()
	delete(c.inflight, path)
	c.inflightMu.Unlock()
	call.wg.Done()
	return body, err
}

// do sends a request with Basic Auth (if configured) and, for POST, form as
// the body. If the box answers 412 — its "sessionid missing" response,
// only sent with config.plugins.Webinterface.extended_security on — it
// fetches a session from /web/session once and retries with it, the same
// way WebControl's own UI does (ARCHITECTURE.md).
func (c *HTTPClient) do(ctx context.Context, method, path string, form url.Values) (io.ReadCloser, error) {
	resp, err := c.send(ctx, method, path, form, c.session())
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusPreconditionFailed {
		resp.Body.Close()
		sid, err := c.refreshSession(ctx)
		if err != nil {
			return nil, err
		}
		if resp, err = c.send(ctx, method, path, form, sid); err != nil {
			return nil, err
		}
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return resp.Body, nil
	case http.StatusUnauthorized:
		resp.Body.Close()
		return nil, fmt.Errorf("webcontrol: %s: %w", path, ErrUnauthorized)
	case http.StatusNotFound:
		resp.Body.Close()
		return nil, fmt.Errorf("webcontrol: %s: %w", path, ErrNotFound)
	default:
		resp.Body.Close()
		return nil, fmt.Errorf("webcontrol: %s returned status %d", path, resp.StatusCode)
	}
}

func (c *HTTPClient) send(ctx context.Context, method, path string, form url.Values, sessionID string) (*http.Response, error) {
	var body io.Reader
	if method == http.MethodPost {
		f := url.Values{}
		for k, v := range form {
			f[k] = v
		}
		if sessionID != "" {
			f.Set("sessionid", sessionID)
		}
		body = strings.NewReader(f.Encode())
	} else if sessionID != "" {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		path += sep + "sessionid=" + url.QueryEscape(sessionID)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("webcontrol: building request for %s: %w", path, err)
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if c.Credentials != nil {
		if user, pass := c.Credentials(); user != "" {
			req.SetBasicAuth(user, pass)
		}
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webcontrol: calling %s: %w", path, err)
	}
	return resp, nil
}

func (c *HTTPClient) session() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID
}

type sessionResult struct {
	XMLName xml.Name `xml:"e2sessionid"`
	ID      string   `xml:",chardata"`
}

func (c *HTTPClient) refreshSession(ctx context.Context) (string, error) {
	resp, err := c.send(ctx, http.MethodPost, "/web/session", nil, "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("webcontrol: /web/session: %w", ErrUnauthorized)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("webcontrol: /web/session returned status %d", resp.StatusCode)
	}
	var res sessionResult
	if err := decodeXML(resp.Body, "/web/session", &res); err != nil {
		return "", err
	}
	id := strings.TrimSpace(res.ID)
	c.mu.Lock()
	c.sessionID = id
	c.mu.Unlock()
	return id, nil
}

func (c *HTTPClient) getXML(ctx context.Context, path string, v any) error {
	body, err := c.get(ctx, path)
	if err != nil {
		return err
	}
	defer body.Close()
	return decodeXML(body, path, v)
}

func decodeXML(r io.Reader, path string, v any) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("webcontrol: reading %s response: %w", path, err)
	}
	if err := xml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("webcontrol: parsing %s response: %w", path, err)
	}
	return nil
}

func streamTransport() http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 15 * time.Second
	return t
}

// LiveStream: see Client. The stream proxy answers 200 at once and then
// sends nothing when it can't get a tuner; the caller notices that.
func (c *HTTPClient) LiveStream(ctx context.Context, serviceReference string) (io.ReadCloser, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("webcontrol: base URL: %w", err)
	}
	u.Host = net.JoinHostPort(u.Hostname(), StreamPort)
	u.Path, u.RawQuery = "/"+serviceReference, ""
	resp, err := c.stream(ctx, u.String(), "")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, statusError("stream", resp.StatusCode)
	}
	return resp.Body, nil
}

// RecordingFile: see Client.
func (c *HTTPClient) RecordingFile(ctx context.Context, path, rangeHeader string) (*FileResponse, error) {
	resp, err := c.stream(ctx, c.BaseURL+"/file?"+url.Values{"file": {path}}.Encode(), rangeHeader)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, statusError("/file", resp.StatusCode)
	}
	return &FileResponse{Status: resp.StatusCode, ContentLength: resp.ContentLength, ContentRange: resp.Header.Get("Content-Range"), Body: resp.Body}, nil
}

func (c *HTTPClient) stream(ctx context.Context, rawURL, rangeHeader string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("webcontrol: building stream request: %w", err)
	}
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	if c.Credentials != nil {
		if user, pass := c.Credentials(); user != "" {
			req.SetBasicAuth(user, pass)
		}
	}
	hc := c.Streams
	if hc == nil {
		hc = &http.Client{Transport: streamTransport()}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webcontrol: stream: %w", err)
	}
	return resp, nil
}

func statusError(what string, status int) error {
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("webcontrol: %s: %w", what, ErrUnauthorized)
	case http.StatusNotFound:
		return fmt.Errorf("webcontrol: %s: %w", what, ErrNotFound)
	}
	return fmt.Errorf("webcontrol: %s returned status %d", what, status)
}

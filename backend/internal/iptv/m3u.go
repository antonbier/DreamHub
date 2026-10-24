// Package iptv reads an M3U playlist and turns its channels into the plain
// 4097 service references enigma2 plays itself (docs/NETCAST_PLAN.md, N1).
// It only parses and builds strings; fetching a list is in fetch.go and
// putting channels into a bouquet is ChannelHub's job (internal/hubs).
//
// A stream address often carries a login ("user:pass@host", "?password=…").
// Nothing in this package writes an address into an error or a log line,
// and Display gives the form that may be shown to people.
package iptv

import (
	"bufio"
	"errors"
	"io"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"
)

// MaxEntries is how many channels one list may hold; provider lists with
// more are cut here and Playlist.Truncated says so.
const MaxEntries = 30000

// ErrNotPlaylist: the text is not an M3U (an HTML page, an empty answer).
var ErrNotPlaylist = errors.New("iptv: not an M3U playlist")

// Entry is one channel of a list.
type Entry struct {
	Name    string
	URL     string
	TvgID   string
	TvgName string
	Logo    string // tvg-logo, an address on the list's own server
	Group   string // group-title (or #EXTGRP)
	Radio   bool
	// Options are the #EXTVLCOPT and #KODIPROP lines of the entry (headers,
	// user agent). enigma2's player cannot take them; they are kept so the
	// UI can say the channel may not play on the box.
	Options []string
	Line    int // line of the #EXTINF (or of the address), for messages
}

// Skip is an entry that was left out, and why.
type Skip struct {
	Line   int
	Reason string // "scheme" (not a stream address enigma2 plays), "invalid"
}

// Playlist is a parsed list.
type Playlist struct {
	Entries   []Entry
	Skipped   []Skip
	Truncated bool
}

// schemes enigma2's player (gstreamer or exteplayer3) opens as a stream.
var schemes = map[string]bool{"http": true, "https": true, "rtsp": true, "rtmp": true, "udp": true}

// Parse reads an M3U list. Lists without the #EXTM3U header are accepted
// when they hold at least one stream address.
func Parse(r io.Reader) (*Playlist, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	pl := &Playlist{}
	var (
		cur     *Entry // the #EXTINF waiting for its address
		group   string // #EXTGRP, for the entry after it
		options []string
		line    int
		sawAny  bool // a header, an #EXTINF or an address
		header  bool // #EXTM3U or an #EXTINF: it is meant as a playlist
	)
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if line == 1 {
			text = strings.TrimPrefix(text, "\xef\xbb\xbf")
		}
		if text == "" {
			continue
		}
		if !sawAny && (text[0] == '<' || !utf8.ValidString(text)) {
			return nil, ErrNotPlaylist
		}
		switch {
		case strings.HasPrefix(text, "#EXTM3U"):
			sawAny, header = true, true
		case strings.HasPrefix(text, "#EXTINF:"):
			sawAny, header = true, true
			e := parseExtinf(text[len("#EXTINF:"):])
			e.Line = line
			if e.Group == "" {
				e.Group = group
			}
			cur = &e
			cur.Options = options
			options = nil
		case strings.HasPrefix(text, "#EXTGRP:"):
			group = strings.TrimSpace(text[len("#EXTGRP:"):])
			if cur != nil && cur.Group == "" {
				cur.Group = group
			}
		case strings.HasPrefix(text, "#EXTVLCOPT:"), strings.HasPrefix(text, "#KODIPROP:"):
			if cur != nil {
				cur.Options = append(cur.Options, text)
			} else {
				options = append(options, text)
			}
		case text[0] == '#':
			// a comment or a tag this reader does not use
		default:
			sawAny = true
			e := Entry{Line: line}
			if cur != nil {
				e = *cur
			}
			cur, options = nil, nil
			if len(pl.Entries) >= MaxEntries {
				pl.Truncated = true
				return pl, nil
			}
			if reason := checkURL(text); reason != "" {
				pl.Skipped = append(pl.Skipped, Skip{Line: e.Line, Reason: reason})
				continue
			}
			e.URL = text
			if e.Name == "" {
				e.Name = e.TvgName
			}
			if e.Name == "" {
				e.Name = nameFromURL(text)
			}
			pl.Entries = append(pl.Entries, e)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, ErrNotPlaylist
	}
	if !sawAny || (!header && len(pl.Entries) == 0) {
		return nil, ErrNotPlaylist
	}
	return pl, nil
}

// parseExtinf reads what follows "#EXTINF:" — the duration, attributes
// (key="value", the value may hold commas) and, after the first comma outside
// quotes, the channel's name.
func parseExtinf(s string) Entry {
	var e Entry
	i := 0
	// the duration: up to the first space or comma
	for i < len(s) && s[i] != ' ' && s[i] != ',' && s[i] != '\t' {
		i++
	}
	attrs := map[string]string{}
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}
		if s[i] == ',' {
			e.Name = strings.TrimSpace(s[i+1:])
			break
		}
		start := i
		for i < len(s) && s[i] != '=' && s[i] != ',' && s[i] != ' ' {
			i++
		}
		if i >= len(s) || s[i] != '=' {
			// no attribute here: the rest up to a comma is noise; look for the name
			if c := strings.IndexByte(s[start:], ','); c >= 0 {
				e.Name = strings.TrimSpace(s[start+c+1:])
			}
			break
		}
		key := strings.ToLower(s[start:i])
		i++ // '='
		var val string
		if i < len(s) && s[i] == '"' {
			i++
			end := strings.IndexByte(s[i:], '"')
			if end < 0 {
				val, i = s[i:], len(s)
			} else {
				val, i = s[i:i+end], i+end+1
			}
		} else {
			vs := i
			for i < len(s) && s[i] != ' ' && s[i] != ',' {
				i++
			}
			val = s[vs:i]
		}
		attrs[key] = strings.TrimSpace(val)
	}
	e.TvgID = attrs["tvg-id"]
	e.TvgName = attrs["tvg-name"]
	e.Logo = attrs["tvg-logo"]
	e.Group = attrs["group-title"]
	e.Radio = strings.EqualFold(attrs["radio"], "true") || strings.EqualFold(attrs["tvg-type"], "radio")
	return e
}

// checkURL returns "" for an address enigma2 plays, else why not.
func checkURL(s string) string {
	if strings.ContainsAny(s, " \t\r\n") {
		return "invalid"
	}
	u, err := url.Parse(s)
	if err != nil {
		return "invalid"
	}
	if !schemes[strings.ToLower(u.Scheme)] {
		return "scheme"
	}
	if u.Host == "" {
		return "invalid"
	}
	return ""
}

func nameFromURL(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return "Channel"
	}
	if base := path.Base(u.Path); base != "." && base != "/" && base != "" {
		return base
	}
	return u.Hostname()
}

// loginKeys are query parameters that carry a login or a key.
var loginKeys = map[string]bool{
	"username": true, "user": true, "password": true, "pass": true, "pwd": true,
	"token": true, "auth": true, "key": true, "apikey": true, "api_key": true,
}

// HasLogin reports whether the address carries credentials, in its user info
// or in a query parameter like password= or token=.
func HasLogin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.User != nil {
		return true
	}
	for k := range u.Query() {
		if loginKeys[strings.ToLower(k)] {
			return true
		}
	}
	return false
}

// Display is the address in a form that may be shown and logged: scheme,
// host and path, with no user info, no query and no fragment.
func Display(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme) + "://" + u.Host + u.Path
}

// Host is the host name alone, for a list row.
func Host(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// Scheme is the address's scheme in lower case.
func Scheme(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Scheme)
}

// Ref is the service reference enigma2 plays for a stream address: service
// type 4097 (the gstreamer player) and the address with '%' and ':' escaped,
// the form the box itself writes (ARCHITECTURE.md, "Netcast, EPG and playing
// 4097 streams"). It has no name: the box appends ":<name>" when the entry
// is added with one.
func Ref(streamURL string) string {
	return "4097:0:1:0:0:0:0:0:0:0:" + escapeURL(streamURL)
}

func escapeURL(streamURL string) string {
	esc := strings.ReplaceAll(streamURL, "%", "%25")
	return strings.ReplaceAll(esc, ":", "%3a")
}

// TwinRef is the reference for a stream that carries the identity of a DVB
// service of the box (type, sid, tsid, onid, namespace), so enigma2 shows
// that service's guide for it: the same stream address with the identity of
// an existing channel of the same station (ARCHITECTURE.md, "The identity is
// what carries the guide"). ok is false when dvbRef is not a DVB service.
func TwinRef(dvbRef, streamURL string) (string, bool) {
	parts := strings.SplitN(dvbRef, ":", 11)
	// the second field is the flags: 0 for a service, 7 for a bouquet, 64 for a heading
	if len(parts) < 10 || parts[0] != "1" || parts[1] != "0" {
		return "", false
	}
	for _, f := range parts[1:10] {
		if f == "" || len(f) > 8 || strings.Trim(f, "0123456789abcdefABCDEF") != "" {
			return "", false
		}
	}
	return "4097:" + strings.Join(parts[1:10], ":") + ":" + escapeURL(streamURL), true
}

// WithURL gives a 4097 reference a new stream address and keeps its identity
// (and so its guide); "" when ref is not a 4097 reference.
func WithURL(ref, streamURL string) string {
	parts := strings.SplitN(ref, ":", 11)
	if len(parts) < 10 || parts[0] != "4097" {
		return ""
	}
	return strings.Join(parts[:10], ":") + ":" + escapeURL(streamURL)
}

// URLPart returns the escaped address of a 4097 reference ("" for any other
// reference), so an entry already in a bouquet can be compared with Ref's
// output whatever name the box added after it.
func URLPart(ref string) string {
	parts := strings.SplitN(ref, ":", 12)
	if len(parts) < 11 || parts[0] != "4097" {
		return ""
	}
	return lowerEscapes(parts[10])
}

// lowerEscapes writes the hex digits of every %XX in lower case, so the same
// address compares equal however the box spells it.
func lowerEscapes(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	b := []byte(s)
	for i := 0; i+2 < len(b); i++ {
		if b[i] == '%' {
			for k := 1; k <= 2; k++ {
				if c := b[i+k]; c >= 'A' && c <= 'F' {
					b[i+k] = c + ('a' - 'A')
				}
			}
			i += 2
		}
	}
	return string(b)
}

// RedactRef hides the address of a 4097 reference for messages and logs.
func RedactRef(ref string) string {
	parts := strings.SplitN(ref, ":", 12)
	if len(parts) < 11 || parts[10] == "" || parts[0] != "4097" {
		return ref
	}
	return strings.Join(parts[:10], ":") + ":<stream>"
}

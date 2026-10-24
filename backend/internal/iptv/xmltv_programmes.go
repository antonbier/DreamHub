package iptv

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// What is on now and next, from an XMLTV file the person has set up in
// Netcast (docs/NETCAST_PLAN.md, "A page of its own"). The file is streamed
// and only the programmes around "now" are kept, so a week-long guide of
// many megabytes costs a few hundred kilobytes. Nothing is stored.

// MaxProgrammeBytes caps what is read of a guide file, after decompression.
const MaxProgrammeBytes = 400 << 20

// Programme is one entry of a channel's schedule.
type Programme struct {
	Title string
	Start time.Time
	End   time.Time
}

// Schedule is the window of a guide file: programmes by channel id, and the
// channel names so a channel without a matching id can be found by name.
type Schedule struct {
	ByID   map[string][]Programme
	ByName map[string]string // normalised display name -> channel id
	From   time.Time
	To     time.Time
}

// NormName makes names comparable: lower case, letters and digits only, a
// trailing "hd" dropped.
func NormName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimSuffix(b.String(), "hd")
}

// Find returns the programme running at at and the one after it for a
// channel known by its id (tvg-id) or, failing that, its name.
func (s *Schedule) Find(id, name string, at time.Time) (now, next *Programme) {
	list := s.ByID[id]
	if len(list) == 0 || id == "" {
		if cid, ok := s.ByName[NormName(name)]; ok && NormName(name) != "" {
			list = s.ByID[cid]
		}
	}
	for i := range list {
		p := &list[i]
		if !at.Before(p.Start) && at.Before(p.End) {
			now = p
			if i+1 < len(list) {
				next = &list[i+1]
			}
			return
		}
		if at.Before(p.Start) {
			return nil, p
		}
	}
	return nil, nil
}

// ReadSchedule streams the XMLTV file at rawURL and keeps the programmes
// ending after from and starting before to.
func (f *Fetcher) ReadSchedule(ctx context.Context, rawURL string, from, to time.Time) (*Schedule, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, &FetchError{Kind: KindInvalidURL}
	}
	if strings.HasSuffix(strings.ToLower(u.Path), ".xz") {
		return nil, &FetchError{Kind: KindUnsupported}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &FetchError{Kind: KindInvalidURL}
	}
	ua := f.UserAgent
	if ua == "" {
		ua = "DreamHub"
	}
	req.Header.Set("User-Agent", ua)
	resp, err := f.httpClient().Do(req)
	if err != nil {
		var fe *FetchError
		if errors.As(err, &fe) {
			return nil, &FetchError{Kind: fe.Kind}
		}
		return nil, &FetchError{Kind: KindUnreachable}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &FetchError{Kind: KindStatus, Status: resp.StatusCode}
	}
	return ParseSchedule(resp.Body, from, to)
}

// ParseSchedule reads an XMLTV stream (gzip is detected).
func ParseSchedule(r io.Reader, from, to time.Time) (*Schedule, error) {
	br := bufio.NewReader(r)
	var src io.Reader = br
	if head, _ := br.Peek(2); bytes.Equal(head, []byte{0x1f, 0x8b}) {
		zr, err := gzip.NewReader(br)
		if err != nil {
			return nil, &FetchError{Kind: KindNotGuide}
		}
		defer zr.Close()
		src = zr
	}
	dec := xml.NewDecoder(bufio.NewReader(&capReader{r: src, left: MaxProgrammeBytes}))
	dec.Strict = false
	dec.CharsetReader = func(label string, in io.Reader) (io.Reader, error) { return in, nil }
	out := &Schedule{ByID: map[string][]Programme{}, ByName: map[string]string{}, From: from, To: to}
	sawTV := false
	for {
		tok, err := dec.Token()
		if err != nil {
			var fe *FetchError
			if errors.As(err, &fe) {
				return nil, fe
			}
			if err == io.EOF && sawTV {
				return out, nil
			}
			return nil, &FetchError{Kind: KindNotGuide}
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "tv":
			sawTV = true
		case "channel":
			var c struct {
				ID    string   `xml:"id,attr"`
				Names []string `xml:"display-name"`
			}
			if dec.DecodeElement(&c, &se) == nil && c.ID != "" {
				for _, n := range c.Names {
					if k := NormName(n); k != "" {
						if _, dup := out.ByName[k]; !dup {
							out.ByName[k] = c.ID
						}
					}
				}
			}
		case "programme":
			var p struct {
				Start   string   `xml:"start,attr"`
				Stop    string   `xml:"stop,attr"`
				Channel string   `xml:"channel,attr"`
				Titles  []string `xml:"title"`
			}
			if dec.DecodeElement(&p, &se) != nil || p.Channel == "" || len(p.Titles) == 0 {
				continue
			}
			st, e1 := parseXMLTVTime(p.Start)
			en, e2 := parseXMLTVTime(p.Stop)
			if e1 != nil || e2 != nil || !en.After(from) || !st.Before(to) {
				continue
			}
			out.ByID[p.Channel] = append(out.ByID[p.Channel], Programme{Title: strings.TrimSpace(p.Titles[0]), Start: st, End: en})
		}
	}
}

// parseXMLTVTime reads "20261024190000 +0200" (zone optional, then UTC).
func parseXMLTVTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse("20060102150405 -0700", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("200601021504 -0700", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("20060102150405", s); err == nil {
		return t, nil
	}
	return time.Parse("200601021504", s)
}

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
)

// Guide sources (docs/NETCAST_PLAN.md, N5). A guide file is XMLTV; DreamHub
// only needs to know which channels it has, so it reads the <channel>
// elements and stops at the first <programme> (the format puts channels
// first). The file is streamed, never kept.

// MaxGuideBytes caps what is read of a guide file, after decompression. Only
// the channel section is read, so this is generous.
const MaxGuideBytes = 32 << 20

// MaxGuideChannels caps the channels read from one file.
const MaxGuideChannels = 60000

// ErrNotGuide: the answer is not XMLTV (an HTML page, a playlist).
var ErrNotGuide = errors.New("iptv: not an XMLTV file")

// KindNotGuide and KindUnsupported are further FetchError kinds, for guide files.
const (
	KindNotGuide    = "not_guide"
	KindUnsupported = "unsupported" // .xz: no decompressor in this build
)

// GuideChannel is a <channel> of an XMLTV file.
type GuideChannel struct {
	ID    string
	Names []string
}

// GuideChannels is what a guide file knows.
type GuideChannels struct {
	Channels  []GuideChannel
	Truncated bool
}

// capReader fails with KindTooLarge instead of silently cutting the stream.
type capReader struct {
	r    io.Reader
	left int64
}

func (c *capReader) Read(p []byte) (int, error) {
	if c.left <= 0 {
		return 0, &FetchError{Kind: KindTooLarge}
	}
	if int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.r.Read(p)
	c.left -= int64(n)
	return n, err
}

// ReadGuideChannels reads the channels of the XMLTV file at rawURL.
func (f *Fetcher) ReadGuideChannels(ctx context.Context, rawURL string) (*GuideChannels, error) {
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
	return ParseGuideChannels(&capReader{r: resp.Body, left: MaxGuideBytes})
}

// ParseGuideChannels reads the channels of an XMLTV stream (gzip is
// detected). It stops at the first programme.
func ParseGuideChannels(r io.Reader) (*GuideChannels, error) {
	br := bufio.NewReader(r)
	if bom, _ := br.Peek(3); bytes.Equal(bom, []byte{0xef, 0xbb, 0xbf}) {
		br.Discard(3)
	}
	if head, _ := br.Peek(2); bytes.Equal(head, []byte{0x1f, 0x8b}) {
		zr, err := gzip.NewReader(br)
		if err != nil {
			return nil, &FetchError{Kind: KindNotGuide}
		}
		defer zr.Close()
		zb := bufio.NewReader(&capReader{r: zr, left: MaxGuideBytes})
		if bom, _ := zb.Peek(3); bytes.Equal(bom, []byte{0xef, 0xbb, 0xbf}) {
			zb.Discard(3)
		}
		return decodeGuide(zb)
	}
	return decodeGuide(br)
}

func decodeGuide(r io.Reader) (*GuideChannels, error) {
	dec := xml.NewDecoder(r)
	dec.Strict = false
	dec.CharsetReader = func(label string, in io.Reader) (io.Reader, error) { return in, nil }
	out := &GuideChannels{}
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
		case "programme":
			return out, nil
		case "channel":
			if !sawTV {
				return nil, &FetchError{Kind: KindNotGuide}
			}
			if len(out.Channels) >= MaxGuideChannels {
				out.Truncated = true
				return out, nil
			}
			c := GuideChannel{}
			for _, a := range se.Attr {
				if a.Name.Local == "id" {
					c.ID = strings.TrimSpace(a.Value)
				}
			}
			var el struct {
				Names []string `xml:"display-name"`
			}
			if err := dec.DecodeElement(&el, &se); err != nil {
				var fe *FetchError
				if errors.As(err, &fe) {
					return nil, fe
				}
				return nil, &FetchError{Kind: KindNotGuide}
			}
			for _, n := range el.Names {
				if n = strings.TrimSpace(n); n != "" {
					c.Names = append(c.Names, n)
				}
			}
			if c.ID != "" {
				out.Channels = append(out.Channels, c)
			}
		}
	}
}

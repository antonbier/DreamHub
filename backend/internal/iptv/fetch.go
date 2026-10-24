package iptv

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// Fetch error kinds (FetchError.Kind).
const (
	KindInvalidURL  = "invalid_url"  // not an http(s) address
	KindBlocked     = "blocked"      // the address leads to this box itself or a link-local address
	KindUnreachable = "unreachable"  // no answer, a TLS or DNS error, a timeout
	KindStatus      = "status"       // the server answered with an error code
	KindTooLarge    = "too_large"    // more than MaxBytes
	KindNotPlaylist = "not_playlist" // the answer is not an M3U
)

// FetchError is why a list could not be read. Its text never holds the
// address, which may carry a login.
type FetchError struct {
	Kind   string
	Status int // for KindStatus
}

func (e *FetchError) Error() string {
	if e.Kind == KindStatus {
		return fmt.Sprintf("iptv: the server answered %d", e.Status)
	}
	return "iptv: " + e.Kind
}

// Fetcher reads a playlist from an http(s) address on behalf of a person
// (docs/NETCAST_PLAN.md, N1): only the backend does this, never the browser.
// Nothing is fetched until someone asks.
type Fetcher struct {
	// MaxBytes caps the list after decompression; 0 means 8 MB.
	MaxBytes int64
	// Timeout for the whole request; 0 means 30 s.
	Timeout time.Duration
	// UserAgent sent to the server; "" means "DreamHub".
	UserAgent string
	// AllowLoopback lets tests reach a server on 127.0.0.1.
	AllowLoopback bool

	client *http.Client
}

// NewFetcher returns a Fetcher with its defaults.
func NewFetcher() *Fetcher { return &Fetcher{} }

func (f *Fetcher) maxBytes() int64 {
	if f.MaxBytes > 0 {
		return f.MaxBytes
	}
	return 8 << 20
}

// blocked reports whether a resolved address must not be dialled: this
// machine itself, an unspecified or multicast address, and link-local ones
// (169.254.0.0/16 holds cloud metadata services). A private LAN address is
// fine — a playlist on the person's own NAS or Tvheadend is a normal source.
func (f *Fetcher) blocked(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() {
		return !f.AllowLoopback
	}
	return ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func (f *Fetcher) httpClient() *http.Client {
	if f.client != nil {
		return f.client
	}
	timeout := f.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		// Control runs once per resolved address, right before connecting,
		// so a name that resolves to a forbidden address (or changes between
		// a check and the connection) is stopped here, redirects included.
		Control: func(network, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil || f.blocked(ap.Addr()) {
				return &FetchError{Kind: KindBlocked}
			}
			return nil
		},
	}
	f.client = &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                 nil, // an environment proxy would bypass the check above
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
			MaxIdleConns:          2,
			DisableKeepAlives:     true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return &FetchError{Kind: KindUnreachable}
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return &FetchError{Kind: KindInvalidURL}
			}
			return nil
		},
	}
	return f.client
}

// Fetch downloads the list at rawURL. The body is read up to MaxBytes after
// decompression; a gzip body is unpacked whether or not the server said so.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, &FetchError{Kind: KindInvalidURL}
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
	req.Header.Set("Accept", "*/*")
	resp, err := f.httpClient().Do(req)
	if err != nil {
		// err is a *url.Error that holds the address; report only the kind.
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
	return f.readLimited(resp.Body)
}

// ReadUpload applies the same size limit and gzip handling to a list the
// person uploaded.
func (f *Fetcher) ReadUpload(r io.Reader) ([]byte, error) { return f.readLimited(r) }

func (f *Fetcher) readLimited(r io.Reader) ([]byte, error) {
	max := f.maxBytes()
	raw, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, &FetchError{Kind: KindUnreachable}
	}
	if int64(len(raw)) > max {
		return nil, &FetchError{Kind: KindTooLarge}
	}
	if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, &FetchError{Kind: KindNotPlaylist}
		}
		raw, err = io.ReadAll(io.LimitReader(zr, max+1))
		if err != nil {
			return nil, &FetchError{Kind: KindNotPlaylist}
		}
		if int64(len(raw)) > max {
			return nil, &FetchError{Kind: KindTooLarge}
		}
	}
	return raw, nil
}

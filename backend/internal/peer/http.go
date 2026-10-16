package peer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// csrfHeader is what DreamHub wants on every changing request
// (httpx.CSRFHeader; not imported to keep this package free of the router).
const csrfHeader = "X-DreamHub-Request"

// HTTPClient is the real Client.
type HTTPClient struct {
	HTTP *http.Client
}

// NewHTTPClient returns a client with a short timeout: pairing is
// interactive and a box that is off must not hang the hub.
func NewHTTPClient() *HTTPClient {
	return &HTTPClient{HTTP: &http.Client{Timeout: 8 * time.Second}}
}

func (c *HTTPClient) do(ctx context.Context, method, baseURL, path, token string, body, out any) error {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+"/api/v1/peer"+path, rd)
	if err != nil {
		return ErrUnreachable
	}
	req.Header.Set(csrfHeader, "1")
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return ErrUnreachable
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized && token != "":
		return ErrUnauthorized
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusUnauthorized:
		return ErrNoPeer
	case resp.StatusCode == http.StatusConflict:
		return ErrNotPairing
	case resp.StatusCode == http.StatusForbidden:
		return ErrBadCode
	case resp.StatusCode != http.StatusOK:
		return ErrNoPeer
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return ErrNoPeer
	}
	return nil
}

func (c *HTTPClient) Info(ctx context.Context, baseURL string) (Info, error) {
	var i Info
	err := c.do(ctx, http.MethodGet, baseURL, "/info", "", nil, &i)
	if err == nil && i.API == 0 {
		return Info{}, ErrNoPeer // JSON of something else
	}
	return i, err
}

func (c *HTTPClient) Pair(ctx context.Context, baseURL, code, hubName string) (PairResult, error) {
	var r PairResult
	err := c.do(ctx, http.MethodPost, baseURL, "/pair", "", map[string]string{"code": code, "hubName": hubName}, &r)
	if err == nil && r.Token == "" {
		return PairResult{}, ErrNoPeer
	}
	return r, err
}

func (c *HTTPClient) Capabilities(ctx context.Context, baseURL, token string) (Capabilities, error) {
	var r Capabilities
	err := c.do(ctx, http.MethodGet, baseURL, "/capabilities", token, nil, &r)
	if errors.Is(err, ErrNoPeer) {
		return Capabilities{}, ErrNoPeer
	}
	return r, err
}

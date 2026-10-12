package dockerd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// callTimeout bounds one call to the helper process, the same
// not-forever-hung choice extension.HTTPClient makes for an extension's
// own process.
const callTimeout = 10 * time.Second

// HTTPClient is how DreamHub's main backend reaches the helper process
// (cmd/dreamhub-dockerd-helper) — the only thing it is ever allowed to
// say about Docker, since it never holds the socket itself. BaseURL is
// the helper's own loopback address.
type HTTPClient struct {
	BaseURL string
	// Transport overrides http.DefaultTransport (tests).
	Transport http.RoundTripper
}

var _ Client = HTTPClient{}

func (c HTTPClient) client() *http.Client {
	return &http.Client{Timeout: callTimeout, Transport: c.Transport}
}

func (c HTTPClient) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return resp, nil
}

func statusErr(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusServiceUnavailable:
		return ErrUnavailable
	default:
		return fmt.Errorf("dockerd: helper: unexpected status %d", resp.StatusCode)
	}
}

func (c HTTPClient) List(ctx context.Context) ([]Container, error) {
	resp, err := c.do(ctx, http.MethodGet, "/containers", nil)
	if err != nil {
		return nil, err
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, statusErr(resp)
	}
	var list []Container
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	return list, nil
}

func (c HTTPClient) Start(ctx context.Context, id string) error {
	resp, err := c.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/start", nil)
	if err != nil {
		return err
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return statusErr(resp)
	}
	return nil
}

func (c HTTPClient) Stop(ctx context.Context, id string) error {
	resp, err := c.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/stop", nil)
	if err != nil {
		return err
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return statusErr(resp)
	}
	return nil
}

func (c HTTPClient) Logs(ctx context.Context, id string, tail int) ([]byte, error) {
	q := url.Values{}
	if tail > 0 {
		q.Set("tail", fmt.Sprint(tail))
	}
	resp, err := c.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/logs?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		drainClosed(resp.Body)
		return nil, statusErr(resp)
	}
	return io.ReadAll(resp.Body)
}

func (c HTTPClient) Exec(ctx context.Context, id string, cmd []string) ([]byte, error) {
	body, err := json.Marshal(map[string][]string{"cmd": cmd})
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/exec", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		drainClosed(resp.Body)
		return nil, statusErr(resp)
	}
	return io.ReadAll(resp.Body)
}

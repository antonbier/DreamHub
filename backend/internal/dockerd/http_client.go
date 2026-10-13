package dockerd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
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
	case http.StatusConflict:
		return ErrAlreadyExists
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

func (c HTTPClient) Create(ctx context.Context, spec ContainerSpec) (Container, error) {
	body, err := json.Marshal(map[string]any{
		"appId": spec.AppID, "service": spec.Service, "image": spec.Image, "env": spec.Env, "cmd": spec.Cmd,
		"hostPort": spec.HostPort, "containerPort": spec.ContainerPort,
		"hostDir": spec.HostDir, "containerDir": spec.ContainerDir,
		"dataHostDir": spec.DataHostDir, "dataContainerDir": spec.DataContainerDir,
		"memoryLimitBytes": spec.MemoryLimitBytes, "hostNetwork": spec.HostNetwork,
	})
	if err != nil {
		return Container{}, err
	}
	resp, err := c.do(ctx, http.MethodPost, "/containers", bytes.NewReader(body))
	if err != nil {
		return Container{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		drainClosed(resp.Body)
		return Container{}, statusErr(resp)
	}
	var created Container
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return Container{}, err
	}
	return created, nil
}

func (c HTTPClient) Remove(ctx context.Context, id string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/containers/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return statusErr(resp)
	}
	return nil
}

func (c HTTPClient) ImageDigest(ctx context.Context, id string) (string, error) {
	resp, err := c.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/image-digest", nil)
	if err != nil {
		return "", err
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", statusErr(resp)
	}
	var out struct {
		Digest string `json:"digest"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Digest, nil
}

// Attach dials the helper directly (not through c.do/c.client, which
// hand back a parsed *http.Response and close the connection afterwards)
// — the same raw-hijack approach Exec.Attach uses one hop further out,
// against the real socket, applies here against the helper's own
// loopback listener instead.
func (c HTTPClient) Attach(ctx context.Context, id string) (io.ReadWriteCloser, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req := "GET /containers/" + url.PathEscape(id) + "/attach HTTP/1.1\r\n" +
		"Host: " + u.Host + "\r\n" +
		"Connection: Upgrade\r\n" +
		"Upgrade: tcp\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols && resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, statusErr(resp)
	}
	return &hijackedConn{Reader: br, conn: conn}, nil
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

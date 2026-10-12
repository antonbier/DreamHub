package dockerd

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
)

// Exec talks to a real Docker daemon over its unix socket — the only
// thing in DreamHub that does. It is hand-rolled HTTP against Docker's
// own Engine API rather than the official SDK, the same choice this repo
// already made for Enigma2's WebControl and apt/dpkg: one small, legible
// client instead of a large dependency this project doesn't need most of.
// Only ever constructed inside cmd/dreamhub-dockerd-helper.
type Exec struct {
	// SocketPath is the Docker daemon's unix socket, usually
	// "/var/run/docker.sock".
	SocketPath string
}

var _ Client = Exec{}

func (e Exec) client() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", e.SocketPath)
			},
		},
	}
}

// do issues one request against the daemon. Docker ignores the host part
// of the URL for a unix-socket connection, so a fixed placeholder is used
// throughout.
func (e Exec) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return resp, nil
}

type dockerContainer struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Labels map[string]string `json:"Labels"`
}

func fromDocker(c dockerContainer) Container {
	name := c.ID
	if len(c.Names) > 0 {
		// Docker's own names carry a leading slash.
		name = c.Names[0]
		if len(name) > 0 && name[0] == '/' {
			name = name[1:]
		}
	}
	return Container{ID: c.ID, Name: name, Image: c.Image, State: c.State, Status: c.Status, Labels: c.Labels}
}

func (e Exec) List(ctx context.Context) ([]Container, error) {
	filters, err := json.Marshal(map[string][]string{"label": {AppLabel}})
	if err != nil {
		return nil, err
	}
	q := url.Values{"all": {"1"}, "filters": {string(filters)}}
	resp, err := e.do(ctx, http.MethodGet, "/containers/json?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dockerd: list: unexpected status %d", resp.StatusCode)
	}
	var raw []dockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]Container, 0, len(raw))
	for _, c := range raw {
		// Belt and suspenders: the daemon already filtered by label, but
		// a container missing AppLabel (shouldn't happen) must never
		// surface here regardless.
		if _, ok := c.Labels[AppLabel]; !ok {
			continue
		}
		out = append(out, fromDocker(c))
	}
	return out, nil
}

// owns reports whether id carries AppLabel, the gate every mutating or
// reading call below passes through so nothing outside DreamHub's own
// Docker apps is ever reachable through this package.
func (e Exec) owns(ctx context.Context, id string) (bool, error) {
	resp, err := e.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/json", nil)
	if err != nil {
		return false, err
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("dockerd: inspect: unexpected status %d", resp.StatusCode)
	}
	var c struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		return false, err
	}
	_, ok := c.Config.Labels[AppLabel]
	return ok, nil
}

func (e Exec) requireOwned(ctx context.Context, id string) error {
	ok, err := e.owns(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

func (e Exec) Start(ctx context.Context, id string) error {
	if err := e.requireOwned(ctx, id); err != nil {
		return err
	}
	resp, err := e.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/start", nil)
	if err != nil {
		return err
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotModified {
		return fmt.Errorf("dockerd: start: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (e Exec) Stop(ctx context.Context, id string) error {
	if err := e.requireOwned(ctx, id); err != nil {
		return err
	}
	resp, err := e.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/stop", nil)
	if err != nil {
		return err
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotModified {
		return fmt.Errorf("dockerd: stop: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (e Exec) Logs(ctx context.Context, id string, tail int) ([]byte, error) {
	if err := e.requireOwned(ctx, id); err != nil {
		return nil, err
	}
	if tail <= 0 {
		tail = 200
	}
	q := url.Values{"stdout": {"1"}, "stderr": {"1"}, "tail": {fmt.Sprint(tail)}}
	resp, err := e.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/logs?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dockerd: logs: unexpected status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return demux(raw), nil
}

func (e Exec) Exec(ctx context.Context, id string, cmd []string) ([]byte, error) {
	if err := e.requireOwned(ctx, id); err != nil {
		return nil, err
	}
	createBody, err := json.Marshal(map[string]any{
		"Cmd": cmd, "AttachStdout": true, "AttachStderr": true, "Tty": false,
	})
	if err != nil {
		return nil, err
	}
	resp, err := e.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/exec", bytes.NewReader(createBody))
	if err != nil {
		return nil, err
	}
	var created struct{ ID string }
	decErr := json.NewDecoder(resp.Body).Decode(&created)
	drainClosed(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("dockerd: exec create: unexpected status %d", resp.StatusCode)
	}
	if decErr != nil {
		return nil, decErr
	}

	startBody, err := json.Marshal(map[string]any{"Detach": false, "Tty": false})
	if err != nil {
		return nil, err
	}
	resp, err = e.do(ctx, http.MethodPost, "/exec/"+url.PathEscape(created.ID)+"/start", bytes.NewReader(startBody))
	if err != nil {
		return nil, err
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dockerd: exec start: unexpected status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return demux(raw), nil
}

// demux strips Docker's multiplexed stdout/stderr frame headers (used
// whenever a container wasn't started with a tty: 1 byte stream type, 3
// reserved bytes, a 4-byte big-endian length, then that many bytes of
// payload) and concatenates stdout and stderr in the order they appear.
// Already-plain output (no recognizable frame header) is returned as is,
// so a tty-attached container's logs don't come back mangled.
func demux(raw []byte) []byte {
	var out bytes.Buffer
	for len(raw) >= 8 {
		streamType := raw[0]
		if streamType > 2 {
			break // not a framed stream; treat the rest as plain text
		}
		size := binary.BigEndian.Uint32(raw[4:8])
		raw = raw[8:]
		if uint32(len(raw)) < size {
			out.Write(raw)
			raw = nil
			break
		}
		out.Write(raw[:size])
		raw = raw[size:]
	}
	out.Write(raw)
	return out.Bytes()
}

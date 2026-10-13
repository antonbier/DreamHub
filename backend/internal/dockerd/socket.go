package dockerd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	Ports  []struct {
		PublicPort uint16 `json:"PublicPort"`
		Type       string `json:"Type"`
	} `json:"Ports"`
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
	out := Container{ID: c.ID, Name: name, Image: c.Image, State: c.State, Status: c.Status, Labels: c.Labels}
	for _, p := range c.Ports {
		if p.Type == "tcp" && p.PublicPort != 0 {
			out.HostPort = int(p.PublicPort)
			break
		}
	}
	if out.HostPort == 0 {
		if port, err := strconv.Atoi(c.Labels[PortLabel]); err == nil {
			out.HostPort = port
		}
	}
	return out
}

func networkMode(host bool) string {
	if host {
		return "host"
	}
	return "default"
}

// containerName is the fixed, deterministic name a given AppID's
// container gets — "one container per app" is enforced by Create seeing
// this name already taken (ErrAlreadyExists), not by a separate lookup
// that could race.
func containerName(appID, service string) string {
	if service != "" {
		return "dreamhub-app-" + appID + "-" + service
	}
	return "dreamhub-app-" + appID
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

// pullImage asks the daemon to pull spec's image and drains the
// (streamed, newline-delimited JSON progress) response fully before
// returning — Docker only guarantees the pull finished once that stream
// closes.
func (e Exec) pullImage(ctx context.Context, image string) error {
	resp, err := e.do(ctx, http.MethodPost, "/images/create?fromImage="+url.QueryEscape(image), nil)
	if err != nil {
		return err
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("dockerd: pull %s: unexpected status %d", image, resp.StatusCode)
	}
	// A failed pull (e.g. unknown tag) still answers 200 with an "error"
	// line partway through the stream rather than a non-200 status —
	// Docker's own streamed-progress convention — so the body is scanned
	// for one instead of trusted just because the status line was OK.
	var lastErr error
	dec := json.NewDecoder(resp.Body)
	for {
		var line struct {
			Error string `json:"error"`
		}
		if err := dec.Decode(&line); err != nil {
			break
		}
		if line.Error != "" {
			lastErr = errors.New(line.Error)
		}
	}
	if lastErr != nil {
		return fmt.Errorf("dockerd: pull %s: %w", image, lastErr)
	}
	return nil
}

func (e Exec) Create(ctx context.Context, spec ContainerSpec) (Container, error) {
	if spec.HostNetwork && spec.HostPort != spec.ContainerPort {
		return Container{}, fmt.Errorf("dockerd: create %s: on the host network the port can't be remapped", spec.AppID)
	}
	if !ValidService(spec.Service) {
		return Container{}, ErrInvalidService
	}
	name := containerName(spec.AppID, spec.Service)
	resp, err := e.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(name)+"/json", nil)
	if err != nil {
		return Container{}, err
	}
	drainClosed(resp.Body)
	if resp.StatusCode == http.StatusOK {
		return Container{}, ErrAlreadyExists
	}

	if err := e.pullImage(ctx, spec.Image); err != nil {
		return Container{}, err
	}

	env := make([]string, 0, len(spec.Env))
	for k, v := range spec.Env {
		env = append(env, k+"="+v)
	}
	exposed := map[string]any{}
	bindings := map[string]any{}
	labels := map[string]string{AppLabel: spec.AppID}
	if spec.Service != "" {
		labels[ServiceLabel] = spec.Service
	}
	if spec.Root != "" {
		labels[RootLabel] = spec.Root
	}
	if spec.HostNetwork && spec.ContainerPort != 0 {
		labels[PortLabel] = fmt.Sprint(spec.ContainerPort)
	}
	if spec.ContainerPort != 0 && !spec.HostNetwork {
		port := fmt.Sprintf("%d/tcp", spec.ContainerPort)
		exposed[port] = map[string]any{}
		bindings[port] = []map[string]string{{"HostPort": fmt.Sprint(spec.HostPort)}}
	}
	create := map[string]any{
		"Image":        spec.Image,
		"Env":          env,
		"ExposedPorts": exposed,
		"Labels":       labels,
		"HostConfig": map[string]any{
			"NetworkMode":  networkMode(spec.HostNetwork),
			"PortBindings": bindings,
			"Binds":        binds(spec),
			"Memory":       spec.MemoryLimitBytes,
			// A crash-looping app is reported, not restarted forever
			// (docs/APPSHUB_PLAN.md, "Autostart and crash-loop
			// protection") — App5 wires the notification; this caps
			// Docker's own retries either way.
			"RestartPolicy": map[string]any{"Name": "on-failure", "MaximumRetryCount": 5},
		},
	}
	if len(spec.Cmd) > 0 {
		create["Cmd"] = spec.Cmd
	}
	body, err := json.Marshal(create)
	if err != nil {
		return Container{}, err
	}
	resp, err = e.do(ctx, http.MethodPost, "/containers/create?name="+url.QueryEscape(name), bytes.NewReader(body))
	if err != nil {
		return Container{}, err
	}
	var created struct{ ID string }
	decErr := json.NewDecoder(resp.Body).Decode(&created)
	drainClosed(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return Container{}, fmt.Errorf("dockerd: create %s: unexpected status %d", spec.AppID, resp.StatusCode)
	}
	if decErr != nil {
		return Container{}, decErr
	}

	resp, err = e.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(created.ID)+"/start", nil)
	if err != nil {
		return Container{}, err
	}
	drainClosed(resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return Container{}, fmt.Errorf("dockerd: start newly created %s: unexpected status %d", spec.AppID, resp.StatusCode)
	}

	return Container{
		ID: created.ID, Name: name, Image: spec.Image, State: "running", HostPort: spec.HostPort,
		Labels: labels,
	}, nil
}

// binds is HostDir plus, when set, the data directory.
func binds(spec ContainerSpec) []string {
	b := []string{spec.HostDir + ":" + spec.ContainerDir}
	if spec.DataHostDir != "" && spec.DataContainerDir != "" {
		b = append(b, spec.DataHostDir+":"+spec.DataContainerDir)
	}
	return b
}

func (e Exec) Remove(ctx context.Context, id string) error {
	if err := e.requireOwned(ctx, id); err != nil {
		return err
	}
	resp, err := e.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/stop", nil)
	if err != nil {
		return err
	}
	drainClosed(resp.Body)
	// Stopping an already-stopped container 304s — fine, proceed to remove.

	resp, err = e.do(ctx, http.MethodDelete, "/containers/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("dockerd: remove: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (e Exec) ImageDigest(ctx context.Context, id string) (string, error) {
	resp, err := e.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/json", nil)
	if err != nil {
		return "", err
	}
	var insp struct {
		Image  string `json:"Image"` // the resolved image ID, e.g. "sha256:abcd…"
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	decErr := json.NewDecoder(resp.Body).Decode(&insp)
	drainClosed(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return "", ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("dockerd: inspect: unexpected status %d", resp.StatusCode)
	}
	if decErr != nil {
		return "", decErr
	}
	if _, ok := insp.Config.Labels[AppLabel]; !ok {
		return "", ErrNotFound
	}

	resp, err = e.do(ctx, http.MethodGet, "/images/"+url.PathEscape(insp.Image)+"/json", nil)
	if err != nil {
		return "", err
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("dockerd: image inspect: unexpected status %d", resp.StatusCode)
	}
	var img struct {
		RepoDigests []string `json:"RepoDigests"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&img); err != nil {
		return "", err
	}
	if len(img.RepoDigests) == 0 {
		return "", nil // locally built or never pulled from a registry — nothing to compare
	}
	// Docker's own "repo@sha256:…" shape; the digest is what dockerhub's
	// own Digest() answers in, so only the part after "@" is kept.
	if _, digest, ok := strings.Cut(img.RepoDigests[0], "@"); ok {
		return digest, nil
	}
	return img.RepoDigests[0], nil
}

// Attach opens an interactive PTY session inside a running container by
// hand-rolling Docker's own "docker exec -it" protocol: create an exec
// instance, then start it with Tty:true over a connection the caller
// keeps raw (Docker calls this a "hijacked" connection — once the HTTP
// response headers are read, the socket itself becomes a plain
// bidirectional byte stream, stdin in, stdout+stderr out, no further
// framing). The standard net/http client can't stay in that mode after a
// request, so this dials and writes the HTTP request by hand, the same
// way Create's pullImage and every other call here already avoid an SDK
// — just one step further, since this one needs the raw net.Conn itself
// afterwards rather than a parsed response.
func (e Exec) Attach(ctx context.Context, id string) (io.ReadWriteCloser, error) {
	if err := e.requireOwned(ctx, id); err != nil {
		return nil, err
	}
	createBody, err := json.Marshal(map[string]any{
		"Cmd": []string{"/bin/sh"}, "AttachStdin": true, "AttachStdout": true, "AttachStderr": true, "Tty": true,
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
		return nil, fmt.Errorf("dockerd: attach: exec create: unexpected status %d", resp.StatusCode)
	}
	if decErr != nil {
		return nil, decErr
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", e.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	startBody, err := json.Marshal(map[string]any{"Detach": false, "Tty": true})
	if err != nil {
		conn.Close()
		return nil, err
	}
	req := "POST /exec/" + created.ID + "/start HTTP/1.1\r\n" +
		"Host: docker\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Length: " + fmt.Sprint(len(startBody)) + "\r\n" +
		"Connection: Upgrade\r\n" +
		"Upgrade: tcp\r\n\r\n"
	// One write for headers and body: a daemon may hijack the connection
	// as soon as it has the headers, and a second write could then hit a
	// closed socket (seen as a flaky broken pipe in CI).
	if _, err := conn.Write(append([]byte(req), startBody...)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	br := bufio.NewReader(conn)
	startResp, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	startResp.Body.Close()
	// Docker answers either 101 (Upgrade: tcp, confirmed) or a plain 200
	// with "application/vnd.docker.raw-stream" (older daemons) — both
	// mean the connection is now raw.
	if startResp.StatusCode != http.StatusSwitchingProtocols && startResp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("dockerd: attach: exec start: unexpected status %d", startResp.StatusCode)
	}
	// br may already hold bytes the daemon sent right after the headers;
	// hijackedConn serves those first, then falls through to conn itself.
	return &hijackedConn{Reader: br, conn: conn}, nil
}

// hijackedConn is a raw Docker connection after its HTTP upgrade: reads
// drain whatever bufio.Reader buffered past the response headers first,
// then read straight from the underlying net.Conn; writes and Close go
// straight to it.
type hijackedConn struct {
	io.Reader
	conn net.Conn
}

func (h *hijackedConn) Write(p []byte) (int, error) { return h.conn.Write(p) }
func (h *hijackedConn) Close() error                { return h.conn.Close() }

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

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
	"slices"
	"strconv"
	"strings"
	"sync"
)

// imageMu keeps "remove unused images" from running while an install pulls
// an image that no container uses yet (pull, create and start are one Create
// call).
var imageMu sync.Mutex

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
	// Policy decides which host paths a container may mount; the zero
	// value is DefaultPolicy (/media only). The helper's own Server checks
	// every create request with its policy as well — the same rules,
	// enforced at both the HTTP surface and the socket.
	Policy MountPolicy
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
		cc := fromDocker(c)
		cc.RestartCount = e.restartCount(ctx, c.ID)
		out = append(out, cc)
	}
	return out, nil
}

// restartCount reads how often the daemon restarted a container (0 when it
// can't be read — the crash-loop notice then simply doesn't see it).
func (e Exec) restartCount(ctx context.Context, id string) int {
	resp, err := e.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/json", nil)
	if err != nil {
		return 0
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0
	}
	var insp struct {
		State struct {
			RestartCount int `json:"RestartCount"`
		} `json:"State"`
		RestartCount int `json:"RestartCount"` // older daemons keep it at the top
	}
	if json.NewDecoder(resp.Body).Decode(&insp) != nil {
		return 0
	}
	if insp.State.RestartCount > 0 {
		return insp.State.RestartCount
	}
	return insp.RestartCount
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
	imageMu.Lock()
	defer imageMu.Unlock()
	if spec.HostNetwork && spec.HostPort != spec.ContainerPort {
		return Container{}, fmt.Errorf("dockerd: create %s: on the host network the port can't be remapped", spec.AppID)
	}
	if err := ValidateSpec(spec, e.Policy); err != nil {
		return Container{}, err
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

	if spec.FromKept {
		if err := e.restoreKept(ctx, spec); err != nil {
			return Container{}, err
		}
	} else if err := e.pullImage(ctx, spec.Image); err != nil {
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
	if spec.Name != "" {
		labels[NameLabel] = spec.Name
	}
	if spec.HostNetwork && spec.ContainerPort != 0 {
		labels[PortLabel] = fmt.Sprint(spec.ContainerPort)
	}
	if spec.ContainerPort != 0 && !spec.HostNetwork {
		port := fmt.Sprintf("%d/tcp", spec.ContainerPort)
		exposed[port] = map[string]any{}
		bindings[port] = []map[string]string{{"HostPort": fmt.Sprint(spec.HostPort)}}
	}
	hostConfig := map[string]any{
		"NetworkMode":  networkMode(spec.HostNetwork),
		"PortBindings": bindings,
		"Binds":        binds(spec),
		"Memory":       spec.MemoryLimitBytes,
		// A crash-looping app is reported, not restarted forever
		// (docs/APPSHUB_PLAN.md, "Autostart and crash-loop
		// protection") — App5 wires the notification; this caps
		// Docker's own retries either way.
		"RestartPolicy": map[string]any{"Name": "on-failure", "MaximumRetryCount": 5},
	}
	// Only ever reached with Elevated.Confirmed (ValidateSpec above).
	if el := spec.Elevated; el.Any() {
		if el.Privileged {
			hostConfig["Privileged"] = true
		}
		if len(el.CapAdd) > 0 {
			hostConfig["CapAdd"] = el.CapAdd
		}
		if len(el.Devices) > 0 {
			devs := make([]map[string]string, 0, len(el.Devices))
			for _, d := range el.Devices {
				parts := strings.Split(d, ":")
				dev := map[string]string{"PathOnHost": parts[0], "PathInContainer": parts[0], "CgroupPermissions": "rwm"}
				if len(parts) > 1 {
					dev["PathInContainer"] = parts[1]
				}
				if len(parts) > 2 {
					dev["CgroupPermissions"] = parts[2]
				}
				devs = append(devs, dev)
			}
			hostConfig["Devices"] = devs
		}
		if el.PidHost {
			hostConfig["PidMode"] = "host"
		}
		if el.IpcHost {
			hostConfig["IpcMode"] = "host"
		}
	}
	create := map[string]any{
		"Image":        spec.Image,
		"Env":          env,
		"ExposedPorts": exposed,
		"Labels":       labels,
		"HostConfig":   hostConfig,
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

// binds is the bind mounts as Docker's "host:container[:ro]" strings; the
// paths were checked by ValidateSpec.
func binds(spec ContainerSpec) []string {
	b := make([]string, 0, len(spec.Mounts))
	for _, m := range spec.Mounts {
		s := m.HostDir + ":" + m.ContainerDir
		if m.ReadOnly {
			s += ":ro"
		}
		b = append(b, s)
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

// imageRow is one entry of the daemon's image list.
type imageRow struct {
	ID         string   `json:"Id"`
	RepoTags   []string `json:"RepoTags"`
	Size       int64    `json:"Size"`
	SharedSize int64    `json:"SharedSize"`
}

func (r imageRow) own() int64 {
	own := r.Size
	if r.SharedSize > 0 {
		own -= r.SharedSize
	}
	if own < 0 {
		own = 0
	}
	return own
}

// kept reports whether the image carries a rollback name.
func (r imageRow) kept() bool {
	for _, t := range r.RepoTags {
		if _, _, ok := keptOf(t); ok {
			return true
		}
	}
	return false
}

func (e Exec) listImages(ctx context.Context) ([]imageRow, error) {
	resp, err := e.do(ctx, http.MethodGet, "/images/json?shared-size=1", nil)
	if err != nil {
		return nil, err
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dockerd: list images: unexpected status %d", resp.StatusCode)
	}
	var imgs []imageRow
	if err := json.NewDecoder(resp.Body).Decode(&imgs); err != nil {
		return nil, err
	}
	return imgs, nil
}

// unusedRows are the images no container (of any kind, running or stopped)
// uses, kept rollback images aside. Dangling or tagged alike; intermediate
// layers the daemon hides by default stay hidden.
func (e Exec) unusedRows(ctx context.Context) ([]imageRow, error) {
	resp, err := e.do(ctx, http.MethodGet, "/containers/json?all=1", nil)
	if err != nil {
		return nil, err
	}
	var cs []struct {
		ImageID string `json:"ImageID"`
	}
	decErr := json.NewDecoder(resp.Body).Decode(&cs)
	drainClosed(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dockerd: list containers: unexpected status %d", resp.StatusCode)
	}
	if decErr != nil {
		return nil, decErr
	}
	used := map[string]bool{}
	for _, c := range cs {
		used[c.ImageID] = true
	}
	imgs, err := e.listImages(ctx)
	if err != nil {
		return nil, err
	}
	var out []imageRow
	for _, i := range imgs {
		if !used[i.ID] && !i.kept() {
			out = append(out, i)
		}
	}
	return out, nil
}

func (e Exec) UnusedImages(ctx context.Context) ([]Image, error) {
	rows, err := e.unusedRows(ctx)
	if err != nil {
		return nil, err
	}
	out := []Image{}
	for _, i := range rows {
		tags := i.RepoTags
		if tags == nil {
			tags = []string{}
		}
		out = append(out, Image{ID: i.ID, Tags: tags, SizeBytes: i.own()})
	}
	return out, nil
}

// removeImage deletes one image by id (forced: it is unused, and an image
// with two names would otherwise refuse), reporting whether the daemon
// really deleted it.
func (e Exec) removeImage(ctx context.Context, ref string) (bool, error) {
	resp, err := e.do(ctx, http.MethodDelete, "/images/"+refPath(ref)+"?force=1", nil)
	if err != nil {
		return false, err
	}
	defer drainClosed(resp.Body)
	switch resp.StatusCode {
	case http.StatusOK:
		var out []struct {
			Deleted string `json:"Deleted"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		for _, d := range out {
			if d.Deleted != "" {
				return true, nil
			}
		}
		return false, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("dockerd: remove image: unexpected status %d", resp.StatusCode)
	}
}

// PruneImages removes every unused image — what UnusedImages lists, nothing
// a caller selects — except the images kept for rollback. (Docker's own
// "image prune -a" can't leave those out, so the list is walked instead.)
func (e Exec) PruneImages(ctx context.Context) (PruneResult, error) {
	imageMu.Lock()
	defer imageMu.Unlock()
	rows, err := e.unusedRows(ctx)
	if err != nil {
		return PruneResult{}, err
	}
	var res PruneResult
	for _, i := range rows {
		deleted, err := e.removeImage(ctx, i.ID)
		if err != nil {
			return res, err
		}
		if deleted {
			res.Removed++
			res.ReclaimedBytes += i.own()
		}
	}
	return res, nil
}

// imageIDOf reads the image id and labels of a container the helper owns.
func (e Exec) imageIDOf(ctx context.Context, id string) (imageID string, labels map[string]string, origin string, err error) {
	resp, err := e.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/json", nil)
	if err != nil {
		return "", nil, "", err
	}
	var insp struct {
		Image  string `json:"Image"`
		Config struct {
			Image  string            `json:"Image"`
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	decErr := json.NewDecoder(resp.Body).Decode(&insp)
	drainClosed(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return "", nil, "", ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, "", fmt.Errorf("dockerd: inspect: unexpected status %d", resp.StatusCode)
	}
	if decErr != nil {
		return "", nil, "", decErr
	}
	if _, ok := insp.Config.Labels[AppLabel]; !ok {
		return "", nil, "", ErrNotFound
	}
	return insp.Image, insp.Config.Labels, insp.Config.Image, nil
}

// tagImage gives an image another name.
func (e Exec) tagImage(ctx context.Context, image, ref string) error {
	repo, tag := splitRef(ref)
	resp, err := e.do(ctx, http.MethodPost, "/images/"+refPath(image)+"/tag?"+url.Values{"repo": {repo}, "tag": {tag}}.Encode(), nil)
	if err != nil {
		return err
	}
	defer drainClosed(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("dockerd: tag image: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// refPath is an image name or id as a URL path: the slashes in a repository
// name stay slashes, as the daemon's own CLI sends them.
func refPath(ref string) string { return strings.ReplaceAll(url.PathEscape(ref), "%2F", "/") }

// splitRef splits "registry:5000/repo/name:tag" at the tag; no tag is latest.
func splitRef(ref string) (repo, tag string) {
	slash := strings.LastIndex(ref, "/")
	if colon := strings.LastIndex(ref, ":"); colon > slash {
		return ref[:colon], ref[colon+1:]
	}
	return ref, "latest"
}

func (e Exec) KeepImage(ctx context.Context, id string) (string, error) {
	imageMu.Lock()
	defer imageMu.Unlock()
	imageID, labels, origin, err := e.imageIDOf(ctx, id)
	if err != nil {
		return "", err
	}
	app, svc := labels[AppLabel], labels[ServiceLabel]
	// One level: what was kept before gives up its origin name (the image
	// keeps its :previous name until that moves below).
	if err := e.untagOrigins(ctx, app, svc); err != nil {
		return "", err
	}
	if err := e.tagImage(ctx, imageID, KeptRef(app, svc)); err != nil {
		return "", err
	}
	if origin != "" {
		repo, _ := splitRef(KeptRef(app, svc))
		if err := e.tagImage(ctx, imageID, repo+":"+originTag(origin)); err != nil {
			return "", err
		}
	}
	return imageID, nil
}

// untagOrigins removes the origin names of an app's kept image.
func (e Exec) untagOrigins(ctx context.Context, app, svc string) error {
	imgs, err := e.listImages(ctx)
	if err != nil {
		return err
	}
	repo, _ := splitRef(KeptRef(app, svc))
	for _, i := range imgs {
		for _, t := range i.RepoTags {
			if strings.HasPrefix(t, repo+":origin-") {
				if _, err := e.removeImage(ctx, t); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (e Exec) KeptImages(ctx context.Context) ([]KeptImage, error) {
	imgs, err := e.listImages(ctx)
	if err != nil {
		return nil, err
	}
	out := []KeptImage{}
	for _, i := range imgs {
		for _, t := range i.RepoTags {
			if app, svc, ok := keptOf(t); ok {
				out = append(out, KeptImage{AppID: app, Service: svc, ImageID: i.ID, SizeBytes: i.own()})
			}
		}
	}
	return out, nil
}

func (e Exec) DropKeptImages(ctx context.Context, appID string) (PruneResult, error) {
	imageMu.Lock()
	defer imageMu.Unlock()
	if appID != "" && !appIDRe.MatchString(appID) {
		return PruneResult{}, refuse("invalid", "", "the app id %q is not valid", appID)
	}
	imgs, err := e.listImages(ctx)
	if err != nil {
		return PruneResult{}, err
	}
	var res PruneResult
	for _, i := range imgs {
		var names []string // origin names first, :previous last — that one takes the image with it
		var previous string
		app := ""
		for _, t := range i.RepoTags {
			if a, _, ok := keptOf(t); ok {
				previous, app = t, a
			} else if strings.HasPrefix(t, RollbackRepo) {
				names = append(names, t)
			}
		}
		if previous == "" || (appID != "" && app != appID) {
			continue
		}
		names = append(names, previous)
		deleted := false
		for _, n := range names {
			d, err := e.removeImage(ctx, n)
			if err != nil {
				return res, err
			}
			deleted = deleted || d
		}
		if deleted {
			res.Removed++
			res.ReclaimedBytes += i.own()
		}
	}
	return res, nil
}

// restoreKept gives the kept image of spec's app and service the name
// spec.Image has, so a container created from that name runs it. Only the
// name the image was kept under is accepted (see originTag).
func (e Exec) restoreKept(ctx context.Context, spec ContainerSpec) error {
	kept := KeptRef(spec.AppID, spec.Service)
	resp, err := e.do(ctx, http.MethodGet, "/images/"+refPath(kept)+"/json", nil)
	if err != nil {
		return err
	}
	var insp struct {
		RepoTags []string `json:"RepoTags"`
	}
	decErr := json.NewDecoder(resp.Body).Decode(&insp)
	drainClosed(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return ErrNoKeptImage
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("dockerd: inspect kept image: unexpected status %d", resp.StatusCode)
	}
	if decErr != nil {
		return decErr
	}
	repo, _ := splitRef(kept)
	want := repo + ":" + originTag(spec.Image)
	if !slices.Contains(insp.RepoTags, want) {
		return refuse("invalid", "", "the kept image of %s was not in use as %s", spec.AppID, spec.Image)
	}
	return e.tagImage(ctx, kept, spec.Image)
}

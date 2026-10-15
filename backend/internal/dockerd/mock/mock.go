// Package mock implements dockerd.Client without a real Docker daemon —
// two demo containers (one running, one stopped) plus working, in-memory
// Create/Remove, so AppsHub's "Apps" tab has something to show and
// install against in mock mode and development (PRINCIPLES.md,
// "Mock-first development").
package mock

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	dockerhubmock "github.com/antonbier/DreamHub/backend/internal/dockerhub/mock"
)

// Client is a deterministic dockerd.Client over an in-memory set of
// containers, keyed by AppID (App4's Create is one container per AppID,
// same as the real Exec).
type Client struct {
	// Policy is the mount policy Create applies, like the real helper;
	// the zero value is the helper's own (/media only).
	Policy     dockerd.MountPolicy
	mu         sync.Mutex
	containers map[string]*dockerd.Container
	// images are the demo images no container uses (what "clean up unused
	// images" removes).
	images []dockerd.Image

	// What each container runs (an image id and the registry digest it was
	// pulled at), and the images kept for rollback by app[.service] — the
	// same bookkeeping the real helper does with image names.
	imageOf  map[string]string
	digestOf map[string]string
	kept     map[string]keptEntry
	pulls    int

	// Hooks for tests and demos; all optional. FailCreate makes a create
	// fail with that error (a failed pull, say); ExitAfterCreate makes the
	// new container stop right after it started (a crash); FailKeep makes
	// KeepImage fail.
	FailCreate      func(spec dockerd.ContainerSpec) error
	ExitAfterCreate func(spec dockerd.ContainerSpec) bool
	FailKeep        error
	// Calls records the mutating calls in order ("keep pihole", "create pihole",
	// "create pihole from kept", "remove pihole", "drop pihole"), for tests.
	Calls []string
}

type keptEntry struct {
	imageID, digest, origin string
	size                    int64
}

func keptName(appID, service string) string {
	if service != "" {
		return appID + "." + service
	}
	return appID
}

var _ dockerd.Client = (*Client)(nil)

// New returns a mock client seeded with two demo apps — the same two
// catalog entries App3's own mock Docker Hub fixtures cover
// (internal/dockerhub/mock), so a mock-mode session can see them as
// "already installed" consistently across both.
func New() *Client {
	return &Client{imageOf: map[string]string{"pihole": "sha256:mock-pihole-0", "adguardhome": "sha256:mock-adguardhome-0"},
		digestOf: map[string]string{"pihole": "sha256:pihole-pulled-old", "adguardhome": "sha256:adguardhome-current"},
		kept:     map[string]keptEntry{}, images: []dockerd.Image{
			{ID: "sha256:aaaa1111", Tags: []string{"pihole/pihole:2024.01.0"}, SizeBytes: 214 << 20},
			{ID: "sha256:bbbb2222", Tags: []string{"nextcloud:27"}, SizeBytes: 395 << 20},
			{ID: "sha256:cccc3333", Tags: []string{}, SizeBytes: 38 << 20},
		}, containers: map[string]*dockerd.Container{
			"pihole": {
				ID: "pihole", Name: "dreamhub-app-pihole", Image: "pihole/pihole:latest",
				State: "running", Status: "Up 2 hours", HostPort: 8082,
				Labels: map[string]string{dockerd.AppLabel: "pihole"},
			},
			"adguardhome": {
				ID: "adguardhome", Name: "dreamhub-app-adguardhome", Image: "adguard/adguardhome:latest",
				State: "exited", Status: "Exited (0) 10 minutes ago", HostPort: 3000,
				Labels: map[string]string{dockerd.AppLabel: "adguardhome"},
			},
		}}
}

func (c *Client) List(ctx context.Context) ([]dockerd.Container, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]dockerd.Container, 0, len(c.containers))
	for _, cc := range c.containers {
		out = append(out, *cc)
	}
	return out, nil
}

func (c *Client) get(id string) (*dockerd.Container, error) {
	cc, ok := c.containers[id]
	if !ok {
		return nil, dockerd.ErrNotFound
	}
	return cc, nil
}

func (c *Client) Start(ctx context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cc, err := c.get(id)
	if err != nil {
		return err
	}
	cc.State, cc.Status = "running", "Up less than a second"
	return nil
}

func (c *Client) Stop(ctx context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cc, err := c.get(id)
	if err != nil {
		return err
	}
	cc.State, cc.Status = "exited", "Exited (0) less than a second ago"
	return nil
}

func (c *Client) Logs(ctx context.Context, id string, tail int) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.get(id); err != nil {
		return nil, err
	}
	return []byte("[mock] " + id + ": no real log output without a Docker daemon\n"), nil
}

func (c *Client) Exec(ctx context.Context, id string, cmd []string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.get(id); err != nil {
		return nil, err
	}
	return []byte("[mock] exec not run against a real container\n"), nil
}

func (c *Client) Create(ctx context.Context, spec dockerd.ContainerSpec) (dockerd.Container, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Keyed by AppID, not Docker's own container id (there's no real
	// daemon to hand one out) — List/get use the same id scheme the real
	// Exec's /containers/{id}/json inspect expects an id for, which is
	// fine since AppID-as-id is unique per app the same way a real
	// container id is unique per container.
	// The same checks the real helper makes, so mock mode shows the same
	// refusals.
	if err := dockerd.ValidateSpec(spec, c.Policy); err != nil {
		return dockerd.Container{}, err
	}
	id := spec.AppID
	labels := map[string]string{dockerd.AppLabel: spec.AppID}
	if spec.Service != "" {
		id += "-" + spec.Service
		labels[dockerd.ServiceLabel] = spec.Service
	}
	if spec.Root != "" {
		labels[dockerd.RootLabel] = spec.Root
	}
	if spec.Name != "" {
		labels[dockerd.NameLabel] = spec.Name
	}
	if _, exists := c.containers[id]; exists {
		return dockerd.Container{}, dockerd.ErrAlreadyExists
	}
	if c.FailCreate != nil {
		if err := c.FailCreate(spec); err != nil {
			return dockerd.Container{}, err
		}
	}
	c.pulls++
	if spec.FromKept {
		k, ok := c.kept[keptName(spec.AppID, spec.Service)]
		if !ok {
			return dockerd.Container{}, dockerd.ErrNoKeptImage
		}
		if k.origin != spec.Image {
			return dockerd.Container{}, &dockerd.SpecError{Code: "invalid", Message: "the kept image was not in use as " + spec.Image}
		}
		c.imageOf[id], c.digestOf[id] = k.imageID, k.digest
		c.Calls = append(c.Calls, "create "+id+" from kept")
	} else {
		image, tag, _ := strings.Cut(spec.Image, ":")
		digest := "sha256:mock-pulled-" + fmt.Sprint(c.pulls)
		if d, err := (dockerhubmock.Client{}).Digest(context.Background(), image, tag); err == nil && d != "" {
			digest = d
		}
		c.imageOf[id], c.digestOf[id] = fmt.Sprintf("sha256:mock-%s-%d", id, c.pulls), digest
		c.Calls = append(c.Calls, "create "+id)
	}
	created := dockerd.Container{
		ID: id, Name: "dreamhub-app-" + id, Image: spec.Image,
		State: "running", Status: "Up less than a second", HostPort: spec.HostPort,
		Labels: labels,
	}
	if c.ExitAfterCreate != nil && c.ExitAfterCreate(spec) {
		created.State, created.Status = "exited", "Exited (1) less than a second ago"
	}
	c.containers[id] = &created
	return created, nil
}

// The seeded digests: pihole's deliberately lags behind the dockerhub mock
// fixture's current digest (internal/dockerhub/mock, "sha256:pihole-upstream-new"),
// so mock mode has a real "update available" case to develop and demo the
// watch (App5) against; adguardhome's matches, an "already current" case.
// An update (a create that pulls) brings the container to the fixture's digest.

func (c *Client) ImageDigest(ctx context.Context, id string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.get(id); err != nil {
		return "", err
	}
	return c.digestOf[id], nil
}

// mockPTY is a trivial in-memory console: writes are echoed back with a
// "$ " prompt prefix, close is a close — enough to prove the WS↔PTY
// bridge end to end (App5) without a real Docker daemon. Read blocks on
// cond until there's something to drain or Close wakes it, the same
// "a reader that can legitimately have nothing yet" shape a real PTY has
// — returning (0, nil) instead would make an io.Copy-driven bridge spin.
type mockPTY struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    bytes.Buffer
	closed bool
}

func newMockPTY() *mockPTY {
	p := &mockPTY{}
	p.cond = sync.NewCond(&p.mu)
	p.buf.WriteString("[mock] attached — no real shell without a Docker daemon\r\n$ ")
	return p
}

func (p *mockPTY) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for p.buf.Len() == 0 && !p.closed {
		p.cond.Wait()
	}
	if p.buf.Len() == 0 {
		return 0, io.EOF
	}
	return p.buf.Read(b)
}

func (p *mockPTY) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, io.ErrClosedPipe
	}
	p.buf.Write(b)
	p.buf.WriteString("\r\n$ ")
	p.cond.Signal()
	return len(b), nil
}

func (p *mockPTY) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.cond.Broadcast()
	return nil
}

func (c *Client) Attach(ctx context.Context, id string) (io.ReadWriteCloser, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.get(id); err != nil {
		return nil, err
	}
	return newMockPTY(), nil
}

func (c *Client) Remove(ctx context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.get(id); err != nil {
		return err
	}
	delete(c.containers, id)
	delete(c.imageOf, id)
	delete(c.digestOf, id)
	c.Calls = append(c.Calls, "remove "+id)
	return nil
}

// SetState changes a container's state and restart count (tests: a crash, a
// crash loop).
func (c *Client) SetState(id, state string, restarts int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cc, ok := c.containers[id]; ok {
		cc.State, cc.Status, cc.RestartCount = state, state, restarts
	}
}

func (c *Client) KeepImage(ctx context.Context, id string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cc, err := c.get(id)
	if err != nil {
		return "", err
	}
	if c.FailKeep != nil {
		return "", c.FailKeep
	}
	app, svc := cc.Labels[dockerd.AppLabel], cc.Labels[dockerd.ServiceLabel]
	c.kept[keptName(app, svc)] = keptEntry{imageID: c.imageOf[id], digest: c.digestOf[id], origin: cc.Image, size: 120 << 20}
	c.Calls = append(c.Calls, "keep "+id)
	return c.imageOf[id], nil
}

func (c *Client) KeptImages(ctx context.Context) ([]dockerd.KeptImage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []dockerd.KeptImage{}
	for name, k := range c.kept {
		app, svc, _ := strings.Cut(name, ".")
		out = append(out, dockerd.KeptImage{AppID: app, Service: svc, ImageID: k.imageID, SizeBytes: k.size})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AppID+out[i].Service < out[j].AppID+out[j].Service })
	return out, nil
}

func (c *Client) DropKeptImages(ctx context.Context, appID string) (dockerd.PruneResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var res dockerd.PruneResult
	for name, k := range c.kept {
		app, _, _ := strings.Cut(name, ".")
		if appID != "" && app != appID {
			continue
		}
		delete(c.kept, name)
		res.Removed++
		res.ReclaimedBytes += k.size
		c.Calls = append(c.Calls, "drop "+app)
	}
	return res, nil
}

func (c *Client) UnusedImages(ctx context.Context) ([]dockerd.Image, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]dockerd.Image{}, c.images...), nil
}

func (c *Client) PruneImages(ctx context.Context) (dockerd.PruneResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var res dockerd.PruneResult
	for _, i := range c.images {
		res.Removed++
		res.ReclaimedBytes += i.SizeBytes
	}
	c.images = nil
	return res, nil
}

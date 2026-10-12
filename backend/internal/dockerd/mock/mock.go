// Package mock implements dockerd.Client without a real Docker daemon —
// two demo containers (one running, one stopped) plus working, in-memory
// Create/Remove, so AppsHub's "Apps" tab has something to show and
// install against in mock mode and development (PRINCIPLES.md,
// "Mock-first development").
package mock

import (
	"context"
	"sync"

	"github.com/antonbier/DreamHub/backend/internal/dockerd"
)

// Client is a deterministic dockerd.Client over an in-memory set of
// containers, keyed by AppID (App4's Create is one container per AppID,
// same as the real Exec).
type Client struct {
	mu         sync.Mutex
	containers map[string]*dockerd.Container
}

var _ dockerd.Client = (*Client)(nil)

// New returns a mock client seeded with two demo apps — the same two
// catalog entries App3's own mock Docker Hub fixtures cover
// (internal/dockerhub/mock), so a mock-mode session can see them as
// "already installed" consistently across both.
func New() *Client {
	return &Client{containers: map[string]*dockerd.Container{
		"pihole": {
			ID: "pihole", Name: "dreamhub-app-pihole", Image: "pihole/pihole:latest",
			State: "running", Status: "Up 2 hours", HostPort: 8081,
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
	if _, exists := c.containers[spec.AppID]; exists {
		return dockerd.Container{}, dockerd.ErrAlreadyExists
	}
	created := dockerd.Container{
		ID: spec.AppID, Name: "dreamhub-app-" + spec.AppID, Image: spec.Image,
		State: "running", Status: "Up less than a second", HostPort: spec.HostPort,
		Labels: map[string]string{dockerd.AppLabel: spec.AppID},
	}
	c.containers[spec.AppID] = &created
	return created, nil
}

func (c *Client) Remove(ctx context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.get(id); err != nil {
		return err
	}
	delete(c.containers, id)
	return nil
}

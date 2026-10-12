// Package mock implements dockerd.Client without a real Docker daemon —
// two demo containers (one running, one stopped), so AppsHub's "Apps" tab
// has something to show in mock mode and development (PRINCIPLES.md,
// "Mock-first development") before App3/App4 build real install/run.
package mock

import (
	"context"
	"sync"

	"github.com/antonbier/DreamHub/backend/internal/dockerd"
)

// Client is a deterministic dockerd.Client over a fixed, in-memory set
// of containers.
type Client struct {
	mu         sync.Mutex
	containers map[string]*dockerd.Container
}

var _ dockerd.Client = (*Client)(nil)

// New returns a mock client seeded with two demo apps.
func New() *Client {
	return &Client{containers: map[string]*dockerd.Container{
		"pihole": {
			ID: "pihole", Name: "pihole", Image: "linuxserver/pihole:latest",
			State: "running", Status: "Up 2 hours",
			Labels: map[string]string{dockerd.AppLabel: "pihole"},
		},
		"adguard": {
			ID: "adguard", Name: "adguard", Image: "linuxserver/adguardhome:latest",
			State: "exited", Status: "Exited (0) 10 minutes ago",
			Labels: map[string]string{dockerd.AppLabel: "adguard"},
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

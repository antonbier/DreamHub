// Package mock implements autotimer.Client without a box: either with a
// small example configuration, or — NotInstalled — like a box without the
// plugin, so "optional plugin missing" is testable (docs/archive/V1_PLAN.md, rule 1).
package mock

import (
	"context"

	"github.com/antonbier/DreamHub/backend/internal/autotimer"
)

// Client is a fixed AutoTimer configuration.
type Client struct {
	// NotInstalled makes every call fail like a box without the plugin.
	NotInstalled bool
}

var _ autotimer.Client = (*Client)(nil)

// New returns a mock with one example AutoTimer.
func New() *Client { return &Client{} }

func (c *Client) List(ctx context.Context) ([]autotimer.AutoTimer, error) {
	if c.NotInstalled {
		return nil, autotimer.ErrNotInstalled
	}
	return []autotimer.AutoTimer{{
		ID: 1, Name: "Tatort", Match: "Tatort", Enabled: true,
		Services: []autotimer.Service{{ServiceReference: "1:0:19:283D:3FB:1:C00000:0:0:0:", ServiceName: "Das Erste HD"}},
	}}, nil
}

func (c *Client) Simulate(ctx context.Context) ([]autotimer.SimulatedTimer, error) {
	if c.NotInstalled {
		return nil, autotimer.ErrNotInstalled
	}
	return []autotimer.SimulatedTimer{}, nil
}

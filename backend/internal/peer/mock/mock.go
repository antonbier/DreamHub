// Package mock is a fake DreamHub on another box, for mock mode and tests:
// it accepts one fixed code and answers like a Full box.
package mock

import (
	"context"
	"strings"
	"sync"

	"github.com/antonbier/DreamHub/backend/internal/peer"
)

// Code is the pairing code the mock box "shows".
const Code = "1234 5678"

// Token is what pairing with the mock yields.
const Token = "dh_mockpeertoken"

// Client is a mock peer.Client.
type Client struct {
	mu      sync.Mutex
	Info_   peer.Info
	Caps    []string
	Revoked bool
	// Down makes every call fail like an unreachable box.
	Down bool
}

// New returns a mock whose DreamHub speaks the current API level.
func New() *Client {
	return &Client{
		Info_: peer.Info{Version: "0.8.0", API: peer.APILevel, MinHubAPI: 1, DeviceName: "Dreambox ONE (mock)", Pairing: true},
		Caps:  []string{"webcontrol", "recordings.local", "packages", "services", "logs", "terminal", "docker", "extensions"},
	}
}

func (c *Client) Info(context.Context, string) (peer.Info, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Down {
		return peer.Info{}, peer.ErrUnreachable
	}
	return c.Info_, nil
}

func (c *Client) Pair(_ context.Context, _ string, code, _ string) (peer.PairResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Down {
		return peer.PairResult{}, peer.ErrUnreachable
	}
	if strings.ReplaceAll(code, " ", "") != strings.ReplaceAll(Code, " ", "") {
		return peer.PairResult{}, peer.ErrBadCode
	}
	return peer.PairResult{Token: Token, Info: c.Info_}, nil
}

func (c *Client) Capabilities(_ context.Context, _ string, token string) (peer.Capabilities, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.Down:
		return peer.Capabilities{}, peer.ErrUnreachable
	case c.Revoked || token != Token:
		return peer.Capabilities{}, peer.ErrUnauthorized
	}
	return peer.Capabilities{Info: c.Info_, Capabilities: c.Caps}, nil
}

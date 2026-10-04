// Package mock implements push.Client by remembering what was sent.
package mock

import (
	"context"
	"sync"

	"github.com/antonbier/DreamHub/backend/internal/push"
)

type Client struct {
	mu   sync.Mutex
	Sent []push.Message
}

func New() *Client { return &Client{} }

var _ push.Client = (*Client)(nil)

func (c *Client) Send(ctx context.Context, cfg push.Config, m push.Message) error {
	if err := cfg.Check(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Sent = append(c.Sent, m)
	return nil
}

// Messages returns a copy of what was sent so far.
func (c *Client) Messages() []push.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]push.Message(nil), c.Sent...)
}

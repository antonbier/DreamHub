// Package push sends DreamHub's notifications to the user's phone through
// ntfy (https://ntfy.sh, or a self-hosted server): DreamHub publishes a
// short message to a topic, the ntfy app on the phone subscribed to that
// topic shows it. No account, no app of DreamHub's own, nothing to open on
// the router — the box only needs to reach the ntfy server.
//
// It only forwards what the core event bus (internal/events) already
// detects — a recording finished, the disk filling up, a timer conflict,
// a missing episode airing — "detection, not automation": nothing on the
// box changes because of a push.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Config is where to publish.
type Config struct {
	Server string // "https://ntfy.sh"
	Topic  string
	Token  string // optional access token for a protected topic
}

// Message is one notification.
type Message struct {
	Title    string
	Body     string
	Tags     []string // ntfy tags; emoji short codes show as icons ("tv")
	Priority int      // 1-5, 0 = default (3)
}

// Client is implemented by HTTPClient and ./mock.
type Client interface {
	Send(ctx context.Context, cfg Config, m Message) error
}

var (
	ErrNotConfigured = errors.New("push: no topic configured")
	ErrInvalidConfig = errors.New("push: invalid server or topic")
)

var validTopic = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Check validates a configuration: an http(s) server and ntfy's own topic
// syntax.
func (c Config) Check() error {
	if c.Topic == "" {
		return ErrNotConfigured
	}
	u, err := url.Parse(c.Server)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || !validTopic.MatchString(c.Topic) {
		return ErrInvalidConfig
	}
	return nil
}

// HTTPClient publishes with ntfy's JSON API (a POST to the server root),
// which carries UTF-8 titles cleanly, unlike HTTP headers.
type HTTPClient struct{ HTTP *http.Client }

func NewHTTPClient() *HTTPClient { return &HTTPClient{HTTP: &http.Client{Timeout: 10 * time.Second}} }

func (c *HTTPClient) Send(ctx context.Context, cfg Config, m Message) error {
	if err := cfg.Check(); err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{
		"topic": cfg.Topic, "title": m.Title, "message": m.Body, "tags": m.Tags, "priority": m.Priority,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.Server, "/")+"/", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("push: reaching %s: %w", cfg.Server, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("push: %s answered status %d", cfg.Server, resp.StatusCode)
	}
	return nil
}

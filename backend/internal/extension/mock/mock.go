// Package mock implements extension.Client without a real extension
// process — a demo "smart socket" (status: on/off, one toggle action),
// deliberately the same smallest-possible shape
// docs/EXTENSIONHUB_PLAN.md names for the first real reference extension
// (eWeLink/Sonoff), so the Extensions nav entry has something to show in
// mock mode and development, per PRINCIPLES.md's "Mock-first development".
// Registered directly into the registry (app.go) rather than over the
// real loopback handshake, since there's no separate process to run one
// from.
package mock

import (
	"context"
	"strconv"
	"sync"

	"github.com/antonbier/DreamHub/backend/internal/extension"
)

const actionToggle = "toggle"

// Client is a deterministic extension.Client: one socket, flips on Apply.
type Client struct {
	mu  sync.Mutex
	on  bool
	rev int // bumped on every flip, the fingerprint
}

var _ extension.Client = (*Client)(nil)

// New returns a mock client, socket on.
func New() *Client { return &Client{on: true} }

func (c *Client) Widgets(ctx context.Context) ([]extension.Widget, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, tone := "Aus", extension.ToneBad
	label := "Einschalten"
	if c.on {
		value, tone, label = "An", extension.ToneGood, "Ausschalten"
	}
	return []extension.Widget{
		{Type: extension.WidgetStatus, Label: "Steckdose", Value: value, Tone: tone},
		{Type: extension.WidgetAction, Label: "Steckdose", ActionID: actionToggle, ButtonLabel: label},
	}, nil
}

func (c *Client) PlanAction(ctx context.Context, actionID string) (extension.ActionPreview, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if actionID != actionToggle {
		return extension.ActionPreview{}, extensionUnknownAction(actionID)
	}
	summary := "Steckdose wird ausgeschaltet."
	if !c.on {
		summary = "Steckdose wird eingeschaltet."
	}
	return extension.ActionPreview{Summary: summary, Fingerprint: fingerprint(c.on, c.rev)}, nil
}

func (c *Client) Apply(ctx context.Context, actionID, fp string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if actionID != actionToggle {
		return extensionUnknownAction(actionID)
	}
	if fp != fingerprint(c.on, c.rev) {
		return extension.ErrStale
	}
	c.on = !c.on
	c.rev++
	return nil
}

func fingerprint(on bool, rev int) string {
	if on {
		return "on-" + strconv.Itoa(rev)
	}
	return "off-" + strconv.Itoa(rev)
}

type unknownActionError string

func (e unknownActionError) Error() string { return "extension/mock: unknown action " + string(e) }

func extensionUnknownAction(id string) error { return unknownActionError(id) }

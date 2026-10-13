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
	mu   sync.Mutex
	name string
	key  string
	on   bool
	rev  int // bumped on every flip, the fingerprint
}

var (
	_ extension.Client         = (*Client)(nil)
	_ extension.SettingsClient = (*Client)(nil)
)

// New returns a mock client, socket on.
func New() *Client { return &Client{on: true, name: "Steckdose"} }

func (c *Client) Widgets(ctx context.Context) ([]extension.Widget, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, tone := "Aus", extension.ToneBad
	label := "Einschalten"
	if c.on {
		value, tone, label = "An", extension.ToneGood, "Ausschalten"
	}
	return []extension.Widget{
		{Type: extension.WidgetStatus, Label: c.name, Value: value, Tone: tone},
		{Type: extension.WidgetAction, Label: c.name, ActionID: actionToggle, ButtonLabel: label},
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

// Settings is a demo form: a name and a secret key, kept in memory.
func (c *Client) Settings(context.Context) ([]extension.SettingField, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return []extension.SettingField{
		{Key: "name", Label: "Name", Type: extension.SettingText, Value: c.name},
		{Key: "key", Label: "Gerätekey", Type: extension.SettingSecret, Set: c.key != "", Help: "Aus dem Konto des Herstellers."},
	}, nil
}

func (c *Client) SaveSettings(_ context.Context, values map[string]string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := values["name"]; ok {
		if v == "" {
			return &extension.SettingsError{Message: "Der Name darf nicht leer sein."}
		}
		c.name = v
	}
	if v := values["key"]; v != "" {
		c.key = v
	}
	return nil
}

package mock

import (
	"context"
	"fmt"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// The mock box's setup menus (webcontrol.BoxSettingsReader): a small box of
// its own in OpenWebif's shape — a few sections with every kind of item,
// a title row, a risky A/V setting, a restart-needing one and a PIN. The
// real answers (a VU+ Solo2's 18 sections) are the fixtures in
// testdata/fixtures/vti15_boxsettings, read by the client's own tests.

type mockSection struct {
	webcontrol.ConfigSection
	items []webcontrol.ConfigItem
}

func choices(pairs ...string) []webcontrol.ConfigChoice {
	var out []webcontrol.ConfigChoice
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, webcontrol.ConfigChoice{Value: pairs[i], Label: pairs[i+1]})
	}
	return out
}

func mockSettings() []mockSection {
	return []mockSection{
		{webcontrol.ConfigSection{Key: "usage", Title: "Customize"}, []webcontrol.ConfigItem{
			{Path: "config.usage.setup_level", Label: "Setup mode", Kind: webcontrol.KindChoice, Value: "intermediate",
				Choices: choices("simple", "Simple", "intermediate", "Advanced", "expert", "Expert")},
			{Path: "config.recording.margin_before", Label: "Margin before record (minutes)", Kind: webcontrol.KindNumber, Value: float64(5)},
			{Path: "config.recording.margin_after", Label: "Margin after record (minutes)", Kind: webcontrol.KindNumber, Value: float64(10)},
			{Path: "config.usage.show_message_when_recording_starts", Label: "Show message when recording starts", Kind: webcontrol.KindBool, Value: true},
			{Path: "config.seek.speeds_forward", Label: "Fast forward speeds", Kind: webcontrol.KindMultiChoice,
				Value: []any{float64(2), float64(4), float64(8)}, Choices: choices("2", "2", "4", "4", "8", "8", "16", "16")},
		}},
		{webcontrol.ConfigSection{Key: "vti_user_interface", Title: "User interface"}, []webcontrol.ConfigItem{
			{Path: "config.usage.description", Label: "Channel list", Kind: webcontrol.KindHeading},
			{Path: "config.usage.servicelist_picon", Label: "Show picons", Kind: webcontrol.KindBool, Value: false},
			{Path: "config.usage.servicelist_mode", Label: "Channel list style", Kind: webcontrol.KindChoice, Value: "standard",
				Choices: choices("standard", "Standard", "compact", "Compact")},
			{Path: "config.usage.picon_dir", Label: "Picon folder", Kind: webcontrol.KindReadOnly},
		}},
		{webcontrol.ConfigSection{Key: "avsetup", Title: "A/V settings"}, []webcontrol.ConfigItem{
			{Path: "config.av.colorformat", Label: "Color format", Kind: webcontrol.KindChoice, Value: "hdmi_rgb",
				Choices: choices("hdmi_rgb", "RGB", "hdmi_yuv", "YUV")},
			{Path: "config.av.generalAC3delay", Label: "General Dolby Digital delay (ms)", Kind: webcontrol.KindNumber, Value: float64(0)},
		}},
		{webcontrol.ConfigSection{Key: "parental", Title: "Parental control"}, []webcontrol.ConfigItem{
			{Path: "config.ParentalControl.setuppin", Label: "Setup PIN", Kind: webcontrol.KindText, Value: "0000"},
		}},
		// A part of the config tree, as a Dreambox's plugin lists it: key
		// names only.
		{webcontrol.ConfigSection{Key: "tree.settings.plugins.demo", Title: "plugins.demo", Group: webcontrol.GroupTree}, []webcontrol.ConfigItem{
			{Path: "config.plugins.demo.enabled", Label: "enabled", Kind: webcontrol.KindBool, Value: true},
			{Path: "config.plugins.demo.interval", Label: "interval", Kind: webcontrol.KindNumber, Value: float64(30)},
			{Path: "config.plugins.demo.lastscan", Label: "lastscan", Kind: webcontrol.KindNumber, Value: float64(1761300000)},
			{Path: "config.plugins.demo.password", Label: "password", Kind: webcontrol.KindText, Secret: true, IsSet: true},
		}},
	}
}

func mockSetup() []webcontrol.SetupSection {
	return []webcontrol.SetupSection{
		{Key: "usage", Title: "Customize", Items: []webcontrol.SetupItem{
			{Path: "config.usage.setup_level", Text: "Setup Mode", Level: 0},
			{Path: "config.recording.margin_before", Text: "Margin before record (minutes)", Level: 0,
				Description: "How long before the programme starts a recording begins."},
			{Path: "config.recording.margin_after", Text: "Margin after record (minutes)", Level: 0},
			{Path: "config.usage.show_message_when_recording_starts", Text: "Show Message when Recording starts", Level: 2},
			{Path: "config.seek.speeds_forward", Text: "Fast forward speeds", Level: 1},
			{Path: "config.usage.hidden_expert_item", Text: "An expert-only setting", Level: 2}, // not listed: the mock box is at "Advanced"
		}},
		{Key: "vti_user_interface", Title: "User interface", OpenWebif: true, Items: []webcontrol.SetupItem{
			{Path: "config.usage.servicelist_picon", Text: "Show picons", Level: 1, GuiRestart: true},
			{Path: "config.usage.servicelist_mode", Text: "Channel list style", Level: 1, GuiRestart: true,
				Description: "Changes how the channel list looks on the TV."},
		}},
		{Key: "avsetup", Title: "A/V Settings", Items: []webcontrol.SetupItem{
			{Path: "config.av.colorformat", Text: "Color Format", Level: 0},
			{Path: "config.av.generalAC3delay", Text: "General Dolby Digital delay (ms)", Level: 1},
		}},
	}
}

// noSettings is the mock box without setup menus: an OpenWebif-less box
// (NoBoxSettings) or a Dreambox whose plugin is too old (OldSettingsPlugin).
func (c *Client) noSettings(what string) error {
	if c.OldSettingsPlugin {
		return fmt.Errorf("mock: %s: %w", what, webcontrol.ErrNeedsPlugin)
	}
	if c.NoBoxSettings {
		return fmt.Errorf("mock: %s: %w", what, webcontrol.ErrNotFound)
	}
	return nil
}

// ConfigSections lists the mock box's sections.
func (c *Client) ConfigSections(ctx context.Context) ([]webcontrol.ConfigSection, error) {
	if err := c.disturb(ctx); err != nil {
		return nil, err
	}
	if err := c.noSettings("/api/config"); err != nil {
		return nil, err
	}
	var out []webcontrol.ConfigSection
	for _, s := range mockSettings() {
		out = append(out, s.ConfigSection)
	}
	return out, nil
}

// ConfigItems lists one mock section's items.
func (c *Client) ConfigItems(ctx context.Context, key string) ([]webcontrol.ConfigItem, error) {
	if err := c.disturb(ctx); err != nil {
		return nil, err
	}
	if err := c.noSettings("/api/config"); err != nil {
		return nil, err
	}
	for _, s := range mockSettings() {
		if s.Key == key {
			return s.items, nil
		}
	}
	return nil, fmt.Errorf("mock: /api/config/%s: %w", key, webcontrol.ErrNotFound)
}

// SetupDefinition is the mock box's setup.xml.
func (c *Client) SetupDefinition(ctx context.Context) ([]webcontrol.SetupSection, error) {
	if err := c.disturb(ctx); err != nil {
		return nil, err
	}
	if c.NoBoxSettings {
		return nil, fmt.Errorf("mock: setup.xml: %w", webcontrol.ErrNotFound)
	}
	return mockSetup(), nil
}

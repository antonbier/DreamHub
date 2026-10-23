package webcontrol

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// BoxSettingsReader is the read side of the receiver's own setup menus
// (docs/BOXSETTINGS_PLAN.md, S1): the sections and items OpenWebif lists
// (/api/config, /api/config/<section>) and the setup definition that holds
// what that answer leaves out — each item's level, whether a change needs
// a GUI restart, a description (/usr/share/enigma2/setup.xml, read through
// /file). Only an OpenWebif box has it; a Dreambox answers ErrNotFound
// until the TV plugin serves the same (S2).
type BoxSettingsReader interface {
	// ConfigSections lists the sections the box shows on the web.
	ConfigSections(ctx context.Context) ([]ConfigSection, error)
	// ConfigItems lists one section's items, with the current value.
	ConfigItems(ctx context.Context, key string) ([]ConfigItem, error)
	// SetupDefinition reads the box's setup.xml files (the system's and
	// those plugins ship) — empty when none can be read.
	SetupDefinition(ctx context.Context) ([]SetupSection, error)
}

// ConfigSection is one entry of /api/config. Title is in the box's language.
// Group is "" for a section of the box's own setup menus and GroupTree for a
// group of the config tree a Dreambox's plugin walks (every setting there is,
// by key name — ARCHITECTURE.md, "Box settings on both boxes").
type ConfigSection struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Group string `json:"group,omitempty"`
}

// GroupTree marks a section that is a part of the config tree, not a menu.
const GroupTree = "tree"

// Item kinds, as DreamHub names them (OpenWebif: select, checkbox, number,
// text, multicheckbox, unknown).
const (
	KindChoice      = "choice"
	KindBool        = "bool"
	KindNumber      = "number"
	KindText        = "text"
	KindMultiChoice = "multichoice"
	KindReadOnly    = "readonly" // a type OpenWebif cannot show or set
	KindHeading     = "heading"  // a title row between items, not a setting
)

// ConfigItem is one setting as OpenWebif lists it. Label is the box's own
// text in its language. Value is a bool, a number, a string or a list of
// numbers, as the box sent it; nil for a heading or a read-only item.
type ConfigItem struct {
	Path    string         `json:"path"`
	Label   string         `json:"label"`
	Kind    string         `json:"kind"`
	Value   any            `json:"value"`
	Choices []ConfigChoice `json:"choices,omitempty"`
	// Secret and IsSet: the box's plugin says this is a password or PIN and
	// whether it holds a value; Value is nil then. Left false by OpenWebif,
	// which hides such items by showing them as a type it cannot read.
	Secret bool `json:"secret,omitempty"`
	IsSet  bool `json:"isSet,omitempty"`
}

// ConfigChoice is one value a choice or multi-choice item accepts.
type ConfigChoice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// SetupSection is one <setup> of a setup.xml.
type SetupSection struct {
	Key       string      `json:"key"`
	Title     string      `json:"title"`
	OpenWebif bool        `json:"openWebif"` // showOpenWebIF="1": the box lists it on the web
	Items     []SetupItem `json:"items"`
}

// SetupItem is one <item> of a setup section: Text is the label in English
// (the file's own), Level 0 simple, 1 advanced, 2 expert.
type SetupItem struct {
	Path        string `json:"path"`
	Text        string `json:"text"`
	Description string `json:"description,omitempty"`
	Level       int    `json:"level"`
	GuiRestart  bool   `json:"guiRestart,omitempty"`
	Requires    string `json:"requires,omitempty"`
}

// pluginSetupFiles are the setup.xml files the plugins of a VTi image ship
// (ARCHITECTURE.md, "Box settings on both boxes"); a box without one just
// answers 404 and the file is skipped.
var pluginSetupFiles = []string{
	"/usr/lib/enigma2/python/Plugins/SystemPlugins/HDMICEC/setup.xml",
	"/usr/lib/enigma2/python/Plugins/SystemPlugins/AutoShutDown/setup.xml",
	"/usr/lib/enigma2/python/Plugins/SystemPlugins/TransCodingSetup/setup.xml",
}

const systemSetupFile = "/usr/share/enigma2/setup.xml"

// ConfigSections reads /api/config. ErrNotFound on a box that is not an
// OpenWebif image.
func (c *HTTPClient) ConfigSections(ctx context.Context) ([]ConfigSection, error) {
	if !c.runsOpenWebif(ctx) {
		return c.dreamboxSections(ctx)
	}
	body, err := c.get(ctx, "/api/config")
	if err != nil {
		return nil, err
	}
	defer body.Close()
	var res struct {
		Sections []struct {
			Key         string `json:"key"`
			Description string `json:"description"`
		} `json:"sections"`
	}
	if err := json.NewDecoder(body).Decode(&res); err != nil {
		return nil, fmt.Errorf("webcontrol: parsing /api/config response: %w", err)
	}
	out := make([]ConfigSection, 0, len(res.Sections))
	for _, s := range res.Sections {
		if s.Key != "" {
			out = append(out, ConfigSection{Key: s.Key, Title: s.Description})
		}
	}
	return out, nil
}

// ConfigItems reads /api/config/<key>. The key goes into the path, so only
// the characters a section key has are let through.
func (c *HTTPClient) ConfigItems(ctx context.Context, key string) ([]ConfigItem, error) {
	if !validSectionKey(key) {
		return nil, fmt.Errorf("webcontrol: invalid section key %q", key)
	}
	if !c.runsOpenWebif(ctx) {
		return c.dreamboxItems(ctx, key)
	}
	body, err := c.get(ctx, "/api/config/"+url.PathEscape(key))
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return DecodeConfigItems(body)
}

func validSectionKey(key string) bool {
	if key == "" || len(key) > 96 {
		return false
	}
	for _, r := range key {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.') {
			return false
		}
	}
	return true
}

// DecodeConfigItems reads one section's answer (fixtures/vti15_boxsettings).
// A title row between settings comes as a "unknown" item whose path is
// config.usage.description — a dummy element the box's setup.xml uses for
// headings; any other "unknown" item is a setting OpenWebif can't show.
func DecodeConfigItems(r io.Reader) ([]ConfigItem, error) {
	var res struct {
		Configs []struct {
			Path        string `json:"path"`
			Description string `json:"description"`
			Data        struct {
				Type    string            `json:"type"`
				Current json.RawMessage   `json:"current"`
				Choices []json.RawMessage `json:"choices"`
			} `json:"data"`
		} `json:"configs"`
	}
	if err := json.NewDecoder(r).Decode(&res); err != nil {
		return nil, fmt.Errorf("webcontrol: parsing /api/config/<section> response: %w", err)
	}
	out := make([]ConfigItem, 0, len(res.Configs))
	for _, c := range res.Configs {
		it := ConfigItem{Path: c.Path, Label: strings.TrimSpace(c.Description)}
		switch c.Data.Type {
		case "select":
			it.Kind = KindChoice
		case "checkbox":
			it.Kind = KindBool
		case "number":
			it.Kind = KindNumber
		case "text":
			it.Kind = KindText
		case "multicheckbox":
			it.Kind = KindMultiChoice
		default:
			it.Kind = KindReadOnly
			if strings.HasSuffix(c.Path, ".description") {
				it.Kind = KindHeading
			}
		}
		if it.Kind != KindHeading && it.Kind != KindReadOnly && len(c.Data.Current) > 0 {
			var v any
			if err := json.Unmarshal(c.Data.Current, &v); err == nil {
				it.Value = v
			}
		}
		for _, raw := range c.Data.Choices {
			if ch, ok := decodeChoice(raw); ok {
				it.Choices = append(it.Choices, ch)
			}
		}
		out = append(out, it)
	}
	return out, nil
}

// decodeChoice reads one entry of a "choices" list: [value, label] for a
// select, a bare number for a multi-checkbox.
func decodeChoice(raw json.RawMessage) (ConfigChoice, bool) {
	var pair []any
	if json.Unmarshal(raw, &pair) == nil {
		if len(pair) < 1 {
			return ConfigChoice{}, false
		}
		v := scalarString(pair[0])
		label := v
		if len(pair) > 1 {
			label = scalarString(pair[1])
		}
		return ConfigChoice{Value: v, Label: label}, true
	}
	var one any
	if json.Unmarshal(raw, &one) != nil {
		return ConfigChoice{}, false
	}
	v := scalarString(one)
	return ConfigChoice{Value: v, Label: v}, true
}

func scalarString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}

// SetupDefinition reads the system setup.xml and the plugins' ones through
// /file. The system file is required (its failure is the error); a plugin's
// missing file is skipped. A Dreambox's is the same file without the
// description and restart attributes and has no plugin files (ARCHITECTURE.md).
func (c *HTTPClient) SetupDefinition(ctx context.Context) ([]SetupSection, error) {
	sections, err := c.setupFile(ctx, systemSetupFile)
	if err != nil {
		return nil, err
	}
	if !c.runsOpenWebif(ctx) {
		return sections, nil
	}
	for _, f := range pluginSetupFiles {
		more, err := c.setupFile(ctx, f)
		if err != nil {
			continue
		}
		sections = append(sections, more...)
	}
	return sections, nil
}

func (c *HTTPClient) setupFile(ctx context.Context, file string) ([]SetupSection, error) {
	q := url.Values{"action": {"download"}, "file": {file}}
	body, err := c.get(ctx, "/file?"+q.Encode())
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return ParseSetupXML(body)
}

// ParseSetupXML reads a setup.xml: <setup key title [showOpenWebIF]> with
// <item level text [description] [requires] [guirestart]>config.x.y</item>,
// the items sometimes inside <if>/<elif>/<else> — all of them are taken, in
// file order (the condition itself is the box's to evaluate; its answer
// already has only the items that apply). Items without a path are skipped.
func ParseSetupXML(r io.Reader) ([]SetupSection, error) {
	dec := xml.NewDecoder(r)
	var out []SetupSection
	var cur *SetupSection
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("webcontrol: parsing setup.xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "setup":
				s := SetupSection{}
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "key":
						s.Key = a.Value
					case "title":
						s.Title = a.Value
					case "showOpenWebIF":
						s.OpenWebif = a.Value == "1"
					}
				}
				out = append(out, s)
				cur = &out[len(out)-1]
			case "item":
				var item SetupItem
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "level":
						item.Level, _ = strconv.Atoi(a.Value)
					case "text":
						item.Text = a.Value
					case "description":
						item.Description = a.Value
					case "requires":
						item.Requires = a.Value
					case "guirestart":
						item.GuiRestart = a.Value == "yes"
					}
				}
				var path string
				if err := dec.DecodeElement(&path, &t); err != nil {
					return nil, fmt.Errorf("webcontrol: parsing setup.xml: %w", err)
				}
				item.Path = strings.TrimSpace(path)
				if cur != nil && item.Path != "" {
					cur.Items = append(cur.Items, item)
				}
			}
		case xml.EndElement:
			if t.Name.Local == "setup" {
				cur = nil
			}
		}
	}
	return out, nil
}

package webcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// A Dreambox's WebControl has no call for the receiver's settings, so the
// DreamHub plugin inside enigma2 serves them (enigma2-plugin/src/websettings.py,
// GET /dreamhub/settings/*, read only — docs/BOXSETTINGS_PLAN.md, S2): the
// box's setup.xml sections with each setting described, and the whole config
// tree by key name. Recorded answers: testdata/fixtures/dreambox_boxsettings.

// ErrNeedsPlugin: the box has no /dreamhub/settings — the DreamHub plugin on
// it is missing or older than this backend.
var ErrNeedsPlugin = errors.New("webcontrol: the DreamHub plugin on the box has no settings route")

const dreamboxSettingsPath = "/dreamhub/settings/"

// treePrefix starts the key of a tree section: tree.<store>.<root without "config.">.
const treePrefix = "tree."

// treeKey and parseTreeKey translate between a store and root and the section
// key the rest of DreamHub uses ("tree.settings.plugins.autotimer").
func treeKey(store, root string) string {
	return treePrefix + store + "." + strings.TrimPrefix(root, "config.")
}

func parseTreeKey(key string) (store, root string, ok bool) {
	rest, found := strings.CutPrefix(key, treePrefix)
	if !found {
		return "", "", false
	}
	store, name, found := strings.Cut(rest, ".")
	if !found || store == "" || name == "" {
		return "", "", false
	}
	return store, "config." + name, true
}

// dreamboxGet asks the plugin and decodes its answer. A 404 means no plugin
// route (ErrNeedsPlugin); {"ok": false} is the plugin's own refusal.
func (c *HTTPClient) dreamboxGet(ctx context.Context, route string, q url.Values, v any) error {
	path := dreamboxSettingsPath + route
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	body, err := c.get(ctx, path)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("%w (%s)", ErrNeedsPlugin, path)
		}
		return err
	}
	defer body.Close()
	var raw json.RawMessage
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return fmt.Errorf("webcontrol: parsing %s response: %w", path, err)
	}
	var status struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &status); err != nil || !status.OK {
		return fmt.Errorf("webcontrol: %s: the plugin answered %q %s", path, status.Error, status.Message)
	}
	return json.Unmarshal(raw, v)
}

// dreamboxSections lists the box's own setup sections and the groups of the
// config tree (config.plugins.<name> one by one, every other config.<name>,
// and the Gemini plugins' second store when it is there).
func (c *HTTPClient) dreamboxSections(ctx context.Context) ([]ConfigSection, error) {
	var info struct {
		API    int `json:"api"`
		Stores []struct {
			ID string `json:"id"`
		} `json:"stores"`
	}
	if err := c.dreamboxGet(ctx, "info", nil, &info); err != nil {
		return nil, err
	}
	if info.API < 1 {
		return nil, fmt.Errorf("%w (api %d)", ErrNeedsPlugin, info.API)
	}
	var menus struct {
		Sections []struct {
			Key       string `json:"key"`
			Title     string `json:"title"`
			Available bool   `json:"available"`
		} `json:"sections"`
	}
	if err := c.dreamboxGet(ctx, "sections", nil, &menus); err != nil {
		return nil, err
	}
	var out []ConfigSection
	for _, s := range menus.Sections {
		if s.Available && s.Key != "" {
			out = append(out, ConfigSection{Key: s.Key, Title: s.Title})
		}
	}
	for _, st := range info.Stores {
		var roots struct {
			Roots []struct {
				Root string `json:"root"`
			} `json:"roots"`
		}
		if err := c.dreamboxGet(ctx, "roots", url.Values{"store": {st.ID}}, &roots); err != nil {
			return nil, err
		}
		for _, r := range roots.Roots {
			title := strings.TrimPrefix(r.Root, "config.")
			if st.ID != "settings" {
				title = "Gemini · " + title
			}
			out = append(out, ConfigSection{Key: treeKey(st.ID, r.Root), Title: title, Group: GroupTree})
		}
	}
	return out, nil
}

// dhItem is the plugin's description of one setting (boxsettings.describe).
type dhItem struct {
	Path      string          `json:"path"`
	Kind      string          `json:"kind"`
	Secret    bool            `json:"secret"`
	IsSet     bool            `json:"isSet"`
	ReadOnly  *string         `json:"readOnly"`
	Value     json.RawMessage `json:"value"`
	ValueText string          `json:"valueText"`
	Choices   [][]any         `json:"choices"`
	Truncated bool            `json:"choicesTruncated"`
}

// dreamboxItems reads one section: a setup.xml section of the box (in its
// order, with its titles) or a part of the config tree.
func (c *HTTPClient) dreamboxItems(ctx context.Context, key string) ([]ConfigItem, error) {
	if store, root, ok := parseTreeKey(key); ok {
		return c.dreamboxTree(ctx, store, root)
	}
	var res struct {
		Items []struct {
			Heading   bool    `json:"heading"`
			Label     string  `json:"label"`
			Available bool    `json:"available"`
			Missing   string  `json:"missing"`
			Item      *dhItem `json:"item"`
		} `json:"items"`
	}
	if err := c.dreamboxGet(ctx, "items", url.Values{"section": {key}}, &res); err != nil {
		return nil, err
	}
	out := make([]ConfigItem, 0, len(res.Items))
	for _, r := range res.Items {
		switch {
		case r.Heading:
			out = append(out, ConfigItem{Path: "", Label: strings.TrimSpace(r.Label), Kind: KindHeading})
		case !r.Available || r.Missing != "" || r.Item == nil:
			// not offered by this box (requires=), or no element behind it
		case r.Item.Kind == "nothing":
			// a ConfigNothing the TV leaves out as well
		default:
			out = append(out, r.Item.configItem(strings.TrimSpace(r.Label)))
		}
	}
	return out, nil
}

// dreamboxTreePage is how many settings one request to the plugin asks for.
const dreamboxTreePage = 1000

func (c *HTTPClient) dreamboxTree(ctx context.Context, store, root string) ([]ConfigItem, error) {
	var out []ConfigItem
	for offset := 0; ; {
		var page struct {
			Total int      `json:"total"`
			Items []dhItem `json:"items"`
		}
		q := url.Values{"store": {store}, "root": {root}, "limit": {strconv.Itoa(dreamboxTreePage)}, "offset": {strconv.Itoa(offset)}}
		if err := c.dreamboxGet(ctx, "items", q, &page); err != nil {
			return nil, err
		}
		for _, it := range page.Items {
			if it.Kind == "nothing" {
				continue
			}
			out = append(out, it.configItem(strings.TrimPrefix(it.Path, root+".")))
		}
		offset += len(page.Items)
		if len(page.Items) == 0 || offset >= page.Total {
			return out, nil
		}
	}
}

// configItem maps the plugin's kinds onto DreamHub's. Kinds with a value form
// of their own (an IP, a clock, a float) come as the text the TV shows.
func (d dhItem) configItem(label string) ConfigItem {
	it := ConfigItem{Path: d.Path, Label: label, Secret: d.Secret, IsSet: d.IsSet}
	switch d.Kind {
	case "choice", "selectionNumber":
		it.Kind = KindChoice
	case "bool":
		it.Kind = KindBool
	case "integer", "pin", "number", "slider":
		it.Kind = KindNumber
	case "text", "password", "directory", "ip", "mac", "clock", "position", "sequence", "float":
		it.Kind = KindText
	case "multichoice":
		it.Kind = KindMultiChoice
	default:
		it.Kind = KindReadOnly
	}
	if d.ReadOnly != nil && *d.ReadOnly == "unreadable" {
		it.Kind = KindReadOnly
	}
	if d.Secret || it.Kind == KindReadOnly {
		return it
	}
	switch d.Kind {
	case "ip", "mac", "clock", "position", "sequence", "float":
		it.Value = d.ValueText
	default:
		var v any
		if len(d.Value) > 0 && json.Unmarshal(d.Value, &v) == nil {
			it.Value = v
		}
	}
	if d.Kind == "selectionNumber" {
		it.Value = scalarString(it.Value)
	}
	for _, pair := range d.Choices {
		if len(pair) == 0 {
			continue
		}
		v := scalarString(pair[0])
		label := v
		if len(pair) > 1 {
			label = scalarString(pair[1])
		}
		it.Choices = append(it.Choices, ConfigChoice{Value: v, Label: label})
	}
	// A long choice list is cut by the plugin; the current value must still
	// have a label.
	if it.Kind == KindChoice && d.Truncated {
		cur := scalarString(it.Value)
		found := false
		for _, ch := range it.Choices {
			found = found || ch.Value == cur
		}
		if !found {
			it.Choices = append(it.Choices, ConfigChoice{Value: cur, Label: cur})
		}
	}
	return it
}

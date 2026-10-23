package boxsettings

import (
	"sort"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Catalog is one box's setup menus, read once: every section with its
// items merged from OpenWebif's answer and the box's setup.xml.
type Catalog struct {
	// BoxLevel is the box's own setup level (0 simple, 1 advanced, 2
	// expert), -1 when it could not be read; BoxLevelName is that level's
	// name in the box's language.
	BoxLevel     int
	BoxLevelName string
	sections     []section
}

type section struct {
	info  SectionInfo
	items []Item
}

// SectionInfo is one section of the list.
type SectionInfo struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	// Count is the settings in it (titles between them not counted).
	Count int `json:"count"`
	// HiddenByLevel is how many more settings the box's own setup file
	// holds for a higher level than the box is set to: the box does not
	// list them on the web until its level is raised.
	HiddenByLevel int `json:"hiddenByLevel"`
}

// Item is one setting, for the browser.
type Item struct {
	Path  string `json:"path"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
	// Value is nil for a heading, a read-only item and a secret.
	Value   any                       `json:"value"`
	Choices []webcontrol.ConfigChoice `json:"choices,omitempty"`
	// Level is 0 simple, 1 advanced, 2 expert; LevelKnown false when the
	// box's setup files do not name the item (then it counts as simple).
	Level      int  `json:"level"`
	LevelKnown bool `json:"levelKnown"`
	// GuiRestart: a change takes effect only after a GUI restart.
	GuiRestart bool `json:"guiRestart,omitempty"`
	// Risk and RiskCategory: see lists.go; empty when not risky.
	Risk         Risk   `json:"risk,omitempty"`
	RiskCategory string `json:"riskCategory,omitempty"`
	// Secret: a password, PIN or login — set or not, never shown.
	Secret bool `json:"secret,omitempty"`
	// IsSet is true for a secret that holds a value.
	IsSet bool `json:"isSet,omitempty"`
	// Help is the box's own description from its setup file, in the file's
	// language (English); empty when it has none.
	Help string `json:"help,omitempty"`
	// SetupText is the item's text in the setup file (English), kept for
	// the search.
	SetupText string `json:"-"`
}

// Hit is one search result.
type Hit struct {
	SectionKey   string `json:"sectionKey"`
	SectionTitle string `json:"sectionTitle"`
	Item         Item   `json:"item"`
}

// levelPath is the setup files' dummy element for a title row.
func isHeadingPath(path string) bool { return strings.HasSuffix(path, ".description") }

// Build merges the sections and their items with the setup definition.
func Build(sections []webcontrol.ConfigSection, items map[string][]webcontrol.ConfigItem, setup []webcontrol.SetupSection) *Catalog {
	bySection := map[string]webcontrol.SetupItem{} // section key + "\x00" + path
	byPath := map[string]webcontrol.SetupItem{}
	for _, s := range setup {
		for _, it := range s.Items {
			if isHeadingPath(it.Path) {
				continue
			}
			k := s.Key + "\x00" + it.Path
			if _, ok := bySection[k]; !ok {
				bySection[k] = it
			}
			if _, ok := byPath[it.Path]; !ok {
				byPath[it.Path] = it
			}
		}
	}

	c := &Catalog{BoxLevel: -1}
	for _, secItems := range items {
		for _, it := range secItems {
			if it.Path == "config.usage.setup_level" {
				c.BoxLevel, c.BoxLevelName = levelOf(it)
			}
		}
	}

	for _, s := range sections {
		out := section{info: SectionInfo{Key: s.Key, Title: s.Title}}
		listed := map[string]bool{}
		for _, raw := range items[s.Key] {
			it := Item{Path: raw.Path, Label: raw.Label, Kind: raw.Kind, Value: raw.Value, Choices: raw.Choices}
			if raw.Kind != webcontrol.KindHeading {
				listed[raw.Path] = true
				out.info.Count++
				si, ok := bySection[s.Key+"\x00"+raw.Path]
				if !ok {
					si, ok = byPath[raw.Path]
				}
				if ok {
					it.Level, it.LevelKnown, it.GuiRestart = si.Level, true, si.GuiRestart
					it.Help, it.SetupText = si.Description, si.Text
				}
				it.Risk, it.RiskCategory = RiskOf(raw.Path)
				if NeedsGuiRestart(raw.Path) {
					it.GuiRestart = true
				}
				if IsSecret(raw.Path) {
					it.Secret = true
					it.IsSet = raw.Value != nil && raw.Value != "" && raw.Value != false
					it.Value = nil
				}
			}
			out.items = append(out.items, it)
		}
		if c.BoxLevel >= 0 {
			for _, ss := range setup {
				if ss.Key != s.Key {
					continue
				}
				for _, si := range ss.Items {
					if !isHeadingPath(si.Path) && !listed[si.Path] && si.Level > c.BoxLevel {
						out.info.HiddenByLevel++
					}
				}
			}
		}
		c.sections = append(c.sections, out)
	}
	return c
}

// levelOf reads the box's own level from its setup_level item: the
// position of the current value among the choices (simple, intermediate,
// expert).
func levelOf(it webcontrol.ConfigItem) (int, string) {
	cur, _ := it.Value.(string)
	for i, ch := range it.Choices {
		if ch.Value == cur {
			return i, ch.Label
		}
	}
	return -1, ""
}

// Sections lists the sections in the box's order.
func (c *Catalog) Sections() []SectionInfo {
	out := make([]SectionInfo, 0, len(c.sections))
	for _, s := range c.sections {
		out = append(out, s.info)
	}
	return out
}

// Section returns one section with its items.
func (c *Catalog) Section(key string) (SectionInfo, []Item, bool) {
	for _, s := range c.sections {
		if s.info.Key == key {
			return s.info, s.items, true
		}
	}
	return SectionInfo{}, nil, false
}

// Search finds settings whose label, setup text, path or description holds
// every word of q (case-insensitive); label matches come first. At most
// limit results.
func (c *Catalog) Search(q string, limit int) []Hit {
	words := strings.Fields(strings.ToLower(q))
	if len(words) == 0 || limit <= 0 {
		return nil
	}
	type scored struct {
		hit   Hit
		score int
	}
	var found []scored
	for _, s := range c.sections {
		for _, it := range s.items {
			if it.Kind == webcontrol.KindHeading {
				continue
			}
			label := strings.ToLower(it.Label)
			rest := strings.ToLower(it.SetupText + " " + it.Path + " " + it.Help)
			score, ok := 0, true
			for _, w := range words {
				switch {
				case strings.Contains(label, w):
				case strings.Contains(rest, w):
					score++
				default:
					ok = false
				}
			}
			if ok {
				found = append(found, scored{Hit{SectionKey: s.info.Key, SectionTitle: s.info.Title, Item: it}, score})
			}
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].score < found[j].score })
	if len(found) > limit {
		found = found[:limit]
	}
	out := make([]Hit, len(found))
	for i, f := range found {
		out[i] = f.hit
	}
	return out
}

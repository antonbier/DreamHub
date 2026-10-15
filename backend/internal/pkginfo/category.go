package pkginfo

import "strings"

// Category is a group of packages with a stable id and a label per language.
type Category struct {
	ID    string            `json:"id"`
	Label map[string]string `json:"label"`
}

// Categories in the order the UI lists them. The ids are stable (they are
// stored nowhere but used in URLs and by entries.json); labels may change.
var Categories = []Category{
	{ID: "extensions", Label: map[string]string{"de": "Erweiterungen", "en": "Extensions", "it": "Estensioni"}},
	{ID: "skins", Label: map[string]string{"de": "Skins", "en": "Skins", "it": "Skin"}},
	{ID: "systemplugins", Label: map[string]string{"de": "Systemplugins", "en": "System plugins", "it": "Plugin di sistema"}},
	{ID: "drivers", Label: map[string]string{"de": "Treiber", "en": "Drivers", "it": "Driver"}},
	{ID: "picons", Label: map[string]string{"de": "Senderlogos", "en": "Channel logos", "it": "Loghi dei canali"}},
	{ID: "kernel", Label: map[string]string{"de": "Kernel-Module", "en": "Kernel modules", "it": "Moduli del kernel"}},
	{ID: "system", Label: map[string]string{"de": "System", "en": "System", "it": "Sistema"}},
}

// categoryPrefixes: the Enigma2 package naming convention, checked in
// order. The picon rule also covers the feeds' "<feed>-picon-<set>" names
// (e.g. gp4-picon-hotbird13e) seen on the box; the rest follows
// enigma2-plugin-<kind>-<name>. To be checked against the real DreamOS
// index once it is read (docs/APPSHUB_PLAN.md, App11).
var categoryPrefixes = []struct{ prefix, id string }{
	{"enigma2-plugin-extensions-", "extensions"},
	{"enigma2-plugin-skins-", "skins"},
	{"enigma2-plugin-skincomponents-", "skins"},
	{"enigma2-skin-", "skins"},
	{"enigma2-plugin-systemplugins-", "systemplugins"},
	{"enigma2-plugin-drivers-", "drivers"},
	{"enigma2-plugin-picons-", "picons"},
	{"kernel-module-", "kernel"},
}

// CategoryOf derives the category id from the package name alone.
func CategoryOf(name string) string {
	for _, p := range categoryPrefixes {
		if strings.HasPrefix(name, p.prefix) {
			return p.id
		}
	}
	if strings.Contains(name, "-picon-") || strings.HasPrefix(name, "picon-") {
		return "picons"
	}
	return "system"
}

// CategoryLabel is a category's name in the asked language ("" for an
// unknown id).
func CategoryLabel(id, lang string) string {
	lang = Lang(lang)
	for _, c := range Categories {
		if c.ID == id {
			return c.Label[lang]
		}
	}
	return ""
}

// ValidCategory reports whether id is one of Categories.
func ValidCategory(id string) bool { return CategoryLabel(id, "en") != "" }

// Package pkginfo explains the box's packages in plain words without an AI
// (docs/APPSHUB_PLAN.md, App11). Three layers, best first:
//
//  1. curated: an embedded entry (entries.json) with a short description in
//     de/en/it, optionally a category and the DreamHub features that need
//     the package;
//  2. package: the package's own Description from apt's data;
//  3. none: nothing known (the UI may offer the optional AI explanation).
//
// The category never needs an entry: it is derived from the name
// convention (category.go) and only overridden by an entry that says so.
//
// The entries file is deliberately incomplete — it holds only what could
// be written offline with confidence. The owner and the box feed fill the
// rest; cmd/pkginfo-missing lists the feed packages that still lack one.
package pkginfo

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Source says where a description came from.
type Source string

const (
	SourceCurated Source = "curated" // an entry in entries.json
	SourcePackage Source = "package" // the package's own apt Description
	SourceNone    Source = "none"    // nothing known
)

// Languages the entries and category labels are written in. "en" is the
// fallback for anything else.
var Languages = []string{"de", "en", "it"}

// Entry is one curated package.
type Entry struct {
	// Description by language; every entry has all of Languages.
	Description map[string]string `json:"description"`
	// Category is a category id; empty means "derive it from the name".
	Category string `json:"category,omitempty"`
	// NeededBy lists hub ids (the UI shows their plain page names).
	NeededBy []string `json:"neededBy,omitempty"`
}

// Resolved is everything the Packages tab shows about one package.
type Resolved struct {
	Description   string   `json:"description"`
	Category      string   `json:"category"`      // stable id
	CategoryLabel string   `json:"categoryLabel"` // in the asked language
	NeededBy      []string `json:"neededBy,omitempty"`
	Source        Source   `json:"source"`
}

//go:embed entries.json
var entriesJSON []byte

var entries = mustLoad(entriesJSON)

func mustLoad(data []byte) map[string]Entry {
	var f struct {
		Packages map[string]Entry `json:"packages"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		panic(fmt.Sprintf("pkginfo: entries.json: %v", err))
	}
	return f.Packages
}

// Lang reduces a language tag ("de-DE", "it_IT", "") to one of Languages.
func Lang(tag string) string {
	tag = strings.ToLower(tag)
	if len(tag) >= 2 && slices.Contains(Languages, tag[:2]) {
		return tag[:2]
	}
	return "en"
}

// Lookup returns the curated entry for a package.
func Lookup(name string) (Entry, bool) {
	e, ok := entries[name]
	return e, ok
}

// Names lists every package with a curated entry, sorted.
func Names() []string {
	out := make([]string, 0, len(entries))
	for n := range entries {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Missing returns the names without a curated entry, in their given order.
func Missing(names []string) []string {
	var out []string
	for _, n := range names {
		if _, ok := entries[n]; !ok {
			out = append(out, n)
		}
	}
	return out
}

// Resolve gives the information for one package. aptDescription is the
// package's own Description (the first line is enough), used when there is
// no curated entry.
func Resolve(name, aptDescription, lang string) Resolved {
	lang = Lang(lang)
	r := Resolved{Category: CategoryOf(name), Source: SourceNone}
	if e, ok := entries[name]; ok {
		if e.Category != "" {
			r.Category = e.Category
		}
		r.NeededBy = slices.Clone(e.NeededBy)
		if d := e.Description[lang]; d != "" {
			r.Description, r.Source = d, SourceCurated
		} else if d := e.Description["en"]; d != "" {
			r.Description, r.Source = d, SourceCurated
		}
	}
	if r.Source == SourceNone {
		if d := strings.TrimSpace(aptDescription); d != "" {
			r.Description, r.Source = d, SourcePackage
		}
	}
	r.CategoryLabel = CategoryLabel(r.Category, lang)
	return r
}

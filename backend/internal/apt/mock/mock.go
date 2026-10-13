// Package mock implements apt.Lister and apt.FeedLister in memory: a
// handful of packages across the categories actually seen on the
// reference box (docs/ADMINHUB_PLAN.md: "Section field confirmed live,
// 2026-10-11"), one of them upgradable, and a few feeds, one added by hand
// (unknown), so the browser has something to show.
package mock

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/antonbier/DreamHub/backend/internal/apt"
)

// Lister is an in-memory apt.Lister and apt.FeedLister.
type Lister struct {
	// Packages overrides the defaults below (tests).
	Packages []apt.Package
	// Feeds overrides the defaults below (tests).
	Feeds []apt.Feed
	// Avail is what the feeds offer that isn't installed (the catalog).
	Avail []apt.Package
	// Applied records every Apply call (tests).
	Applied []string

	mu sync.Mutex
}

var (
	_ apt.Lister      = (*Lister)(nil)
	_ apt.FeedLister  = (*Lister)(nil)
	_ apt.Manager     = (*Lister)(nil)
	_ apt.FeedManager = (*Lister)(nil)
)

func New() *Lister {
	return &Lister{
		Packages: []apt.Package{
			// DreamHub's own package sets X-DreamHub-Category (control.in)
			// so AdminHub never needs a bundled category lookup for it;
			// an extension's .deb sets "extension" the same way
			// (docs/EXTENSIONHUB_PLAN.md, E2) — demoed here since there's
			// no real extension package to read it from yet.
			{Name: "dreamhub", Version: "0.3.0", Category: "self", Description: "DreamHub web interface"},
			{Name: "dreamhub-ext-ewelink", Version: "1.0", Category: "extension", Description: "DreamHub extension: eWeLink/Sonoff bridge (demo)"},
			{Name: "enigma2", Version: "4.6-r1", Category: "base", Description: "Enigma2 set-top box middleware"},
			{Name: "enigma2-plugin-skincomponents-dreamhubbutton", Version: "1.0", Category: "plugin-skincomponents", Description: "Skin button for DreamHub"},
			{Name: "gp4-picon-hotbird13e", Version: "20260901", Category: "base", Description: "Picons for Hotbird 13E"},
			{Name: "dropbear", Version: "2022.83", Category: "console/network", Description: "Lightweight SSH server"},
			{Name: "python3-core", Version: "3.11.6-r2", Category: "devel/python", Description: "Minimal Python 3 interpreter"},
			{Name: "oscam-cycecplus", Version: "11742", Category: "console/network", Description: "Open Source Conditional Access Module", Upgradable: true, AvailableVersion: "11760"},
		},
		Avail: []apt.Package{
			{Name: "ffmpeg", Version: "6.1.1-r0", Category: "multimedia", Description: "A complete solution to record, convert and stream audio and video"},
			{Name: "htop", Version: "1.0.3-r0.0", Category: "console/utils", Description: "Interactive process viewer"},
			{Name: "kernel-module-overlay", Version: "4.9-r0", Category: "kernel", Description: "overlay kernel module"},
			{Name: "gp4-picon-astra19e", Version: "20260901", Category: "picon-skin", Description: "Picons for Astra 19.2E"},
		},
		Feeds: []apt.Feed{
			{Name: "gp4-unstable-aio-feed", URL: "http://feed.gemini-project.net/unstable", Enabled: true, Known: true},
			{Name: "svenh-feed", URL: "http://svenh-feeds.de/feeds/dreambox-sh4/unstable", Enabled: true, Known: true},
			{Name: "dreamhub-feed", URL: "https://feed.dreamhub.example/stable", Enabled: true, Known: true},
			{Name: "dreambox-feed", URL: "http://opendreambox.net/feeds", Enabled: false, Known: true},
			{Name: "homebrew-feed", URL: "http://192.168.1.50:8080/feed", Enabled: true, Known: false},
		},
	}
}

func (l *Lister) List(ctx context.Context) ([]apt.Package, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.Packages), nil
}

func (l *Lister) ListFeeds(ctx context.Context) ([]apt.Feed, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.Feeds), nil
}

func (l *Lister) SetFeedEnabled(ctx context.Context, name string, enabled bool) error {
	if !apt.ValidFeedName(name) {
		return apt.ErrInvalidFeed
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.Feeds {
		if l.Feeds[i].Name == name {
			l.Feeds[i].Enabled = enabled
			return nil
		}
	}
	return apt.ErrFeedMissing
}

func (l *Lister) AddFeed(ctx context.Context, name, url string) error {
	if !apt.ValidFeedName(name) || !apt.ValidFeedURL(url) {
		return apt.ErrInvalidFeed
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, f := range l.Feeds {
		if f.Name == name {
			return apt.ErrFeedExists
		}
	}
	l.Feeds = append(l.Feeds, apt.Feed{Name: name, URL: url, Enabled: true})
	return nil
}

func (l *Lister) RemoveFeed(ctx context.Context, name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Feeds = slices.DeleteFunc(l.Feeds, func(f apt.Feed) bool { return f.Name == name })
	return nil
}

func (l *Lister) RefreshIndex(ctx context.Context) error { return nil }

func (l *Lister) Available(ctx context.Context) ([]apt.Package, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.Avail), nil
}

// Simulate mirrors what apt would say for the in-memory lists. Removing
// "enigma2-plugin-skincomponents-dreamhubbutton" also removes "dreamhub",
// a stand-in for the real cascade seen on the box (ARCHITECTURE.md, "Box
// checks for 0.4.0") so the preview's extra-removal warning has a case.
func (l *Lister) Simulate(ctx context.Context, action apt.Action, names []string) ([]apt.Step, error) {
	if err := apt.ValidNames(action, names); err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.simulate(action, names)
}

func (l *Lister) simulate(action apt.Action, names []string) ([]apt.Step, error) {
	installed := func(n string) (apt.Package, bool) {
		for _, p := range l.Packages {
			if p.Name == n {
				return p, true
			}
		}
		return apt.Package{}, false
	}
	var steps []apt.Step
	switch action {
	case apt.ActionUpgrade:
		for _, p := range l.Packages {
			if p.Upgradable && (len(names) == 0 || slices.Contains(names, p.Name)) {
				steps = append(steps, apt.Step{Kind: "upgrade", Name: p.Name, OldVersion: p.Version, NewVersion: p.AvailableVersion})
			}
		}
	case apt.ActionInstall:
		for _, n := range names {
			if p, ok := installed(n); ok {
				if p.Upgradable {
					steps = append(steps, apt.Step{Kind: "upgrade", Name: n, OldVersion: p.Version, NewVersion: p.AvailableVersion})
				}
				continue
			}
			i := slices.IndexFunc(l.Avail, func(p apt.Package) bool { return p.Name == n })
			if i < 0 {
				return nil, &apt.SimulateError{Output: "E: Unable to locate package " + n}
			}
			steps = append(steps, apt.Step{Kind: "install", Name: n, NewVersion: l.Avail[i].Version})
		}
	case apt.ActionRemove:
		for _, n := range names {
			p, ok := installed(n)
			if !ok {
				continue
			}
			steps = append(steps, apt.Step{Kind: "remove", Name: n, OldVersion: p.Version})
			if n == "enigma2-plugin-skincomponents-dreamhubbutton" {
				if d, ok := installed("dreamhub"); ok {
					steps = append(steps, apt.Step{Kind: "remove", Name: d.Name, OldVersion: d.Version})
				}
			}
		}
	}
	return steps, nil
}

func (l *Lister) Apply(ctx context.Context, action apt.Action, names []string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	steps, err := l.simulate(action, names)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, s := range steps {
		switch s.Kind {
		case "install":
			i := slices.IndexFunc(l.Avail, func(p apt.Package) bool { return p.Name == s.Name })
			l.Packages = append(l.Packages, l.Avail[i])
			l.Avail = slices.Delete(l.Avail, i, i+1)
			out.WriteString("Setting up " + s.Name + " (" + s.NewVersion + ") ...\n")
		case "upgrade":
			for i := range l.Packages {
				if l.Packages[i].Name == s.Name {
					l.Packages[i].Version, l.Packages[i].Upgradable, l.Packages[i].AvailableVersion = s.NewVersion, false, ""
				}
			}
			out.WriteString("Unpacking " + s.Name + " (" + s.NewVersion + ") over (" + s.OldVersion + ") ...\n")
		case "remove":
			i := slices.IndexFunc(l.Packages, func(p apt.Package) bool { return p.Name == s.Name })
			l.Avail = append(l.Avail, l.Packages[i])
			l.Packages = slices.Delete(l.Packages, i, i+1)
			out.WriteString("Removing " + s.Name + " (" + s.OldVersion + ") ...\n")
		}
	}
	l.Applied = append(l.Applied, string(action)+" "+strings.Join(names, " "))
	return out.String(), nil
}

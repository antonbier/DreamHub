package apt

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Feed is one apt source (docs/ADMINHUB_PLAN.md, "Feeds, with a trust
// signal").
type Feed struct {
	// Name is the .list file's name, with ".list"/".list.off" stripped.
	Name    string `json:"name"`
	URL     string `json:"url,omitempty"` // the first "deb" line's URL; empty if unreadable/empty
	Enabled bool   `json:"enabled"`       // false for a ".list.off" file — disabled, not loaded
	// Known marks a feed from the box's own image, DreamHub's own, or the
	// Extension Hub's, as opposed to one someone added by hand — a real
	// supply-chain decision on a box with root access
	// (docs/ADMINHUB_PLAN.md). Matched by name against the reference box's
	// confirmed factory feed set (ARCHITECTURE.md, "apt feed structure") —
	// a heuristic, not a guarantee: a box imaged slightly differently could
	// ship a factory feed this doesn't recognize yet.
	Known bool `json:"known"`
}

// FeedLister lists the box's configured apt feeds.
type FeedLister interface {
	ListFeeds(ctx context.Context) ([]Feed, error)
}

// SourcesListDir is where DreamOS keeps one .list file per feed, confirmed
// live on the reference box (ARCHITECTURE.md, "apt feed structure").
// A var, not a const, so tests can point it at a fixture directory.
var SourcesListDir = "/etc/apt/sources.list.d"

var _ FeedLister = Exec{}

func (Exec) ListFeeds(ctx context.Context) ([]Feed, error) {
	entries, err := os.ReadDir(SourcesListDir)
	if os.IsNotExist(err) {
		return []Feed{}, nil
	}
	if err != nil {
		return nil, err
	}
	var feeds []Feed
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		var base string
		var enabled bool
		switch {
		case strings.HasSuffix(name, ".list.off"):
			base, enabled = strings.TrimSuffix(name, ".list.off"), false
		case strings.HasSuffix(name, ".list"):
			base, enabled = strings.TrimSuffix(name, ".list"), true
		default:
			continue // not a feed file at all
		}
		content, _ := os.ReadFile(filepath.Join(SourcesListDir, name))
		feeds = append(feeds, Feed{Name: base, URL: firstDebURL(content), Enabled: enabled, Known: knownFeeds[base]})
	}
	sort.Slice(feeds, func(i, j int) bool { return feeds[i].Name < feeds[j].Name })
	return feeds, nil
}

// firstDebURL returns the first "deb"/"deb-src" line's URL — the second
// field, after an optional "[trusted=yes]"-style options bracket this
// platform's feeds use (ARCHITECTURE.md, "Package feeds are unsigned").
func firstDebURL(content []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "deb ") && !strings.HasPrefix(line, "deb-src ") {
			continue
		}
		fields := strings.Fields(line)
		for _, f := range fields[1:] {
			if strings.HasPrefix(f, "[") {
				continue
			}
			return f
		}
	}
	return ""
}

// knownFeeds are the reference box's factory-shipped feeds, active and
// disabled (ARCHITECTURE.md, "apt feed structure", confirmed live
// 2026-10-11), plus DreamHub's own and the Extension Hub's reserved names.
// Not exhaustive across every DreamOS image — see Feed.Known's doc comment.
var knownFeeds = map[string]bool{
	"gp4-unstable-aio-feed": true, "gp4-unstable-aarch64-feed": true, "gp4-unstable-all-feed": true,
	"gp4-unstable-allcodes-feed": true, "gp4-unstable-extraPluginsAarch64-feed": true, "gp4-unstable-nobody-feed": true,
	"kodi-aarch64-feed": true, "mediaportal": true, "plugins-boxpirates": true, "svenh-feed": true,
	"aarch64-feed": true, "all-feed": true, "armv7vehf-neon-vfpv4-feed": true, "dreambox-feed": true,
	"gutemine": true, "mpgz": true,
	"dreamhub-feed": true, "dreamhub-extensions-feed": true,
}

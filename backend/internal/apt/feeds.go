package apt

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
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

// FeedManager switches feeds on and off and adds one, then refreshes
// apt's package index so what a feed offers shows up (or goes away).
type FeedManager interface {
	SetFeedEnabled(ctx context.Context, name string, enabled bool) error
	AddFeed(ctx context.Context, name, url string) error
	// RemoveFeed deletes a feed's file again — used to undo an AddFeed
	// whose index refresh failed.
	RemoveFeed(ctx context.Context, name string) error
	RefreshIndex(ctx context.Context) error
}

var _ FeedManager = Exec{}

// ErrInvalidFeed is a feed name or URL that isn't acceptable; ErrFeedExists
// a name already taken, ErrFeedMissing one that isn't there.
var (
	ErrInvalidFeed = errors.New("apt: invalid feed name or address")
	ErrFeedExists  = errors.New("apt: a feed with this name already exists")
	ErrFeedMissing = errors.New("apt: no such feed")
)

var feedNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,59}$`)

// ValidFeedName: a file name that can only ever be a plain name in
// SourcesListDir, never a path.
func ValidFeedName(name string) bool {
	return feedNameRe.MatchString(name) && !strings.HasSuffix(name, ".list") && !strings.HasSuffix(name, ".off")
}

// ValidFeedURL: a plain http(s) address with no spaces or quotes — it goes
// into a "deb" line, so nothing may smuggle in a second field or line.
func ValidFeedURL(raw string) bool {
	u, err := neturl.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	return !strings.ContainsAny(raw, " \t\r\n\"'\\[]")
}

func (Exec) SetFeedEnabled(ctx context.Context, name string, enabled bool) error {
	if !ValidFeedName(name) {
		return ErrInvalidFeed
	}
	on, off := filepath.Join(SourcesListDir, name+".list"), filepath.Join(SourcesListDir, name+".list.off")
	from, to := off, on
	if !enabled {
		from, to = on, off
	}
	if _, err := os.Stat(from); os.IsNotExist(err) {
		if _, err := os.Stat(to); err == nil {
			return nil // already so
		}
		return ErrFeedMissing
	}
	if _, err := os.Stat(to); err == nil {
		return ErrFeedExists
	}
	return os.Rename(from, to)
}

// AddFeed writes "<name>.list" in the form every feed on these boxes has:
// unsigned ([trusted=yes], ARCHITECTURE.md "Package feeds are unsigned")
// and flat ("./"). It never overwrites a file.
func (Exec) AddFeed(ctx context.Context, name, url string) error {
	if !ValidFeedName(name) || !ValidFeedURL(url) {
		return ErrInvalidFeed
	}
	for _, suffix := range []string{".list", ".list.off"} {
		if _, err := os.Stat(filepath.Join(SourcesListDir, name+suffix)); err == nil {
			return ErrFeedExists
		}
	}
	f, err := os.OpenFile(filepath.Join(SourcesListDir, name+".list"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.WriteString("deb [trusted=yes] " + strings.TrimRight(url, "/") + " ./\n")
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(f.Name())
	}
	return werr
}

func (Exec) RemoveFeed(ctx context.Context, name string) error {
	if !ValidFeedName(name) {
		return ErrInvalidFeed
	}
	return os.Remove(filepath.Join(SourcesListDir, name+".list"))
}

// RefreshIndex runs "apt-get update", bounded.
func (Exec) RefreshIndex(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "apt-get", "update")
	cmd.Env = aptEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apt-get update: %w: %s", err, lastLines(out, 5))
	}
	return nil
}

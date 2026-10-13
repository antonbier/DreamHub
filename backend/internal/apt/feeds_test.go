package apt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFirstDebURL(t *testing.T) {
	tests := []struct {
		content string
		want    string
	}{
		{"deb [trusted=yes] http://example.com/feed unstable main\n", "http://example.com/feed"},
		{"# a comment\ndeb http://example.com/feed unstable main\n", "http://example.com/feed"},
		{"deb-src http://example.com/src unstable main\n", "http://example.com/src"},
		{"# nothing here\n", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := firstDebURL([]byte(tt.content)); got != tt.want {
			t.Errorf("firstDebURL(%q) = %q, want %q", tt.content, got, tt.want)
		}
	}
}

func TestListFeeds(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("gp4-unstable-aio-feed.list", "deb [trusted=yes] http://feed.gemini-project.net/unstable unstable main\n")
	write("dreambox-feed.list.off", "deb http://opendreambox.net/feeds unstable main\n")
	write("homebrew-feed.list", "deb http://192.168.1.50:8080/feed unstable main\n")
	write("not-a-feed.txt", "ignored")

	orig := SourcesListDir
	SourcesListDir = dir
	defer func() { SourcesListDir = orig }()

	feeds, err := Exec{}.ListFeeds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(feeds) != 3 {
		t.Fatalf("len = %d, want 3: %+v", len(feeds), feeds)
	}
	byName := map[string]Feed{}
	for _, f := range feeds {
		byName[f.Name] = f
	}
	if f := byName["gp4-unstable-aio-feed"]; !f.Enabled || !f.Known {
		t.Errorf("gp4-unstable-aio-feed = %+v", f)
	}
	if f := byName["dreambox-feed"]; f.Enabled || !f.Known {
		t.Errorf("dreambox-feed = %+v", f)
	}
	if f := byName["homebrew-feed"]; !f.Enabled || f.Known {
		t.Errorf("homebrew-feed = %+v", f)
	}
}

func TestFeedManager_SwitchAddRemove(t *testing.T) {
	dir := t.TempDir()
	old := SourcesListDir
	SourcesListDir = dir
	defer func() { SourcesListDir = old }()
	ctx := context.Background()
	e := Exec{}

	if err := e.AddFeed(ctx, "my-feed", "https://example.org/feed/"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "my-feed.list"))
	if string(b) != "deb [trusted=yes] https://example.org/feed ./\n" {
		t.Fatalf("file: %q", b)
	}
	if err := e.AddFeed(ctx, "my-feed", "https://example.org/x"); !errors.Is(err, ErrFeedExists) {
		t.Fatalf("second add: %v", err)
	}
	if err := e.SetFeedEnabled(ctx, "my-feed", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "my-feed.list.off")); err != nil {
		t.Fatal("not switched off")
	}
	if err := e.SetFeedEnabled(ctx, "my-feed", true); err != nil {
		t.Fatal(err)
	}
	if err := e.SetFeedEnabled(ctx, "nope", true); !errors.Is(err, ErrFeedMissing) {
		t.Fatalf("missing: %v", err)
	}
	if err := e.RemoveFeed(ctx, "my-feed"); err != nil {
		t.Fatal(err)
	}
}

func TestFeedManager_RefusesNamesAndURLsThatAreNotPlain(t *testing.T) {
	for _, name := range []string{"../etc/passwd", "a/b", ".hidden", "x.list", "UPPER", "", "a b"} {
		if ValidFeedName(name) {
			t.Errorf("name %q accepted", name)
		}
	}
	for _, u := range []string{"ftp://x/y", "https://", "https://a.b/c d", "https://a.b/\n deb evil", "file:///etc", "https://a.b/[x]"} {
		if ValidFeedURL(u) {
			t.Errorf("url %q accepted", u)
		}
	}
}

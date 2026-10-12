package apt

import (
	"context"
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

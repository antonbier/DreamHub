package hubs

import (
	"context"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/recfs"
)

// The recording index: every recording below the recording folders, for
// MediaHub's search and the recordings view source — not just the top
// folder /web/movielist answers for. Walking every folder through
// WebControl takes ~40 s on the reference box and blocks enigma2 meanwhile
// (ARCHITECTURE.md), so the index reads the file system directly instead,
// which is only possible when DreamHub runs on the box itself
// (recfs.Available — the same condition as file operations). Off the box,
// callers fall back to the top folder.
//
// Built in the background and kept for indexMaxAge; a file operation marks
// it stale, so a rename shows up in search right away.

const (
	indexMaxAge   = 10 * time.Minute
	indexMaxDepth = 6     // SerienRecorder's per-series folders are 1-2 deep
	indexMaxItems = 20000 // a runaway guard, far above any real disk
)

type indexEntry struct {
	Path   string // the .ts file
	Folder string // its folder, with a trailing slash
	Title  string
	Time   int64 // from .ts.meta line 4 — never from the file name
	Size   int64
}

type recIndex struct {
	mu       sync.Mutex
	entries  []indexEntry
	built    time.Time
	building bool
}

// recordingIndex returns the index's entries, or ok=false when the file
// system isn't available (off the box). The very first call builds it
// synchronously — reading local directories and small .meta files is fast
// — later calls return the current entries and rebuild in the background
// once they're older than indexMaxAge.
func (h *Media) recordingIndex(ctx context.Context) ([]indexEntry, bool) {
	roots, err := h.roots(ctx)
	if err != nil || !recfs.Available(h.FS, roots) {
		return nil, false
	}
	x := &h.index
	x.mu.Lock()
	if x.entries == nil && !x.building {
		x.building = true
		x.mu.Unlock()
		return h.buildIndex(roots), true
	}
	entries := x.entries
	if time.Since(x.built) >= indexMaxAge && !x.building {
		x.building = true
		go h.buildIndex(roots)
	}
	x.mu.Unlock()
	return entries, entries != nil
}

func (h *Media) buildIndex(roots []string) []indexEntry {
	built := walkRecordings(h.FS, roots)
	x := &h.index
	x.mu.Lock()
	x.entries, x.built, x.building = built, time.Now(), false
	x.mu.Unlock()
	return built
}

// refreshIndex rebuilds the index in the background after a file
// operation, so search and views show the change right away. Nothing to do
// before the first use built one.
func (h *Media) refreshIndex(ctx context.Context) {
	x := &h.index
	x.mu.Lock()
	if x.entries == nil || x.building {
		x.mu.Unlock()
		return
	}
	x.building = true
	x.mu.Unlock()
	roots, err := h.roots(ctx)
	if err != nil {
		x.mu.Lock()
		x.building = false
		x.mu.Unlock()
		return
	}
	go h.buildIndex(roots)
}

// mediaFile reports whether a file is something the box's movie list
// shows: its own .ts recordings, and other video files placed alongside.
func mediaFile(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".ts", ".mkv", ".mp4", ".avi", ".m2ts", ".mpg", ".mpeg":
		return true
	}
	return false
}

func walkRecordings(fsys recfs.FS, roots []string) []indexEntry {
	out := []indexEntry{}
	seen := map[string]bool{}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > indexMaxDepth || len(out) >= indexMaxItems || seen[dir] {
			return
		}
		seen[dir] = true
		entries, err := fsys.ReadDir(dir)
		if err != nil {
			return
		}
		folder := strings.TrimSuffix(dir, "/") + "/"
		for _, e := range entries {
			full := folder + e.Name()
			if e.IsDir() {
				// Not into hidden folders (.Trash, .AppleDouble …) or symlinks
				// (DirEntry.IsDir is false for a symlink), so the walk stays
				// inside the recording folders.
				if !strings.HasPrefix(e.Name(), ".") {
					walk(full, depth+1)
				}
				continue
			}
			if !mediaFile(e.Name()) {
				continue
			}
			// Like /web/movielist: a recording's title from its .ts.meta, any
			// other media file by its file name.
			entry := indexEntry{Path: full, Folder: folder, Title: e.Name()}
			if strings.HasSuffix(e.Name(), ".ts") {
				entry.Title = recfs.Base(full)
			}
			if meta, err := fsys.ReadFile(full + ".meta"); err == nil {
				lines := strings.SplitN(string(meta), "\n", 5)
				if len(lines) > 1 && strings.TrimSpace(lines[1]) != "" {
					entry.Title = strings.TrimSpace(lines[1])
				}
				if len(lines) > 3 {
					entry.Time, _ = strconv.ParseInt(strings.TrimSpace(lines[3]), 10, 64)
				}
			}
			if fi, err := fsys.Lstat(full); err == nil {
				entry.Size = fi.Size()
			}
			out = append(out, entry)
			if len(out) >= indexMaxItems {
				return
			}
		}
	}
	for _, r := range roots {
		walk(path.Clean(r), 0)
	}
	return out
}

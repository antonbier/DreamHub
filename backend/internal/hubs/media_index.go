package hubs

import (
	"context"
	"github.com/antonbier/DreamHub/backend/internal/box"
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
	// again: a file operation happened while a build ran — that build may
	// have read the old state, so build once more when it's done.
	again bool
	// first is closed once the very first build finished; callers that
	// arrive while it runs wait on it instead of seeing no index.
	first chan struct{}
}

// recordingIndex returns the index's entries, or ok=false when the file
// system isn't available (off the box). The very first call builds it
// synchronously — reading local directories and small .meta files is fast
// — later calls return the current entries and rebuild in the background
// once they're older than indexMaxAge.
// index returns the box's own recording index.
func (h *Media) index(ctx context.Context) *recIndex {
	h.indexMu.Lock()
	defer h.indexMu.Unlock()
	if h.indexes == nil {
		h.indexes = map[string]*recIndex{}
	}
	id := box.Key(ctx)
	x := h.indexes[id]
	if x == nil {
		x = &recIndex{}
		h.indexes[id] = x
	}
	return x
}

func (h *Media) recordingIndex(ctx context.Context) ([]indexEntry, bool) {
	roots, err := h.roots(ctx)
	if err != nil || !onBox(ctx, h.FS, roots) {
		return nil, false
	}
	x := h.index(ctx)
	x.mu.Lock()
	if x.entries == nil && !x.building {
		x.building = true
		x.first = make(chan struct{})
		x.mu.Unlock()
		return h.buildIndex(x, roots), true
	}
	if x.entries == nil && x.first != nil {
		first := x.first
		x.mu.Unlock()
		select {
		case <-first:
		case <-ctx.Done():
			return nil, false
		}
		x.mu.Lock()
	}
	entries := x.entries
	if time.Since(x.built) >= indexMaxAge && !x.building {
		x.building = true
		go h.buildIndex(x, roots)
	}
	x.mu.Unlock()
	return entries, entries != nil
}

func (h *Media) buildIndex(x *recIndex, roots []string) []indexEntry {
	for {
		built := walkRecordings(h.FS, roots)
		x.mu.Lock()
		x.entries, x.built = built, time.Now()
		if x.first != nil {
			close(x.first)
			x.first = nil
		}
		if !x.again {
			x.building = false
			x.mu.Unlock()
			return built
		}
		x.again = false
		x.mu.Unlock()
	}
}

// refreshIndex rebuilds the index in the background after a file
// operation, so search and views show the change right away. Nothing to do
// before the first use built one.
func (h *Media) refreshIndex(ctx context.Context) {
	x := h.index(ctx)
	x.mu.Lock()
	if x.building {
		x.again = true
		x.mu.Unlock()
		return
	}
	if x.entries == nil {
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
	go h.buildIndex(x, roots)
}

// readRegular reads a file only if it's a regular file, not a link to
// something outside the recording folders.
func readRegular(fsys recfs.FS, name string) ([]byte, error) {
	fi, err := fsys.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, recfs.ErrOutsideFolders
	}
	return fsys.ReadFile(name)
}

func walkRecordings(fsys recfs.FS, roots []string) []indexEntry {
	found := recfs.Walk(fsys, roots, indexMaxDepth, indexMaxItems)
	out := make([]indexEntry, len(found))
	for i, e := range found {
		out[i] = indexEntry{Path: e.Path, Folder: e.Folder, Title: e.Title, Time: e.Time, Size: e.Size}
	}
	return out
}

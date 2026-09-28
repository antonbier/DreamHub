package enrich

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ImageCache keeps fetched images on disk (under DreamHub's data dir, on
// the box's /data partition — ARCHITECTURE.md: root is small, /data has
// room) and caps their total size, dropping the least recently used first.
type ImageCache struct {
	Dir      string
	MaxBytes int64

	mu     sync.Mutex
	writes int
}

// DefaultImageCacheBytes is the cap used on the box: a w185 poster is
// ~15 KB, a w780 backdrop ~100 KB, so this is thousands of pictures.
const DefaultImageCacheBytes = 100 << 20

// pruneEvery is how many writes pass between size checks — a directory
// scan per image would be wasted work.
const pruneEvery = 25

func (c *ImageCache) file(size, path string) string {
	return filepath.Join(c.Dir, size, strings.TrimPrefix(path, "/"))
}

// Get returns a cached image and marks it as recently used.
func (c *ImageCache) Get(size, path string) ([]byte, string, bool) {
	f := c.file(size, path)
	b, err := os.ReadFile(f)
	if err != nil {
		return nil, "", false
	}
	now := time.Now()
	_ = os.Chtimes(f, now, now)
	return b, contentType(path), true
}

// Put stores an image; failures are ignored (the image is just fetched
// again next time).
func (c *ImageCache) Put(size, path string, b []byte) {
	f := c.file(size, path)
	if err := os.MkdirAll(filepath.Dir(f), 0o700); err != nil {
		return
	}
	tmp := f + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, f); err != nil {
		_ = os.Remove(tmp)
		return
	}
	c.mu.Lock()
	c.writes++
	prune := c.writes%pruneEvery == 0
	c.mu.Unlock()
	if prune {
		c.Prune()
	}
}

// Prune deletes the least recently used images until the cache is under
// MaxBytes.
func (c *ImageCache) Prune() {
	c.mu.Lock()
	defer c.mu.Unlock()
	type entry struct {
		path string
		size int64
		mod  int64
	}
	var entries []entry
	var total int64
	_ = filepath.WalkDir(c.Dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		entries = append(entries, entry{p, info.Size(), info.ModTime().UnixNano()})
		total += info.Size()
		return nil
	})
	if total <= c.MaxBytes {
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].mod < entries[j].mod })
	for _, e := range entries {
		if total <= c.MaxBytes {
			break
		}
		if os.Remove(e.path) == nil {
			total -= e.size
		}
	}
}

func contentType(path string) string {
	switch {
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".webp"):
		return "image/webp"
	}
	return "image/jpeg"
}

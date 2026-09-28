package recfs

import (
	"path"
	"strconv"
	"strings"
)

// Entry is one media file found by Walk.
type Entry struct {
	Path        string // the file
	Folder      string // its folder, with a trailing slash
	Title       string // .ts.meta line 2 for a recording, else the file name
	Description string // .ts.meta line 3
	Time        int64  // .ts.meta line 4 — never from the file name
	Size        int64
}

// Walk lists the media files below roots straight from the file system —
// what the box's movie list would show, across every folder, without
// asking WebControl folder by folder (a full walk through it takes ~40 s
// on the reference box and blocks enigma2 meanwhile, ARCHITECTURE.md).
// Hidden folders and links aren't followed, so the walk stays inside
// roots; maxDepth and maxItems guard against runaway trees.
func Walk(fsys FS, roots []string, maxDepth, maxItems int) []Entry {
	out := []Entry{}
	seen := map[string]bool{}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > maxDepth || len(out) >= maxItems || seen[dir] {
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
				// DirEntry.IsDir is false for a symlink: links aren't followed.
				if !strings.HasPrefix(e.Name(), ".") {
					walk(full, depth+1)
				}
				continue
			}
			if !MediaFile(e.Name()) {
				continue
			}
			entry := Entry{Path: full, Folder: folder, Title: e.Name()}
			if strings.HasSuffix(e.Name(), ".ts") {
				entry.Title = Base(full)
			}
			if fi, err := fsys.Lstat(full + ".meta"); err == nil && fi.Mode().IsRegular() {
				if meta, err := fsys.ReadFile(full + ".meta"); err == nil {
					lines := strings.SplitN(string(meta), "\n", 5)
					if len(lines) > 1 && strings.TrimSpace(lines[1]) != "" {
						entry.Title = strings.TrimSpace(lines[1])
					}
					if len(lines) > 2 {
						entry.Description = strings.TrimSpace(lines[2])
					}
					if len(lines) > 3 {
						entry.Time, _ = strconv.ParseInt(strings.TrimSpace(lines[3]), 10, 64)
					}
				}
			}
			if fi, err := fsys.Lstat(full); err == nil {
				entry.Size = fi.Size()
			}
			out = append(out, entry)
			if len(out) >= maxItems {
				return
			}
		}
	}
	for _, r := range roots {
		walk(path.Clean(r), 0)
	}
	return out
}

// MediaFile reports whether a file is something the box's movie list
// shows: its own .ts recordings, and other video files placed alongside.
func MediaFile(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".ts", ".mkv", ".mp4", ".avi", ".m2ts", ".mpg", ".mpeg":
		return true
	}
	return false
}

package recfs

import (
	"io/fs"
	"sort"
	"strings"
	"sync"
	"testing/fstest"
	"time"
)

// Mem is an in-memory FS for mock mode and tests (PRINCIPLES.md,
// "Mock-first development"). Paths are absolute ("/media/hdd/movie/x.ts");
// folders exist implicitly through the files inside them. Safe for
// concurrent use.
type Mem struct {
	mu sync.RWMutex
	m  fstest.MapFS
}

// NewMem returns a Mem holding files (path → content).
func NewMem(files map[string]string) *Mem {
	m := fstest.MapFS{}
	for p, content := range files {
		m[key(p)] = &fstest.MapFile{Data: []byte(content), Mode: 0o644, ModTime: time.Date(2026, 9, 1, 20, 15, 0, 0, time.UTC)}
	}
	return &Mem{m: m}
}

func key(p string) string {
	k := strings.Trim(p, "/")
	if k == "" {
		return "."
	}
	return k
}

func (s *Mem) ReadDir(name string) ([]fs.DirEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fs.ReadDir(s.m, key(name))
}

func (s *Mem) Lstat(name string) (fs.FileInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fs.Stat(s.m, key(name))
}

func (s *Mem) ReadFile(name string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fs.ReadFile(s.m, key(name))
}

func (s *Mem) Rename(from, to string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.m[key(from)]
	if !ok {
		return &fs.PathError{Op: "rename", Path: from, Err: fs.ErrNotExist}
	}
	delete(s.m, key(from))
	s.m[key(to)] = f
	return nil
}

func (s *Mem) WriteFile(name string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.m[key(name)]
	if !ok {
		return &fs.PathError{Op: "write", Path: name, Err: fs.ErrNotExist}
	}
	s.m[key(name)] = &fstest.MapFile{Data: append([]byte(nil), data...), Mode: f.Mode, ModTime: time.Now()}
	return nil
}

func (s *Mem) Create(name string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[key(name)]; ok {
		return &fs.PathError{Op: "create", Path: name, Err: fs.ErrExist}
	}
	s.m[key(name)] = &fstest.MapFile{Data: append([]byte(nil), data...), Mode: 0o644, ModTime: time.Now()}
	return nil
}

// Mkdir is a no-op: folders exist implicitly through the files in them.
func (s *Mem) Mkdir(name string) error { return nil }

func (s *Mem) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[key(name)]; !ok {
		return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrNotExist}
	}
	delete(s.m, key(name))
	return nil
}

// Put adds or overwrites one file — for tests that need a fixture beyond
// what NewMem's initial set covers.
func (s *Mem) Put(path, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key(path)] = &fstest.MapFile{Data: []byte(content), Mode: 0o644, ModTime: time.Date(2026, 9, 1, 20, 15, 0, 0, time.UTC)}
}

// Paths lists every file, sorted — for tests and the mock's movie listing.
func (s *Mem) Paths() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.m))
	for k := range s.m {
		out = append(out, "/"+k)
	}
	sort.Strings(out)
	return out
}

// Size of a file's content.
func (s *Mem) Size(name string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if f, ok := s.m[key(name)]; ok {
		return int64(len(f.Data))
	}
	return 0
}

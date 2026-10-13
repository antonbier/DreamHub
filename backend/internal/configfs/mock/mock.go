// Package mock is an in-memory configfs.FS, keyed by root then relative
// path — no real file system needed for AppsHub's backup tests (App6).
package mock

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/configfs"
)

// FS is a deterministic, in-memory configfs.FS.
type FS struct {
	// Files maps root -> relative path -> content. Tests populate this
	// directly to seed a given app's "disk" state.
	Files map[string]map[string][]byte
}

var _ configfs.FS = (*FS)(nil)

func New() *FS { return &FS{Files: map[string]map[string][]byte{}} }

func (f *FS) Size(root string) (int64, error) {
	var total int64
	for _, data := range f.Files[root] {
		total += int64(len(data))
	}
	return total, nil
}

func (f *FS) Archive(root string) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, data := range f.Files[root] {
		if err := tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(name), Mode: 0o644, Size: int64(len(data))}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (f *FS) Restore(root string, archive []byte) error {
	gr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	if f.Files[root] == nil {
		f.Files[root] = map[string][]byte{}
	}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		clean := filepath.Clean(hdr.Name)
		if clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return err
		}
		f.Files[root][clean] = data
	}
}

// ReadFile/WriteFile keep a single file under Files[dir][base].
func (f *FS) ReadFile(path string) ([]byte, error) {
	data, ok := f.Files[filepath.Dir(path)][filepath.Base(path)]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return data, nil
}

func (f *FS) WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if f.Files[dir] == nil {
		f.Files[dir] = map[string][]byte{}
	}
	f.Files[dir][filepath.Base(path)] = append([]byte(nil), data...)
	return nil
}

// Package configfs reads and writes a small directory tree as a single
// gzip-compressed tar archive — AppsHub's backup section (App6,
// docs/APPSHUB_PLAN.md) uses it to back up and restore an installed
// app's own config directory (its HostDir, App4), the "configuration
// only" default the plan calls for. Deliberately narrow: not a general
// sync tool. Restore only ever adds or overwrites files from the
// archive — it never deletes a file already on disk that the archive
// doesn't mention, the simplest safe behavior for "restore my settings
// back" without needing to reason about what else might legitimately
// have been written to the directory since the backup was taken (a
// database file growing, say).
package configfs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FS is implemented by OS (the real file system) and Mem (tests).
type FS interface {
	// Size returns the total size in bytes of every regular file under
	// root, recursively. 0, nil if root doesn't exist yet (an app that
	// has never written any config).
	Size(root string) (int64, error)
	// Archive tars and gzips every regular file under root, with
	// archive-internal paths relative to root. An empty or nonexistent
	// root produces a valid, empty archive, not an error — there's
	// nothing wrong with an app that hasn't written any config yet.
	Archive(root string) ([]byte, error)
	// Restore extracts archive into root (created if missing), adding or
	// overwriting files; nothing already in root but absent from archive
	// is touched.
	Restore(root string, archive []byte) error
}

// OS is the real file system.
type OS struct{}

var _ FS = OS{}

func (OS) Size(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == root {
				return filepath.SkipDir
			}
			return err
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	if err != nil && os.IsNotExist(err) {
		return 0, nil
	}
	return total, err
}

func (OS) Archive(root string) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == root {
				return filepath.SkipDir
			}
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(rel), Mode: 0o644, Size: int64(len(data))}); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if cerr := tw.Close(); cerr != nil {
		return nil, cerr
	}
	if cerr := gw.Close(); cerr != nil {
		return nil, cerr
	}
	return buf.Bytes(), nil
}

func (OS) Restore(root string, archive []byte) error {
	gr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// Reject anything that would escape root (a crafted ".." entry) —
		// the archive is data from a backup file the admin uploaded, not
		// trusted input.
		clean := filepath.Clean(hdr.Name)
		if clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
			return errors.New("configfs: archive entry escapes its root: " + hdr.Name)
		}
		dest := filepath.Join(root, clean)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
	}
}

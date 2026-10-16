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
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Restore limits against archive bombs (a tiny gzip can expand to
// terabytes). Generous: far above any app's config directory. Variables so
// tests can lower them.
var (
	MaxRestoreTotal int64 = 8 << 30 // all files together
	MaxRestoreFile  int64 = 4 << 30 // one file

	// ErrTooLarge is returned when an archive would extract past the limits.
	ErrTooLarge = errors.New("configfs: archive is too large to restore")
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
	// ReadFile reads one small file (an error wrapping fs.ErrNotExist if
	// it isn't there); WriteFile writes one, readable by its owner only,
	// creating its directory — for an app's generated passwords
	// (StorageRoot/<id>.secrets.json, AppsHub).
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte) error
	// List returns the names of the regular files directly in dir (nil if
	// it does not exist) — AppsHub finds stored app definitions with it.
	List(dir string) ([]string, error)
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
		if path == root || !d.Type().IsRegular() && !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		// Mode and owner travel along: a database or app running as its own
		// user must find its files owned and readable again after a restore.
		hdr := &tar.Header{Name: filepath.ToSlash(rel), Mode: int64(info.Mode().Perm())}
		hdr.Uid, hdr.Gid = fileOwner(info)
		if d.IsDir() {
			hdr.Typeflag, hdr.Name = tar.TypeDir, hdr.Name+"/"
			return tw.WriteHeader(hdr)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hdr.Size = int64(len(data))
		if err := tw.WriteHeader(hdr); err != nil {
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
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeDir {
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
		if hdr.Typeflag == tar.TypeDir {
			if clean == "." {
				continue
			}
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			applyOwner(dest, hdr, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		// Count what is really extracted, not the header's claim.
		f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		n, err := io.Copy(f, io.LimitReader(tr, MaxRestoreFile+1))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		total += n
		if n > MaxRestoreFile {
			_ = os.Remove(dest)
			return fmt.Errorf("%w: %q is larger than %d bytes", ErrTooLarge, hdr.Name, MaxRestoreFile)
		}
		if total > MaxRestoreTotal {
			return fmt.Errorf("%w: more than %d bytes in total", ErrTooLarge, MaxRestoreTotal)
		}
		applyOwner(dest, hdr, 0o644)
	}
}

// applyOwner gives path the archived mode and owner, as far as this
// process may (a non-root run keeps its own owner). An archive from before
// modes were recorded has mode 0644 and owner 0; those stay as written.
func applyOwner(path string, hdr *tar.Header, def os.FileMode) {
	if m := os.FileMode(hdr.Mode).Perm(); m != 0 && m != def {
		_ = os.Chmod(path, m)
	}
	if hdr.Uid != 0 || hdr.Gid != 0 {
		_ = os.Lchown(path, hdr.Uid, hdr.Gid)
	}
}

func (OS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (OS) WriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (OS) List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.Type().IsRegular() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

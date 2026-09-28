package recfs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestCleanIn_RefusesSymlinkedFolders(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "movie")
	outside := filepath.Join(tmp, "outside")
	for _, d := range []string{filepath.Join(root, "real"), outside} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks not supported here:", err)
	}
	roots := []string{root}

	if p, err := CleanIn(OS{}, root+"/real/a.ts", roots); err != nil || p != root+"/real/a.ts" {
		t.Errorf("a real subfolder is fine: %q %v", p, err)
	}
	if _, err := CleanIn(OS{}, root+"/link/b.ts", roots); !errors.Is(err, ErrOutsideFolders) {
		t.Errorf("a path through a linked folder must be refused, got %v", err)
	}
	if _, err := CleanIn(OS{}, root+"/../outside/b.ts", roots); !errors.Is(err, ErrOutsideFolders) {
		t.Errorf("a path climbing out must be refused, got %v", err)
	}
}

func TestSetMetaTitle_OnlyRegularFiles(t *testing.T) {
	old := syscall.Umask(0o077) // as DreamHub runs
	defer syscall.Umask(old)
	tmp := t.TempDir()
	secret := filepath.Join(tmp, "secret")
	os.WriteFile(secret, []byte("root:x\nhash\nmore\n"), 0o600)
	linked := filepath.Join(tmp, "a.ts")
	if err := os.Symlink(secret, linked+".meta"); err != nil {
		t.Skip("symlinks not supported here:", err)
	}
	if _, err := SetMetaTitle(OS{}, linked, "New"); !errors.Is(err, ErrNoMeta) {
		t.Errorf("a linked .ts.meta must be refused, got %v", err)
	}

	real := filepath.Join(tmp, "b.ts")
	os.WriteFile(real+".meta", []byte("1:0:19:2EE3:441:1:C00000:0:0:0:\nOld\nDesc\n1780944521\n"), 0o644)
	os.Chmod(real+".meta", 0o644) // as Enigma2 writes it, whatever this process's umask
	if _, err := SetMetaTitle(OS{}, real, "New title"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(real + ".meta")
	fi, _ := os.Stat(real + ".meta")
	if string(b) != "1:0:19:2EE3:441:1:C00000:0:0:0:\nNew title\nDesc\n1780944521\n" || fi.Mode().Perm() != 0o644 {
		t.Errorf("only line 2 changes, permissions kept: %q %v", b, fi.Mode())
	}
	entries, _ := os.ReadDir(tmp)
	for _, e := range entries {
		if strings.Contains(e.Name(), "tmp") {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}
}

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

func TestApplyInto_CreatesFoldersAndRollsBack(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.ts"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "a.eit"), []byte("x"), 0o644)

	moves, err := PlanRenameTo(OS{}, root+"/a.ts", root+"/Serie/Staffel 01", "Serie - S01E01")
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := MissingDirs(OS{}, moves)
	if err != nil || len(dirs) != 2 || dirs[0] != root+"/Serie" {
		t.Fatalf("missing folders, outermost first: %v %v", dirs, err)
	}
	// A move that can't work: everything goes back, the new folders too.
	bad := append([]Move{}, moves...)
	bad = append(bad, Move{From: root + "/nope.ts", To: root + "/Serie/Staffel 01/nope.ts"})
	if err := ApplyInto(OS{}, dirs, bad); err == nil {
		t.Fatal("expected a failure")
	}
	if _, err := os.Stat(root + "/a.ts"); err != nil {
		t.Error("the recording must be back in place")
	}
	if _, err := os.Stat(root + "/Serie"); !os.IsNotExist(err) {
		t.Errorf("the created folders must be gone again: %v", err)
	}
	if err := ApplyInto(OS{}, dirs, moves); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(root + "/Serie/Staffel 01/Serie - S01E01.ts")
	di, _ := os.Stat(root + "/Serie")
	if err != nil || fi == nil || di.Mode().Perm() != 0o755 {
		t.Errorf("moved into new folders with mode 0755: %v %v", err, di.Mode())
	}
	// A folder segment that is a link is refused.
	os.Symlink(t.TempDir(), root+"/Link")
	m2, _ := PlanRenameTo(OS{}, root+"/Serie/Staffel 01/Serie - S01E01.ts", root+"/Link/x", "y")
	if _, err := MissingDirs(OS{}, m2); err == nil {
		t.Error("a path through a link must be refused")
	}
}

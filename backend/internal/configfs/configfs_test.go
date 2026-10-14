package configfs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOS_SizeOfNonexistentRootIsZero(t *testing.T) {
	size, err := OS{}.Size(filepath.Join(t.TempDir(), "nope"))
	if err != nil || size != 0 {
		t.Fatalf("got size=%d err=%v, want 0, nil", size, err)
	}
}

func TestOS_ArchiveAndRestoreRoundTrip(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.conf"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "b.conf"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	size, err := OS{}.Size(src)
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len("hello")+len("world")) {
		t.Fatalf("got size %d", size)
	}

	archive, err := OS{}.Archive(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(archive) == 0 {
		t.Fatal("got empty archive")
	}

	dst := filepath.Join(t.TempDir(), "restored")
	fs := OS{}
	if err := fs.Restore(dst, archive); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "a.conf"))
	if err != nil || string(got) != "hello" {
		t.Fatalf("a.conf: got %q, %v", got, err)
	}
	got, err = os.ReadFile(filepath.Join(dst, "sub", "b.conf"))
	if err != nil || string(got) != "world" {
		t.Fatalf("sub/b.conf: got %q, %v", got, err)
	}
}

func TestOS_RestoreNeverDeletesExistingFiles(t *testing.T) {
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(dst, "keep.conf"), []byte("stays"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "new.conf"), []byte("added"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive, err := OS{}.Archive(src)
	if err != nil {
		t.Fatal(err)
	}
	fs := OS{}
	if err := fs.Restore(dst, archive); err != nil {
		t.Fatal(err)
	}

	if got, err := os.ReadFile(filepath.Join(dst, "keep.conf")); err != nil || string(got) != "stays" {
		t.Fatalf("keep.conf: got %q, %v, want it untouched", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "new.conf")); err != nil || string(got) != "added" {
		t.Fatalf("new.conf: got %q, %v", got, err)
	}
}

func TestOS_RestoreRejectsPathEscape(t *testing.T) {
	// A hand-built archive, not one from Archive() (which can never
	// produce an escaping name itself) — Restore's own guard has to hold
	// against untrusted input (an uploaded backup file), not just
	// against archives this package wrote itself.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	evil := []byte("pwned")
	if err := tw.WriteHeader(&tar.Header{Name: "../evil.conf", Mode: 0o644, Size: int64(len(evil))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(evil); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gw.Close()

	dst := t.TempDir()
	fs2 := OS{}
	if err := fs2.Restore(dst, buf.Bytes()); err == nil {
		t.Fatal("want an error for a path-escaping archive entry")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dst), "evil.conf")); err == nil {
		t.Fatal("the escaping file must not have been written")
	}
}

// Modes and empty folders survive a round trip (a private key stays
// private, an app's empty data folder is still there).
func TestOS_RoundTripKeepsModesAndEmptyFolders(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "empty", "deeper"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "key"), []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(src, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	archive, err := OS{}.Archive(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "restored")
	if err := (OS{}).Restore(dst, archive); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(dst, "key")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("key: %v %v", fi, err)
	}
	if fi, err := os.Stat(filepath.Join(dst, "empty", "deeper")); err != nil || !fi.IsDir() {
		t.Fatalf("empty folder missing: %v", err)
	}
	if fi, err := os.Stat(filepath.Join(dst, "empty")); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("folder mode: %v %v", fi, err)
	}
}

func tarOf(t *testing.T, files map[string]int) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, n := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(n)}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(make([]byte, n)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func TestOS_RestoreCapsFileAndTotalSize(t *testing.T) {
	oldT, oldF := MaxRestoreTotal, MaxRestoreFile
	defer func() { MaxRestoreTotal, MaxRestoreFile = oldT, oldF }()
	MaxRestoreTotal, MaxRestoreFile = 1000, 600

	if err := (OS{}).Restore(t.TempDir(), tarOf(t, map[string]int{"big": 601})); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("one file over the cap: %v", err)
	}
	if err := (OS{}).Restore(t.TempDir(), tarOf(t, map[string]int{"a": 500, "b": 500, "c": 500})); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("total over the cap: %v", err)
	}
	if err := (OS{}).Restore(t.TempDir(), tarOf(t, map[string]int{"a": 500, "b": 400})); err != nil {
		t.Fatalf("within the caps: %v", err)
	}
}

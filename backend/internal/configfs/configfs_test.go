package configfs

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

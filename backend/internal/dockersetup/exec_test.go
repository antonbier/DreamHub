package dockersetup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The official feed's real entry, as found on the box 2026-10-12.
func TestFindPackage(t *testing.T) {
	idx := `Package: kernel-module-ext4
Version: 1:4.9.269-dream-219-g6f9ca1e-r0.1
Filename: ./kernel-module-ext4.deb
SHA256: aaa

Package: kernel-module-overlay
Version: 1:4.9.113-643176-g72b0573-r0.1
Filename: ./old.deb
SHA256: bbb

Package: kernel-module-overlay
Version: 1:4.9.269-dream-219-g6f9ca1e-r0.1
Filename: ./kernel-module-overlay_4.9.269-dream-219-g6f9ca1e-r0.1_arm64.deb
Size: 20040
SHA256: 49127c2800b6a798e321c7a0c179c98c2e4011f6dcd5b5f427488c464cfac644
`
	file, sum := FindPackage(strings.NewReader(idx), "kernel-module-overlay", "1:4.9.269-dream-219-g6f9ca1e-r0.1")
	if file != "./kernel-module-overlay_4.9.269-dream-219-g6f9ca1e-r0.1_arm64.deb" || !strings.HasPrefix(sum, "49127c28") {
		t.Fatalf("got %q %q", file, sum)
	}
	if f, _ := FindPackage(strings.NewReader(idx), "kernel-module-overlay", "1:4.9.999"); f != "" {
		t.Fatal("a different kernel build must not match")
	}
}

func tgz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestFetchVerifiedAndExtract(t *testing.T) {
	archive := tgz(t, map[string]string{"docker/dockerd": "bin", "docker/runc": "bin", "../evil": "x", "docker/../../evil2": "x"})
	sum := sha256.Sum256(archive)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(archive) }))
	defer srv.Close()
	dir := t.TempDir()
	e := Exec{}

	if _, err := e.fetchVerified(context.Background(), srv.URL, "00", filepath.Join(dir, "bad.tgz")); err == nil {
		t.Fatal("a checksum mismatch must fail")
	}
	if _, err := os.Stat(filepath.Join(dir, "bad.tgz")); !os.IsNotExist(err) {
		t.Fatal("a mismatched download must be deleted")
	}

	f, err := e.fetchVerified(context.Background(), srv.URL, hex.EncodeToString(sum[:]), filepath.Join(dir, "ok.tgz"))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "bin")
	if err := extractDocker(f, out); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(out)
	if len(entries) != 2 {
		t.Fatalf("want exactly dockerd and runc, got %v", entries)
	}
	if _, err := os.Stat(filepath.Join(dir, "evil")); err == nil {
		t.Fatal("a path outside docker/ was written")
	}
}

func TestDaemonConfig(t *testing.T) {
	b, err := daemonConfig()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"data-root": "/data/dreamhub-docker/root"`, `"bridge": "none"`, `"iptables": false`, `"unix:///run/dreamhub-docker.sock"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("config lacks %s:\n%s", want, b)
		}
	}
}

func TestFeedURL(t *testing.T) {
	if got := feedURL([]byte("deb http://dreamboxupdate.com/opendreambox/2.6/unstable/r0/dreambox/deb/dreambox ./\n")); got != "http://dreamboxupdate.com/opendreambox/2.6/unstable/r0/dreambox/deb/dreambox" {
		t.Fatal(got)
	}
}

func TestReleasesPinned(t *testing.T) {
	for arch, r := range Releases {
		if r.Version != Version || len(r.SHA256) != 64 || !strings.Contains(r.URL, Version) {
			t.Errorf("%s: %+v", arch, r)
		}
	}
}

func TestSafeDataPath(t *testing.T) {
	for p, want := range map[string]bool{
		DataRoot:                       true,
		Base + "/anything":             true,
		"/media/hdd/dreamhub-apps":     true,
		"/media/usb/dreamhub-apps":     true,
		Base:                           false, // removed only when empty
		"/media/hdd":                   false,
		"/media/hdd/movie":             false,
		"/dreamhub-apps":               false,
		"/etc/enigma2":                 false,
		"/data/dreamhub-docker/../etc": false,
		"relative/dreamhub-apps":       false,
		"":                             false,
	} {
		if got := SafeDataPath(p); got != want {
			t.Errorf("SafeDataPath(%q) = %v, want %v", p, got, want)
		}
	}
}

package dockersetup

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Exec sets Docker up on the real box.
type Exec struct {
	// HTTP fetches the Docker release and the official feed's overlay
	// module; nil means a client with a generous timeout.
	HTTP *http.Client
	// Arch overrides runtime.GOARCH (tests).
	Arch string
}

var _ Setup = Exec{}

// officialFeedFiles are where the box keeps the official Dreambox feed's
// machine-specific entry — usually disabled (.off) on a Gemini image,
// which is fine: only its URL is read, the feed itself is never switched
// on (that would offer the official image's packages over Gemini's).
var officialFeedFiles = []string{
	"/etc/apt/sources.list.d/dreambox-feed.list",
	"/etc/apt/sources.list.d/dreambox-feed.list.off",
}

func (e Exec) arch() string {
	if e.Arch != "" {
		return e.Arch
	}
	return runtime.GOARCH
}

func (e Exec) client() *http.Client {
	if e.HTTP != nil {
		return e.HTTP
	}
	return &http.Client{Timeout: 15 * time.Minute}
}

func (e Exec) Status(ctx context.Context) (State, error) {
	var st State
	rel, ok := Releases[e.arch()]
	st.Supported = ok
	st.Version = installedVersion()
	st.Overlay = overlayAvailable()
	_, err := os.Stat(ConfigFile)
	st.Configured = err == nil
	st.DockerRunning = unitActive(ctx, DockerService)
	st.HelperRunning = unitActive(ctx, HelperService)
	st.FreeBytes = freeBytes("/data")
	st.Ready = ok && st.Version == rel.Version && st.Overlay && st.Configured && st.DockerRunning && st.HelperRunning
	return st, nil
}

func (e Exec) Plan(ctx context.Context) ([]Step, error) {
	rel, ok := Releases[e.arch()]
	if !ok {
		return nil, ErrUnsupported
	}
	var steps []Step
	if !overlayAvailable() {
		s, err := e.overlayStep(ctx)
		if err != nil {
			return nil, err
		}
		steps = append(steps, s)
	}
	if installedVersion() != rel.Version {
		steps = append(steps, Step{Kind: "download", Detail: map[string]string{"version": rel.Version, "url": rel.URL, "to": BinDir}})
	}
	if want, err := daemonConfig(); err == nil {
		if have, err := os.ReadFile(ConfigFile); err != nil || !bytes.Equal(have, want) {
			steps = append(steps, Step{Kind: "config", Detail: map[string]string{"file": ConfigFile, "dataRoot": DataRoot}})
		}
	}
	if !unitActive(ctx, DockerService) || !unitActive(ctx, HelperService) {
		steps = append(steps, Step{Kind: "services", Detail: map[string]string{"docker": DockerService, "helper": HelperService}})
	}
	return steps, nil
}

// overlayStep works out where the overlay module comes from: an
// installed module file just needs loading; otherwise the enabled feeds
// if their version is exactly the running kernel's build, otherwise the
// official Dreambox feed's matching one. A module from a different
// kernel build is never used — on the box, the Gemini feed's is
// 4.9.113 while the kernel is 4.9.269-dream.
func (e Exec) overlayStep(ctx context.Context) (Step, error) {
	if moduleFileExists() {
		return Step{Kind: "overlay", Detail: map[string]string{"source": "installed"}}, nil
	}
	kernel, err := dpkgVersion(ctx, "kernel-image")
	if err != nil || kernel == "" {
		return Step{}, ErrNoMatchingOverlay
	}
	if cand := aptCandidate(ctx, "kernel-module-overlay"); cand == kernel {
		return Step{Kind: "overlay", Detail: map[string]string{"source": "feed", "version": kernel}}, nil
	}
	url, sum, err := e.officialOverlay(ctx, kernel)
	if err != nil {
		return Step{}, err
	}
	return Step{Kind: "overlay", Detail: map[string]string{"source": "official", "version": kernel, "url": url, "sha256": sum}}, nil
}

func (e Exec) Apply(ctx context.Context, steps []Step) (string, error) {
	var log strings.Builder
	logf := func(format string, a ...any) { fmt.Fprintf(&log, format+"\n", a...) }
	for _, s := range steps {
		var err error
		switch s.Kind {
		case "overlay":
			err = e.applyOverlay(ctx, s, logf)
		case "download":
			err = e.applyDownload(ctx, logf)
		case "config":
			err = applyConfig(logf)
		case "services":
			err = applyServices(ctx, logf)
		default:
			err = fmt.Errorf("unknown step %q", s.Kind)
		}
		if err != nil {
			logf("FAILED: %v", err)
			return log.String(), err
		}
	}
	return log.String(), nil
}

func (e Exec) applyOverlay(ctx context.Context, s Step, logf func(string, ...any)) error {
	switch s.Detail["source"] {
	case "feed":
		out, err := run(ctx, "apt-get", "-y", "-q", "-o", "Dpkg::Use-Pty=0", "install", "--", "kernel-module-overlay="+s.Detail["version"])
		logf("%s", out)
		if err != nil {
			return err
		}
	case "official":
		deb, err := e.fetchVerified(ctx, s.Detail["url"], s.Detail["sha256"], filepath.Join(Base, "kernel-module-overlay.deb"))
		if err != nil {
			return err
		}
		defer os.Remove(deb)
		logf("Downloaded and verified %s", s.Detail["url"])
		out, err := run(ctx, "dpkg", "-i", deb)
		logf("%s", out)
		if err != nil {
			return err
		}
	}
	if out, err := run(ctx, "modprobe", "overlay"); err != nil {
		logf("%s", out)
		return err
	}
	// Loaded again at every boot.
	if err := os.MkdirAll("/etc/modules-load.d", 0o755); err == nil {
		_ = os.WriteFile("/etc/modules-load.d/dreamhub-docker.conf", []byte("overlay\n"), 0o644)
	}
	logf("overlay module loaded")
	return nil
}

func (e Exec) applyDownload(ctx context.Context, logf func(string, ...any)) error {
	rel := Releases[e.arch()]
	if err := os.MkdirAll(Base, 0o755); err != nil {
		return err
	}
	tgz, err := e.fetchVerified(ctx, rel.URL, rel.SHA256, filepath.Join(Base, "docker-"+rel.Version+".tgz"))
	if err != nil {
		return err
	}
	defer os.Remove(tgz)
	logf("Downloaded Docker %s, checksum verified", rel.Version)
	staging := BinDir + ".new"
	_ = os.RemoveAll(staging)
	if err := extractDocker(tgz, staging); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(staging, "VERSION"), []byte(rel.Version+"\n"), 0o644); err != nil {
		return err
	}
	_ = os.RemoveAll(BinDir + ".old")
	if _, err := os.Stat(BinDir); err == nil {
		if err := os.Rename(BinDir, BinDir+".old"); err != nil {
			return err
		}
	}
	if err := os.Rename(staging, BinDir); err != nil {
		return err
	}
	_ = os.RemoveAll(BinDir + ".old")
	logf("Docker %s unpacked to %s", rel.Version, BinDir)
	return nil
}

func applyConfig(logf func(string, ...any)) error {
	cfg, err := daemonConfig()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(DataRoot, 0o711); err != nil {
		return err
	}
	if err := os.WriteFile(ConfigFile+".tmp", cfg, 0o644); err != nil {
		return err
	}
	if err := os.Rename(ConfigFile+".tmp", ConfigFile); err != nil {
		return err
	}
	logf("Wrote %s", ConfigFile)
	return nil
}

func applyServices(ctx context.Context, logf func(string, ...any)) error {
	for _, args := range [][]string{
		{"daemon-reload"},
		{"enable", DockerService, HelperService},
		{"restart", DockerService},
		{"restart", HelperService},
	} {
		out, err := run(ctx, "systemctl", args...)
		if err != nil {
			logf("%s", out)
			return fmt.Errorf("systemctl %s: %w", strings.Join(args, " "), err)
		}
	}
	// dockerd takes a few seconds to answer on its socket.
	for i := 0; i < 30; i++ {
		if unitActive(ctx, DockerService) && unitActive(ctx, HelperService) {
			if _, err := os.Stat(Socket); err == nil {
				logf("Docker and DreamHub's Docker helper are running")
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return errors.New("the Docker services didn't come up; see the device logs for dreamhub-docker")
}

// daemonConfig is dockerd's config: everything on /data, no bridge and no
// iptables (the box's kernel can't do Docker's networking; apps use the
// host network), its own socket so it never collides with a vendor
// Docker, and bounded container logs on the flash.
func daemonConfig() ([]byte, error) {
	return json.MarshalIndent(map[string]any{
		"data-root":  DataRoot,
		"exec-root":  "/run/dreamhub-docker",
		"pidfile":    "/run/dreamhub-docker.pid",
		"hosts":      []string{"unix://" + Socket},
		"bridge":     "none",
		"iptables":   false,
		"ip6tables":  false,
		"log-driver": "json-file",
		"log-opts":   map[string]string{"max-size": "5m", "max-file": "2"},
	}, "", "  ")
}

// fetchVerified downloads url to dest and checks its SHA-256; a mismatch
// deletes the file and fails.
func (e Exec) fetchVerified(ctx context.Context, url, sum, dest string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := e.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dest)
		return "", err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != sum {
		os.Remove(dest)
		return "", fmt.Errorf("download %s: checksum mismatch (got %s)", url, got)
	}
	return dest, nil
}

// extractDocker unpacks the release's docker/ directory into dir —
// regular files only, no paths outside it.
func extractDocker(tgz, dir string) error {
	f, err := os.Open(tgz)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	n := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := filepath.Base(hdr.Name)
		if filepath.Dir(filepath.Clean(hdr.Name)) != "docker" || name == "." || name == ".." {
			continue
		}
		out, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		n++
	}
	if _, err := os.Stat(filepath.Join(dir, "dockerd")); err != nil || n == 0 {
		return errors.New("the Docker archive had no dockerd in it")
	}
	return nil
}

// officialOverlay finds the overlay module for kernel in the official
// Dreambox feed's index and returns its URL and SHA-256.
func (e Exec) officialOverlay(ctx context.Context, kernel string) (string, string, error) {
	base := ""
	for _, f := range officialFeedFiles {
		if b, err := os.ReadFile(f); err == nil {
			if base = feedURL(b); base != "" {
				break
			}
		}
	}
	if base == "" {
		return "", "", ErrNoMatchingOverlay
	}
	for _, name := range []string{"Packages.gz", "Packages"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/"+name, nil)
		if err != nil {
			return "", "", err
		}
		resp, err := e.client().Do(req)
		if err != nil {
			continue
		}
		var r io.Reader = resp.Body
		if strings.HasSuffix(name, ".gz") && resp.StatusCode == http.StatusOK {
			gz, err := gzip.NewReader(resp.Body)
			if err != nil {
				resp.Body.Close()
				continue
			}
			r = gz
		}
		if resp.StatusCode == http.StatusOK {
			file, sum := FindPackage(r, "kernel-module-overlay", kernel)
			resp.Body.Close()
			if file != "" && sum != "" {
				return base + "/" + strings.TrimPrefix(file, "./"), sum, nil
			}
			return "", "", ErrNoMatchingOverlay
		}
		resp.Body.Close()
	}
	return "", "", ErrNoMatchingOverlay
}

// FindPackage scans an apt Packages index for name at exactly version
// and returns its Filename and SHA256.
func FindPackage(r io.Reader, name, version string) (file, sum string) {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 1<<20)
	var pkg, ver, fn, sha string
	check := func() bool { return pkg == name && ver == version && fn != "" && sha != "" }
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if check() {
				return fn, sha
			}
			pkg, ver, fn, sha = "", "", "", ""
			continue
		}
		k, v, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		switch k {
		case "Package":
			pkg = v
		case "Version":
			ver = v
		case "Filename":
			fn = v
		case "SHA256":
			sha = v
		}
	}
	if check() {
		return fn, sha
	}
	return "", ""
}

func feedURL(content []byte) string {
	for _, l := range strings.Split(string(content), "\n") {
		f := strings.Fields(l)
		if len(f) >= 2 && f[0] == "deb" {
			for _, x := range f[1:] {
				if strings.HasPrefix(x, "http") {
					return strings.TrimSuffix(x, "/")
				}
			}
		}
	}
	return ""
}

func installedVersion() string {
	b, err := os.ReadFile(filepath.Join(BinDir, "VERSION"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func overlayAvailable() bool {
	b, err := os.ReadFile("/proc/filesystems")
	return err == nil && bytes.Contains(b, []byte("\toverlay\n"))
}

func moduleFileExists() bool {
	release, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join("/lib/modules", strings.TrimSpace(string(release)), "kernel/fs/overlayfs/overlay.ko"))
	return err == nil
}

func unitActive(ctx context.Context, unit string) bool {
	return exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", unit).Run() == nil
}

func dpkgVersion(ctx context.Context, pkg string) (string, error) {
	out, err := exec.CommandContext(ctx, "dpkg-query", "-W", "-f", "${Version}", pkg).Output()
	return strings.TrimSpace(string(out)), err
}

func aptCandidate(ctx context.Context, pkg string) string {
	out, err := exec.CommandContext(ctx, "apt-cache", "policy", pkg).Output()
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), "Candidate: "); ok {
			return v
		}
	}
	return ""
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

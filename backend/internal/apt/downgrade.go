package apt

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Downgrading a package to the version before (docs/APPSHUB_PLAN.md, App14).
// Only where the enabled feeds still carry an older version: `apt-cache
// madison` lists every version the feeds offer, and the installed one is
// compared against them with dpkg's own version order. Feeds usually carry
// only the latest version, so for most packages there is nothing to offer —
// and nothing is shown. The change itself is an ordinary plan: apt-get's
// simulate run (with --allow-downgrades) lists everything it would touch,
// including packages that depend on the one going back, and the same
// command without -s applies it.

// Downgrader offers the older versions of an installed package and changes
// to one of them.
type Downgrader interface {
	// OlderVersions lists the versions older than the installed one that the
	// feeds carry, newest first; none for a package that is not installed.
	OlderVersions(ctx context.Context, name string) ([]string, error)
	SimulateDowngrade(ctx context.Context, name, version string) ([]Step, error)
	ApplyDowngrade(ctx context.Context, name, version string) (output string, err error)
}

// ErrInvalidVersion: a version string dpkg itself wouldn't accept. Checked
// before anything runs, so a version can never turn into an apt-get option.
var ErrInvalidVersion = errors.New("apt: invalid version")

// Debian policy: alphanumerics and . + - ~ : (an epoch has the colon),
// starting with a digit or letter.
var versionRE = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+~:-]*$`)

// ValidVersion reports whether v can be given to apt-get as name=v.
func ValidVersion(v string) bool { return len(v) <= 128 && versionRE.MatchString(v) }

// ParseMadison reads `apt-cache madison name`: "name | version | source",
// one line per version and feed, newest first. Each version once.
func ParseMadison(out []byte) []string {
	var versions []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Split(sc.Text(), "|")
		if len(f) < 3 {
			continue
		}
		v := strings.TrimSpace(f[1])
		if v != "" && !seen[v] {
			seen[v] = true
			versions = append(versions, v)
		}
	}
	return versions
}

// maxOlder bounds how many older versions are looked at (each is one dpkg
// comparison).
const maxOlder = 8

// versionLess is dpkg's own comparison; replaced in tests.
var versionLess = func(ctx context.Context, a, b string) bool {
	return exec.CommandContext(ctx, "dpkg", "--compare-versions", a, "lt", b).Run() == nil
}

func (Exec) OlderVersions(ctx context.Context, name string) ([]string, error) {
	if err := ValidNames(ActionInstall, []string{name}); err != nil {
		return nil, err
	}
	installed, err := exec.CommandContext(ctx, "dpkg-query", "-W", "-f", "${Version}", name).Output()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, ErrUnavailable
	}
	if err != nil || len(bytes.TrimSpace(installed)) == 0 {
		return nil, nil // not installed
	}
	madison, err := exec.CommandContext(ctx, "apt-cache", "madison", "--", name).Output()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, ErrUnavailable
	}
	if err != nil {
		return nil, err
	}
	return olderOf(ctx, ParseMadison(madison), strings.TrimSpace(string(installed))), nil
}

// olderOf keeps the versions that dpkg orders before installed.
func olderOf(ctx context.Context, versions []string, installed string) []string {
	var older []string
	for _, v := range versions {
		if v == installed || !ValidVersion(v) {
			continue
		}
		if versionLess(ctx, v, installed) {
			older = append(older, v)
			if len(older) == maxOlder {
				break
			}
		}
	}
	return older
}

func downgradeArgs(name, version string, simulate bool) []string {
	a := []string{"-y", "-q", "-o", "Dpkg::Options::=--force-confold", "-o", "Dpkg::Use-Pty=0"}
	if simulate {
		a = append(a, "-s")
	}
	return append(a, "install", "--allow-downgrades", "--", name+"="+version)
}

func checkDowngrade(name, version string) error {
	if err := ValidNames(ActionInstall, []string{name}); err != nil {
		return err
	}
	if !ValidVersion(version) {
		return fmt.Errorf("%w: %q", ErrInvalidVersion, version)
	}
	return nil
}

func (Exec) SimulateDowngrade(ctx context.Context, name, version string) ([]Step, error) {
	if err := checkDowngrade(name, version); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "apt-get", downgradeArgs(name, version, true)...)
	cmd.Env = aptEnv()
	out, err := cmd.CombinedOutput()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, ErrUnavailable
	}
	if err != nil {
		return nil, &SimulateError{Output: lastLines(out, 12)}
	}
	return MarkDowngrade(ctx, ParseSimulate(out), name), nil
}

// MarkDowngrade names the steps that go back a downgrade (apt prints them
// like an upgrade): the package asked for, and any other whose new version
// dpkg orders before the old one.
func MarkDowngrade(ctx context.Context, steps []Step, name string) []Step {
	for i := range steps {
		s := &steps[i]
		if s.Kind != "upgrade" {
			continue
		}
		if s.Name == name || (s.OldVersion != "" && s.NewVersion != "" && versionLess(ctx, s.NewVersion, s.OldVersion)) {
			s.Kind = "downgrade"
		}
	}
	return steps
}

func (Exec) ApplyDowngrade(ctx context.Context, name, version string) (string, error) {
	if err := checkDowngrade(name, version); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, applyTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "apt-get", downgradeArgs(name, version, false)...)
	cmd.Env = aptEnv()
	out, err := cmd.CombinedOutput()
	if len(out) > outputCap {
		out = append([]byte("…\n"), out[len(out)-outputCap:]...)
	}
	return string(out), err
}

var _ Downgrader = Exec{}

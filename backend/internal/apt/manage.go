package apt

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Install, upgrade and remove as a plan (docs/ADMINHUB_PLAN.md, A2,
// "Install/remove as a plan, with a real preview"): apt-get's own simulate
// run is the preview, the same command without -s is the apply. Output
// formats confirmed on the reference box, apt 1.2.12 (ARCHITECTURE.md,
// "Box checks for 0.4.0").

// Action is what a package plan does.
type Action string

const (
	ActionInstall Action = "install" // also upgrades an installed package
	ActionRemove  Action = "remove"  // keeps the package's config files
	// ActionUpgrade with no names upgrades everything upgradable.
	ActionUpgrade Action = "upgrade"
)

// Step is one package apt would touch.
type Step struct {
	// Kind: "install", "upgrade" or "remove".
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	OldVersion string `json:"oldVersion,omitempty"`
	NewVersion string `json:"newVersion,omitempty"`
}

// Manager simulates and applies package changes, and lists what the
// configured feeds offer beyond what's installed.
type Manager interface {
	Simulate(ctx context.Context, action Action, names []string) ([]Step, error)
	Apply(ctx context.Context, action Action, names []string) (output string, err error)
	Available(ctx context.Context) ([]Package, error)
}

var _ Manager = Exec{}

// ErrInvalidName: a package name apt itself wouldn't accept. Checked
// before anything runs, so a name can never turn into an apt-get option.
var ErrInvalidName = errors.New("apt: invalid package name")

// ErrInvalidAction: not one of the three actions.
var ErrInvalidAction = errors.New("apt: invalid action")

// Debian policy: lowercase letters, digits, + - . — at least two
// characters, starting alphanumeric.
var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]+$`)

// ValidNames checks names and the action together.
func ValidNames(action Action, names []string) error {
	switch action {
	case ActionInstall, ActionRemove:
		if len(names) == 0 {
			return ErrInvalidName
		}
	case ActionUpgrade:
	default:
		return ErrInvalidAction
	}
	for _, n := range names {
		if !nameRE.MatchString(n) {
			return fmt.Errorf("%w: %q", ErrInvalidName, n)
		}
	}
	return nil
}

// applyTimeout bounds one real apt-get run: downloading and unpacking a
// handful of packages on the box takes seconds to a minute, a skin or
// picon set a few minutes.
const applyTimeout = 15 * time.Minute

// outputCap is how much of apt-get's output is kept for the result.
const outputCap = 256 << 10

func args(action Action, names []string, simulate bool) []string {
	// Dpkg::Use-Pty=0: no "(Reading database ... 65%" progress noise in
	// the output kept for the result (seen on the box).
	a := []string{"-y", "-q", "-o", "Dpkg::Options::=--force-confold", "-o", "Dpkg::Use-Pty=0"}
	if simulate {
		a = append(a, "-s")
	}
	switch action {
	case ActionUpgrade:
		if len(names) == 0 {
			return append(a, "upgrade")
		}
		// Upgrading named packages: install them again, which apt treats
		// as "bring to the candidate version".
		a = append(a, "install", "--only-upgrade")
	default:
		a = append(a, string(action))
	}
	return append(append(a, "--"), names...)
}

func aptEnv() []string {
	return append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "LC_ALL=C")
}

func (Exec) Simulate(ctx context.Context, action Action, names []string) ([]Step, error) {
	if err := ValidNames(action, names); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "apt-get", args(action, names, true)...)
	cmd.Env = aptEnv()
	out, err := cmd.CombinedOutput()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, ErrUnavailable
	}
	if err != nil {
		return nil, &SimulateError{Output: lastLines(out, 12)}
	}
	return ParseSimulate(out), nil
}

// SimulateError: apt refused the change (an unknown package, a
// dependency it can't satisfy). Output is apt's own explanation.
type SimulateError struct{ Output string }

func (e *SimulateError) Error() string { return "apt: simulate failed: " + e.Output }

func (Exec) Apply(ctx context.Context, action Action, names []string) (string, error) {
	if err := ValidNames(action, names); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, applyTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "apt-get", args(action, names, false)...)
	cmd.Env = aptEnv()
	out, err := cmd.CombinedOutput()
	if len(out) > outputCap {
		out = append([]byte("…\n"), out[len(out)-outputCap:]...)
	}
	return string(out), err
}

// ParseSimulate reads apt-get -s output: "Inst name [old] (new suite
// [arch])" is an upgrade, "Inst name (new suite [arch])" a new install,
// "Remv name [old]" a removal. "Conf" lines repeat the Inst ones and are
// skipped.
func ParseSimulate(out []byte) []Step {
	var steps []Step
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		var kind string
		switch {
		case strings.HasPrefix(line, "Inst "):
			kind = "install"
		case strings.HasPrefix(line, "Remv "):
			kind = "remove"
		default:
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		s := Step{Kind: kind, Name: fields[1]}
		if len(fields) > 2 && strings.HasPrefix(fields[2], "[") {
			s.OldVersion = strings.Trim(fields[2], "[]")
			if kind == "install" {
				s.Kind = "upgrade"
			}
		}
		if open, close := strings.Index(line, "("), strings.LastIndex(line, ")"); kind == "install" && open >= 0 && close > open {
			if inside := strings.Fields(line[open+1 : close]); len(inside) > 0 {
				s.NewVersion = inside[0]
			}
		}
		steps = append(steps, s)
	}
	return steps
}

// Available lists every package the enabled feeds offer that isn't
// installed — the "not installed" catalog. apt-cache dumpavail prints
// one stanza per package version from the local index.
func (Exec) Available(ctx context.Context) ([]Package, error) {
	out, err := exec.CommandContext(ctx, "apt-cache", "dumpavail").Output()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, ErrUnavailable
	}
	if err != nil {
		return nil, err
	}
	installed, err := exec.CommandContext(ctx, "dpkg-query", "-W", "-f", "${Package}\t${Status}\n").Output()
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, l := range strings.Split(string(installed), "\n") {
		if name, st, ok := strings.Cut(l, "\t"); ok && strings.HasSuffix(st, " installed") {
			have[name] = true
		}
	}
	var pkgs []Package
	for _, p := range ParseDumpavail(out) {
		if !have[p.Name] {
			pkgs = append(pkgs, p)
		}
	}
	return pkgs, nil
}

// ParseDumpavail reads apt-cache dumpavail's stanzas into packages, one
// per name (the first stanza wins — dumpavail lists the candidate first).
func ParseDumpavail(out []byte) []Package {
	seen := map[string]bool{}
	var pkgs []Package
	var cur Package
	var xcat string // X-DreamHub-Category wins over Section, in any field order
	flush := func() {
		if xcat != "" {
			cur.Category = xcat
		}
		xcat = ""
		if cur.Name != "" && !seen[cur.Name] {
			seen[cur.Name] = true
			pkgs = append(pkgs, cur)
		}
		cur = Package{}
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			flush()
			continue
		}
		key, val, ok := strings.Cut(line, ": ")
		if !ok || strings.HasPrefix(line, " ") {
			continue
		}
		switch key {
		case "Package":
			cur.Name = val
		case "Version":
			cur.Version = val
		case "Section":
			cur.Category = val
		case "X-DreamHub-Category":
			xcat = val
		case "Description":
			cur.Description = val
		}
	}
	flush()
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Name < pkgs[j].Name })
	return pkgs
}

func lastLines(out []byte, n int) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

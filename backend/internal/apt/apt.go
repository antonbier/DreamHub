// Package apt lists the box's installed and upgradable system packages for
// AdminHub's package browser (docs/ADMINHUB_PLAN.md, A2, #47). Read-only for
// now — install/remove as a plan with a real `apt-get -s` simulate preview
// is the next slice, and needs the box to confirm that output and the
// service-restart check before it's built. Like internal/systemd: Exec
// shells out for real, ./mock answers from memory.
package apt

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os/exec"
	"sort"
	"strings"
)

// Package is one row of the browser.
type Package struct {
	Name string `json:"name"`
	// Category groups the list. It's apt's own Section field
	// (docs/ADMINHUB_PLAN.md, "Categories come from apt's own metadata") —
	// never a DreamHub-maintained lookup, so a package with a Section
	// DreamHub has never seen just becomes a new group.
	Category         string `json:"category"`
	Version          string `json:"version"`
	AvailableVersion string `json:"availableVersion,omitempty"` // set only when Upgradable
	Upgradable       bool   `json:"upgradable"`
	Description      string `json:"description"`
}

// Lister lists the box's installed packages.
type Lister interface {
	List(ctx context.Context) ([]Package, error)
}

// ErrUnavailable means dpkg/apt isn't on this system (a development
// machine without them).
var ErrUnavailable = errors.New("apt: dpkg-query not available")

// Exec runs dpkg-query and apt.
type Exec struct{}

var _ Lister = Exec{}

// dpkgFields is dpkg-query's own format mini-language, one package per
// line, fields separated by a tab that can't appear inside any of them.
const dpkgFields = `${Package}\t${Version}\t${Section}\t${Description}\n`

func (Exec) List(ctx context.Context) ([]Package, error) {
	out, err := exec.CommandContext(ctx, "dpkg-query", "-W", "-f", dpkgFields).Output()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, ErrUnavailable
	}
	if err != nil {
		return nil, err
	}
	pkgs := ParseDpkgQuery(out)

	// apt-get's own simulate run of "upgrade" lists exactly the packages
	// that would be touched, with old/new version in its "Inst" lines —
	// the same mechanism the real install/remove preview will reuse
	// (docs/ADMINHUB_PLAN.md). Not fatal if it fails (older apt without a
	// cache yet, or run without the index refreshed): the installed list
	// above still stands, just without upgrade info.
	if up, err := exec.CommandContext(ctx, "apt-get", "upgrade", "--simulate").Output(); err == nil {
		applyUpgradable(pkgs, ParseAptSimulate(up))
	}

	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Name < pkgs[j].Name })
	return pkgs, nil
}

// ParseDpkgQuery parses dpkgFields-formatted output into packages, keyed by
// name for applyUpgradable to look up.
func ParseDpkgQuery(out []byte) []Package {
	var pkgs []Package
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		f := strings.SplitN(sc.Text(), "\t", 4)
		if len(f) < 3 || f[0] == "" {
			continue
		}
		p := Package{Name: f[0], Version: f[1], Category: f[2]}
		if len(f) == 4 {
			p.Description = f[3]
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

// ParseAptSimulate reads the "Inst name [old (new ...)]" lines from
// `apt-get upgrade --simulate`'s output into name -> new version.
func ParseAptSimulate(out []byte) map[string]string {
	versions := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "Inst ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[1]
		open, close := strings.Index(line, "("), strings.LastIndex(line, ")")
		if open < 0 || close < open {
			continue
		}
		// Inside the parens: "newversion suite [arch]" — the version is
		// always the first field.
		inside := strings.Fields(line[open+1 : close])
		if len(inside) == 0 {
			continue
		}
		versions[name] = inside[0]
	}
	return versions
}

func applyUpgradable(pkgs []Package, versions map[string]string) {
	for i := range pkgs {
		if v, ok := versions[pkgs[i].Name]; ok {
			pkgs[i].Upgradable, pkgs[i].AvailableVersion = true, v
		}
	}
}

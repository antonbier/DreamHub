package dockersetup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// "Uninstall Docker" on the real box. Only what setup created is touched:
// the two services (their unit files belong to the .deb and stay), the
// overlay loading file, the binaries and daemon config under Base — and,
// only when asked, the data. enigma2 and the recordings are never part of
// it. Every step is looked up first, so running it again (or after a
// half-finished run) just finds less to do.

func daemonFiles() []string {
	return []string{BinDir, BinDir + ".new", BinDir + ".old", ConfigFile, ConfigFile + ".tmp"}
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func unitEnabled(ctx context.Context, unit string) bool {
	return exec.CommandContext(ctx, "systemctl", "is-enabled", "--quiet", unit).Run() == nil
}

func (e Exec) PlanRemove(ctx context.Context, opts RemoveOptions) ([]Step, error) {
	var steps []Step
	units := map[string]string{"docker": DockerService, "helper": HelperService}
	if unitActive(ctx, DockerService) || unitActive(ctx, HelperService) || unitEnabled(ctx, DockerService) || unitEnabled(ctx, HelperService) {
		steps = append(steps, Step{Kind: "services", Detail: units})
	}
	if exists(ModulesFile) {
		steps = append(steps, Step{Kind: "modules", Detail: map[string]string{"file": ModulesFile}})
	}
	var files []string
	for _, f := range daemonFiles() {
		if exists(f) {
			files = append(files, f)
		}
	}
	if len(files) > 0 {
		steps = append(steps, Step{Kind: "files", Detail: map[string]string{"paths": strings.Join(files, ",")}})
	}
	if opts.DeleteData {
		if paths := dataPaths(opts); len(paths) > 0 {
			steps = append(steps, Step{Kind: "data", Detail: map[string]string{"paths": strings.Join(paths, ",")}})
		}
	}
	return steps, nil
}

// dataPaths are the existing places "delete the app data" would clear.
func dataPaths(opts RemoveOptions) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range append([]string{DataRoot}, opts.AppRoots...) {
		if !SafeDataPath(p) || seen[p] || !exists(p) {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func (e Exec) Remove(ctx context.Context, steps []Step, opts RemoveOptions) (string, error) {
	var log strings.Builder
	logf := func(format string, a ...any) { fmt.Fprintf(&log, format+"\n", a...) }
	for _, s := range steps {
		var err error
		switch s.Kind {
		case "services":
			for _, args := range [][]string{
				{"stop", HelperService, DockerService},
				{"disable", HelperService, DockerService},
			} {
				// A unit that's already gone or stopped is fine.
				_, _ = run(ctx, "systemctl", args...)
			}
			if unitActive(ctx, DockerService) || unitActive(ctx, HelperService) {
				err = fmt.Errorf("the Docker services are still running")
				break
			}
			logf("Docker and DreamHub's Docker helper stopped and switched off")
		case "modules":
			err = removeIfThere(ModulesFile, logf)
		case "files":
			for _, f := range daemonFiles() {
				if err = removeIfThere(f, logf); err != nil {
					break
				}
			}
		case "data":
			for _, p := range dataPaths(opts) {
				if err = removeIfThere(p, logf); err != nil {
					break
				}
			}
			if err == nil {
				// The folder itself, if nothing else lives in it.
				if os.Remove(Base) == nil {
					logf("Removed %s", Base)
				}
			}
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

func removeIfThere(p string, logf func(string, ...any)) error {
	if !exists(p) {
		return nil
	}
	if err := os.RemoveAll(p); err != nil {
		return err
	}
	logf("Removed %s", p)
	return nil
}

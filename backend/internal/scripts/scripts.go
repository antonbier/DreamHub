// Package scripts runs an admin-uploaded script for AdminHub's library
// (docs/ADMINHUB_PLAN.md, A3, #47) — by hand, after confirmation, never
// automatically. DreamHub runs on the box itself (ARCHITECTURE.md), so this
// executes locally; like internal/systemd and internal/journald, Exec does
// it for real, ./mock answers from memory.
package scripts

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

// Runner runs a script's content and reports what happened. err is only set
// when the script couldn't even be started (e.g. the temp file couldn't be
// written); a script that runs and exits non-zero is just a non-zero
// exitCode, not an error.
type Runner interface {
	Run(ctx context.Context, content string) (output string, exitCode int, err error)
}

// Timeout bounds a single run — long enough for an admin's one-off script,
// short enough that a hung one doesn't tie up the apply request forever.
// Running scripts through the background job queue instead (ROADMAP.md,
// AdminHub) is future work if this turns out too short in practice.
const Timeout = 2 * time.Minute

// MaxOutput caps what's kept — a script that floods stdout shouldn't bloat
// the database; the admin still sees it was truncated.
const MaxOutput = 256 * 1024

// Exec runs the script as a temporary executable file.
type Exec struct{}

func (Exec) Run(ctx context.Context, content string) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	f, err := os.CreateTemp("", "dreamhub-script-*")
	if err != nil {
		return "", -1, err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return "", -1, err
	}
	if err := f.Close(); err != nil {
		return "", -1, err
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return "", -1, err
	}

	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, path)
	cmd.Stdout, cmd.Stderr = &out, &out
	// After the timeout kills the script, a child still holding the output
	// pipe must not keep this call (and the apply request) waiting.
	cmd.WaitDelay = 5 * time.Second
	runErr := cmd.Run()

	output := out.String()
	truncated := false
	if len(output) > MaxOutput {
		output, truncated = output[:MaxOutput], true
	}
	if truncated {
		output += "\n…(output truncated)"
	}

	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		return output, 0, nil
	case errors.As(runErr, &exitErr):
		return output, exitErr.ExitCode(), nil
	default:
		// Didn't even start (permissions, interpreter missing, …) or was
		// killed by the timeout above.
		return output, -1, runErr
	}
}

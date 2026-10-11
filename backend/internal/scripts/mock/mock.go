// Package mock implements scripts.Runner in memory, for testing AdminHub's
// script library without actually executing anything.
package mock

import (
	"context"
	"sync"

	"github.com/antonbier/DreamHub/backend/internal/scripts"
)

// Runner answers every run with canned output, recording what it ran.
type Runner struct {
	// Output, ExitCode and Err are returned for every call; the default
	// (empty output, exit 0, no error) is a script that succeeded quietly.
	Output   string
	ExitCode int
	Err      error

	mu  sync.Mutex
	ran []string // scripts' content, in order
}

var _ scripts.Runner = (*Runner)(nil)

func (r *Runner) Run(_ context.Context, content string) (string, int, error) {
	r.mu.Lock()
	r.ran = append(r.ran, content)
	r.mu.Unlock()
	return r.Output, r.ExitCode, r.Err
}

// Ran returns every script content passed to Run, in order.
func (r *Runner) Ran() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ran...)
}

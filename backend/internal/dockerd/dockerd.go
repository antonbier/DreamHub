// Package dockerd is AppsHub's Docker isolation layer (docs/APPSHUB_PLAN.md,
// App2, #46-adjacent — AppsHub has no issue yet). Giving DreamHub's own
// backend the Docker socket would turn any bug in that large, everything
// else process into a full box compromise — the socket is root-equivalent,
// a privileged container can escape to host root. Instead, this package is
// used two ways:
//
//   - inside a small, separate helper process (cmd/dreamhub-dockerd-helper)
//     that is the only thing with real Docker socket access, via Exec;
//   - inside DreamHub's own backend, which only ever talks to the helper
//     over loopback, via HTTPClient — never to the socket directly.
//
// Both sides implement the same Client interface, and it is deliberately
// narrow: list/start/stop/logs/exec, scoped to containers carrying the
// dreamhub.app label (AppLabel) — the "fixed, whitelisted subset of the
// Docker API" the plan calls for, the same shape Tecnativa's
// docker-socket-proxy uses. There is no create/run/pull here (AppsHub
// App4) and no arbitrary mount or --privileged support anywhere in this
// package: the hard denials the plan requires are enforced by this
// surface simply never offering them, not by a runtime check that could
// be bypassed.
package dockerd

import (
	"context"
	"errors"
	"io"
)

// AppLabel is the container label every container this package will touch
// must carry, with its own app's id as the value — "a DreamHub-owned
// label prefix" (docs/APPSHUB_PLAN.md, "Docker access is isolated"). A
// container without it is invisible to this package, not just filtered
// from a listing: Start/Stop/Logs/Exec all refuse one, the same as
// Enigma2's native WebControl staying off-limits (CLAUDE.md).
const AppLabel = "dreamhub.app"

// Container is one container this package is willing to talk about,
// trimmed to what AppsHub needs.
type Container struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	State  string            `json:"state"`  // created, running, paused, exited, dead, …
	Status string            `json:"status"` // Docker's human string, e.g. "Up 3 hours"
	Labels map[string]string `json:"labels"`
}

// AppID is the Labels[AppLabel] value, or "" if unset (Client
// implementations never return a Container missing it).
func (c Container) AppID() string { return c.Labels[AppLabel] }

// ErrNotFound means no container with that id carries AppLabel.
var ErrNotFound = errors.New("dockerd: no such app container")

// ErrUnavailable means the Docker daemon (Exec) or the helper process
// (HTTPClient) didn't answer at all — AppsHub's "Apps" tab shows this as
// the connector's Unreachable state (docs/APPSHUB_PLAN.md, "not installed
// is not degraded... available, just empty" vs. actually wrong).
var ErrUnavailable = errors.New("dockerd: daemon unreachable")

// Client is the whitelisted Docker API subset, implemented by Exec (real
// socket, inside the helper only) and HTTPClient (loopback, inside
// DreamHub's main backend). See the package doc for why both exist.
type Client interface {
	// List returns every container carrying AppLabel, regardless of
	// state.
	List(ctx context.Context) ([]Container, error)
	// Start starts a stopped container. Returns ErrNotFound if id isn't
	// AppLabel-tagged.
	Start(ctx context.Context, id string) error
	// Stop stops a running container. Returns ErrNotFound if id isn't
	// AppLabel-tagged.
	Stop(ctx context.Context, id string) error
	// Logs returns up to tail lines of the container's stdout+stderr,
	// most recent last. Returns ErrNotFound if id isn't AppLabel-tagged.
	Logs(ctx context.Context, id string, tail int) ([]byte, error)
	// Exec runs a one-shot, non-interactive command inside the container
	// and returns its combined stdout+stderr. Not a console (AppsHub
	// App5 builds that, with Terminal's own safety posture, on top of
	// this) — no stdin, no pty. Returns ErrNotFound if id isn't
	// AppLabel-tagged.
	Exec(ctx context.Context, id string, cmd []string) ([]byte, error)
}

// drainClosed discards and closes r, for defer sites that don't care
// about a response body's content.
func drainClosed(r io.ReadCloser) {
	_, _ = io.Copy(io.Discard, r)
	_ = r.Close()
}

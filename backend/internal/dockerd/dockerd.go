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
// narrow: list/start/stop/logs/exec/create/remove, every one of them
// scoped to containers carrying the dreamhub.app label (AppLabel) — the
// "fixed, whitelisted subset of the Docker API" the plan calls for, the
// same shape Tecnativa's docker-socket-proxy uses. Create (AppsHub App4)
// never takes a raw Docker container spec from its caller: it only ever
// accepts the few scalar fields ContainerSpec declares, and builds the
// actual Docker config itself — there is still no way to ask for
// --privileged, an arbitrary mount, or a re-exposed socket anywhere in
// this package. The hard denials the plan requires hold because this
// surface simply never offers them, not because of a runtime check that
// could be bypassed.
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
	// HostPort is the box's own port reaching this container's main web
	// UI, read back from Docker's own port mapping (0 if none, or if the
	// container exposes no mapped TCP port) — AppsHub never stores this
	// itself; it's always read live, so it survives a restart without
	// going stale.
	HostPort int `json:"hostPort,omitempty"`
}

// AppID is the Labels[AppLabel] value, or "" if unset (Client
// implementations never return a Container missing it).
func (c Container) AppID() string { return c.Labels[AppLabel] }

// ErrNotFound means no container with that id carries AppLabel.
var ErrNotFound = errors.New("dockerd: no such app container")

// ErrAlreadyExists means AppID already has a container (Create is one
// app, one container — a second install needs Remove first).
var ErrAlreadyExists = errors.New("dockerd: this app is already installed")

// ErrUnavailable means the Docker daemon (Exec) or the helper process
// (HTTPClient) didn't answer at all — AppsHub's "Apps" tab shows this as
// the connector's Unreachable state (docs/APPSHUB_PLAN.md, "not installed
// is not degraded... available, just empty" vs. actually wrong).
var ErrUnavailable = errors.New("dockerd: daemon unreachable")

// ContainerSpec is everything Create needs — a handful of scalars, never
// a raw Docker container config (see the package doc). AppsHub (App4)
// computes every field itself from the curated catalog plus the admin's
// own choices (host port, memory limit); nothing here is a free-form path
// or flag an image's own documentation could trick a caller into passing
// straight through to Docker.
type ContainerSpec struct {
	// AppID becomes this container's AppLabel and its fixed name
	// ("dreamhub-app-" + AppID) — one container per AppID; Create refuses
	// a second one (ErrAlreadyExists).
	AppID string
	// Image is pulled before the container is created ("image:tag",
	// e.g. "pihole/pihole:latest").
	Image string
	// Env is passed through as-is (e.g. {"TZ": "Europe/Rome"}) — never
	// shell-interpreted, Docker's API takes them as plain KEY=VALUE
	// strings.
	Env map[string]string
	// HostPort/ContainerPort map one TCP port; 0/0 means no port is
	// published at all (e.g. wireguard, which has no web UI — see
	// appcatalog.App.ContainerPort).
	HostPort, ContainerPort int
	// HostDir is bind-mounted to ContainerDir — HostDir is always a path
	// this package's caller computed itself (AppsHub's own storage root
	// plus AppID), never taken from the image or any other untrusted
	// source.
	HostDir, ContainerDir string
	// MemoryLimitBytes caps the container's RAM; 0 means no limit
	// (Create refuses this — AppsHub always sets one, see
	// docs/APPSHUB_PLAN.md's resource-budget decision).
	MemoryLimitBytes int64
}

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
	// Create pulls spec.Image, creates and starts a container labelled
	// for spec.AppID, and returns it. Returns ErrAlreadyExists if
	// spec.AppID already has a container.
	Create(ctx context.Context, spec ContainerSpec) (Container, error)
	// Remove stops (if running) and deletes a container — its bind-mounted
	// HostDir is never touched, only the container itself. Returns
	// ErrNotFound if id isn't AppLabel-tagged.
	Remove(ctx context.Context, id string) error
	// ImageDigest returns the registry digest ("sha256:...") of the image
	// a container was actually created from, read back from the local
	// image's own RepoDigests — what AppsHub's update check (App5)
	// compares against dockerhub.Client.Digest's current answer for the
	// same image:tag. "" (no error) means the local image carries no
	// recorded RepoDigest (e.g. built locally, never pulled from a
	// registry) — not upgradable-checkable, not an error. Returns
	// ErrNotFound if id isn't AppLabel-tagged.
	ImageDigest(ctx context.Context, id string) (string, error)
	// Attach opens an interactive shell inside a running container (a
	// PTY, not Exec's one-shot non-interactive command) for AppsHub's
	// per-container console (App5) — read what the shell prints, write
	// what the user types, Close ends the session. Returns ErrNotFound if
	// id isn't AppLabel-tagged, ErrUnavailable if it isn't running.
	Attach(ctx context.Context, id string) (io.ReadWriteCloser, error)
}

// drainClosed discards and closes r, for defer sites that don't care
// about a response body's content.
func drainClosed(r io.ReadCloser) {
	_, _ = io.Copy(io.Discard, r)
	_ = r.Close()
}

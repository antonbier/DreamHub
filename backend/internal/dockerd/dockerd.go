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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"path"
	"strings"
)

// AppLabel is the container label every container this package will touch
// must carry, with its own app's id as the value — "a DreamHub-owned
// label prefix" (docs/APPSHUB_PLAN.md, "Docker access is isolated"). A
// container without it is invisible to this package, not just filtered
// from a listing: Start/Stop/Logs/Exec all refuse one, the same as
// Enigma2's native WebControl staying off-limits (CLAUDE.md).
const AppLabel = "dreamhub.app"

// PortLabel records an app's web port on its container. On the host
// network (the only kind the reference box's kernel allows — no veth or
// br_netfilter, ARCHITECTURE.md) Docker publishes no port mappings, so
// the port can't be read back from them.
const PortLabel = "dreamhub.port"

// ServiceLabel names a helper container's role inside a multi-container
// app (e.g. "db" next to Nextcloud) — unset on an app's main container.
// Every container of one app shares its AppLabel value.
const ServiceLabel = "dreamhub.service"

// RootLabel records where an app's storage lives (the storage root the
// person chose at install, e.g. /media/hdd/dreamhub-apps) — read back so
// removing, backing up and restoring look in the right place even when
// apps are spread over several disks.
const RootLabel = "dreamhub.root"

// ValidRoot reports whether r is usable as a ContainerSpec.Root: empty
// (the default root) or a clean absolute path.
func ValidRoot(r string) bool {
	return r == "" || (strings.HasPrefix(r, "/") && path.Clean(r) == r && !strings.ContainsAny(r, ":\n\r\x00"))
}

// ValidService reports whether s is usable as a ContainerSpec.Service:
// empty (the app's main container) or 1-20 lowercase letters/digits —
// it ends up in the container's name.
func ValidService(s string) bool {
	if len(s) > 20 {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// NameLabel is the display name of a custom app (one an admin added from a
// compose file or an image), shown instead of its image name. Catalog apps
// are named by their catalog entry and carry none.
const NameLabel = "dreamhub.name"

// ValidName reports whether n is usable as ContainerSpec.Name: empty, or 1-60
// printable characters.
func ValidName(n string) bool {
	if len(n) > 60 {
		return false
	}
	for _, r := range n {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// RollbackRepo is the image repository prefix under which the image a
// container ran before an update is kept (App14): one tag per app (and
// service), "dreamhub-rollback/<app>[.<service>]:previous". The names are
// built here from the container's own labels, never taken from a caller.
const (
	RollbackRepo = "dreamhub-rollback/"
	RollbackTag  = "previous"
)

// KeptRef is the rollback image reference of an app's container (service ""
// is the main one).
func KeptRef(appID, service string) string {
	name := appID
	if service != "" {
		name += "." + service
	}
	return RollbackRepo + name + ":" + RollbackTag
}

// originTag is a second rollback name that remembers which image name the
// kept image had when it was in use ("origin-" and a hash of it). Restoring
// a kept image only ever gives it back that name, so the call can't be
// used to put an image under some other name.
func originTag(ref string) string {
	sum := sha256.Sum256([]byte(ref))
	return "origin-" + hex.EncodeToString(sum[:])[:24]
}

// keptOf is the inverse of KeptRef for one RepoTags entry.
func keptOf(ref string) (appID, service string, ok bool) {
	rest, found := strings.CutPrefix(ref, RollbackRepo)
	if !found {
		return "", "", false
	}
	name, tag, found := strings.Cut(rest, ":")
	if !found || tag != RollbackTag || name == "" {
		return "", "", false
	}
	appID, service, _ = strings.Cut(name, ".")
	return appID, service, true
}

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
	// RestartCount is how often the daemon restarted the container by its
	// restart policy (read from the container's own state, 0 when unknown):
	// what the crash-loop notice (App14) counts.
	RestartCount int `json:"restartCount,omitempty"`
}

// AppID is the Labels[AppLabel] value, or "" if unset (Client
// implementations never return a Container missing it).
func (c Container) AppID() string { return c.Labels[AppLabel] }

// Service is the Labels[ServiceLabel] value: "" for an app's main
// container, the helper's role (e.g. "db") otherwise.
func (c Container) Service() string { return c.Labels[ServiceLabel] }

// Root is the Labels[RootLabel] value: "" for an app installed before the
// storage choice existed (it lives under the default root).
func (c Container) Root() string { return c.Labels[RootLabel] }

// DisplayName is the Labels[NameLabel] value: "" for a catalog app.
func (c Container) DisplayName() string { return c.Labels[NameLabel] }

// Image is one image no container uses, as listed for the "clean up unused
// images" preview. SizeBytes is what removing it alone would free (its own
// layers, not those shared with an image that stays).
type Image struct {
	ID        string   `json:"id"`
	Tags      []string `json:"tags"`
	SizeBytes int64    `json:"sizeBytes"`
}

// PruneResult is what PruneImages did.
type PruneResult struct {
	Removed        int   `json:"removed"`
	ReclaimedBytes int64 `json:"reclaimedBytes"`
}

// KeptImage is an image kept for "Roll back": what an app's container ran
// before its last update.
type KeptImage struct {
	AppID   string `json:"appId"`
	Service string `json:"service,omitempty"`
	ImageID string `json:"imageId"`
	// SizeBytes is what removing it alone would free.
	SizeBytes int64 `json:"sizeBytes"`
}

// ErrNoKeptImage means a create from the kept image found none.
var ErrNoKeptImage = errors.New("dockerd: no previous image is kept for this app")

// ErrNotFound means no container with that id carries AppLabel.
var ErrNotFound = errors.New("dockerd: no such app container")

// ErrAlreadyExists means AppID already has a container (Create is one
// app, one container — a second install needs Remove first).
var ErrAlreadyExists = errors.New("dockerd: this app is already installed")

// ErrInvalidService means ContainerSpec.Service isn't ValidService.
var ErrInvalidService = errors.New("dockerd: invalid service name")

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
	// Service is "" for an app's main container, or a helper's role in a
	// multi-container app (e.g. "db"): its container is named
	// "dreamhub-app-" + AppID + "-" + Service and carries ServiceLabel.
	// One container per AppID and Service.
	Service string
	// Name is a custom app's display name (NameLabel); "" for a catalog app.
	Name string
	// Image is pulled before the container is created ("image:tag",
	// e.g. "pihole/pihole:latest").
	Image string
	// Env is passed through as-is (e.g. {"TZ": "Europe/Rome"}) — never
	// shell-interpreted, Docker's API takes them as plain KEY=VALUE
	// strings.
	Env map[string]string
	// Cmd replaces the image's own CMD when set (its ENTRYPOINT stays) —
	// curated catalog data only (internal/appcatalog), e.g. MariaDB's
	// --bind-address=127.0.0.1 so a database stays off the LAN. It runs
	// inside the container with the same limits as the image's own CMD.
	Cmd []string
	// HostPort/ContainerPort map one TCP port; 0/0 means no port is
	// published at all (e.g. wireguard, which has no web UI — see
	// appcatalog.App.ContainerPort). With HostNetwork the app listens on
	// the box directly and HostPort must equal ContainerPort.
	HostPort, ContainerPort int
	// HostNetwork runs the container in the box's own network namespace
	// (docker run --network host): what DreamHub's Docker setup uses,
	// since the box's kernel can't do Docker's bridge network.
	HostNetwork bool
	// Mounts are the bind mounts. Every HostDir is a path the caller
	// computed itself or took from the person's own input, and Create
	// checks each one against the mount rules (MountPolicy, paths.go)
	// before anything reaches Docker: only the app's own folders under
	// Root and paths below /media, never a system directory, DreamHub's
	// own data, the Docker socket, or anything that resolves outside.
	Mounts []Mount
	// Elevated asks for more than a plain container (privileged,
	// capabilities, devices, host PID/IPC). Create refuses any of it
	// unless Elevated.Confirmed — the person's explicit extra
	// confirmation, which only AppsHub's install plan sets.
	Elevated Elevated
	// Root is the storage root the app's own folders lie under, recorded
	// as RootLabel; "" for the default.
	Root string
	// MemoryLimitBytes caps the container's RAM; 0 means no limit
	// (Create refuses this — AppsHub always sets one, see
	// docs/APPSHUB_PLAN.md's resource-budget decision).
	MemoryLimitBytes int64
	// FromKept creates the container from the image kept for this app
	// (and service) instead of pulling Image: the kept image gets Image's
	// name back first, so the container looks exactly like one that was
	// never updated. Create fails with ErrNoKeptImage when none is kept.
	// This is how "Roll back" and a failed update's undo work.
	FromKept bool
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
	// folders are never touched, only the container itself. Returns
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
	// UnusedImages lists the images no container uses (running or stopped,
	// of any app) except the kept rollback images — the ones PruneImages
	// would remove. Read-only; takes no
	// argument, so nothing about it can be steered by the caller.
	UnusedImages(ctx context.Context) ([]Image, error)
	// PruneImages removes every unused image — exactly what UnusedImages
	// lists, nothing selectable: the daemon itself refuses an image a
	// container uses, and the helper holds off while an install pulls one.
	// Containers, volumes, networks and build cache are never touched.
	PruneImages(ctx context.Context) (PruneResult, error)
	// KeepImage remembers the image a container runs under its rollback
	// name (KeptRef), replacing what was kept for that app and service
	// before — one level only. Returns the image's ID. ErrNotFound if id
	// isn't AppLabel-tagged.
	KeepImage(ctx context.Context, id string) (string, error)
	// KeptImages lists the images kept for rollback. Read-only.
	KeptImages(ctx context.Context) ([]KeptImage, error)
	// DropKeptImages gives up the kept images of one app (appID "" = all
	// apps): their rollback names go, and with them the images when nothing
	// else uses them. The names come from the kept list, nothing else can
	// be removed through it.
	DropKeptImages(ctx context.Context, appID string) (PruneResult, error)
}

// drainClosed discards and closes r, for defer sites that don't care
// about a response body's content.
func drainClosed(r io.ReadCloser) {
	_, _ = io.Copy(io.Discard, r)
	_ = r.Close()
}

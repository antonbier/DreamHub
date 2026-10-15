// Package dockersetup sets Docker up on the box for AppsHub, from the UI
// ("Set up Docker", docs/APPSHUB_PLAN.md): the official static Docker
// binaries at a version and checksum pinned here, the overlay kernel
// module that exactly matches the running kernel, a daemon config that
// keeps everything on /data and uses the host network, and the two
// services (dreamhub-docker and dreamhub-dockerd-helper) the .deb ships
// disabled.
//
// Why this shape, all confirmed on the reference box (ARCHITECTURE.md,
// "Box checks for 0.4.0"): the feed's own docker is 1.13 from 2017; the
// kernel (4.9) has no veth/br_netfilter/xt_addrtype, so Docker's bridge
// network can't work and containers use the host network; the overlay
// module is a separate package, and the feed's version is built for a
// different kernel than the box runs — only the official Dreambox feed
// has the matching one; / (the root flash) has 560 MB free while /data
// (its own partition) has 4 GB, and a full / can stop DreamOS from
// booting.
//
// Like internal/apt: Exec does it for real, ./mock answers from memory.
package dockersetup

import (
	"context"
	"errors"
	"path"
	"strings"
)

// Paths on the box. Everything Docker writes lives under Base, on /data.
const (
	Base       = "/data/dreamhub-docker"
	BinDir     = Base + "/bin"
	DataRoot   = Base + "/root"
	ConfigFile = Base + "/daemon.json"
	Socket     = "/run/dreamhub-docker.sock"

	DockerService = "dreamhub-docker.service"
	HelperService = "dreamhub-dockerd-helper.service"
)

// Release is one pinned Docker static build.
type Release struct {
	Version string
	URL     string
	SHA256  string
	Size    int64
}

// Version is the Docker release DreamHub sets up. Updating it is a code
// change with a new checksum, shipped in a DreamHub release — never
// whatever happens to be newest at setup time.
const Version = "29.9.0"

// Releases by GOARCH (the arch DreamHub itself was built for, which is
// the box's — build-deb.sh builds one binary per arch).
var Releases = map[string]Release{
	"arm64": {Version: Version, Size: 77921845,
		URL:    "https://download.docker.com/linux/static/stable/aarch64/docker-29.9.0.tgz",
		SHA256: "811f6eee271678fc98dbe5c4d896c00fc5fe2daa4f7a37069c17abc036eab85b"},
	"arm": {Version: Version, Size: 79649130,
		URL:    "https://download.docker.com/linux/static/stable/armhf/docker-29.9.0.tgz",
		SHA256: "1f0ef76dbfda7c3ee97d566b2620dbafb2497454bd9c973e953178f03a62b766"},
}

// State is what's there right now.
type State struct {
	// Supported: there's a pinned release for this box's processor.
	Supported bool `json:"supported"`
	// Version of the Docker binaries installed under BinDir ("" if none).
	Version string `json:"version,omitempty"`
	// Overlay: the overlay filesystem is available (module loaded).
	Overlay bool `json:"overlay"`
	// Configured: the daemon config exists.
	Configured bool `json:"configured"`
	// DockerRunning and HelperRunning: the two services are active.
	DockerRunning bool `json:"dockerRunning"`
	HelperRunning bool `json:"helperRunning"`
	// Ready: everything above, at the pinned version.
	Ready bool `json:"ready"`
	// FreeBytes on /data, where Docker would live.
	FreeBytes int64 `json:"freeBytes"`
}

// Step is one thing setup would do.
type Step struct {
	// Kind: "overlay" (install/load the matching kernel module),
	// "download" (fetch and verify the Docker binaries), "config" (write
	// daemon.json), "services" (enable and start both services).
	Kind string `json:"kind"`
	// Detail says what exactly, e.g. the module version, the release.
	Detail map[string]string `json:"detail,omitempty"`
}

// Setup reads the state, works out what's missing, and does it.
type Setup interface {
	Status(ctx context.Context) (State, error)
	Plan(ctx context.Context) ([]Step, error)
	Apply(ctx context.Context, steps []Step) (log string, err error)
	// PlanRemove works out what "Uninstall Docker" would still remove
	// (nothing left means an empty list), Remove does it. The containers
	// DreamHub created are removed by the caller first.
	PlanRemove(ctx context.Context, opts RemoveOptions) ([]Step, error)
	Remove(ctx context.Context, steps []Step, opts RemoveOptions) (log string, err error)
}

// ModulesFile makes the overlay module load at every boot; setup writes
// it and uninstalling removes it (the kernel module package stays).
const ModulesFile = "/etc/modules-load.d/dreamhub-docker.conf"

// Uninstall step kinds: "services" (stop and disable both services),
// "modules" (remove ModulesFile), "files" (the Docker binaries and
// daemon config), "data" (only with DeleteData: Docker's own image and
// volume store and the apps' storage folders).

// RemoveOptions are the choices of an uninstall.
type RemoveOptions struct {
	// DeleteData also deletes the app data: Docker's data root and the
	// apps' storage folders. The default (false) keeps them, so apps can
	// come back after a reinstall.
	DeleteData bool
	// AppRoots are the apps' storage roots (e.g. /media/hdd/dreamhub-apps)
	// that "data" deletes. Only a folder named dreamhub-apps, or one under
	// Base, is ever deleted (SafeDataPath).
	AppRoots []string
}

// SafeDataPath reports whether an uninstall may delete p: an absolute,
// clean path under Base, or an apps storage root (a directory named
// dreamhub-apps below the top level). Never enigma2's or the recordings'
// folders, whatever the caller passes.
func SafeDataPath(p string) bool {
	if !path.IsAbs(p) || path.Clean(p) != p || strings.ContainsAny(p, "\n\r\x00") {
		return false
	}
	if strings.HasPrefix(p, Base+"/") {
		return true
	}
	return path.Base(p) == "dreamhub-apps" && path.Dir(p) != "/"
}

// ErrUnsupported: no pinned release for this processor.
var ErrUnsupported = errors.New("dockersetup: no Docker release pinned for this processor")

// ErrNoMatchingOverlay: neither the enabled feeds nor the official
// Dreambox feed have an overlay module built for the running kernel.
var ErrNoMatchingOverlay = errors.New("dockersetup: no overlay module matching this kernel")

// NeededBytes is the free space on /data setup insists on: the download
// (~80 MB, deleted afterwards), the unpacked binaries (~210 MB) and room
// for the first images.
const NeededBytes = 1 << 30

// Package dockerhub is AppsHub's free-search and arch-check connector
// (docs/APPSHUB_PLAN.md, App3, "Curated list + search"): any arm64 or
// armhf image on Docker Hub should be installable, not just the ones in
// internal/appcatalog's hand-maintained curated list — but checking a
// multi-arch manifest costs one API call, too slow to do for every search
// result. Search stays cheap (one call, no per-result manifest lookups);
// a manifest is only fetched lazily, when the owner actually clicks a
// result, and briefly cached (HTTPClient's own manifestTTL) rather than
// trusted forever — a maintainer can add arm64 support to an image after
// the cache was populated.
//
// Unlike internal/dockerd (holds a privileged socket, must stay isolated
// in its own process), this package only ever makes outbound, read-only
// calls to Docker Hub's public API — no isolation concern, called
// directly from DreamHub's main backend like tmdb/omdb/tvmaze already
// are.
package dockerhub

import (
	"context"
	"errors"
	"runtime"
)

// ErrUnavailable means Docker Hub's API didn't answer.
var ErrUnavailable = errors.New("dockerhub: unavailable")

// ErrNotFound means the named image or tag doesn't exist on Docker Hub.
var ErrNotFound = errors.New("dockerhub: not found")

// Repo is one free-search result, trimmed to what AppsHub shows before
// any manifest check.
type Repo struct {
	Name        string `json:"name"` // "namespace/repo", Docker Hub's own repo_name
	Description string `json:"description"`
	Stars       int    `json:"stars"`
}

// Arch is one platform an image's manifest lists, in Docker's own naming
// (e.g. {"arm64", ""} or {"arm", "v7"}).
type Arch struct {
	Architecture string `json:"architecture"`
	Variant      string `json:"variant,omitempty"`
}

// Client is how AppsHub reaches Docker Hub. HTTPClient (./http_client.go)
// is the real implementation; ./mock is a deterministic fake for tests
// and mock mode.
type Client interface {
	// Search runs a free-text search beyond the curated list. query must
	// be non-empty.
	Search(ctx context.Context, query string) ([]Repo, error)
	// Manifest returns the platforms tag (e.g. "latest") of image (e.g.
	// "pihole/pihole") was built for. Returns ErrNotFound if the image or
	// tag doesn't exist.
	Manifest(ctx context.Context, image, tag string) ([]Arch, error)
}

// BoxArch is the architecture this very binary was built for
// (packaging/build-deb.sh's ARCH, reflected by Go's own GOARCH/GOARM at
// build time) — exactly the platform a Docker image needs to cover for
// this box, with no separate detection needed: armhf boxes run the arm
// binary DreamHub itself is, arm64 boxes run the arm64 one.
func BoxArch() Arch {
	if runtime.GOARCH == "arm" {
		// packaging/build-deb.sh builds armhf with GOARM=7.
		return Arch{Architecture: "arm", Variant: "v7"}
	}
	return Arch{Architecture: runtime.GOARCH}
}

// Supports reports whether arches covers box — exact architecture match,
// and for arm, v6 or v7 either one (an armv6 image's instructions run
// fine on the box's v7 hardware; Docker Hub doesn't offer a finer-grained
// claim than that).
func Supports(arches []Arch, box Arch) bool {
	for _, a := range arches {
		if a.Architecture != box.Architecture {
			continue
		}
		if box.Architecture != "arm" || a.Variant == "" || a.Variant == "v6" || a.Variant == "v7" {
			return true
		}
	}
	return false
}

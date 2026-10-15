// Package mock implements dockerhub.Client without a real Docker Hub —
// deterministic search results and manifests, so AppsHub's free search
// and arch check have something to show in mock mode and development
// (PRINCIPLES.md, "Mock-first development").
package mock

import (
	"context"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/dockerhub"
)

// Client is a deterministic dockerhub.Client over a small, fixed set of
// images — real ones, so BoxArch()-based filtering behaves the same in
// mock mode as against the real API.
type Client struct{}

var _ dockerhub.Client = Client{}

type fixture struct {
	arches []dockerhub.Arch
	// digest is what hub.docker.com would currently say "image:latest"
	// resolves to — compared against dockerd/mock's own seeded
	// containers (internal/dockerd/mock) by AppsHub's update watch
	// (App5): pihole's differ on purpose, so mock mode has a real
	// "update available" case to develop and demo against; adguardhome's
	// match, an "already current" case.
	digest string
}

var multiArch = []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm64"}, {Architecture: "arm", Variant: "v7"}, {Architecture: "arm", Variant: "v6"}}

var fixtures = map[string]fixture{
	"pihole/pihole":        {arches: []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm64"}, {Architecture: "arm", Variant: "v7"}, {Architecture: "arm", Variant: "v6"}}, digest: "sha256:pihole-upstream-new"},
	"adguard/adguardhome":  {arches: []dockerhub.Arch{{Architecture: "386"}, {Architecture: "amd64"}, {Architecture: "arm", Variant: "v6"}, {Architecture: "arm", Variant: "v7"}, {Architecture: "arm64"}, {Architecture: "ppc64le"}}, digest: "sha256:adguardhome-current"},
	"linuxserver/heimdall": {arches: []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm64"}}, digest: "sha256:heimdall-current"},
	// Images the custom-container demos and tests use.
	"library/nginx":           {arches: multiArch, digest: "sha256:nginx-current"},
	"traefik/whoami":          {arches: multiArch, digest: "sha256:whoami-current"},
	"library/redis":           {arches: multiArch, digest: "sha256:redis-current"},
	"koenkk/zigbee2mqtt":      {arches: multiArch, digest: "sha256:z2m-current"},
	"library/postgres":        {arches: multiArch, digest: "sha256:postgres-current"},
	"linuxserver/mariadb-old": {arches: []dockerhub.Arch{{Architecture: "amd64"}}, digest: "sha256:mariadb-old-current"}, // deliberately amd64-only, to exercise "not compatible"
}

func (Client) Search(ctx context.Context, query string) ([]dockerhub.Repo, error) {
	q := strings.ToLower(query)
	all := []dockerhub.Repo{
		{Name: "pihole/pihole", Description: "Network-wide ad blocking via your own Linux hardware", Stars: 2900},
		{Name: "linuxserver/heimdall", Description: "An application dashboard", Stars: 900},
		{Name: "linuxserver/mariadb-old", Description: "A deliberately amd64-only fixture for mock mode", Stars: 1},
	}
	out := make([]dockerhub.Repo, 0, len(all))
	for _, r := range all {
		if q == "" || strings.Contains(strings.ToLower(r.Name), q) || strings.Contains(strings.ToLower(r.Description), q) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (Client) Manifest(ctx context.Context, image, tag string) ([]dockerhub.Arch, error) {
	f, ok := fixtures[image]
	if !ok {
		return nil, dockerhub.ErrNotFound
	}
	return f.arches, nil
}

func (Client) Digest(ctx context.Context, image, tag string) (string, error) {
	f, ok := fixtures[image]
	if !ok {
		return "", dockerhub.ErrNotFound
	}
	return f.digest, nil
}

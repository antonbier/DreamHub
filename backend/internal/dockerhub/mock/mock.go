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

var fixtures = map[string][]dockerhub.Arch{
	"pihole/pihole":           {{Architecture: "amd64"}, {Architecture: "arm64"}, {Architecture: "arm", Variant: "v7"}, {Architecture: "arm", Variant: "v6"}},
	"linuxserver/heimdall":    {{Architecture: "amd64"}, {Architecture: "arm64"}},
	"linuxserver/mariadb-old": {{Architecture: "amd64"}}, // deliberately amd64-only, to exercise "not compatible"
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
	arches, ok := fixtures[image]
	if !ok {
		return nil, dockerhub.ErrNotFound
	}
	return arches, nil
}

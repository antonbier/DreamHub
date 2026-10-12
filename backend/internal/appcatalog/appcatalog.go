// Package appcatalog is AppsHub's curated app list (docs/APPSHUB_PLAN.md,
// App3: "Curated list, verified multi-arch, no AI and no ongoing manual
// curation of taste"). Apps is plain, hand-maintained project data —
// never AI-generated or AI-kept-current, the first concrete instance of
// PRINCIPLES.md's "AI is always optional" rule — deliberately lean:
// name, category, image, and which architectures were actually checked
// against Docker Hub's own API, nothing requiring ongoing subjective
// judgment (e.g. no "is this suited to an intermittently-on box" score,
// explicitly rejected by the owner).
//
// Arches here is a point-in-time fact, not a live check: verified against
// https://hub.docker.com/v2/repositories/<image>/tags/latest on
// 2026-10-12. A maintainer can change an image's supported platforms at
// any time; App4's install flow re-verifies through dockerhub.Manifest
// before ever pulling, the same lazy, on-click check free search results
// get (docs/APPSHUB_PLAN.md) — this list is a useful default, not a
// promise.
package appcatalog

import "github.com/antonbier/DreamHub/backend/internal/dockerhub"

// App is one curated entry.
type App struct {
	// ID is stable, used as the dreamhub.app label value once installed
	// (internal/dockerd.AppLabel) — lowercase, no spaces.
	ID       string
	Name     string
	Category string
	// Description is plain English text, like a fetched programme
	// synopsis elsewhere in this codebase — data, not UI chrome, so it
	// isn't a locale key (CLAUDE.md's "no hardcoded user-facing strings"
	// is about this app's own interface, not third-party content it
	// displays).
	Description string
	// Image is the Docker Hub "namespace/repo" to pull.
	Image string
	// Arches are the platforms confirmed in Docker Hub's own manifest for
	// Image's "latest" tag — see the package doc for when and how this
	// was checked.
	Arches []dockerhub.Arch
	// ContainerPort is the image's own web UI port, confirmed from its
	// Docker Hub description's own `docker run -p` example on 2026-10-12
	// — 0 means the app has no web UI to open (e.g. wireguard), so App4's
	// install flow offers no host port and no "open app" link for it.
	ContainerPort int
	// ConfigPath is the in-container path App4 bind-mounts this app's own
	// storage directory to, confirmed the same way as ContainerPort.
	// Every linuxserver.io image here uses its own documented "/config"
	// convention; the two vendor images (pihole, AdGuard Home) each use
	// their own single documented path — a deliberate v1 simplification:
	// AdGuard Home's second, optional "work" volume isn't modeled, and
	// Pi-hole's optional dnsmasq.d override volume isn't either.
	ConfigPath string
}

// Categories, in the fixed order the frontend shows them.
const (
	CategoryNetwork = "network"
	CategoryMedia   = "media"
	CategoryFiles   = "files"
)

// Apps is the curated list — see the package doc for why it stays short
// and who maintains it.
var Apps = []App{
	{
		ID: "pihole", Name: "Pi-hole", Category: CategoryNetwork,
		Description:   "Network-wide ad and tracker blocking via DNS.",
		Image:         "pihole/pihole",
		Arches:        []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm64"}, {Architecture: "arm", Variant: "v7"}, {Architecture: "arm", Variant: "v6"}, {Architecture: "386"}, {Architecture: "riscv64"}},
		ContainerPort: 80, ConfigPath: "/etc/pihole",
	},
	{
		ID: "adguardhome", Name: "AdGuard Home", Category: CategoryNetwork,
		Description:   "Network-wide ad and tracker blocking via DNS, with its own DNS-over-HTTPS/TLS.",
		Image:         "adguard/adguardhome",
		Arches:        []dockerhub.Arch{{Architecture: "386"}, {Architecture: "amd64"}, {Architecture: "arm", Variant: "v6"}, {Architecture: "arm", Variant: "v7"}, {Architecture: "arm64"}, {Architecture: "ppc64le"}},
		ContainerPort: 3000, ConfigPath: "/opt/adguardhome/conf",
	},
	{
		ID: "wireguard", Name: "WireGuard", Category: CategoryNetwork,
		Description:   "A VPN server to reach the household network from outside.",
		Image:         "linuxserver/wireguard",
		Arches:        []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm64"}},
		ContainerPort: 0, ConfigPath: "/config",
	},
	{
		ID: "syncthing", Name: "Syncthing", Category: CategoryFiles,
		Description:   "Continuous, peer-to-peer file synchronization between devices.",
		Image:         "linuxserver/syncthing",
		Arches:        []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm64"}},
		ContainerPort: 8384, ConfigPath: "/config",
	},
	{
		ID: "qbittorrent", Name: "qBittorrent", Category: CategoryFiles,
		Description:   "A BitTorrent client with a web UI.",
		Image:         "linuxserver/qbittorrent",
		Arches:        []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm64"}},
		ContainerPort: 8080, ConfigPath: "/config",
	},
	{
		ID: "heimdall", Name: "Heimdall", Category: CategoryFiles,
		Description:   "A dashboard linking to the household's other self-hosted apps.",
		Image:         "linuxserver/heimdall",
		Arches:        []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm64"}},
		ContainerPort: 80, ConfigPath: "/config",
	},
	{
		ID: "sonarr", Name: "Sonarr", Category: CategoryMedia,
		Description:   "Tracks and fetches new episodes of TV series.",
		Image:         "linuxserver/sonarr",
		Arches:        []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm64"}},
		ContainerPort: 8989, ConfigPath: "/config",
	},
	{
		ID: "radarr", Name: "Radarr", Category: CategoryMedia,
		Description:   "Tracks and fetches films.",
		Image:         "linuxserver/radarr",
		Arches:        []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm64"}},
		ContainerPort: 7878, ConfigPath: "/config",
	},
	{
		ID: "prowlarr", Name: "Prowlarr", Category: CategoryMedia,
		Description:   "A shared indexer manager for Sonarr/Radarr-style apps.",
		Image:         "linuxserver/prowlarr",
		Arches:        []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm64"}},
		ContainerPort: 9696, ConfigPath: "/config",
	},
	{
		ID: "bazarr", Name: "Bazarr", Category: CategoryMedia,
		Description:   "Fetches subtitles for Sonarr/Radarr libraries.",
		Image:         "linuxserver/bazarr",
		Arches:        []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm64"}},
		ContainerPort: 6767, ConfigPath: "/config",
	},
}

// ByID returns the curated entry with that id, and whether it exists.
func ByID(id string) (App, bool) {
	for _, a := range Apps {
		if a.ID == id {
			return a, true
		}
	}
	return App{}, false
}

// SupportsArch reports whether a was verified for box — dockerhub.Supports
// against this entry's own curated Arches, rather than a fresh API call.
func (a App) SupportsArch(box dockerhub.Arch) bool {
	return dockerhub.Supports(a.Arches, box)
}

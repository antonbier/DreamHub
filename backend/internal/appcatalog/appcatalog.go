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
//
// Reworked 2026-10-13 with the owner's wishes (docs/APPSHUB_PLAN.md,
// "Curated list rework"): no *arr apps; Vaultwarden, Nextcloud and Home
// Assistant added. Every app runs on the box's own network (App7), so
// each one's ports are the box's ports — an app whose port can't be moved
// off one the box already uses (80 for WebControl, 8080 for DreamHub) is
// moved through the image's own documented setting (Env), or left out.
package appcatalog

import (
	"regexp"
	"sort"

	"github.com/antonbier/DreamHub/backend/internal/dockerhub"
)

// Port is one more port an app binds on the box besides its web port —
// all of them are checked before installing, since on the host network
// two apps (or an app and the box) can't share one.
type Port struct {
	Port int
	UDP  bool
	// Local: the app binds it on 127.0.0.1 only (a database next to its
	// app), so it's checked there and never reachable from the LAN.
	Local bool
}

// Service is a helper container of a multi-container app (e.g. Nextcloud's
// database) — same fields as App's own container, each its own storage
// directory (StorageRoot/<app id>.<service name>). All containers of an
// app share the box's network, so they reach each other on 127.0.0.1.
type Service struct {
	// Name is the helper's role, dockerd.ValidService ("db").
	Name       string
	Image      string
	Tag        string
	Arches     []dockerhub.Arch
	ConfigPath string
	Env        map[string]string
	Cmd        []string
	Ports      []Port
	MemoryMB   int
}

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
	// displays). Note is the same kind of text: what to know before or
	// right after installing it on this box ("" if nothing).
	Description string
	Note        string
	// Image is the Docker Hub "namespace/repo" to pull (official images
	// as "library/<name>", which Docker pulls the same way). Tag is the
	// tag to follow, "" meaning "latest".
	Image string
	Tag   string
	// Arches are the platforms confirmed in Docker Hub's own manifest for
	// Image's tag — see the package doc for when and how this was checked.
	Arches []dockerhub.Arch
	// ContainerPort is the app's web UI port, confirmed from its image's
	// own documentation — on the host network it's also the box's port. 0
	// means the app has no web UI to open.
	ContainerPort int
	// ConfigPath is the in-container path App4 bind-mounts this app's own
	// storage directory (StorageRoot/<id>) to, confirmed the same way.
	ConfigPath string
	// DataPath, when set, is where the app keeps bulk user data (files,
	// downloads), mounted from its own directory (StorageRoot/<id>.data)
	// and never in a backup by default (docs/APPSHUB_PLAN.md: "configuration
	// only, never bulk volume data automatically").
	DataPath string
	// LargeConfig: the config directory holds the app's own code as well
	// (Nextcloud copies itself there), so it's opt-in in a backup too.
	LargeConfig bool
	// Env is fixed per app — mostly moving its web port off one the box
	// uses. A value "{{secret:<name>}}" becomes a random password
	// generated once at install and kept next to the app's storage
	// (StorageRoot/<id>.secrets.json, outside every container's mount);
	// the same name means the same value across the app's services.
	Env map[string]string
	// Cmd replaces the image's CMD (its ENTRYPOINT stays), when the image
	// offers no setting for something the box needs.
	Cmd []string
	// Ports are the app's other ports (see Port).
	Ports []Port
	// MemoryMB is the default memory limit for this app's own container.
	MemoryMB int
	// Services are the app's helper containers, installed, started,
	// stopped and removed together with it.
	Services []Service
}

// Categories, in the fixed order the frontend shows them.
const (
	CategoryNetwork   = "network"
	CategoryCloud     = "cloud"
	CategoryHome      = "home"
	CategoryDownloads = "downloads"
)

var (
	archesLinuxserver = []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm64"}}
	archesOfficialDB  = []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm64", Variant: "v8"}, {Architecture: "ppc64le"}, {Architecture: "s390x"}}
)

// Apps is the curated list — see the package doc for why it stays short
// and who maintains it. Left out on purpose (docs/APPSHUB_PLAN.md):
// Sonarr/Radarr/Prowlarr/Bazarr (the owner doesn't want *arr apps),
// Heimdall (always binds 80 and 443, no setting to move it), WireGuard
// (needs NET_ADMIN and a kernel module — DreamHub's helper never grants a
// container capabilities), a reverse proxy (needs 80/443, and 80 is
// WebControl's) and a Plex server (the box can't transcode in software).
var Apps = []App{
	{
		ID: "adguardhome", Name: "AdGuard Home", Category: CategoryNetwork,
		Description:   "Network-wide ad and tracker blocking via DNS, with its own DNS-over-HTTPS/TLS.",
		Note:          "In its setup wizard, pick a web port other than 80 — that one is the box's own web interface.",
		Image:         "adguard/adguardhome",
		Arches:        []dockerhub.Arch{{Architecture: "386"}, {Architecture: "amd64"}, {Architecture: "arm", Variant: "v6"}, {Architecture: "arm", Variant: "v7"}, {Architecture: "arm64"}, {Architecture: "ppc64le"}},
		ContainerPort: 3000, ConfigPath: "/opt/adguardhome/conf",
		Ports:    []Port{{Port: 53}, {Port: 53, UDP: true}},
		MemoryMB: 128,
	},
	{
		ID: "pihole", Name: "Pi-hole", Category: CategoryNetwork,
		Description: "Network-wide ad and tracker blocking via DNS.",
		Note:        "Its admin password is generated on first start and printed in the app's logs.",
		Image:       "pihole/pihole",
		Arches:      []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm64"}, {Architecture: "arm", Variant: "v7"}, {Architecture: "arm", Variant: "v6"}, {Architecture: "386"}, {Architecture: "riscv64"}},
		// Its web server defaults to 80 and 443; FTLCONF_webserver_port
		// is Pi-hole's own documented setting for that.
		ContainerPort: 8082, ConfigPath: "/etc/pihole",
		Env:      map[string]string{"FTLCONF_webserver_port": "8082"},
		Ports:    []Port{{Port: 53}, {Port: 53, UDP: true}},
		MemoryMB: 128,
	},
	{
		ID: "nextcloud", Name: "Nextcloud", Category: CategoryCloud,
		Description:   "Your own cloud for files, calendars and contacts, with its own database.",
		Note:          "Create the admin account on first open. Files go to the app's own data folder on the HDD, which backups leave out unless you pick it.",
		Image:         "library/nextcloud",
		Arches:        []dockerhub.Arch{{Architecture: "386"}, {Architecture: "amd64"}, {Architecture: "arm", Variant: "v5"}, {Architecture: "arm", Variant: "v7"}, {Architecture: "arm64", Variant: "v8"}, {Architecture: "ppc64le"}, {Architecture: "riscv64"}, {Architecture: "s390x"}},
		ContainerPort: 8088, ConfigPath: "/var/www/html", DataPath: "/var/www/html/data", LargeConfig: true,
		Env: map[string]string{
			"MYSQL_HOST": "127.0.0.1", "MYSQL_DATABASE": "nextcloud", "MYSQL_USER": "nextcloud", "MYSQL_PASSWORD": "{{secret:db}}",
			// The image runs its install/upgrade step only for its own
			// apache CMD, or with this set — needed since Cmd replaces it.
			"NEXTCLOUD_UPDATE": "1",
		},
		// Apache listens on 80 and the image has no setting for it, so
		// its two Listen/VirtualHost lines are moved before it starts.
		Cmd:      []string{"sh", "-c", `sed -i 's/^Listen 80$/Listen 8088/' /etc/apache2/ports.conf && sed -i 's/<VirtualHost \*:80>/<VirtualHost *:8088>/' /etc/apache2/sites-available/000-default.conf && exec apache2-foreground`},
		MemoryMB: 512,
		Services: []Service{{
			Name: "db", Image: "library/mariadb", Tag: "lts", Arches: archesOfficialDB,
			ConfigPath: "/var/lib/mysql",
			Env: map[string]string{
				"MARIADB_ROOT_PASSWORD": "{{secret:dbroot}}", "MARIADB_DATABASE": "nextcloud",
				"MARIADB_USER": "nextcloud", "MARIADB_PASSWORD": "{{secret:db}}",
			},
			// The image passes arguments starting with "-" to mariadbd:
			// the database stays on 127.0.0.1, off the LAN.
			Cmd:      []string{"--bind-address=127.0.0.1"},
			Ports:    []Port{{Port: 3306, Local: true}},
			MemoryMB: 256,
		}},
	},
	{
		ID: "vaultwarden", Name: "Vaultwarden", Category: CategoryCloud,
		Description: "A password manager server that works with the Bitwarden apps.",
		Note:        "Browsers open its web vault only over HTTPS — reach it through your reverse proxy.",
		Image:       "vaultwarden/server",
		Arches:      []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm", Variant: "v6"}, {Architecture: "arm", Variant: "v7"}, {Architecture: "arm64"}},
		// It listens on 80 by default; ROCKET_PORT is its own setting.
		ContainerPort: 8222, ConfigPath: "/data",
		Env:      map[string]string{"ROCKET_PORT": "8222"},
		MemoryMB: 128,
	},
	{
		ID: "syncthing", Name: "Syncthing", Category: CategoryCloud,
		Description:   "Continuous, peer-to-peer file synchronization between devices.",
		Image:         "linuxserver/syncthing",
		Arches:        archesLinuxserver,
		ContainerPort: 8384, ConfigPath: "/config",
		Ports:    []Port{{Port: 22000}, {Port: 22000, UDP: true}, {Port: 21027, UDP: true}},
		MemoryMB: 256,
	},
	{
		ID: "homeassistant", Name: "Home Assistant", Category: CategoryHome,
		Description: "Home automation: lights, sockets, sensors and their rules in one place.",
		Note:        "A large download (about 620 MB) and one of the hungrier apps for memory.",
		Image:       "homeassistant/home-assistant", Tag: "stable",
		Arches:        archesLinuxserver,
		ContainerPort: 8123, ConfigPath: "/config",
		MemoryMB: 512,
	},
	{
		ID: "qbittorrent", Name: "qBittorrent", Category: CategoryDownloads,
		Description: "A BitTorrent client with a web UI.",
		Image:       "linuxserver/qbittorrent",
		Arches:      archesLinuxserver,
		// 8080 is DreamHub's; WEBUI_PORT is the image's own setting.
		ContainerPort: 8095, ConfigPath: "/config", DataPath: "/downloads",
		Env:      map[string]string{"WEBUI_PORT": "8095"},
		Ports:    []Port{{Port: 6881}, {Port: 6881, UDP: true}},
		MemoryMB: 256,
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

// SupportsArch reports whether every container of a was verified for box
// — dockerhub.Supports against the curated Arches, rather than a fresh
// API call.
func (a App) SupportsArch(box dockerhub.Arch) bool {
	if !dockerhub.Supports(a.Arches, box) {
		return false
	}
	for _, s := range a.Services {
		if !dockerhub.Supports(s.Arches, box) {
			return false
		}
	}
	return true
}

// ImageRef is Image:Tag, with "latest" for an empty Tag.
func ImageRef(image, tag string) string {
	if tag == "" {
		tag = "latest"
	}
	return image + ":" + tag
}

// TotalMemoryMB is the memory limit of all of a's containers together.
func (a App) TotalMemoryMB() int {
	total := a.MemoryMB
	for _, s := range a.Services {
		total += s.MemoryMB
	}
	return total
}

var secretRef = regexp.MustCompile(`^\{\{secret:([a-z0-9]+)\}\}$`)

// SecretNames lists the distinct "{{secret:<name>}}" names a's containers
// use, sorted.
func (a App) SecretNames() []string {
	seen := map[string]bool{}
	collect := func(env map[string]string) {
		for _, v := range env {
			if m := secretRef.FindStringSubmatch(v); m != nil {
				seen[m[1]] = true
			}
		}
	}
	collect(a.Env)
	for _, s := range a.Services {
		collect(s.Env)
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ExpandEnv returns env with every "{{secret:<name>}}" value replaced by
// secrets[name].
func ExpandEnv(env map[string]string, secrets map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		if m := secretRef.FindStringSubmatch(v); m != nil {
			v = secrets[m[1]]
		}
		out[k] = v
	}
	return out
}

package appcatalog_test

// The catalog as it was before App12 moved it into compose files — a frozen
// copy, used only to prove the migration changed no behaviour (legacy_test.go
// compares every resolved entry against it).

import (
	"strconv"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/appcatalog"
	"github.com/antonbier/DreamHub/backend/internal/dockerhub"
)

// Port is one more port an app binds on the box besides its web port —
// all of them are checked before installing, since on the host network
// two apps (or an app and the box) can't share one.
type legacyPort struct {
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
type legacyService struct {
	// Name is the helper's role, dockerd.ValidService ("db").
	Name       string
	Image      string
	Tag        string
	Arches     []dockerhub.Arch
	ConfigPath string
	Env        map[string]string
	Cmd        []string
	Ports      []legacyPort
	MemoryMB   int
}

// App is one curated entry.
type legacyApp struct {
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
	// PortEnv names the env var that holds the web port (the image's own
	// setting), and PortInCmd says the port is written into Cmd as well:
	// either one lets the person pick another port at install (WithPort).
	// An app with neither always uses ContainerPort.
	PortEnv   string
	PortInCmd bool
	// Ports are the app's other ports (see Port).
	Ports []legacyPort
	// MemoryMB is the default memory limit for this app's own container.
	MemoryMB int
	// Services are the app's helper containers, installed, started,
	// stopped and removed together with it.
	Services []legacyService
}

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
var legacyApps = []legacyApp{
	{
		ID: "adguardhome", Name: "AdGuard Home", Category: appcatalog.CategoryNetwork,
		Description:   "Network-wide ad and tracker blocking via DNS, with its own DNS-over-HTTPS/TLS.",
		Note:          "In its setup wizard, pick a web port other than 80 — that one is the box's own web interface. The box's own DNS resolver already holds port 53, so this can only be installed once that is switched off.",
		Image:         "adguard/adguardhome",
		Arches:        []dockerhub.Arch{{Architecture: "386"}, {Architecture: "amd64"}, {Architecture: "arm", Variant: "v6"}, {Architecture: "arm", Variant: "v7"}, {Architecture: "arm64"}, {Architecture: "ppc64le"}},
		ContainerPort: 3000, ConfigPath: "/opt/adguardhome/conf",
		Ports:    []legacyPort{{Port: 53}, {Port: 53, UDP: true}},
		MemoryMB: 128,
	},
	{
		ID: "pihole", Name: "Pi-hole", Category: appcatalog.CategoryNetwork,
		Description: "Network-wide ad and tracker blocking via DNS.",
		Note:        "Its admin password is generated on first start and printed in the app's logs. The box's own DNS resolver already holds port 53, so this can only be installed once that is switched off.",
		Image:       "pihole/pihole",
		Arches:      []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm64"}, {Architecture: "arm", Variant: "v7"}, {Architecture: "arm", Variant: "v6"}, {Architecture: "386"}, {Architecture: "riscv64"}},
		// Its web server defaults to 80 and 443; FTLCONF_webserver_port
		// is Pi-hole's own documented setting for that.
		ContainerPort: 8082, ConfigPath: "/etc/pihole",
		Env:      map[string]string{"FTLCONF_webserver_port": "8082"},
		PortEnv:  "FTLCONF_webserver_port",
		Ports:    []legacyPort{{Port: 53}, {Port: 53, UDP: true}},
		MemoryMB: 128,
	},
	{
		ID: "nextcloud", Name: "Nextcloud", Category: appcatalog.CategoryCloud,
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
		Cmd:       []string{"sh", "-c", `sed -i 's/^Listen 80$/Listen 8088/' /etc/apache2/ports.conf && sed -i 's/<VirtualHost \*:80>/<VirtualHost *:8088>/' /etc/apache2/sites-available/000-default.conf && exec apache2-foreground`},
		MemoryMB:  512,
		PortInCmd: true,
		Services: []legacyService{{
			Name: "db", Image: "library/mariadb", Tag: "lts", Arches: archesOfficialDB,
			ConfigPath: "/var/lib/mysql",
			Env: map[string]string{
				"MARIADB_ROOT_PASSWORD": "{{secret:dbroot}}", "MARIADB_DATABASE": "nextcloud",
				"MARIADB_USER": "nextcloud", "MARIADB_PASSWORD": "{{secret:db}}",
			},
			// The image passes arguments starting with "-" to mariadbd:
			// the database stays on 127.0.0.1, off the LAN.
			Cmd:      []string{"--bind-address=127.0.0.1"},
			Ports:    []legacyPort{{Port: 3306, Local: true}},
			MemoryMB: 256,
		}},
	},
	{
		ID: "vaultwarden", Name: "Vaultwarden", Category: appcatalog.CategoryCloud,
		Description: "A password manager server that works with the Bitwarden apps.",
		Note:        "Browsers open its web vault only over HTTPS — reach it through your reverse proxy.",
		Image:       "vaultwarden/server",
		Arches:      []dockerhub.Arch{{Architecture: "amd64"}, {Architecture: "arm", Variant: "v6"}, {Architecture: "arm", Variant: "v7"}, {Architecture: "arm64"}},
		// It listens on 80 by default; ROCKET_PORT is its own setting.
		ContainerPort: 8222, ConfigPath: "/data",
		Env:      map[string]string{"ROCKET_PORT": "8222"},
		PortEnv:  "ROCKET_PORT",
		MemoryMB: 128,
	},
	{
		ID: "syncthing", Name: "Syncthing", Category: appcatalog.CategoryCloud,
		Description:   "Continuous, peer-to-peer file synchronization between devices.",
		Image:         "linuxserver/syncthing",
		Arches:        archesLinuxserver,
		ContainerPort: 8384, ConfigPath: "/config",
		Ports:    []legacyPort{{Port: 22000}, {Port: 22000, UDP: true}, {Port: 21027, UDP: true}},
		MemoryMB: 256,
	},
	{
		ID: "homeassistant", Name: "Home Assistant", Category: appcatalog.CategoryHome,
		Description: "Home automation: lights, sockets, sensors and their rules in one place.",
		Note:        "A large download (about 620 MB) and one of the hungrier apps for memory.",
		Image:       "homeassistant/home-assistant", Tag: "stable",
		Arches:        archesLinuxserver,
		ContainerPort: 8123, ConfigPath: "/config",
		MemoryMB: 512,
	},
	{
		ID: "qbittorrent", Name: "qBittorrent", Category: appcatalog.CategoryDownloads,
		Description: "A BitTorrent client with a web UI.",
		Image:       "linuxserver/qbittorrent",
		Arches:      archesLinuxserver,
		// 8080 is the DreamOS stream server's on a DM900/DM920; WEBUI_PORT is the image's own setting.
		ContainerPort: 8095, ConfigPath: "/config", DataPath: "/downloads",
		Env:      map[string]string{"WEBUI_PORT": "8095"},
		PortEnv:  "WEBUI_PORT",
		Ports:    []legacyPort{{Port: 6881}, {Port: 6881, UDP: true}},
		MemoryMB: 256,
	},
}

func (a legacyApp) portEditable() bool {
	return a.ContainerPort != 0 && (a.PortEnv != "" || a.PortInCmd)
}

func (a legacyApp) withPort(port int) legacyApp {
	if !a.portEditable() {
		return a
	}
	old, now := strconv.Itoa(a.ContainerPort), strconv.Itoa(port)
	env := make(map[string]string, len(a.Env)+1)
	for k, v := range a.Env {
		env[k] = v
	}
	if a.PortEnv != "" {
		env[a.PortEnv] = now
	}
	a.Env = env
	if a.PortInCmd {
		cmd := make([]string, len(a.Cmd))
		for i, c := range a.Cmd {
			cmd[i] = strings.ReplaceAll(c, old, now)
		}
		a.Cmd = cmd
	}
	a.ContainerPort = port
	return a
}

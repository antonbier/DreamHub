package hubs

import (
	"errors"
	"github.com/antonbier/DreamHub/backend/internal/dockersetup"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/appcatalog"
	"github.com/antonbier/DreamHub/backend/internal/auth"
	"github.com/antonbier/DreamHub/backend/internal/configfs"
	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/dockerhub"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/meminfo"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Apps is AppsHub's "Apps" tab backend: the curated catalog
// (internal/appcatalog, plain hand-maintained data, never AI-generated),
// free search and an on-click multi-arch check against Docker Hub
// (internal/dockerhub, App3), and installing/running/removing a Docker
// app through internal/dockerd's loopback helper (App4, apps_install.go
// — docs/APPSHUB_PLAN.md).
type Apps struct {
	// DockerHub is nil in no known build (dockerhub.NewHTTPClient or
	// ./mock always set in app.go), kept as a pointer-shaped dependency
	// like every other optional connector in this codebase for the same
	// "missing degrades, never panics" rule.
	DockerHub dockerhub.Client
	// Docker reaches the isolated helper process (internal/dockerd,
	// App2) — nil disables install/remove/start/stop and the installed
	// list, the same degrade-gracefully rule as DockerHub above.
	Docker dockerd.Client
	// Mem reads free memory for the install-time resource budget check
	// (App4); nil refuses every install with a clear reason rather than
	// skipping the check.
	Mem meminfo.Reader
	// WC reads the HDD's free space for the same check (reusing
	// DeviceInfo, the same source events.Poller's own disk.aboveThreshold
	// already trusts); nil behaves the same as Mem being nil.
	WC webcontrol.Client
	// StorageRoot is where every installed app's own config directory
	// lives, one subdirectory per AppID — the box's recording HDD by
	// default (docs/APPSHUB_PLAN.md: "every app's storage goes on the
	// recording HDD"; letting an app opt into flash storage instead is
	// deferred past this first slice).
	StorageRoot string
	Plans       *plan.Service
	ReadOnly    bool
	// Store appends the change log entries the console's enable/disable
	// make (apps_console.go), the same bar admin_terminal.go's own
	// enable/disable already sets.
	Store *store.Store
	// Auth gives the console's enable step something to re-check the
	// password against (apps_console.go, Terminal's exact posture,
	// A4/#36); nil hides the console entirely.
	Auth auth.ConfigSource
	// console is the per-container console's own off-by-default,
	// not-persisted-across-a-restart gate — see consoleState's own doc
	// comment (apps_console.go).
	console consoleState
	// ConfigFS reads/writes an installed app's own config directory for
	// the backup section (apps_backup.go, App6); nil means configfs.OS,
	// the real file system — tests inject configfs/mock instead.
	ConfigFS configfs.FS
	// Setup sets Docker itself up on the box ("Set up Docker",
	// apps_docker_setup.go); nil hides that.
	Setup     dockersetup.Setup
	lastSetup dockerSetupRun
	setupMu   sync.Mutex
	// Now overrides the clock (tests); nil means time.Now.
	Now func() time.Time
}

func (h *Apps) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// Perms: everything here needs the system permission, the same bar
// AdminHub's package browser uses (docs/APPSHUB_PLAN.md, "Admin-only to
// manage") — there's no viewer-facing read of this tab yet.
func (h *Apps) Perms() map[string]perm.Perm {
	return map[string]perm.Perm{
		"GET /api/v1/apps/catalog":                     perm.System,
		"GET /api/v1/apps/search":                      perm.System,
		"GET /api/v1/apps/manifest":                    perm.System,
		"GET /api/v1/apps/installed":                   perm.System,
		"POST /api/v1/apps/install/plan":               perm.System,
		"POST /api/v1/apps/installed/{id}/remove/plan": perm.System,
		"POST /api/v1/apps/installed/{id}/start":       perm.System,
		"POST /api/v1/apps/installed/{id}/stop":        perm.System,
		"GET /api/v1/apps/installed/{id}/logs":         perm.System,
		"GET /api/v1/apps/console/status":              perm.System,
		"POST /api/v1/apps/console/enable":             perm.System,
		"POST /api/v1/apps/console/disable":            perm.System,
		"GET /api/v1/apps/installed/{id}/console":      perm.System,
		"GET /api/v1/apps/docker":                      perm.System,
		"POST /api/v1/apps/docker/setup/plan":          perm.System,
	}
}

func (h *Apps) Mount(mux httpx.Mux) {
	mux.HandleFunc("GET /api/v1/apps/catalog", h.catalog)
	mux.HandleFunc("GET /api/v1/apps/search", h.search)
	mux.HandleFunc("GET /api/v1/apps/manifest", h.manifest)
	mux.HandleFunc("GET /api/v1/apps/installed", h.listInstalled)
	mux.HandleFunc("POST /api/v1/apps/install/plan", h.planInstall)
	mux.HandleFunc("POST /api/v1/apps/installed/{id}/remove/plan", h.planRemove)
	mux.HandleFunc("POST /api/v1/apps/installed/{id}/start", h.startInstalled)
	mux.HandleFunc("POST /api/v1/apps/installed/{id}/stop", h.stopInstalled)
	mux.HandleFunc("GET /api/v1/apps/installed/{id}/logs", h.containerLogs)
	mux.HandleFunc("GET /api/v1/apps/console/status", h.consoleStatus)
	mux.HandleFunc("POST /api/v1/apps/console/enable", h.enableConsole)
	mux.HandleFunc("POST /api/v1/apps/console/disable", h.disableConsole)
	mux.HandleFunc("GET /api/v1/apps/installed/{id}/console", h.containerConsole)
	mux.HandleFunc("GET /api/v1/apps/docker", h.dockerStatus)
	mux.HandleFunc("POST /api/v1/apps/docker/setup/plan", h.planDockerSetup)
}

// catalogEntry is one curated app with box compatibility already
// resolved — the frontend never computes this itself.
type catalogEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Image       string `json:"image"`
	Compatible  bool   `json:"compatible"`
	// Installed is set from a live List against the helper (App4), not
	// stored anywhere — the same "always read live, never go stale"
	// choice dockerd.Container.HostPort makes.
	Installed bool `json:"installed"`
}

func (h *Apps) catalog(w http.ResponseWriter, r *http.Request) {
	box := dockerhub.BoxArch()
	installed := map[string]bool{}
	if h.Docker != nil {
		if list, err := h.Docker.List(r.Context()); err == nil {
			for _, c := range list {
				installed[c.AppID()] = true
			}
		}
	}
	out := make([]catalogEntry, 0, len(appcatalog.Apps))
	for _, a := range appcatalog.Apps {
		out = append(out, catalogEntry{
			ID: a.ID, Name: a.Name, Category: a.Category, Description: a.Description,
			Image: a.Image, Compatible: a.SupportsArch(box), Installed: installed[a.ID],
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"apps": out})
}

// searchResult is one free-search hit — never pre-checked for arch
// compatibility (docs/APPSHUB_PLAN.md: "checks the image's own multi-arch
// manifest lazily, on click", not per result).
type searchResult struct {
	Image       string `json:"image"`
	Description string `json:"description"`
	Stars       int    `json:"stars"`
}

func (h *Apps) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_query", "q is required")
		return
	}
	if h.DockerHub == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "dockerhub_unavailable", "Docker Hub search isn't available")
		return
	}
	results, err := h.DockerHub.Search(r.Context(), q)
	if errors.Is(err, dockerhub.ErrUnavailable) {
		httpx.WriteError(w, r, http.StatusBadGateway, "dockerhub_unavailable", "Docker Hub didn't answer")
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "dockerhub_failed", "the search failed")
		return
	}
	out := make([]searchResult, 0, len(results))
	for _, repo := range results {
		out = append(out, searchResult{Image: repo.Name, Description: repo.Description, Stars: repo.Stars})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": out})
}

// validDockerImage is deliberately strict: Docker Hub's own
// namespace/repository naming (lowercase letters, digits, and `.`, `_`,
// `-` separators) — the gate before an image string is ever interpolated
// into a Docker Hub request path (dockerhub.HTTPClient.fetchManifest,
// which cannot use url.PathEscape on the "/" without breaking the path).
var validDockerImage = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*/[a-z0-9]+(?:[._-][a-z0-9]+)*$`)

func (h *Apps) manifest(w http.ResponseWriter, r *http.Request) {
	image := r.URL.Query().Get("image")
	if !validDockerImage.MatchString(image) {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_image", `image must look like "namespace/repository"`)
		return
	}
	tag := r.URL.Query().Get("tag")
	if tag == "" {
		tag = "latest"
	}
	if h.DockerHub == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "dockerhub_unavailable", "Docker Hub isn't available")
		return
	}
	arches, err := h.DockerHub.Manifest(r.Context(), image, tag)
	if errors.Is(err, dockerhub.ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such image or tag on Docker Hub")
		return
	}
	if errors.Is(err, dockerhub.ErrUnavailable) {
		httpx.WriteError(w, r, http.StatusBadGateway, "dockerhub_unavailable", "Docker Hub didn't answer")
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "dockerhub_failed", "the manifest check failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"architectures": arches,
		"compatible":    dockerhub.Supports(arches, dockerhub.BoxArch()),
	})
}

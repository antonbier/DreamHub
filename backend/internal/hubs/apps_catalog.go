package hubs

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/appcatalog"
	"github.com/antonbier/DreamHub/backend/internal/dockerhub"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/perm"
)

// Apps is AppsHub's "Apps" tab backend (docs/APPSHUB_PLAN.md, App3 —
// "Curated list + search"): the curated catalog (internal/appcatalog,
// plain hand-maintained data, never AI-generated) plus free search and
// an on-click multi-arch check against Docker Hub (internal/dockerhub).
// Installing, running or removing a Docker app — anything that would
// actually touch internal/dockerd's helper — is App4, not this; every
// route here is read-only.
type Apps struct {
	// DockerHub is nil in no known build (dockerhub.NewHTTPClient or
	// ./mock always set in app.go), kept as a pointer-shaped dependency
	// like every other optional connector in this codebase for the same
	// "missing degrades, never panics" rule.
	DockerHub dockerhub.Client
}

// Perms: everything here needs the system permission, the same bar
// AdminHub's package browser uses (docs/APPSHUB_PLAN.md, "Admin-only to
// manage") — there's no viewer-facing read of this tab yet.
func (h *Apps) Perms() map[string]perm.Perm {
	return map[string]perm.Perm{
		"GET /api/v1/apps/catalog":  perm.System,
		"GET /api/v1/apps/search":   perm.System,
		"GET /api/v1/apps/manifest": perm.System,
	}
}

func (h *Apps) Mount(mux httpx.Mux) {
	mux.HandleFunc("GET /api/v1/apps/catalog", h.catalog)
	mux.HandleFunc("GET /api/v1/apps/search", h.search)
	mux.HandleFunc("GET /api/v1/apps/manifest", h.manifest)
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
}

func (h *Apps) catalog(w http.ResponseWriter, r *http.Request) {
	box := dockerhub.BoxArch()
	out := make([]catalogEntry, 0, len(appcatalog.Apps))
	for _, a := range appcatalog.Apps {
		out = append(out, catalogEntry{
			ID: a.ID, Name: a.Name, Category: a.Category, Description: a.Description,
			Image: a.Image, Compatible: a.SupportsArch(box),
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

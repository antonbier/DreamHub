package hubs

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/apt"
	"github.com/antonbier/DreamHub/backend/internal/httpx"
)

// AdminHub's package browser (docs/ADMINHUB_PLAN.md, A2, #47): the box's
// installed and upgradable packages, grouped by apt's own Section field,
// filterable. Read-only — install/remove as a plan is a later slice, once
// the box has confirmed `apt-get -s` simulate output and the generalized
// "don't restart a service while recording" check.

// listPackages answers GET /api/v1/admin/packages, filtered by:
//   - q: a case-insensitive substring of the name or description
//   - category: an exact match against Package.Category
//   - status: "upgradable" (only those with an update pending) or
//     "installed" (everything dpkg knows about, the default — there's no
//     "not installed" catalog yet, see docs/ADMINHUB_PLAN.md)
func (h *Admin) listPackages(w http.ResponseWriter, r *http.Request) {
	if h.Apt == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "apt_unavailable", "dpkg/apt aren't available on this system")
		return
	}
	pkgs, err := h.Apt.List(r.Context())
	if errors.Is(err, apt.ErrUnavailable) {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "apt_unavailable", "dpkg/apt aren't available on this system")
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadGateway, "apt_failed", "could not read the package list")
		return
	}

	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	category := r.URL.Query().Get("category")
	status := r.URL.Query().Get("status")

	out := make([]apt.Package, 0, len(pkgs))
	categories := map[string]bool{}
	for _, p := range pkgs {
		categories[p.Category] = true
		if q != "" && !strings.Contains(strings.ToLower(p.Name), q) && !strings.Contains(strings.ToLower(p.Description), q) {
			continue
		}
		if category != "" && p.Category != category {
			continue
		}
		if status == "upgradable" && !p.Upgradable {
			continue
		}
		out = append(out, p)
	}

	cats := make([]string, 0, len(categories))
	for c := range categories {
		cats = append(cats, c)
	}
	sort.Strings(cats)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"packages": out, "categories": cats})
}

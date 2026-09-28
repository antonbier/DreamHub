// Package hub is the core's registry of hubs — the user-facing feature areas
// (CONCEPT.md, "The hub contract"): identity, navigation placement,
// required role, requirements and the resulting status, an enable switch
// per hub, and — as a hub registers one — a search provider (search.go)
// and status-change events (watcher.go). Dashboard tile types contributed
// per hub are not part of the contract yet (internal/dashboard still owns
// a fixed catalog); that's added as more hubs have something to tile.
package hub

import (
	"fmt"
	"sort"

	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/status"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// Group is a navigation group. The order of groupOrder is the order in the
// sidebar; regrouping a hub is a change to its Descriptor, not to code.
type Group string

const (
	GroupMedia   Group = "media"
	GroupProgram Group = "program"
	GroupDevice  Group = "device"
	GroupBottom  Group = "bottom" // pinned to the bottom next to Settings (AdminHub)
)

var groupOrder = []Group{GroupMedia, GroupProgram, GroupDevice, GroupBottom}

// Requirement is something a hub needs. Exactly one of Connector, Capability
// or Hub is set. A failing optional requirement degrades the hub; a failing
// required one makes it unavailable.
type Requirement struct {
	Connector  string `json:"connector,omitempty"`
	Capability string `json:"capability,omitempty"`
	Hub        string `json:"hub,omitempty"`
	Optional   bool   `json:"optional,omitempty"`
}

// Descriptor declares a hub.
type Descriptor struct {
	ID    string // stable identifier, e.g. "media"
	Name  string // fixed hub name, e.g. "MediaHub"
	Label string // i18n key of the descriptive label, e.g. "hub.media"
	Icon  string // icon identifier for the frontend
	Path  string // frontend route
	Group Group
	Order int
	// Role is the minimum role that can see the hub.
	Role     store.Role
	Requires []Requirement
}

// Capability is something on the device a hub may need (a binary, a
// service). Check must be cheap.
type Capability struct {
	ID      string
	Check   func() bool
	Problem status.Problem
}

// Registry holds hub descriptors and capabilities.
type Registry struct {
	hubs   []Descriptor
	byID   map[string]Descriptor
	caps   map[string]Capability
	search map[string]SearchProvider
	views  map[string]ViewProvider
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byID: map[string]Descriptor{}, caps: map[string]Capability{}, search: map[string]SearchProvider{}, views: map[string]ViewProvider{}}
}

// Add registers a hub.
func (r *Registry) Add(d Descriptor) error {
	if _, dup := r.byID[d.ID]; dup {
		return fmt.Errorf("hub: duplicate id %q", d.ID)
	}
	valid := false
	for _, g := range groupOrder {
		valid = valid || g == d.Group
	}
	if !valid {
		return fmt.Errorf("hub: %q has unknown group %q", d.ID, d.Group)
	}
	if d.Role == "" {
		d.Role = store.RoleUser
	}
	r.hubs = append(r.hubs, d)
	r.byID[d.ID] = d
	return nil
}

// AddCapability registers a device capability.
func (r *Registry) AddCapability(c Capability) { r.caps[c.ID] = c }

// EnabledKey is the settings key of a hub's enable switch.
func EnabledKey(hubID string) string { return "hubs." + hubID + ".enabled" }

// SettingsSection returns the "hubs" settings section: one enable switch per
// registered hub (used by the settings menu and the onboarding wizard).
// Call it after all hubs are added.
func (r *Registry) SettingsSection() settings.Section {
	sec := settings.Section{ID: "hubs", Label: "settings.hubs"}
	for _, d := range r.hubs {
		sec.Fields = append(sec.Fields, settings.Field{
			Key: EnabledKey(d.ID), Type: settings.TypeBool, Scope: settings.ScopeGlobal, Label: d.Label, Default: true,
		})
	}
	return sec
}

// Status is a hub's availability as seen by the core.
type Status struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Label    string           `json:"label"`
	Icon     string           `json:"icon"`
	Path     string           `json:"path"`
	Group    Group            `json:"group"`
	Role     store.Role       `json:"role"`
	Enabled  bool             `json:"enabled"`
	Level    status.Level     `json:"status"`
	Problems []status.Problem `json:"problems"`
	Requires []Requirement    `json:"requires"`
}

func worse(a, b status.Level) status.Level {
	rank := map[status.Level]int{status.Available: 0, status.Degraded: 1, status.Unavailable: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func failLevel(req Requirement) status.Level {
	if req.Optional {
		return status.Degraded
	}
	return status.Unavailable
}

// Statuses evaluates every hub against the connector reports, capabilities
// and enable switches in v. Hub-on-hub requirements are evaluated after
// every hub's own status is known (one level deep, which is all the v1 hubs
// need).
func (r *Registry) Statuses(v settings.Values, reports map[string]connector.Report) []Status {
	out := make([]Status, 0, len(r.hubs))
	idx := map[string]int{}
	for _, d := range r.hubs {
		st := Status{
			ID: d.ID, Name: d.Name, Label: d.Label, Icon: d.Icon, Path: d.Path, Group: d.Group, Role: d.Role,
			Enabled: v.Bool(EnabledKey(d.ID)), Level: status.Available, Problems: []status.Problem{}, Requires: d.Requires,
		}
		if st.Requires == nil {
			st.Requires = []Requirement{}
		}
		for _, req := range d.Requires {
			switch {
			case req.Connector != "":
				rep, ok := reports[req.Connector]
				if !ok {
					st.Level = worse(st.Level, failLevel(req))
					st.Problems = append(st.Problems, status.Problem{Code: status.CodeConnectorUnknown, Params: map[string]any{"connector": req.Connector}})
					continue
				}
				if rep.State != connector.OK {
					st.Level = worse(st.Level, failLevel(req))
					if rep.Problem != nil {
						st.Problems = append(st.Problems, *rep.Problem)
					}
				}
			case req.Capability != "":
				c, ok := r.caps[req.Capability]
				if !ok || !c.Check() {
					st.Level = worse(st.Level, failLevel(req))
					p := c.Problem
					if !ok {
						p = status.Problem{Code: status.CodeCapabilityMissing, Params: map[string]any{"capability": req.Capability}}
					}
					st.Problems = append(st.Problems, p)
				}
			}
		}
		idx[d.ID] = len(out)
		out = append(out, st)
	}

	// Hub-on-hub requirements, against each hub's own (pre-dependency) status.
	own := make([]Status, len(out))
	copy(own, out)
	for i, d := range r.hubs {
		for _, req := range d.Requires {
			if req.Hub == "" {
				continue
			}
			j, ok := idx[req.Hub]
			if ok && own[j].Enabled && own[j].Level != status.Unavailable {
				continue
			}
			name := req.Hub
			if ok {
				name = own[j].Name
			}
			out[i].Level = worse(out[i].Level, failLevel(req))
			p := status.Problem{Code: status.CodeHubDependencyUnavailable, Params: map[string]any{"hub": name}}
			if ok && !own[j].Enabled {
				p.Fix, p.FixParams = status.FixEnableHub, map[string]any{"hub": name}
			}
			out[i].Problems = append(out[i].Problems, p)
		}
	}
	return out
}

// Visible reports whether a user with role can see hubs requiring min.
func Visible(min, role store.Role) bool { return min != store.RoleAdmin || role == store.RoleAdmin }

// NavItem is one entry in the sidebar.
type NavItem struct {
	ID       string           `json:"id"`
	Kind     string           `json:"kind"` // "core" or "hub"
	Name     string           `json:"name,omitempty"`
	Label    string           `json:"label"`
	Icon     string           `json:"icon"`
	Path     string           `json:"path"`
	Level    status.Level     `json:"status,omitempty"`
	Problems []status.Problem `json:"problems,omitempty"`
}

// NavSection is a group of sidebar entries.
type NavSection struct {
	ID    string    `json:"id"`
	Label string    `json:"label,omitempty"`
	Items []NavItem `json:"items"`
}

// Navigation is the complete sidebar for one user.
type Navigation struct {
	// Labels is the user's choice of "descriptive" or "hub" names.
	Labels   string       `json:"labels"`
	Sections []NavSection `json:"sections"`
}

// Navigation builds the sidebar: Dashboard first, then the hub groups in
// order, then the bottom section with AdminHub and Settings. Disabled hubs
// and hubs the role can't see are left out; unavailable hubs stay in (the
// UI greys them out and shows why). Empty groups are omitted.
func (r *Registry) Navigation(statuses []Status, role store.Role, labels string) Navigation {
	byGroup := map[Group][]Status{}
	for _, s := range statuses {
		if !s.Enabled || !Visible(s.Role, role) {
			continue
		}
		byGroup[s.Group] = append(byGroup[s.Group], s)
	}
	order := map[string]int{}
	for _, d := range r.hubs {
		order[d.ID] = d.Order
	}

	nav := Navigation{Labels: labels, Sections: []NavSection{{
		ID:    "main",
		Items: []NavItem{{ID: "dashboard", Kind: "core", Label: "nav.dashboard", Icon: "dashboard", Path: "/"}},
	}}}
	for _, g := range groupOrder {
		hubs := byGroup[g]
		sort.SliceStable(hubs, func(i, j int) bool { return order[hubs[i].ID] < order[hubs[j].ID] })
		sec := NavSection{ID: string(g), Items: []NavItem{}}
		if g != GroupBottom {
			sec.Label = "nav.group." + string(g)
		}
		for _, h := range hubs {
			item := NavItem{ID: h.ID, Kind: "hub", Name: h.Name, Label: h.Label, Icon: h.Icon, Path: h.Path, Level: h.Level}
			if len(h.Problems) > 0 {
				item.Problems = h.Problems
			}
			sec.Items = append(sec.Items, item)
		}
		if g == GroupBottom {
			// Diagnostics used to be its own core nav entry; it's now a tab
			// inside SystemHub's screen (docs/V1_PLAN.md, WP6), matching
			// CONCEPT.md's "a diagnostics page in SystemHub".
			sec.Items = append(sec.Items, NavItem{ID: "settings", Kind: "core", Label: "nav.settings", Icon: "settings", Path: "/settings"})
		}
		if len(sec.Items) > 0 {
			nav.Sections = append(nav.Sections, sec)
		}
	}
	return nav
}

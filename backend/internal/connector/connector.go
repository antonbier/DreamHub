// Package connector is the core's registry of data sources and external
// services (CONCEPT.md, "Three building blocks"). Hubs never talk to
// WebControl, TMDB or an AI provider directly — they go through connectors,
// and the core knows at any time which connectors work.
package connector

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/status"
)

// State is a connector's current state.
type State string

const (
	OK             State = "ok"
	NotConfigured  State = "not_configured"
	Unreachable    State = "unreachable"
	NotImplemented State = "not_implemented"
	// NotInstalled: an optional plugin the connector talks to isn't on the
	// box. A normal state, not an error — see PRINCIPLES.md, "Runtime
	// capability detection".
	NotInstalled State = "not_installed"
)

// Report is the result of checking a connector. It never contains secrets.
type Report struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	State     State           `json:"state"`
	Problem   *status.Problem `json:"problem,omitempty"`
	CheckedAt time.Time       `json:"checkedAt"`
}

// Connector is a data source or external service.
type Connector interface {
	ID() string
	Name() string
	// Settings are the connector's own settings sections (e.g. an API key),
	// registered into the central settings schema at startup.
	Settings() []settings.Section
	// Check reports the connector's state. v holds the effective global
	// settings (including unmasked secrets — never pass them on to clients).
	Check(ctx context.Context, v settings.Values) Report
}

// checkTimeout bounds a single connector check, so a hung box can't block
// the navigation or diagnostics endpoints.
const checkTimeout = 2 * time.Second

// Registry holds all connectors and caches their reports.
type Registry struct {
	list []Connector
	byID map[string]Connector
	ttl  time.Duration
	now  func() time.Time

	mu      sync.Mutex
	cache   map[string]Report
	cachedV uint64
	cachedT time.Time
}

// NewRegistry returns an empty registry whose reports are cached for ttl.
func NewRegistry(ttl time.Duration) *Registry {
	return &Registry{byID: map[string]Connector{}, ttl: ttl, now: time.Now}
}

// Add registers a connector.
func (r *Registry) Add(c Connector) {
	r.list = append(r.list, c)
	r.byID[c.ID()] = c
}

// All returns the registered connectors in registration order.
func (r *Registry) All() []Connector { return r.list }

// Get returns a connector by ID.
func (r *Registry) Get(id string) (Connector, bool) {
	c, ok := r.byID[id]
	return c, ok
}

// Reports returns the state of every connector, keyed by ID. Results are
// cached for the registry's TTL and recomputed immediately when the
// settings version changes (e.g. an API key was just entered).
func (r *Registry) Reports(ctx context.Context, v settings.Values, settingsVersion uint64) map[string]Report {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if r.cache != nil && r.cachedV == settingsVersion && now.Sub(r.cachedT) < r.ttl {
		return r.cache
	}

	out := make(map[string]Report, len(r.list))
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, c := range r.list {
		wg.Add(1)
		go func(c Connector) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, checkTimeout)
			defer cancel()
			rep := c.Check(cctx, v)
			rep.ID, rep.Name, rep.CheckedAt = c.ID(), c.Name(), now.UTC()
			mu.Lock()
			out[c.ID()] = rep
			mu.Unlock()
		}(c)
	}
	wg.Wait()

	r.cache, r.cachedV, r.cachedT = out, settingsVersion, now
	return out
}

// SortedReports returns Reports as a slice in registration order.
func (r *Registry) SortedReports(ctx context.Context, v settings.Values, version uint64) []Report {
	m := r.Reports(ctx, v, version)
	order := map[string]int{}
	for i, c := range r.list {
		order[c.ID()] = i
	}
	out := make([]Report, 0, len(m))
	for _, rep := range m {
		out = append(out, rep)
	}
	sort.Slice(out, func(i, j int) bool { return order[out[i].ID] < order[out[j].ID] })
	return out
}

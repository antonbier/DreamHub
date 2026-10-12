package extension

import (
	"sort"
	"sync"
)

// Entry is one registered extension: its declared metadata plus the
// Client the core calls it through.
type Entry struct {
	Registration
	Client Client
}

// Registry holds the extensions registered so far — in memory only,
// like plan.Service: an extension simply registers again after DreamHub
// restarts (ARCHITECTURE.md's systemd notes say ordering between units
// after a boot isn't guaranteed, so a registration that only has to
// survive until the next restart, not persist across one, is the easier
// contract to rely on correctly).
type Registry struct {
	mu      sync.Mutex
	entries map[string]Entry
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{entries: map[string]Entry{}} }

// Register adds or replaces one extension's entry — a re-registration
// (the extension's process restarted) simply overwrites the old one.
func (r *Registry) Register(e Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[e.ID] = e
}

// Unregister removes one extension's entry, e.g. on a clean shutdown.
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, id)
}

// List returns every registered extension, sorted by ID for a stable
// order across requests.
func (r *Registry) List() []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Entry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get returns one extension's entry.
func (r *Registry) Get(id string) (Entry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[id]
	return e, ok
}

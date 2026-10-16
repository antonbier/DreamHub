// Package boxes is the registry of the Dreamboxes one DreamHub controls
// (docs/MULTIDEVICE_PLAN.md, M1) and the clients that make every hub
// box-aware without each hub knowing: a hub keeps one webcontrol.Client,
// and that client forwards each call to the box the request's context
// names (internal/box).
package boxes

import (
	"context"
	"errors"
	"sync"

	"github.com/antonbier/DreamHub/backend/internal/autotimer"
	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// ErrUnknownBox: the context names a box that isn't registered.
var ErrUnknownBox = errors.New("boxes: unknown box")

// Clients are the connections to one box.
type Clients struct {
	WC        webcontrol.Client
	Series    serienrecorder.Client
	AutoTimer autotimer.Client
}

// Entry is a registered box with its connections.
type Entry struct {
	store.Box
	Clients
	// Local: DreamHub runs on this box's machine, so its recordings, logs,
	// services and packages are reachable through this process.
	Local bool
}

// Registry holds the boxes in their order. The first one is the default
// for everything that names no box.
type Registry struct {
	mu      sync.RWMutex
	entries []Entry
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// Add registers a box, or replaces the one with the same ID.
func (r *Registry) Add(b store.Box, c Clients, local bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.entries {
		if r.entries[i].ID == b.ID {
			b.Position = r.entries[i].Position
			r.entries[i] = Entry{b, c, local}
			return
		}
	}
	b.Position = len(r.entries)
	r.entries = append(r.entries, Entry{b, c, local})
}

// Remove unregisters a box.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.entries {
		if r.entries[i].ID == id {
			r.entries = append(r.entries[:i], r.entries[i+1:]...)
			return
		}
	}
}

// List returns the boxes in order.
func (r *Registry) List() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Entry(nil), r.entries...)
}

// IDs returns the box IDs in order.
func (r *Registry) IDs() []string {
	var ids []string
	for _, e := range r.List() {
		ids = append(ids, e.ID)
	}
	return ids
}

// DefaultID is the ID of the first box, "" when there is none.
func (r *Registry) DefaultID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.entries) == 0 {
		return ""
	}
	return r.entries[0].ID
}

// Get returns the box with this ID.
func (r *Registry) Get(id string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.entries {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// For returns the box ctx is addressed to; a context that names none
// means the first box.
func (r *Registry) For(ctx context.Context) (Entry, error) {
	id := box.ID(ctx)
	if id == "" {
		id = r.DefaultID()
	}
	e, ok := r.Get(id)
	if !ok {
		return Entry{}, ErrUnknownBox
	}
	return e, nil
}

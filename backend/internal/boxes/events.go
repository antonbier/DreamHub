package boxes

import (
	"encoding/json"

	"github.com/antonbier/DreamHub/backend/internal/store"
)

// EventBox is the box an event is about: the poller tags its events with the
// box ID once there are several boxes; an untagged one (a hub's watch) is
// about the first box.
func (r *Registry) EventBox(params json.RawMessage) (store.Box, bool) {
	var p struct {
		Box string `json:"box"`
	}
	_ = json.Unmarshal(params, &p)
	if p.Box == "" {
		p.Box = r.DefaultID()
	}
	e, ok := r.Get(p.Box)
	if !ok {
		return store.Box{}, false
	}
	return e.Box, true
}

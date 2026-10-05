package hubs

import (
	"slices"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/systemd"
)

func units(states map[string]string) []systemd.Unit {
	var out []systemd.Unit
	for _, name := range []string{"enigma2.service", "smb.service", "nmb.service", "dreamhub.service"} {
		if st, ok := states[name]; ok {
			out = append(out, systemd.Unit{Name: name, Active: st})
		}
	}
	return out
}

func names(us []systemd.Unit) []string {
	var out []string
	for _, u := range us {
		out = append(out, u.Name)
	}
	return out
}

func TestServiceWatch(t *testing.T) {
	w := newServiceWatch()
	steps := []struct {
		states            map[string]string
		failed, recovered []string
	}{
		// Baseline: nmb already failed at start — not announced.
		{map[string]string{"enigma2.service": "active", "smb.service": "active", "nmb.service": "failed", "dreamhub.service": "active"}, nil, nil},
		// smb fails once: not yet (systemd may restart it).
		{map[string]string{"enigma2.service": "active", "smb.service": "failed", "nmb.service": "failed"}, nil, nil},
		// smb still failed: down. nmb comes back: back (it was down at start).
		{map[string]string{"enigma2.service": "active", "smb.service": "failed", "nmb.service": "active"}, []string{"smb.service"}, []string{"nmb.service"}},
		// smb still failed: not again. DreamHub itself is never reported.
		{map[string]string{"enigma2.service": "active", "smb.service": "failed", "dreamhub.service": "failed"}, nil, nil},
		// smb back.
		{map[string]string{"enigma2.service": "active", "smb.service": "active", "dreamhub.service": "failed"}, nil, []string{"smb.service"}},
		// enigma2 flickers failed once, then is back: nothing.
		{map[string]string{"enigma2.service": "failed", "smb.service": "active"}, nil, nil},
		{map[string]string{"enigma2.service": "active", "smb.service": "active"}, nil, nil},
	}
	for i, st := range steps {
		failed, recovered := w.step(units(st.states))
		if got := names(failed); !slices.Equal(got, st.failed) {
			t.Errorf("step %d: failed %v, want %v", i, got, st.failed)
		}
		if got := names(recovered); !slices.Equal(got, st.recovered) {
			t.Errorf("step %d: recovered %v, want %v", i, got, st.recovered)
		}
	}
}

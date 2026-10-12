package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// The DreamHub extension contract (docs/BUILDING_EXTENSIONS.md): GET
// /widgets, POST /actions/{id}/plan, POST /actions/{id}/apply — plain
// JSON, nothing imported from DreamHub's own code.

// Widget is one entry of GET /widgets (schema version 1).
type Widget struct {
	Type        string    `json:"type"`
	Label       string    `json:"label"`
	Value       string    `json:"value,omitempty"`
	Tone        string    `json:"tone,omitempty"`
	Rows        []ListRow `json:"rows,omitempty"`
	ActionID    string    `json:"actionId,omitempty"`
	ButtonLabel string    `json:"buttonLabel,omitempty"`
}

type ListRow struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type texts struct {
	on, off, turnOn, turnOff, noKey, badKey, notFound, found, readings, previewOn, previewOff string
}

var lang = map[string]texts{
	"de": {on: "An", off: "Aus", turnOn: "Einschalten", turnOff: "Ausschalten",
		noKey: "Geräteschlüssel fehlt", badKey: "Status nicht lesbar — Geräteschlüssel prüfen", notFound: "Nicht im Netzwerk gefunden",
		found: "Gefunden — noch nicht eingerichtet", readings: "Messwerte",
		previewOn: "%s wird eingeschaltet.", previewOff: "%s wird ausgeschaltet."},
	"en": {on: "On", off: "Off", turnOn: "Turn on", turnOff: "Turn off",
		noKey: "Device key missing", badKey: "State unreadable — check the device key", notFound: "Not found on the network",
		found: "Found — not set up yet", readings: "Readings",
		previewOn: "%s will be switched on.", previewOff: "%s will be switched off."},
}

// readings are the measurement fields a power-metering plug may report,
// shown as received.
var readings = []struct{ key, unit string }{
	{"power", "W"}, {"voltage", "V"}, {"current", "A"}, {"dayKwh", "kWh"},
}

type Server struct {
	cfg     Config
	devices *Devices
}

func (s *Server) t() texts { return lang[s.cfg.Lang] }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /widgets", s.widgets)
	mux.HandleFunc("POST /actions/{action}/plan", s.plan)
	mux.HandleFunc("POST /actions/{action}/apply", s.apply)
	return mux
}

func actionID(id string) string { return "toggle-" + id }

func (s *Server) name(id string) string {
	if c, ok := s.cfg.device(id); ok && c.Name != "" {
		return c.Name
	}
	return "eWeLink " + id
}

func (s *Server) widgets(w http.ResponseWriter, r *http.Request) {
	t := s.t()
	var out []Widget
	seen := map[string]bool{}
	for _, d := range s.devices.Snapshot() {
		seen[d.ID] = true
		name := s.name(d.ID)
		conf, configured := s.cfg.device(d.ID)
		switch {
		case !configured:
			out = append(out, Widget{Type: "status", Label: name, Value: t.found, Tone: "warn"})
		case conf.Key == "" && d.State == nil:
			out = append(out, Widget{Type: "status", Label: name, Value: t.noKey, Tone: "bad"})
		case d.State == nil:
			out = append(out, Widget{Type: "status", Label: name, Value: t.badKey, Tone: "bad"})
		default:
			on, known := d.On()
			if !known {
				out = append(out, Widget{Type: "status", Label: name, Value: "?", Tone: "warn"})
				break
			}
			value, tone, button := t.off, "warn", t.turnOn
			if on {
				value, tone, button = t.on, "good", t.turnOff
			}
			out = append(out, Widget{Type: "status", Label: name, Value: value, Tone: tone})
			if rows := readingRows(d.State); len(rows) > 0 {
				out = append(out, Widget{Type: "list", Label: name + " · " + t.readings, Rows: rows})
			}
			out = append(out, Widget{Type: "action", Label: name, ActionID: actionID(d.ID), ButtonLabel: button})
		}
	}
	for _, c := range s.cfg.Devices {
		if !seen[c.ID] {
			out = append(out, Widget{Type: "status", Label: s.name(c.ID), Value: t.notFound, Tone: "bad"})
		}
	}
	if out == nil {
		out = []Widget{}
	}
	writeJSON(w, http.StatusOK, out)
}

func readingRows(state map[string]any) []ListRow {
	var rows []ListRow
	for _, r := range readings {
		if v, ok := state[r.key]; ok {
			rows = append(rows, ListRow{Label: r.key, Value: fmt.Sprint(v) + " " + r.unit})
		}
	}
	return rows
}

// target resolves an action id to a configured, readable device.
func (s *Server) target(action string) (Device, DeviceConfig, bool) {
	id, ok := strings.CutPrefix(action, "toggle-")
	if !ok {
		return Device{}, DeviceConfig{}, false
	}
	conf, configured := s.cfg.device(id)
	d, seen := s.devices.Get(id)
	if !configured || !seen || d.State == nil {
		return Device{}, DeviceConfig{}, false
	}
	return d, conf, true
}

func (s *Server) plan(w http.ResponseWriter, r *http.Request) {
	_ = s.devices.Refresh(r.Context())
	d, conf, ok := s.target(r.PathValue("action"))
	if !ok {
		http.Error(w, "unknown or unreadable device", http.StatusNotFound)
		return
	}
	on, known := d.On()
	if !known {
		http.Error(w, "the device's state isn't known", http.StatusConflict)
		return
	}
	t := s.t()
	summary := fmt.Sprintf(t.previewOn, s.name(d.ID))
	if on {
		summary = fmt.Sprintf(t.previewOff, s.name(d.ID))
		if conf.OffWarning != "" {
			summary += " " + conf.OffWarning
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "fingerprint": d.Fingerprint()})
}

func (s *Server) apply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "expected {\"fingerprint\": ...}", http.StatusBadRequest)
		return
	}
	_ = s.devices.Refresh(r.Context())
	d, _, ok := s.target(r.PathValue("action"))
	if !ok {
		http.Error(w, "unknown or unreadable device", http.StatusNotFound)
		return
	}
	// Someone switched it from the app since the preview: refuse.
	if d.Fingerprint() != body.Fingerprint {
		http.Error(w, "the device changed since the preview", http.StatusConflict)
		return
	}
	on, _ := d.On()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.devices.Switch(ctx, d.ID, !on); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

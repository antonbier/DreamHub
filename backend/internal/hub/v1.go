package hub

import (
	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/status"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// IDs of the v1 hubs (CONCEPT.md, "Hubs").
const (
	IDMedia     = "media"
	IDTimer     = "timer"
	IDGuide     = "guide"
	IDDiscovery = "discovery"
	IDSeries    = "series"
	IDSystem    = "system"
	IDAdmin     = "admin"
)

// CapJournald is the capability to read logs via journalctl (AdminHub).
const CapJournald = "journald"

// V1 returns the descriptors of the hubs planned for v1. The v1.1 hubs
// (RadioHub, ChannelHub, ControlHub) are added when they're built.
func V1() []Descriptor {
	wc := Requirement{Connector: connector.IDWebControl}
	return []Descriptor{
		{ID: IDMedia, Name: "MediaHub", Label: "hub.media", Icon: "recordings", Path: "/media", Group: GroupMedia, Order: 10,
			Requires: []Requirement{wc, {Connector: connector.IDTMDB, Optional: true}}},
		{ID: IDSeries, Name: "SeriesHub", Label: "hub.series", Icon: "series", Path: "/series", Group: GroupMedia, Order: 20,
			Requires: []Requirement{wc, {Connector: connector.IDSerienRecorder}}},
		{ID: IDGuide, Name: "GuideHub", Label: "hub.guide", Icon: "guide", Path: "/guide", Group: GroupProgram, Order: 10,
			Requires: []Requirement{wc, {Connector: connector.IDTMDB, Optional: true}}},
		{ID: IDTimer, Name: "TimerHub", Label: "hub.timer", Icon: "timers", Path: "/timers", Group: GroupProgram, Order: 20,
			Requires: []Requirement{wc}},
		// Recommendations need an AI provider; creating a timer from one
		// needs TimerHub, so without it DiscoveryHub still works, but degraded.
		{ID: IDDiscovery, Name: "DiscoveryHub", Label: "hub.discovery", Icon: "recommendations", Path: "/discovery", Group: GroupProgram, Order: 30,
			Requires: []Requirement{wc, {Connector: connector.IDOpenAI}, {Hub: IDTimer, Optional: true}}},
		{ID: IDSystem, Name: "SystemHub", Label: "hub.system", Icon: "system", Path: "/system", Group: GroupDevice, Order: 10,
			Requires: []Requirement{wc}},
		{ID: IDAdmin, Name: "AdminHub", Label: "hub.admin", Icon: "admin", Path: "/admin", Group: GroupBottom, Order: 10,
			Role: store.RoleAdmin, Requires: []Requirement{{Capability: CapJournald}}},
	}
}

// DefaultCapabilities returns the device capabilities the v1 hubs check.
// lookPath is exec.LookPath in production and a fake in tests.
func DefaultCapabilities(lookPath func(string) (string, error)) []Capability {
	return []Capability{{
		ID:    CapJournald,
		Check: func() bool { _, err := lookPath("journalctl"); return err == nil },
		Problem: status.Problem{
			Code:   status.CodeCapabilityMissing,
			Params: map[string]any{"capability": "journalctl"},
			// On the Dreambox journald is always present (full systemd, see
			// ARCHITECTURE.md); this mostly shows up on development machines.
			Fix: "fix.capability.journald",
		},
	}}
}

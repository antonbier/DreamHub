package hub

import (
	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/events"
	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/status"
)

// IDs of the v1 hubs (CONCEPT.md, "Hubs").
const (
	IDMedia     = "media"
	IDTimer     = "timer"
	IDGuide     = "guide"
	IDDiscovery = "discovery"
	IDSeries    = "series"
	// IDDevice: SystemHub and AdminHub merged into one nav entry, "Gerät"
	// (docs/ADMINHUB_PLAN.md, 2026-10-11) — both were always about this
	// one device, both already needed the same `system` permission (so
	// SystemHub was never actually "for every user" despite CONCEPT.md's
	// earlier description; corrected there too), and the merge follows
	// the same one-entry-many-tabs pattern Settings already uses. Kept
	// deliberately distinct from "System"/"Settings" as a name.
	IDDevice   = "device"
	IDControl  = "control"
	IDChannels = "channels"
	IDRadio    = "radio"
	IDPlex     = "plex"
	IDStream   = "stream"
	// IDApps: AppsHub (docs/APPSHUB_PLAN.md, App1) — renamed and widened
	// from Extension Hub (docs/EXTENSIONHUB_PLAN.md, E1, #46): one nav
	// entry for every registered extension's own card (so installing five
	// extensions grows the navigation by zero entries, not five), plus,
	// as a second tab, Docker container management — a genuinely
	// different kind of feature that doesn't fit E1's widget contract,
	// so it gets its own tab rather than being forced into one.
	IDApps = "apps"
)

// CapJournald is the capability to read logs via journalctl (AdminHub).
const CapJournald = "journald"

// V1 returns the descriptors of the hubs planned for v1. ControlHub was
// originally scoped for v1.1 (CONCEPT.md) but pulled forward: verifying
// WP8's Enigma2 plugin needed a real screenshot/remote-control loop
// anyway (ARCHITECTURE.md, "Real screenshot and remote-control
// endpoints"), and GuideHub's "switch to this channel now" needs it too.
// RadioHub (v1.1) was built early too, against the mock while the box was
// out of reach (2026-10-04), and StreamHub ("later") once a spike showed
// the browser can decode what the box sends (2026-10-06); ChannelHub is
// added when it's built.
func V1() []Descriptor {
	wc := Requirement{Connector: connector.IDWebControl}
	return []Descriptor{
		{ID: IDMedia, Name: "MediaHub", Label: "hub.media", Icon: "recordings", Path: "/media", Group: GroupMedia, Order: 10,
			Requires: []Requirement{wc, {Connector: connector.IDTMDB, Optional: true}},
			Emits:    []string{events.DiskAboveLimit}},
		{ID: IDSeries, Name: "SeriesHub", Label: "hub.series", Icon: "series", Path: "/series", Group: GroupMedia, Order: 20,
			Requires: []Requirement{wc, {Connector: connector.IDSerienRecorder}},
			Emits:    []string{events.MissingAiring}},
		// Plex Mediathek: a Plex server's films and series. Built from the
		// cache while the server is off (CONCEPT.md, "Connectors").
		{ID: IDPlex, Name: "PlexHub", Label: "hub.plex", Icon: "library", Path: "/plex", Group: GroupMedia, Order: 30,
			Requires: []Requirement{{Connector: connector.IDPlex, Cached: true}, {Hub: IDControl, Optional: true}}},
		{ID: IDGuide, Name: "GuideHub", Label: "hub.guide", Icon: "guide", Path: "/guide", Group: GroupProgram, Order: 10,
			Requires: []Requirement{wc, {Connector: connector.IDTMDB, Optional: true}}},
		// Recording a radio programme needs TimerHub; listening doesn't.
		{ID: IDRadio, Name: "RadioHub", Label: "hub.radio", Icon: "radio", Path: "/radio", Group: GroupProgram, Order: 15,
			Requires: []Requirement{wc, {Hub: IDTimer, Optional: true}}},
		// Live TV and recordings in the browser (built early, 2026-10-06).
		{ID: IDStream, Name: "StreamHub", Label: "hub.stream", Icon: "stream", Path: "/stream", Group: GroupProgram, Order: 12,
			Requires: []Requirement{wc}},
		{ID: IDTimer, Name: "TimerHub", Label: "hub.timer", Icon: "timers", Path: "/timers", Group: GroupProgram, Order: 20,
			Requires: []Requirement{wc}, Emits: []string{events.RecordingStarted, events.RecordingFinished, events.TimerConflict}},
		// Recommendations need an AI provider; creating a timer from one
		// needs TimerHub, so without it DiscoveryHub still works, but degraded.
		{ID: IDDiscovery, Name: "DiscoveryHub", Label: "hub.discovery", Icon: "recommendations", Path: "/discovery", Group: GroupProgram, Order: 30,
			Requires: []Requirement{wc, {Connector: connector.IDOpenAI}, {Hub: IDTimer, Optional: true}}},
		{ID: IDControl, Name: "ControlHub", Label: "hub.control", Icon: "control", Path: "/control", Group: GroupDevice, Order: 20,
			Requires: []Requirement{wc}},
		// The channel list (docs/CHANNELHUB_PLAN.md): browsing needs only
		// WebControl; editing goes through the optional BouquetEditor
		// plugin, which the hub detects itself.
		{ID: IDChannels, Name: "ChannelHub", Label: "hub.channels", Icon: "list", Path: "/channels", Group: GroupDevice, Order: 15,
			Perm: perm.ChannelsManage, Requires: []Requirement{wc}},
		// DeviceHub ("Gerät"): device status/diagnostics/change log plus
		// administration (packages, logs, services, terminal — #47) in
		// one nav entry. Both halves' own dependencies are optional here:
		// neither should take the whole hub down when the other's need
		// is missing (the admin logs/services tab never needed
		// WebControl; the device-info/change-log tab never needed
		// journald) — each missing dependency only degrades, the
		// specific reason still shows in the status banner.
		{ID: IDDevice, Name: "DeviceHub", Label: "hub.device", Icon: "admin", Path: "/device", Group: GroupBottom, Order: 10,
			Perm: perm.System, Requires: []Requirement{{Connector: connector.IDWebControl, Optional: true}, {Capability: CapJournald, Optional: true}},
			Emits: []string{events.ServiceFailed, events.ServiceRecovered, events.PackageUpgradable}},
		// AppsHub: no requirements of its own — an individual extension's
		// reachability is its own card's status, not this hub's
		// (extensions.go's list handler), so installing zero extensions
		// (or having no Docker apps installed) is "available", just
		// empty, never "unavailable". perm.Watch (the default): the nav
		// entry itself is for everyone; an admin-only extension's own
		// card is filtered out for a non-admin inside the handler
		// instead, and installing/managing a Docker app needs `system`
		// like any other AdminHub-style action (docs/APPSHUB_PLAN.md).
		{ID: IDApps, Name: "AppsHub", Label: "hub.apps", Icon: "extension", Path: "/apps", Group: GroupBottom, Order: 20,
			Emits: []string{events.AppUpgradable}},
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

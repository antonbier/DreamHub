package boxes

import (
	"context"

	"github.com/antonbier/DreamHub/backend/internal/connector"
)

// What a box can do (docs/MULTIDEVICE_PLAN.md, M2). The list is measured —
// plugins from the connector reports of that box, the machine-level
// features from whether DreamHub runs there — and never assumed from
// what the main box has.
const (
	CapWebControl     = "webcontrol"     // the box answers its web interface
	CapSerienRecorder = "serienrecorder" // the SerienRecorder plugin is installed
	CapAutoTimer      = "autotimer"      // the AutoTimer plugin is installed

	// Full-tier features: they need DreamHub on that machine.
	CapRecordingsLocal = "recordings.local" // recording files read and changed in place
	CapPackages        = "packages"
	CapServices        = "services"
	CapLogs            = "logs"
	CapTerminal        = "terminal"
	CapDocker          = "docker"
	CapExtensions      = "extensions"
)

// fullCapabilities are what DreamHub offers for the machine it runs on.
var fullCapabilities = []string{CapRecordingsLocal, CapPackages, CapServices, CapLogs, CapTerminal, CapDocker, CapExtensions}

// Capabilities lists what the box ctx is addressed to can do, from the
// connector reports of that box (Registry.Reports is already per box).
// A box the registry doesn't know has none.
func (r *Registry) Capabilities(ctx context.Context, reports map[string]connector.Report) []string {
	e, err := r.For(ctx)
	if err != nil {
		return []string{}
	}
	out := []string{}
	for _, c := range []struct{ cap, connector string }{
		{CapWebControl, connector.IDWebControl},
		{CapSerienRecorder, connector.IDSerienRecorder},
		{CapAutoTimer, connector.IDAutoTimer},
	} {
		if rep, ok := reports[c.connector]; ok && rep.State == connector.OK {
			out = append(out, c.cap)
		}
	}
	if e.Local {
		out = append(out, fullCapabilities...)
	}
	return out
}

// Full reports whether DreamHub runs on the box ctx is addressed to (the
// "Full" tier of the plan); every other box is Basic.
func (r *Registry) Full(ctx context.Context) bool {
	e, err := r.For(ctx)
	return err == nil && e.Local
}

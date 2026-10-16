// Package status holds the shared vocabulary for "is this working, and if
// not, why and how to fix it" — used by connectors and hubs alike
// (PRINCIPLES.md, "Hub status: always explain what's missing").
package status

// Level is the availability of a hub.
type Level string

const (
	Available   Level = "available"
	Degraded    Level = "degraded"
	Unavailable Level = "unavailable"
)

// Problem explains why something isn't fully available and how to fix it.
// Code and Fix are i18n keys translated by the client; Params fill them in.
// Never finished text.
type Problem struct {
	Code      string         `json:"code"`
	Params    map[string]any `json:"params,omitempty"`
	Fix       string         `json:"fix,omitempty"`
	FixParams map[string]any `json:"fixParams,omitempty"`
}

// Problem codes shared across the core.
const (
	CodeConnectorUnreachable     = "connector.unreachable"
	CodeConnectorNotConfigured   = "connector.notConfigured"
	CodeConnectorNotImplemented  = "connector.notImplemented"
	CodeConnectorUnknown         = "connector.unknown"
	CodeConnectorAuthRequired    = "connector.authRequired"
	CodeConnectorNotInstalled    = "connector.notInstalled"
	CodeCapabilityMissing        = "capability.missing"
	CodeHubDependencyUnavailable = "hub.dependencyUnavailable"
	// CodeBoxNotFull: the hub needs DreamHub running on the box itself.
	CodeBoxNotFull = "box.notFull"

	FixCheckBoxConnection  = "fix.checkBoxConnection"
	FixConfigureInSettings = "fix.configureInSettings"
	FixEnableHub           = "fix.enableHub"
	FixInstallPlugin       = "fix.installPlugin"
)

// Package extension is Extension Hub's contract (docs/EXTENSIONHUB_PLAN.md,
// E1, #46): how a separate, optional process (an extension) describes a
// small UI to DreamHub's core without shipping any frontend code of its
// own. An extension registers once (Registration, over a loopback-only
// handshake — see hubs.Extensions.Register) and from then on answers two
// calls the core makes of it (Client): "what do you look like right now"
// (Widgets) and "do this" (PlanAction/Apply, through the same plan →
// apply flow every other change in DreamHub goes through).
//
// Versioned additively: SchemaVersion only ever grows when a new,
// optional widget type or field is added; an extension written against
// an older version keeps working forever against a newer core, because it
// simply never sends the newer fields. An extension the running core
// doesn't understand yet (a SchemaVersion ahead of what's known here)
// doesn't break anything — it shows up unavailable with a reason, the
// same as any hub whose connector is missing (CONCEPT.md).
package extension

import (
	"context"

	"github.com/antonbier/DreamHub/backend/internal/status"
)

// SchemaVersion is the widget schema this core understands. Bump only when
// adding a new, optional widget type or field — see the package doc.
const SchemaVersion = 1

// WidgetType names one of the fixed shapes an extension can ask for
// (docs/EXTENSIONHUB_PLAN.md, "Extensions never ship frontend code").
type WidgetType string

const (
	WidgetStatus WidgetType = "status"
	WidgetMeter  WidgetType = "meter"
	WidgetList   WidgetType = "list"
	WidgetAction WidgetType = "action"
)

// Tone is a status widget's at-a-glance color.
type Tone string

const (
	ToneGood Tone = "good"
	ToneWarn Tone = "warn"
	ToneBad  Tone = "bad"
)

// Widget is one row of an extension's card. Exactly the fields its Type
// uses are set; the renderer ignores the rest, and a future WidgetType
// this core doesn't know yet is skipped rather than shown broken (the
// additive-versioning rule above).
type Widget struct {
	Type  WidgetType `json:"type"`
	Label string     `json:"label"`

	// status
	Value string `json:"value,omitempty"`
	Tone  Tone   `json:"tone,omitempty"`

	// meter
	Current float64 `json:"current,omitempty"`
	Max     float64 `json:"max,omitempty"`
	Unit    string  `json:"unit,omitempty"`

	// list
	Rows []ListRow `json:"rows,omitempty"`

	// action: a button that previews and confirms a change through
	// plan.Service, exactly like a built-in hub's own actions
	// (PlanPreview.vue). ActionID is passed back to PlanAction/Apply.
	ActionID    string `json:"actionId,omitempty"`
	ButtonLabel string `json:"buttonLabel,omitempty"`
}

// ListRow is one row of a "list" widget.
type ListRow struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Icon  string `json:"icon,omitempty"`
}

// Registration is what an extension declares about itself over the
// handshake (POST /api/v1/extensions/register, loopback only — see
// hubs.Extensions.Register).
type Registration struct {
	// ID is stable and unique across extensions, e.g. "ewelink".
	ID string `json:"id"`
	// Name is the extension's fixed name, shown next to its widgets —
	// an extension owns none of DreamHub's own locale files, so unlike a
	// built-in hub's Label this is literal text the extension provides
	// itself, not an i18n key.
	Name string `json:"name"`
	// Icon matches one of the frontend's known icon names
	// (frontend/src/components/Icon.vue); an unrecognized name falls back
	// to a generic extension icon there, same as an unset one.
	Icon string `json:"icon"`
	// Admin requires the system permission to see this extension's card —
	// the same binary choice a built-in hub's Perm makes, deliberately
	// not the full internal/perm vocabulary (an extension has no business
	// asking for e.g. channels.manage).
	Admin         bool `json:"admin"`
	SchemaVersion int  `json:"schemaVersion"`
}

// ActionPreview is what PlanAction returns: the same shape a built-in
// hub's own plan.Draft needs, just sourced from the extension's process
// instead of computed locally.
type ActionPreview struct {
	Summary     string           `json:"summary"`
	Warnings    []status.Problem `json:"warnings,omitempty"`
	Fingerprint string           `json:"fingerprint"`
}

// Client is how the core calls a registered extension. HTTPClient (in
// ./http_client.go) is the real implementation, reaching the extension's
// own process over localhost; ./mock is a demo one with no process at all.
type Client interface {
	// Widgets answers "what do you look like right now" — called fresh on
	// every GET /api/v1/extensions, no caching: an extension's own state
	// (a socket on/off, a disk filling up) can change at any time.
	Widgets(ctx context.Context) ([]Widget, error)
	// PlanAction previews one of the extension's own action widgets —
	// read-only, computes nothing yet. actionID is a Widget.ActionID the
	// extension itself handed out via Widgets.
	PlanAction(ctx context.Context, actionID string) (ActionPreview, error)
	// Apply performs the action PlanAction just previewed. fingerprint is
	// the one PlanAction returned; the extension itself decides whether
	// its state has since moved on and the apply should be refused as
	// stale, the same contract plan.Draft.Current/Execute already has
	// with every other hub.
	Apply(ctx context.Context, actionID, fingerprint string) error
}

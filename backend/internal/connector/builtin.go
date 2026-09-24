package connector

import (
	"context"

	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/status"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// IDs of the built-in connectors.
const (
	IDWebControl     = "webcontrol"
	IDSerienRecorder = "serienrecorder"
	IDTMDB           = "tmdb"
	IDOpenAI         = "openai"
)

// Setting keys owned by connectors.
const (
	KeyTMDBAPIKey   = "tmdb.apiKey"
	KeyAIProvider   = "ai.provider"
	KeyOpenAIAPIKey = "ai.openai.apiKey"
	KeyOpenAIModel  = "ai.openai.model"
	KeyAIDailyLimit = "ai.dailyLimit"
)

// --- WebControl -----------------------------------------------------------

// WebControl is the connector to the box's own `/web/*` API.
type WebControl struct{ Client webcontrol.Client }

func (WebControl) ID() string                   { return IDWebControl }
func (WebControl) Name() string                 { return "WebControl" }
func (WebControl) Settings() []settings.Section { return nil }
func (w WebControl) Check(ctx context.Context, _ settings.Values) Report {
	if _, err := w.Client.DeviceInfo(ctx); err != nil {
		return Report{State: Unreachable, Problem: &status.Problem{
			Code:   status.CodeConnectorUnreachable,
			Params: map[string]any{"connector": "WebControl"},
			Fix:    status.FixCheckBoxConnection,
		}}
	}
	return Report{State: OK}
}

// --- SerienRecorder -------------------------------------------------------

// SerienRecorder talks to SerienRecorder's own web interface API
// (internal/serienrecorder), confirmed reachable live 2026-09-24 (issue
// #10, ARCHITECTURE.md) — a real reachability check, the same shape as
// WebControl's, not the placeholder "not implemented" this used to be.
type SerienRecorder struct {
	Client serienrecorder.Client
	Mock   bool
}

func (SerienRecorder) ID() string                   { return IDSerienRecorder }
func (SerienRecorder) Name() string                 { return "SerienRecorder" }
func (SerienRecorder) Settings() []settings.Section { return nil }
func (s SerienRecorder) Check(ctx context.Context, _ settings.Values) Report {
	if s.Mock {
		return Report{State: OK}
	}
	if _, err := s.Client.Info(ctx); err != nil {
		return Report{State: Unreachable, Problem: &status.Problem{
			Code:   status.CodeConnectorUnreachable,
			Params: map[string]any{"connector": "SerienRecorder"},
			Fix:    status.FixCheckBoxConnection,
		}}
	}
	return Report{State: OK}
}

// --- TMDB -----------------------------------------------------------------

// TMDB provides movie/series metadata for enrichment. Optional for every
// hub that uses it.
type TMDB struct{}

func (TMDB) ID() string   { return IDTMDB }
func (TMDB) Name() string { return "TMDB" }
func (TMDB) Settings() []settings.Section {
	return []settings.Section{{
		ID:    "tmdb",
		Label: "settings.tmdb",
		Fields: []settings.Field{
			{Key: KeyTMDBAPIKey, Type: settings.TypeSecret, Scope: settings.ScopeGlobal, Label: "settings.tmdb.apiKey"},
		},
	}}
}
func (TMDB) Check(_ context.Context, v settings.Values) Report {
	if v.String(KeyTMDBAPIKey) == "" {
		return notConfigured("TMDB", "tmdb")
	}
	return Report{State: OK}
}

// --- OpenAI ---------------------------------------------------------------

// OpenAI is the first AI provider. The AI provider interface itself (used by
// DiscoveryHub) comes with DiscoveryHub; this connector owns the settings
// and reports whether AI is usable.
type OpenAI struct{}

func (OpenAI) ID() string   { return IDOpenAI }
func (OpenAI) Name() string { return "OpenAI" }
func (OpenAI) Settings() []settings.Section {
	return []settings.Section{{
		ID:    "ai",
		Label: "settings.ai",
		Fields: []settings.Field{
			{Key: KeyAIProvider, Type: settings.TypeEnum, Scope: settings.ScopeGlobal, Label: "settings.ai.provider", Default: "openai", Options: []string{"openai"}},
			{Key: KeyOpenAIAPIKey, Type: settings.TypeSecret, Scope: settings.ScopeGlobal, Label: "settings.ai.openai.apiKey"},
			// Empty means "the provider's default model".
			{Key: KeyOpenAIModel, Type: settings.TypeString, Scope: settings.ScopeGlobal, Label: "settings.ai.openai.model", Default: ""},
			// Cost guard: maximum AI requests per day.
			{Key: KeyAIDailyLimit, Type: settings.TypeInt, Scope: settings.ScopeGlobal, Label: "settings.ai.dailyLimit", Default: int64(200), Min: int64p(0), Max: int64p(10000)},
		},
	}}
}
func (OpenAI) Check(_ context.Context, v settings.Values) Report {
	if v.String(KeyOpenAIAPIKey) == "" {
		return notConfigured("OpenAI", "ai")
	}
	return Report{State: OK}
}

func notConfigured(name, section string) Report {
	return Report{State: NotConfigured, Problem: &status.Problem{
		Code:      status.CodeConnectorNotConfigured,
		Params:    map[string]any{"connector": name},
		Fix:       status.FixConfigureInSettings,
		FixParams: map[string]any{"section": section},
	}}
}

func int64p(v int64) *int64 { return &v }

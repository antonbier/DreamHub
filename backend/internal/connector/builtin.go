package connector

import (
	"context"
	"errors"
	"net/url"
	"regexp"

	"github.com/antonbier/DreamHub/backend/internal/autotimer"
	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/status"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// IDs of the built-in connectors.
const (
	IDAutoTimer      = "autotimer"
	IDWebControl     = "webcontrol"
	IDSerienRecorder = "serienrecorder"
	IDTMDB           = "tmdb"
	IDOpenAI         = "openai"
	IDTVmaze         = "tvmaze"
	IDOMDb           = "omdb"
	IDFanart         = "fanart"
	IDPush           = "push"
)

// Setting keys owned by connectors.
const (
	KeyTMDBAPIKey = "tmdb.apiKey"
	// KeyTMDBEnrich turns enrichment (posters, ratings, overviews for the
	// titles on screen, internal/enrich) on or off; with it off, TMDB is
	// only asked on an explicit rename suggestion.
	KeyTMDBEnrich = "tmdb.enrich"
	// KeyTVmazeEnabled switches the TVmaze connector (episode lists for
	// SeriesHub's "missing episodes"); it sends followed series' names to
	// TVmaze, so it can be turned off.
	KeyTVmazeEnabled = "tvmaze.enabled"
	// KeyOMDbAPIKey: OMDb's free key, for Rotten Tomatoes, IMDb and
	// Metacritic ratings; KeyFanartAPIKey: fanart.tv's, for title logos.
	// Both optional, both only asked for a title's detail view.
	KeyOMDbAPIKey   = "omdb.apiKey"
	KeyFanartAPIKey = "fanart.apiKey"
	// Push notifications through ntfy (internal/push): where to publish,
	// and which events go out.
	KeyPushServer            = "push.server"
	KeyPushTopic             = "push.topic"
	KeyPushToken             = "push.token"
	KeyPushRecordingStarted  = "push.events.recordingStarted"
	KeyPushRecordingFinished = "push.events.recordingFinished"
	KeyPushDisk              = "push.events.disk"
	KeyPushConflict          = "push.events.conflict"
	KeyPushMissingEpisode    = "push.events.missingEpisode"
	KeyPushService           = "push.events.service"
	KeyAIProvider            = "ai.provider"
	KeyOpenAIAPIKey          = "ai.openai.apiKey"
	KeyOpenAIModel           = "ai.openai.model"
	KeyAIDailyLimit          = "ai.dailyLimit"

	// Optional credentials for WebControl (and every plugin API behind it,
	// e.g. SerienRecorder's). Only needed when the box enforces auth for
	// DreamHub's requests — see ARCHITECTURE.md, "WebControl auth for a
	// backend on the box itself".
	KeyWebControlUsername = "webcontrol.username"
	KeyWebControlPassword = "webcontrol.password"
)

// --- WebControl -----------------------------------------------------------

// WebControl is the connector to the box's own `/web/*` API.
type WebControl struct{ Client webcontrol.Client }

func (WebControl) ID() string   { return IDWebControl }
func (WebControl) Name() string { return "WebControl" }
func (WebControl) Settings() []settings.Section {
	return []settings.Section{{
		ID:    "webcontrol",
		Label: "settings.webcontrol",
		Fields: []settings.Field{
			// The box's own login (usually root). Stored as a secret, not a
			// bcrypt hash: DreamHub has to send it, not verify it.
			{Key: KeyWebControlUsername, Type: settings.TypeString, Scope: settings.ScopeGlobal, Label: "settings.webcontrol.username", Default: ""},
			{Key: KeyWebControlPassword, Type: settings.TypeSecret, Scope: settings.ScopeGlobal, Label: "settings.webcontrol.password"},
		},
	}}
}
func (w WebControl) Check(ctx context.Context, _ settings.Values) Report {
	if _, err := w.Client.DeviceInfo(ctx); errors.Is(err, webcontrol.ErrUnauthorized) {
		return authRequired("WebControl")
	} else if err != nil {
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
	if _, err := s.Client.Info(ctx); errors.Is(err, serienrecorder.ErrNotInstalled) {
		return notInstalled("SerienRecorder")
	} else if errors.Is(err, serienrecorder.ErrUnauthorized) {
		return authRequired("SerienRecorder")
	} else if err != nil {
		return Report{State: Unreachable, Problem: &status.Problem{
			Code:   status.CodeConnectorUnreachable,
			Params: map[string]any{"connector": "SerienRecorder"},
			Fix:    status.FixCheckBoxConnection,
		}}
	}
	return Report{State: OK}
}

// --- AutoTimer ------------------------------------------------------------

// AutoTimer is the optional AutoTimer plugin's web API. No hub *requires*
// it: TimerHub just shows or hides its AutoTimer section, so a box without
// the plugin (a normal setup) doesn't get a permanent "degraded" warning.
// The state still shows up under Diagnostics/connectors.
type AutoTimer struct{ Client autotimer.Client }

func (AutoTimer) ID() string                   { return IDAutoTimer }
func (AutoTimer) Name() string                 { return "AutoTimer" }
func (AutoTimer) Settings() []settings.Section { return nil }
func (a AutoTimer) Check(ctx context.Context, _ settings.Values) Report {
	if a.Client == nil {
		return notInstalled("AutoTimer")
	}
	_, err := a.Client.List(ctx)
	switch {
	case errors.Is(err, autotimer.ErrNotInstalled):
		return notInstalled("AutoTimer")
	case errors.Is(err, autotimer.ErrUnauthorized):
		return authRequired("AutoTimer")
	case err != nil:
		return Report{State: Unreachable, Problem: &status.Problem{
			Code:   status.CodeConnectorUnreachable,
			Params: map[string]any{"connector": "AutoTimer"},
			Fix:    status.FixCheckBoxConnection,
		}}
	}
	return Report{State: OK}
}

func notInstalled(plugin string) Report {
	return Report{State: NotInstalled, Problem: &status.Problem{
		Code:      status.CodeConnectorNotInstalled,
		Params:    map[string]any{"connector": plugin},
		Fix:       status.FixInstallPlugin,
		FixParams: map[string]any{"plugin": plugin},
	}}
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
			{Key: KeyTMDBEnrich, Type: settings.TypeBool, Scope: settings.ScopeGlobal, Label: "settings.tmdb.enrich", Default: true},
		},
	}}
}
func (TMDB) Check(_ context.Context, v settings.Values) Report {
	if v.String(KeyTMDBAPIKey) == "" {
		return notConfigured("TMDB", "tmdb")
	}
	return Report{State: OK}
}

// --- TVmaze ---------------------------------------------------------------

// TVmaze provides complete episode lists (SeriesHub's missing episodes).
// Public API, no key: on unless switched off. Optional for every hub.
type TVmaze struct{}

func (TVmaze) ID() string   { return IDTVmaze }
func (TVmaze) Name() string { return "TVmaze" }
func (TVmaze) Settings() []settings.Section {
	return []settings.Section{{
		ID:    "tvmaze",
		Label: "settings.tvmaze",
		Fields: []settings.Field{
			{Key: KeyTVmazeEnabled, Type: settings.TypeBool, Scope: settings.ScopeGlobal, Label: "settings.tvmaze.enabled", Default: true},
		},
	}}
}
func (TVmaze) Check(_ context.Context, v settings.Values) Report {
	if !v.Bool(KeyTVmazeEnabled) {
		return notConfigured("TVmaze", "tvmaze")
	}
	return Report{State: OK}
}

// --- OMDb -----------------------------------------------------------------

// OMDb provides ratings TMDB doesn't have: Rotten Tomatoes, IMDb,
// Metacritic. Optional; needs a free key.
type OMDb struct{}

func (OMDb) ID() string   { return IDOMDb }
func (OMDb) Name() string { return "OMDb" }
func (OMDb) Settings() []settings.Section {
	return []settings.Section{{
		ID:     "omdb",
		Label:  "settings.omdb",
		Fields: []settings.Field{{Key: KeyOMDbAPIKey, Type: settings.TypeSecret, Scope: settings.ScopeGlobal, Label: "settings.omdb.apiKey"}},
	}}
}
func (OMDb) Check(_ context.Context, v settings.Values) Report {
	if v.String(KeyOMDbAPIKey) == "" {
		return notConfigured("OMDb", "omdb")
	}
	return Report{State: OK}
}

// --- fanart.tv ------------------------------------------------------------

// Fanart provides title logos. Optional; needs a free personal key.
type Fanart struct{}

func (Fanart) ID() string   { return IDFanart }
func (Fanart) Name() string { return "fanart.tv" }
func (Fanart) Settings() []settings.Section {
	return []settings.Section{{
		ID:     "fanart",
		Label:  "settings.fanart",
		Fields: []settings.Field{{Key: KeyFanartAPIKey, Type: settings.TypeSecret, Scope: settings.ScopeGlobal, Label: "settings.fanart.apiKey"}},
	}}
}
func (Fanart) Check(_ context.Context, v settings.Values) Report {
	if v.String(KeyFanartAPIKey) == "" {
		return notConfigured("fanart.tv", "fanart")
	}
	return Report{State: OK}
}

// --- Push (ntfy) -------------------------------------------------------------

// Push sends notifications to the phone through ntfy. Optional: without a
// topic nothing is sent.
type Push struct{}

func (Push) ID() string   { return IDPush }
func (Push) Name() string { return "ntfy" }
func (Push) Settings() []settings.Section {
	return []settings.Section{{
		ID:    "push",
		Label: "settings.push",
		Tab:   settings.TabNotifications,
		Fields: []settings.Field{
			{Key: KeyPushServer, Type: settings.TypeString, Scope: settings.ScopeGlobal, Label: "settings.push.server", Default: "https://ntfy.sh"},
			{Key: KeyPushTopic, Type: settings.TypeString, Scope: settings.ScopeGlobal, Label: "settings.push.topic", Default: ""},
			{Key: KeyPushToken, Type: settings.TypeSecret, Scope: settings.ScopeGlobal, Label: "settings.push.token"},
			{Key: KeyPushRecordingFinished, Type: settings.TypeBool, Scope: settings.ScopeGlobal, Label: "settings.push.recordingFinished", Default: true},
			{Key: KeyPushRecordingStarted, Type: settings.TypeBool, Scope: settings.ScopeGlobal, Label: "settings.push.recordingStarted", Default: false},
			{Key: KeyPushMissingEpisode, Type: settings.TypeBool, Scope: settings.ScopeGlobal, Label: "settings.push.missingEpisode", Default: true},
			{Key: KeyPushConflict, Type: settings.TypeBool, Scope: settings.ScopeGlobal, Label: "settings.push.conflict", Default: true},
			{Key: KeyPushDisk, Type: settings.TypeBool, Scope: settings.ScopeGlobal, Label: "settings.push.disk", Default: true},
			{Key: KeyPushService, Type: settings.TypeBool, Scope: settings.ScopeGlobal, Label: "settings.push.service", Default: true},
		},
		Check: func(v settings.Values) map[string]settings.FieldError {
			errs := map[string]settings.FieldError{}
			if u, err := url.Parse(v.String(KeyPushServer)); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
				errs[KeyPushServer] = settings.FieldError{Code: "push_server"}
			}
			if t := v.String(KeyPushTopic); t != "" && !pushTopic.MatchString(t) {
				errs[KeyPushTopic] = settings.FieldError{Code: "push_topic"}
			}
			if len(errs) == 0 {
				return nil
			}
			return errs
		},
	}}
}
func (Push) Check(_ context.Context, v settings.Values) Report {
	if v.String(KeyPushTopic) == "" {
		return notConfigured("ntfy", "push")
	}
	return Report{State: OK}
}

// pushTopic is ntfy's topic syntax.
var pushTopic = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

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

// authRequired means the box asked for credentials DreamHub doesn't have or
// that are wrong.
func authRequired(name string) Report {
	return Report{State: NotConfigured, Problem: &status.Problem{
		Code:      status.CodeConnectorAuthRequired,
		Params:    map[string]any{"connector": name},
		Fix:       status.FixConfigureInSettings,
		FixParams: map[string]any{"section": "webcontrol"},
	}}
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

// Package app wires DreamHub together: store, settings schema, connectors,
// hubs, auth and the API router. Both cmd/dreamhub and the API tests use it,
// so tests exercise exactly the production wiring.
package app

import (
	"context"
	"github.com/antonbier/DreamHub/backend/internal/omdb"
	"github.com/antonbier/DreamHub/backend/internal/push"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/tmdb"
	"github.com/antonbier/DreamHub/backend/internal/tvmaze"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/ai"
	"github.com/antonbier/DreamHub/backend/internal/api"
	"github.com/antonbier/DreamHub/backend/internal/apitoken"
	"github.com/antonbier/DreamHub/backend/internal/auth"
	"github.com/antonbier/DreamHub/backend/internal/autotimer"
	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/dashboard"
	"github.com/antonbier/DreamHub/backend/internal/dmi"
	"github.com/antonbier/DreamHub/backend/internal/enrich"
	"github.com/antonbier/DreamHub/backend/internal/events"
	"github.com/antonbier/DreamHub/backend/internal/fanart"
	"github.com/antonbier/DreamHub/backend/internal/hub"
	hubs_ "github.com/antonbier/DreamHub/backend/internal/hubs"
	"github.com/antonbier/DreamHub/backend/internal/journald"
	"github.com/antonbier/DreamHub/backend/internal/logging"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/systemd"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Options configure the application.
type Options struct {
	DataDir    string
	WebControl webcontrol.Client
	// Series is the SerienRecorder API client (internal/serienrecorder),
	// used by SeriesHub and the SerienRecorder connector's reachability
	// check. Required whenever Mock is false.
	Series serienrecorder.Client
	// AI is the AI provider's client (internal/ai): DiscoveryHub's
	// recommendations and MediaHub's name suggestions.
	AI ai.Client
	// TMDB is the TMDB client (internal/tmdb), for name suggestions; nil
	// means TMDB suggestions report "not configured".
	TMDB tmdb.Client
	// TVmaze is the TVmaze client (internal/tvmaze), for SeriesHub's
	// episode lists; nil leaves them out.
	TVmaze tvmaze.Client
	// OMDb and Fanart (optional) add ratings and logos to a title's
	// detail view (internal/enrich, extras.go).
	// Push sends notifications to the phone (internal/push); nil turns
	// them off.
	Push push.Client
	// PiconDirs are where channel logos are looked for (GuideHub); nil
	// means the box's usual places.
	PiconDirs []string
	OMDb      omdb.Client
	Fanart    fanart.Client
	// DMI is the optional DreamMediaInfo plugin's database (read-only), the
	// first enrichment source; nil in mock mode.
	DMI *dmi.Reader
	// RecordingsFS is where MediaHub's file operations act: recfs.OS{} on
	// the box, the mock's in-memory FS in mock mode, nil to disable them.
	RecordingsFS recfs.FS
	// ReadOnlyFiles refuses to apply MediaHub file changes (see
	// hubs.Media.ReadOnly).
	ReadOnlyFiles bool
	// AutoTimer is the optional AutoTimer plugin's API (internal/autotimer);
	// nil behaves like a box without the plugin.
	AutoTimer autotimer.Client
	// Logs is AdminHub's journal reader (internal/journald): journald.Exec{}
	// on the box, the mock in mock mode.
	Logs journald.Reader
	// Systemd is AdminHub's service status (internal/systemd): Exec{} on
	// the box, the in-memory mock otherwise; nil hides the section.
	Systemd systemd.Manager
	// Mock is true when running against the mock WebControl/Series clients;
	// the SerienRecorder connector then reports OK without a real check.
	Mock bool
	// LookPath finds binaries for capability checks (exec.LookPath).
	LookPath func(string) (string, error)
	Logger   *slog.Logger
	// Now overrides the clock (tests); nil means time.Now.
	Now func() time.Time
	// ConnectorTTL is how long connector checks are cached; 0 means 30s.
	ConnectorTTL time.Duration
	// Level is the runtime log level shared with the logger; nil creates one.
	Level *logging.Level
	// WebUI serves the embedded frontend (internal/webui) for every path
	// the API router doesn't claim. Nil (the default, and what the tests
	// use) means the handler answers /api/v1/* only, exactly as before the
	// frontend existed.
	WebUI http.Handler
}

// App is a wired DreamHub instance.
type App struct {
	Handler    http.Handler
	Store      *store.Store
	Settings   *settings.Service
	Connectors *connector.Registry
	Hubs       *hub.Registry
	Plans      *plan.Service
	Dashboard  *dashboard.Service
	Tokens     *apitoken.Service
	Level      *logging.Level
	User       store.User
	// Events is the core event bus (internal/events, docs/V1_PLAN.md WP7).
	// Poller isn't started here — cmd/dreamhub runs it as long as the
	// process does; tests just use the Bus directly, no background polling.
	Events *events.Bus
	Poller *events.Poller
	// Background are further long-running jobs cmd/dreamhub starts next to
	// the Poller: push notifications, SeriesHub's missing-episode watch.
	Background []func(context.Context)

	credMu      sync.Mutex
	credLoaded  bool
	credVersion uint64
	credUser    string
	credPass    string
}

// BoxCredentials returns the WebControl login configured in settings
// (connector "webcontrol"), or empty strings. Cached per settings version,
// since the WebControl/SerienRecorder clients ask on every request.
func (a *App) BoxCredentials() (user, password string) {
	version := a.Settings.Version()
	a.credMu.Lock()
	defer a.credMu.Unlock()
	if !a.credLoaded || a.credVersion != version {
		v, err := a.Settings.Values(context.Background(), a.User)
		if err != nil {
			return a.credUser, a.credPass // keep the last known login
		}
		a.credUser, a.credPass = v.String(connector.KeyWebControlUsername), v.String(connector.KeyWebControlPassword)
		a.credLoaded, a.credVersion = true, version
	}
	return a.credUser, a.credPass
}

// New builds the application. Close the returned App's Store when done.
func New(ctx context.Context, o Options) (*App, error) {
	if o.ConnectorTTL == 0 {
		o.ConnectorTTL = 30 * time.Second
	}

	st, err := store.Open(o.DataDir)
	if err != nil {
		return nil, err
	}
	user, err := st.EnsureDefaultUser(ctx)
	if err != nil {
		st.Close()
		return nil, err
	}

	conns := connector.NewRegistry(o.ConnectorTTL)
	conns.Add(connector.WebControl{Client: o.WebControl})
	conns.Add(connector.SerienRecorder{Client: o.Series, Mock: o.Mock})
	conns.Add(connector.AutoTimer{Client: o.AutoTimer})
	conns.Add(connector.OpenAI{})
	conns.Add(connector.TMDB{})
	conns.Add(connector.TVmaze{})
	conns.Add(connector.OMDb{})
	conns.Add(connector.Fanart{})
	conns.Add(connector.Push{})

	hubs := hub.NewRegistry()
	for _, d := range hub.V1() {
		if err := hubs.Add(d); err != nil {
			st.Close()
			return nil, err
		}
	}
	for _, c := range hub.DefaultCapabilities(o.LookPath) {
		hubs.AddCapability(c)
	}

	// Settings schema: core first, then each connector's own sections, then
	// the per-hub enable switches.
	sections := settings.CoreSections()
	for _, c := range conns.All() {
		sections = append(sections, c.Settings()...)
	}
	// Hubs' own sections, then the per-hub enable switches.
	sections = append(sections, hubs_.MediaSettingsSection())
	sections = append(sections, hubs.SettingsSection())
	reg := settings.NewRegistry()
	for _, sec := range sections {
		if err := reg.Register(sec); err != nil {
			st.Close()
			return nil, err
		}
	}
	svc, err := settings.NewService(ctx, st, reg)
	if err != nil {
		st.Close()
		return nil, err
	}

	now := o.Now
	if now == nil {
		now = time.Now
	}
	level := o.Level
	if level == nil {
		level = logging.NewLevel(now)
	}
	plans := plan.NewService(st, now)
	dash := dashboard.NewService(st)
	tokens := apitoken.NewService(st, now)
	watcher := hub.NewStatusWatcher()
	eventBus := events.NewBus(st, now)
	poller := &events.Poller{WC: o.WebControl, Bus: eventBus, Logger: o.Logger, Now: now}

	var enricher *enrich.Service
	if o.TMDB != nil {
		enricher = &enrich.Service{
			DMI: o.DMI, TMDB: o.TMDB, OMDb: o.OMDb, Fanart: o.Fanart, Store: st, Now: now,
			Images: &enrich.ImageCache{Dir: filepath.Join(o.DataDir, "cache", "tmdb"), MaxBytes: enrich.DefaultImageCacheBytes},
		}
	}

	timerHub := &hubs_.Timer{WC: o.WebControl, Plans: plans, Now: now, AT: o.AutoTimer}
	mediaHub := &hubs_.Media{WC: o.WebControl, Plans: plans, FS: o.RecordingsFS, ReadOnly: o.ReadOnlyFiles,
		Settings: svc, Store: st, AI: o.AI, TMDB: o.TMDB, Enrich: enricher, Now: now}
	guideHub := &hubs_.Guide{WC: o.WebControl, Now: now, PiconDirs: o.PiconDirs}
	radioHub := &hubs_.Radio{WC: o.WebControl, Now: now, PiconDirs: o.PiconDirs}
	seriesHub := &hubs_.Series{SR: o.Series, WC: o.WebControl, FS: o.RecordingsFS, TVmaze: o.TVmaze, Store: st, Settings: svc, DMI: o.DMI, Now: now}
	discoveryHub := &hubs_.Discovery{WC: o.WebControl, SR: o.Series, AT: o.AutoTimer, AI: o.AI, Store: st, Settings: svc, Now: now}
	systemHub := &hubs_.System{Store: st, Settings: svc, Dashboard: dash, Plans: plans, Now: now,
		SelfTest: &hubs_.SelfTest{WC: o.WebControl, SR: o.Series, AT: o.AutoTimer, DMI: o.DMI, Systemd: o.Systemd, FS: o.RecordingsFS,
			PiconDirs: o.PiconDirs, Dir: filepath.Join(o.DataDir, "selftest"), Writes: !o.ReadOnlyFiles, Now: now}}
	adminHub := &hubs_.Admin{Store: st, Level: level, Logs: o.Logs, FS: o.RecordingsFS,
		Systemd: o.Systemd, Plans: plans, WC: o.WebControl, Now: now, ReadOnly: o.ReadOnlyFiles}
	controlHub := &hubs_.Control{WC: o.WebControl}

	// TimerHub is the hub contract's first real search provider
	// (internal/hub/search.go) — add more here as other hubs implement it.
	hubs.AddSearchProvider(hub.IDTimer, timerHub)
	hubs.AddSearchProvider(hub.IDMedia, mediaHub)
	hubs.AddSearchProvider(hub.IDSeries, seriesHub)
	hubs.AddSearchProvider(hub.IDGuide, guideHub)
	hubs.AddSearchProvider(hub.IDRadio, radioHub)
	// View sources for custom dashboard tiles (internal/hub/view.go).
	hubs.AddViewProvider(hub.IDMedia, mediaHub)
	hubs.AddViewProvider(hub.IDTimer, timerHub)
	hubs.AddViewProvider(hub.IDGuide, guideHub)
	hubs.AddViewProvider(hub.IDSeries, seriesHub)
	dash.KnownSource = hubs.KnownSource

	var notifier *push.Notifier
	if o.Push != nil {
		notifier = &push.Notifier{Bus: eventBus, Settings: svc, User: user, Client: o.Push, Logger: o.Logger}
	}

	handler := api.NewRouter(api.Deps{
		WebControl: o.WebControl,
		Verifier:   auth.NewSettingsVerifier(svc, tokens),
		Settings:   svc,
		Connectors: conns,
		Hubs:       hubs,
		Plans:      plans,
		Dashboard:  dash,
		Tokens:     tokens,
		Watcher:    watcher,
		Store:      st,
		Events:     eventBus,
		Enrich:     enricher,
		Push:       notifier,
		Mounts: []api.Mount{
			{HubID: hub.IDTimer, Prefixes: []string{"/api/v1/timers"}, Register: timerHub.Mount},
			{HubID: hub.IDMedia, Prefixes: []string{"/api/v1/media"}, Register: mediaHub.Mount},
			{HubID: hub.IDGuide, Prefixes: []string{"/api/v1/guide"}, Register: guideHub.Mount},
			{HubID: hub.IDRadio, Prefixes: []string{"/api/v1/radio"}, Register: radioHub.Mount},
			{HubID: hub.IDSeries, Prefixes: []string{"/api/v1/series"}, Register: seriesHub.Mount},
			{HubID: hub.IDDiscovery, Prefixes: []string{"/api/v1/discovery"}, Register: discoveryHub.Mount},
			{HubID: hub.IDSystem, Prefixes: []string{"/api/v1/changes", "/api/v1/system"}, Register: systemHub.Mount},
			{HubID: hub.IDAdmin, Prefixes: []string{"/api/v1/admin"}, Register: adminHub.Mount},
			{HubID: hub.IDControl, Prefixes: []string{"/api/v1/control"}, Register: controlHub.Mount},
		},
		User:   user,
		Logger: o.Logger,
		Now:    now,
	})
	if o.WebUI != nil {
		outer := http.NewServeMux()
		outer.Handle("/api/v1/", handler)
		outer.Handle("/", o.WebUI)
		handler = outer
	}
	return &App{
		Handler: handler, Store: st, Settings: svc, Connectors: conns, Hubs: hubs,
		Plans: plans, Dashboard: dash, Tokens: tokens, Level: level, User: user,
		Events: eventBus, Poller: poller,
		Background: background(notifier, seriesHub, eventBus, o.Logger),
	}, nil
}

// background lists the long-running jobs besides the poller.
func background(n *push.Notifier, series *hubs_.Series, bus *events.Bus, logger *slog.Logger) []func(context.Context) {
	jobs := []func(context.Context){func(ctx context.Context) { series.WatchMissing(ctx, bus, logger) }}
	if n != nil {
		jobs = append(jobs, n.Run)
	}
	return jobs
}

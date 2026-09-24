// Package app wires DreamHub together: store, settings schema, connectors,
// hubs, auth and the API router. Both cmd/dreamhub and the API tests use it,
// so tests exercise exactly the production wiring.
package app

import (
	"context"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/ai"
	"github.com/antonbier/DreamHub/backend/internal/api"
	"github.com/antonbier/DreamHub/backend/internal/apitoken"
	"github.com/antonbier/DreamHub/backend/internal/auth"
	"github.com/antonbier/DreamHub/backend/internal/autotimer"
	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/dashboard"
	"github.com/antonbier/DreamHub/backend/internal/hub"
	hubs_ "github.com/antonbier/DreamHub/backend/internal/hubs"
	"github.com/antonbier/DreamHub/backend/internal/logging"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
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
	// AI is DiscoveryHub's recommendation client (internal/ai).
	AI ai.Client
	// RecordingsFS is where MediaHub's file operations act: recfs.OS{} on
	// the box, the mock's in-memory FS in mock mode, nil to disable them.
	RecordingsFS recfs.FS
	// AutoTimer is the optional AutoTimer plugin's API (internal/autotimer);
	// nil behaves like a box without the plugin.
	AutoTimer autotimer.Client
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

	timerHub := &hubs_.Timer{WC: o.WebControl, Plans: plans, Now: now, AT: o.AutoTimer}
	mediaHub := &hubs_.Media{WC: o.WebControl, Plans: plans, FS: o.RecordingsFS}
	guideHub := &hubs_.Guide{WC: o.WebControl, Now: now}
	seriesHub := &hubs_.Series{SR: o.Series}
	discoveryHub := &hubs_.Discovery{WC: o.WebControl, AI: o.AI, Store: st, Settings: svc, Now: now}
	systemHub := &hubs_.System{Store: st}
	adminHub := &hubs_.Admin{Store: st, Level: level}

	// TimerHub is the hub contract's first real search provider
	// (internal/hub/search.go) — add more here as other hubs implement it.
	hubs.AddSearchProvider(hub.IDTimer, timerHub)
	hubs.AddSearchProvider(hub.IDMedia, mediaHub)
	hubs.AddSearchProvider(hub.IDSeries, seriesHub)
	hubs.AddSearchProvider(hub.IDGuide, guideHub)

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
		Mounts: []api.Mount{
			{HubID: hub.IDTimer, Prefixes: []string{"/api/v1/timers"}, Register: timerHub.Mount},
			{HubID: hub.IDMedia, Prefixes: []string{"/api/v1/media"}, Register: mediaHub.Mount},
			{HubID: hub.IDGuide, Prefixes: []string{"/api/v1/guide"}, Register: guideHub.Mount},
			{HubID: hub.IDSeries, Prefixes: []string{"/api/v1/series"}, Register: seriesHub.Mount},
			{HubID: hub.IDDiscovery, Prefixes: []string{"/api/v1/discovery"}, Register: discoveryHub.Mount},
			{HubID: hub.IDSystem, Prefixes: []string{"/api/v1/changes"}, Register: systemHub.Mount},
			{HubID: hub.IDAdmin, Prefixes: []string{"/api/v1/admin"}, Register: adminHub.Mount},
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
	}, nil
}

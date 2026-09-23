// Package app wires DreamHub together: store, settings schema, connectors,
// hubs, auth and the API router. Both cmd/dreamhub and the API tests use it,
// so tests exercise exactly the production wiring.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/api"
	"github.com/antonbier/DreamHub/backend/internal/apitoken"
	"github.com/antonbier/DreamHub/backend/internal/auth"
	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/dashboard"
	"github.com/antonbier/DreamHub/backend/internal/hub"
	hubs_ "github.com/antonbier/DreamHub/backend/internal/hubs"
	"github.com/antonbier/DreamHub/backend/internal/logging"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Options configure the application.
type Options struct {
	DataDir    string
	WebControl webcontrol.Client
	// Mock is true when running against the mock WebControl client; the
	// SerienRecorder connector then reports OK instead of "not implemented".
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
	conns.Add(connector.SerienRecorder{Mock: o.Mock})
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

	timerHub := &hubs_.Timer{WC: o.WebControl, Plans: plans, Now: now}
	systemHub := &hubs_.System{Store: st}
	adminHub := &hubs_.Admin{Store: st, Level: level}

	// TimerHub is the hub contract's first real search provider
	// (internal/hub/search.go) — add more here as other hubs implement it.
	hubs.AddSearchProvider(hub.IDTimer, timerHub)

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

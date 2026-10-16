// Package app wires DreamHub together: store, settings schema, connectors,
// hubs, auth and the API router. Both cmd/dreamhub and the API tests use it,
// so tests exercise exactly the production wiring.
package app

import (
	"context"
	"encoding/json"
	"github.com/antonbier/DreamHub/backend/internal/dockersetup"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"errors"
	"github.com/antonbier/DreamHub/backend/internal/apt"
	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/boxes"
	"github.com/antonbier/DreamHub/backend/internal/extension"
	extmock "github.com/antonbier/DreamHub/backend/internal/extension/mock"
	"github.com/antonbier/DreamHub/backend/internal/omdb"
	"github.com/antonbier/DreamHub/backend/internal/plex"
	"github.com/antonbier/DreamHub/backend/internal/profile"
	"github.com/antonbier/DreamHub/backend/internal/push"
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/tmdb"
	"github.com/antonbier/DreamHub/backend/internal/trim"
	"github.com/antonbier/DreamHub/backend/internal/tvmaze"

	"github.com/antonbier/DreamHub/backend/internal/adscan"
	"github.com/antonbier/DreamHub/backend/internal/ai"
	"github.com/antonbier/DreamHub/backend/internal/api"
	"github.com/antonbier/DreamHub/backend/internal/apitoken"
	"github.com/antonbier/DreamHub/backend/internal/auth"
	"github.com/antonbier/DreamHub/backend/internal/autotimer"
	"github.com/antonbier/DreamHub/backend/internal/configfs"
	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/dashboard"
	"github.com/antonbier/DreamHub/backend/internal/dmi"
	"github.com/antonbier/DreamHub/backend/internal/dockerd"
	"github.com/antonbier/DreamHub/backend/internal/dockerhub"
	"github.com/antonbier/DreamHub/backend/internal/enrich"
	"github.com/antonbier/DreamHub/backend/internal/events"
	"github.com/antonbier/DreamHub/backend/internal/fanart"
	"github.com/antonbier/DreamHub/backend/internal/hub"
	hubs_ "github.com/antonbier/DreamHub/backend/internal/hubs"
	"github.com/antonbier/DreamHub/backend/internal/jobs"
	"github.com/antonbier/DreamHub/backend/internal/journald"
	"github.com/antonbier/DreamHub/backend/internal/logging"
	"github.com/antonbier/DreamHub/backend/internal/logosearch"
	"github.com/antonbier/DreamHub/backend/internal/meminfo"
	"github.com/antonbier/DreamHub/backend/internal/peer"
	"github.com/antonbier/DreamHub/backend/internal/plan"
	"github.com/antonbier/DreamHub/backend/internal/scripts"
	"github.com/antonbier/DreamHub/backend/internal/serienrecorder"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/systemd"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Options configure the application.
type Options struct {
	DataDir string
	// WebControl, Series and AutoTimer are the first box's connections
	// (the box with ID box.DefaultID, the one DreamHub runs on or was
	// pointed at by the configuration). Further boxes, saved in the
	// store, get theirs from BoxFactory.
	WebControl webcontrol.Client
	// BoxFactory builds the connections of a further box from its saved
	// row (address and login); nil means DreamHub knows only its first
	// box and ignores saved rows of others.
	BoxFactory func(store.Box) (boxes.Clients, error)
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
	// Plex is the Plex client (internal/plex) for PlexHub; nil turns the
	// connector into "not configured" for good.
	Plex plex.Client
	// StreamBase is how the box reaches this DreamHub ("http://127.0.0.1:
	// 8080"), set only when DreamHub runs on the box; PlexHub plays films
	// there through it.
	StreamBase string
	// StreamPort is the port the other boxes reach this DreamHub on.
	StreamPort string
	// Peer pairs with the DreamHub on other boxes (internal/peer); nil
	// turns pairing off.
	Peer peer.Client
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
	// Scripts runs AdminHub's uploaded scripts (internal/scripts): Exec{}
	// on the box, the in-memory mock otherwise; nil hides the library.
	Scripts scripts.Runner
	// Apt lists AdminHub's packages (internal/apt): Exec{} on the box, the
	// in-memory mock otherwise; nil hides the package browser.
	Apt apt.Lister
	// DockerHub is AppsHub's free-search and arch-check connector
	// (internal/dockerhub): NewHTTPClient() on the box, the deterministic
	// mock otherwise; nil disables free search and the manifest check
	// (the curated catalog itself still works, it needs no live call).
	DockerHub dockerhub.Client
	// LogoSearch finds a channel's logo online for ChannelHub (internal/logosearch);
	// nil hides that.
	LogoSearch logosearch.Source
	// Docker reaches AppsHub's isolated Docker helper
	// (internal/dockerd.HTTPClient on the box, the in-memory mock
	// otherwise): nil disables install/remove/start/stop and the
	// installed list.
	Docker dockerd.Client
	// Mem reads free memory for AppsHub's install-time resource budget
	// check (internal/meminfo); nil refuses every install with a clear
	// reason.
	Mem meminfo.Reader
	// AppsStorageRoot is where an installed app's own config directory
	// lives, one subdirectory per app id — the box's recording HDD by
	// default ("/media/hdd/dreamhub-apps"); nil/"" still works, it just
	// means a path under this empty root, fine for mock mode and tests.
	AppsStorageRoot string
	// AppsMountPolicy decides which host paths a container may mount
	// (AppsHub's install plan applies it before the helper does); nil means
	// /media only, with DataDir protected.
	AppsMountPolicy *dockerd.MountPolicy
	// DockerSetup sets Docker up on the box from AppsHub
	// (internal/dockersetup): Exec{} on the box, ./mock otherwise; nil
	// hides "Set up Docker".
	DockerSetup dockersetup.Setup
	// AppsConfigFS backs the apps backup section's directory
	// reads/writes (App6, internal/configfs); nil means configfs.OS, the
	// real file system — tests inject configfs/mock instead.
	AppsConfigFS configfs.FS
	// BackupDir is where "Save on the box" keeps backup files (App15); ""
	// leaves it out.
	BackupDir string
	// AppsSettle is how long an updated container must keep running before
	// the update counts as good (App14); zero is one minute, two seconds in
	// mock mode.
	AppsSettle time.Duration
	// AppsLocation is the zone the update night window is read in (tests);
	// nil is the box's own.
	AppsLocation *time.Location
	// TerminalTarget overrides shellinaboxd's address (tests only); empty
	// means the box's real "http://127.0.0.1:4200".
	TerminalTarget string
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
	Profiles   *profile.Service
	// Boxes are the registered boxes (docs/MULTIDEVICE_PLAN.md, M1).
	Boxes *boxes.Registry
	// BoxManager adds, changes and removes boxes (tests reach its hooks).
	BoxManager *boxes.Manager
	// Routes are the protected API routes with their permissions.
	Routes []api.Route
	Level  *logging.Level
	User   store.User
	// Events is the core event bus (internal/events, docs/archive/V1_PLAN.md WP7).
	// Poller isn't started here — cmd/dreamhub runs it as long as the
	// process does; tests just use the Bus directly, no background polling.
	Events *events.Bus
	// Poller runs one poller per box.
	Poller *events.Fleet
	// Jobs is the background queue (internal/jobs); hubs register their
	// job kinds on it, cmd/dreamhub runs it with Background.
	Jobs *jobs.Service
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

	// The registry of boxes: the first one from the options, the others
	// from the store. Everything below uses the registry's clients, which
	// forward each call to the box the request's context names.
	boxReg := boxes.NewRegistry()
	saved, err := st.Boxes(ctx)
	if err != nil {
		st.Close()
		return nil, err
	}
	for _, b := range saved {
		switch {
		case b.ID == box.DefaultID:
			boxReg.Add(b, boxes.Clients{WC: o.WebControl, Series: o.Series, AutoTimer: o.AutoTimer}, true)
		case o.BoxFactory != nil:
			c, err := o.BoxFactory(b)
			if err != nil {
				if o.Logger != nil {
					o.Logger.Warn("box ignored: connecting failed", "box", b.ID, "error", err)
				}
				continue
			}
			boxReg.Add(b, c, false)
		}
	}
	if boxReg.DefaultID() == "" { // the first box was removed or never seeded
		st.Close()
		return nil, errors.New("app: no box registered")
	}
	boxReg.SetStreamBase(o.StreamBase, o.StreamPort)
	wc, sr, at := webcontrol.Client(boxReg.WebControl()), boxReg.Series(), boxReg.AutoTimer()

	conns := connector.NewRegistry(o.ConnectorTTL)
	conns.Add(connector.WebControl{Client: wc})
	conns.Add(connector.SerienRecorder{Client: sr, Mock: o.Mock})
	conns.Add(connector.AutoTimer{Client: at})
	conns.Add(connector.OpenAI{})
	conns.Add(connector.TMDB{})
	conns.Add(connector.TVmaze{})
	conns.Add(connector.OMDb{})
	conns.Add(connector.Fanart{})
	conns.Add(connector.Push{})
	conns.Add(connector.Plex{Client: o.Plex})

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
	sections = append(sections, hubs_.ChannelsSettingsSection())
	sections = append(sections, hubs_.AppsSettingsSection())
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
	plans.Logger = o.Logger
	dash := dashboard.NewService(st)
	tokens := apitoken.NewService(st, now)
	profiles := profile.NewService(st, now)
	watcher := hub.NewStatusWatcher()
	eventBus := events.NewBus(st, now)
	// The background queue (#41) holds while the box records: the tuner,
	// the disk and enigma2 belong to the recording.
	jobQueue := &jobs.Service{Store: st, Logger: o.Logger, Now: now,
		Blockers: []jobs.Blocker{recordingBlocker(wc)}}
	poller := &events.Fleet{IDs: boxReg.IDs, New: func(id string) *events.Poller {
		return &events.Poller{WC: wc, Bus: eventBus, Logger: o.Logger, Now: now, BoxID: id,
			Tag:     func() bool { return len(boxReg.List()) > 1 },
			OnReach: func(online bool) { boxReg.SetOnline(id, online) }}
	}}

	var enricher *enrich.Service
	if o.TMDB != nil {
		enricher = &enrich.Service{
			DMI: o.DMI, TMDB: o.TMDB, OMDb: o.OMDb, Fanart: o.Fanart, Store: st, Now: now,
			Images: &enrich.ImageCache{Dir: filepath.Join(o.DataDir, "cache", "tmdb"), MaxBytes: enrich.DefaultImageCacheBytes},
		}
	}

	channelsHub := &hubs_.Channels{WC: wc, Plans: plans, Store: st, Settings: svc, User: user, PiconDirs: o.PiconDirs, LogoDir: filepath.Join(o.DataDir, "logos"), Logos: o.LogoSearch, Now: now}
	channelsHub.Boxes = boxReg.IDs
	channelsHub.Init(ctx)
	timerHub := &hubs_.Timer{WC: wc, Plans: plans, Now: now, AT: at, Owners: st}
	mediaHub := &hubs_.Media{WC: wc, Plans: plans, FS: o.RecordingsFS, ReadOnly: o.ReadOnlyFiles,
		Settings: svc, Store: st, AI: o.AI, TMDB: o.TMDB, Enrich: enricher, Now: now,
		AdScan: adScanOpener(o), Trim: trimCutter(o)}
	mediaHub.RegisterJobs(jobQueue)
	// Built before GuideHub/DiscoveryHub: both flag a broadcast already in
	// the library (#32) through it.
	plexHub := &hubs_.Plex{Client: o.Plex, Settings: svc, Store: st, Plans: plans, WC: wc, User: user,
		Images:     &enrich.ImageCache{Dir: filepath.Join(o.DataDir, "cache", "plex"), MaxBytes: enrich.DefaultImageCacheBytes},
		StreamBase: o.StreamBase, StreamBaseFor: boxReg.StreamBase, Now: now, Logger: o.Logger}
	guideHub := &hubs_.Guide{WC: wc, Now: now, PiconDirs: o.PiconDirs, Plex: plexHub}
	radioHub := &hubs_.Radio{WC: wc, Now: now, PiconDirs: o.PiconDirs}
	streamHub := &hubs_.Stream{WC: wc, FS: o.RecordingsFS, PiconDirs: o.PiconDirs, Boxes: boxReg, Now: now}
	seriesHub := &hubs_.Series{SR: sr, WC: wc, FS: o.RecordingsFS, TVmaze: o.TVmaze, Store: st, Settings: svc, DMI: o.DMI, Now: now}
	discoveryHub := &hubs_.Discovery{WC: wc, SR: sr, AT: at, AI: o.AI, Store: st, Settings: svc, Now: now, Plex: plexHub}
	systemHub := &hubs_.System{Store: st, Settings: svc, Dashboard: dash, Plans: plans, Now: now,
		SelfTest: &hubs_.SelfTest{WC: wc, SR: sr, AT: at, DMI: o.DMI, Systemd: o.Systemd, FS: o.RecordingsFS,
			PiconDirs: o.PiconDirs, Dir: filepath.Join(o.DataDir, "selftest"), Writes: !o.ReadOnlyFiles, Now: now}}
	// Apt's own Lister value usually also implements FeedLister (Exec{},
	// the mock) — asserted here rather than a separate Options field, since
	// there's never a reason for one without the other.
	feeds, _ := o.Apt.(apt.FeedLister)
	// Same for install/remove (apt.Manager, A2): both Exec{} and the mock
	// implement it.
	pkgManager, _ := o.Apt.(apt.Manager)
	feedManager, _ := o.Apt.(apt.FeedManager)
	adminHub := &hubs_.Admin{Store: st, Level: level, Logs: o.Logs, FS: o.RecordingsFS,
		Systemd: o.Systemd, Scripts: o.Scripts, Apt: o.Apt, Feeds: feeds, Packages: pkgManager, FeedManager: feedManager, AI: o.AI, Settings: svc,
		Auth: svc, TerminalTarget: o.TerminalTarget, Plans: plans, WC: wc, Now: now, ReadOnly: o.ReadOnlyFiles}
	controlHub := &hubs_.Control{WC: wc}

	// Extension Hub (docs/EXTENSIONHUB_PLAN.md, E1): the registry is
	// always created (an extension can register at any time, over the
	// loopback handshake); in mock mode it's pre-seeded with a demo
	// "smart socket" so the nav entry has something to show without a
	// real extension process (PRINCIPLES.md, "Mock-first development").
	extRegistry := extension.NewRegistry()
	if o.Mock {
		extRegistry.Register(extension.Entry{
			Registration: extension.Registration{ID: "demo.socket", Name: "Steckdose (Demo)", Icon: "control", SchemaVersion: extension.SchemaVersion, Settings: true},
			Client:       extmock.New(),
		})
	}
	extensionsHub := &hubs_.Extensions{Registry: extRegistry, Plans: plans, ReadOnly: o.ReadOnlyFiles}
	appsHub := &hubs_.Apps{
		DockerHub: o.DockerHub, Docker: o.Docker, Mem: o.Mem, WC: wc,
		StorageRoot: o.AppsStorageRoot, DataDir: o.DataDir, Policy: o.AppsMountPolicy, Plans: plans, ReadOnly: o.ReadOnlyFiles,
		Store: st, Auth: svc, ConfigFS: o.AppsConfigFS, Setup: o.DockerSetup, Now: now,
		Settings: svc, User: user, Admin: adminHub, Settle: o.AppsSettle, Loc: o.AppsLocation,
	}
	if o.Mock && appsHub.Settle == 0 {
		appsHub.Settle = 2 * time.Second
	}
	if o.Mock {
		// Two demo disks, so the install dialog's storage choice has
		// something to show without a box.
		appsHub.Storage = func(context.Context) []hubs_.StorageChoice {
			return []hubs_.StorageChoice{
				{Path: o.AppsStorageRoot, Label: "/media/hdd", FreeBytes: 216 << 30, Default: true},
				{Path: "/media/usb/dreamhub-apps", Label: "/media/usb", FreeBytes: 29 << 30},
			}
		}
	}
	// App6: AppsHub's own installed-app configs join the existing
	// Backup & Restore screen as an opt-in section (system.go's
	// BackupSection) — wired here rather than inside hubs_.System since
	// System has no reason to import AppsHub's package.
	systemHub.Sections = []hubs_.BackupSection{appsHub, appsHub.AppList()}
	systemHub.BackupDir = o.BackupDir

	// Search providers (internal/hub/search.go): every hub with content of
	// its own; ControlHub has none (its results would be actions).
	hubs.AddSearchProvider(hub.IDTimer, timerHub)
	hubs.AddSearchProvider(hub.IDMedia, mediaHub)
	hubs.AddSearchProvider(hub.IDSeries, seriesHub)
	hubs.AddSearchProvider(hub.IDGuide, guideHub)
	hubs.AddSearchProvider(hub.IDRadio, radioHub)
	hubs.AddSearchProvider(hub.IDDiscovery, discoveryHub)
	hubs.AddSearchProvider(hub.IDDevice, hubs_.DeviceSearch{System: systemHub, Admin: adminHub})
	hubs.AddSearchProvider(hub.IDPlex, plexHub)
	// View sources for custom dashboard tiles (internal/hub/view.go).
	hubs.AddViewProvider(hub.IDMedia, mediaHub)
	hubs.AddViewProvider(hub.IDTimer, timerHub)
	hubs.AddViewProvider(hub.IDGuide, guideHub)
	hubs.AddViewProvider(hub.IDSeries, seriesHub)
	hubs.AddViewProvider(hub.IDDiscovery, discoveryHub)
	hubs.AddViewProvider(hub.IDRadio, radioHub)
	hubs.AddViewProvider(hub.IDPlex, plexHub)
	dash.KnownSource = hubs.KnownSource

	var notifier *push.Notifier
	if o.Push != nil {
		notifier = &push.Notifier{Bus: eventBus, Settings: svc, User: user, Client: o.Push, Logger: o.Logger}
		notifier.BoxName = func(params json.RawMessage) string {
			if len(boxReg.List()) < 2 {
				return ""
			}
			b, ok := boxReg.EventBox(params)
			if !ok {
				return ""
			}
			return b.Name
		}
	}

	// The pairing code is also put on this box's TV (the first box is the
	// one DreamHub runs on); a box with writes off just shows it in the UI.
	peerSvc := &peer.Service{Tokens: tokens, Now: now, Announce: func(ctx context.Context, code string) error {
		return wc.Message(box.With(ctx, boxReg.DefaultID()), "DreamHub pairing code: "+code, webcontrol.MessageInfo, 600)
	}}
	boxManager := &boxes.Manager{Store: st, Registry: boxReg, Factory: o.BoxFactory, Peer: o.Peer, OnWake: poller.Boost}
	if o.Mock {
		// Nothing leaves the machine in mock mode.
		boxManager.SendWOL = func(string) error { return nil }
	}
	handler, routes := api.NewRouterWithRoutes(api.Deps{
		WebControl: wc,
		Boxes:      boxReg,
		BoxManager: boxManager,
		Extensions: extRegistry,
		Peer:       peerSvc,
		Verifier:   auth.NewSettingsVerifier(svc, tokens),
		Settings:   svc,
		Connectors: conns,
		Hubs:       hubs,
		Plans:      plans,
		Dashboard:  dash,
		Tokens:     tokens,
		Profiles:   profiles,
		Watcher:    watcher,
		Store:      st,
		Events:     eventBus,
		Enrich:     enricher,
		Push:       notifier,
		Jobs:       jobQueue,
		Mounts: []api.Mount{
			{HubID: hub.IDTimer, Prefixes: []string{"/api/v1/timers"}, Register: timerHub.Mount, Perms: timerHub.Perms()},
			{HubID: hub.IDMedia, Prefixes: []string{"/api/v1/media"}, Register: mediaHub.Mount, Perms: mediaHub.Perms()},
			{HubID: hub.IDGuide, Prefixes: []string{"/api/v1/guide"}, Register: guideHub.Mount},
			{HubID: hub.IDRadio, Prefixes: []string{"/api/v1/radio"}, Register: radioHub.Mount},
			{HubID: hub.IDStream, Prefixes: []string{"/api/v1/stream"}, Register: streamHub.Mount, Perms: streamHub.Perms()},
			{HubID: hub.IDSeries, Prefixes: []string{"/api/v1/series"}, Register: seriesHub.Mount},
			{HubID: hub.IDDiscovery, Prefixes: []string{"/api/v1/discovery"}, Register: discoveryHub.Mount, Perms: discoveryHub.Perms()},
			{HubID: hub.IDDevice, Prefixes: []string{"/api/v1/changes", "/api/v1/system"}, Register: systemHub.Mount, Perms: systemHub.Perms()},
			{HubID: hub.IDDevice, Prefixes: []string{"/api/v1/admin"}, Register: adminHub.Mount},
			{HubID: hub.IDControl, Prefixes: []string{"/api/v1/control"}, Register: controlHub.Mount, Perms: controlHub.Perms()},
			{HubID: hub.IDChannels, Prefixes: []string{"/api/v1/channels"}, Register: channelsHub.Mount, Perms: channelsHub.Perms()},
			{HubID: hub.IDPlex, Prefixes: []string{"/api/v1/plex"}, Register: plexHub.Mount, Perms: plexHub.Perms()},
			{HubID: hub.IDApps, Prefixes: []string{"/api/v1/extensions"}, Register: extensionsHub.Mount, Perms: extensionsHub.Perms()},
			{HubID: hub.IDApps, Prefixes: []string{"/api/v1/apps"}, Register: appsHub.Mount, Perms: appsHub.Perms()},
		},
		// The box's media player fetches a Plex film without a DreamHub
		// login; the handler itself admits only the box (loopback) with a
		// ticket. An extension's own process registers itself the same
		// way — loopback only, no DreamHub login of its own.
		Public: []api.PublicRoute{
			{HubID: hub.IDPlex, Pattern: "GET /api/v1/plex/stream/{ticket}/{name}", Handler: http.HandlerFunc(plexHub.Stream)},
			{HubID: hub.IDStream, Pattern: "GET /api/v1/stream/relay/{ticket}/{name}", Handler: http.HandlerFunc(streamHub.Relay)},
			{HubID: hub.IDApps, Pattern: "POST /api/v1/extensions/register", Handler: http.HandlerFunc(extensionsHub.Register)},
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
		Plans: plans, Dashboard: dash, Tokens: tokens, Profiles: profiles, Routes: routes, Level: level, User: user,
		Events: eventBus, Poller: poller, Jobs: jobQueue, Boxes: boxReg, BoxManager: boxManager,
		Background: append(background(notifier, seriesHub, adminHub, appsHub, plexHub, eventBus, o.Logger),
			func(ctx context.Context) { channelsHub.Watch(ctx, o.Logger) }, jobQueue.Run),
	}, nil
}

// background lists the long-running jobs besides the poller.
func background(n *push.Notifier, series *hubs_.Series, admin *hubs_.Admin, apps *hubs_.Apps, plexHub *hubs_.Plex, bus *events.Bus, logger *slog.Logger) []func(context.Context) {
	jobs := []func(context.Context){
		func(ctx context.Context) { series.WatchMissing(ctx, bus, logger) },
		func(ctx context.Context) { admin.WatchServices(ctx, bus, logger) },
		func(ctx context.Context) { admin.WatchPackages(ctx, bus, logger) },
		func(ctx context.Context) { apps.WatchApps(ctx, bus, logger) },
		func(ctx context.Context) { apps.AutostartOnBoot(ctx, logger) },
		func(ctx context.Context) { apps.WatchContainers(ctx, bus, logger) },
	}
	if plexHub.Client != nil {
		jobs = append(jobs, plexHub.Watch)
	}
	if n != nil {
		jobs = append(jobs, n.Run)
	}
	return jobs
}

// recordingBlocker holds the job queue while a timer records. When the
// timer list can't be read, it holds too: better a late job than one that
// competes with a recording nobody could see.
func recordingBlocker(wc webcontrol.Client) jobs.Blocker {
	return func(ctx context.Context) string {
		timers, err := wc.TimerList(ctx)
		if err != nil {
			return "boxUnreachable"
		}
		for _, t := range timers {
			// Like MediaHub's "in use": a zap timer or a switched-off one
			// records nothing.
			if t.State == webcontrol.TimerRunning && !t.Disabled && !t.JustPlay {
				return "recording"
			}
		}
		return ""
	}
}

// adScanOpener picks where ad detection gets its frames: synthetic ones in
// mock mode (the mock box's recordings hold no video), else the system
// FFmpeg at low priority; nil without FFmpeg (the feature is unavailable).
func adScanOpener(o Options) adscan.Opener {
	if o.Mock {
		return adscan.Synthetic{}
	}
	if o.LookPath == nil {
		return nil
	}
	bin, err := o.LookPath("ffmpeg")
	if err != nil {
		return nil
	}
	var prefix []string
	if nice, err := o.LookPath("nice"); err == nil {
		prefix = append(prefix, nice, "-n", "19")
	}
	if ionice, err := o.LookPath("ionice"); err == nil {
		prefix = append(prefix, ionice, "-c3")
	}
	return adscan.FFmpeg{Path: bin, Prefix: prefix}
}

// trimCutter is adScanOpener's counterpart for cutting: a stand-in that
// writes into the mock FS in mock mode, else the system FFmpeg.
func trimCutter(o Options) trim.Cutter {
	if o.Mock {
		return trim.Mock{FS: o.RecordingsFS}
	}
	ff, ok := adScanOpener(o).(adscan.FFmpeg)
	if !ok {
		return nil
	}
	return trim.FFmpeg{Path: ff.Path, Prefix: ff.Prefix, Probe: ff}
}

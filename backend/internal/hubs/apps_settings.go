package hubs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/httpx"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/updatewindow"
)

// Docker settings (docs/APPSHUB_PLAN.md, App13): ordinary DreamHub settings,
// global (admin only), with defaults. They are edited from the Docker
// settings dialog, so the generic settings form hides them (Hidden).

const (
	// KeyAppsStorageRoot: where NEW apps keep their data. Empty is the box's
	// own default (the recording disk). Installed apps stay where they are.
	KeyAppsStorageRoot = "apps.storageRoot"
	// KeyAppsAutostartDefault: whether a newly installed app starts when the
	// box starts.
	KeyAppsAutostartDefault = "apps.autostartDefault"
	// KeyAppsMemoryMB: the memory limit for a container that states none
	// (an app of your own); 0 is automatic.
	KeyAppsMemoryMB = "apps.memoryLimitMB"
	// KeyAppsUpdateHours: how often the background check looks for newer
	// images. KeyAppsUpdateNotify: whether it tells the notification inbox.
	KeyAppsUpdateHours  = "apps.updateCheckHours"
	KeyAppsUpdateNotify = "apps.updateNotify"
	// Updates (App14). Both automatic switches are off by default. The night
	// window applies to automatic updates only; the guard time is how many
	// minutes before a recording starts updates already stay out of its way.
	KeyAppsAutoContainers = "apps.autoUpdateContainers"
	KeyAppsAutoPackages   = "apps.autoUpdatePackages"
	KeyAppsNightOnly      = "apps.updateNightOnly"
	KeyAppsNightStart     = "apps.updateNightStart"
	KeyAppsNightEnd       = "apps.updateNightEnd"
	KeyAppsGuardMinutes   = "apps.updateGuardMinutes"
	// KeyAppsAutostart: the ids of the apps that start with the box
	// (comma-separated). Not shown anywhere as a setting; the switch on each
	// app's row sets it.
	KeyAppsAutostart = "apps.autostartIds"
	// KeyAppsBootID: the boot the autostart pass last ran for, so a restart of
	// DreamHub alone doesn't start what somebody stopped.
	KeyAppsBootID = "apps.autostartBoot"
	// KeyAppsOpenHost: the host name or address the "Open" link of an app
	// uses. Empty: the address DreamHub itself was opened with, which is wrong
	// behind a reverse proxy (the apps' ports are not on the proxy).
	KeyAppsOpenHost = "apps.openHost"
)

// Defaults, also used when no settings service is wired (tests).
const (
	defaultAutostartNew  = true
	defaultUpdateHours   = 24
	defaultUpdateNotify  = true
	defaultNightStart    = "02:00"
	defaultNightEnd      = "05:00"
	defaultGuardMinutes  = 15
	maxGuardMinutes      = 240
	maxMemoryLimitMB     = 16384
	minCustomMemoryLimit = 64
)

// AppsSettingsSection declares the settings above.
func AppsSettingsSection() settings.Section {
	zero, hours, maxHours := int64(0), int64(1), int64(24*30)
	maxMB := int64(maxMemoryLimitMB)
	maxGuard := int64(maxGuardMinutes)
	return settings.Section{
		ID: "apps", Label: "settings.apps", Tab: settings.TabHubs,
		Fields: []settings.Field{
			{Key: KeyAppsStorageRoot, Type: settings.TypeString, Scope: settings.ScopeGlobal, Label: "settings.apps.storageRoot", Default: "", Hidden: true},
			{Key: KeyAppsAutostartDefault, Type: settings.TypeBool, Scope: settings.ScopeGlobal, Label: "settings.apps.autostartDefault", Default: defaultAutostartNew, Hidden: true},
			{Key: KeyAppsMemoryMB, Type: settings.TypeInt, Scope: settings.ScopeGlobal, Label: "settings.apps.memoryLimitMB", Default: int64(0), Min: &zero, Max: &maxMB, Hidden: true},
			{Key: KeyAppsUpdateHours, Type: settings.TypeInt, Scope: settings.ScopeGlobal, Label: "settings.apps.updateCheckHours", Default: int64(defaultUpdateHours), Min: &hours, Max: &maxHours, Hidden: true},
			{Key: KeyAppsUpdateNotify, Type: settings.TypeBool, Scope: settings.ScopeGlobal, Label: "settings.apps.updateNotify", Default: defaultUpdateNotify, Hidden: true},
			{Key: KeyAppsAutoContainers, Type: settings.TypeBool, Scope: settings.ScopeGlobal, Label: "settings.apps.autoUpdateContainers", Default: false, Hidden: true},
			{Key: KeyAppsAutoPackages, Type: settings.TypeBool, Scope: settings.ScopeGlobal, Label: "settings.apps.autoUpdatePackages", Default: false, Hidden: true},
			{Key: KeyAppsNightOnly, Type: settings.TypeBool, Scope: settings.ScopeGlobal, Label: "settings.apps.updateNightOnly", Default: false, Hidden: true},
			{Key: KeyAppsNightStart, Type: settings.TypeString, Scope: settings.ScopeGlobal, Label: "settings.apps.updateNightStart", Default: defaultNightStart, Hidden: true},
			{Key: KeyAppsNightEnd, Type: settings.TypeString, Scope: settings.ScopeGlobal, Label: "settings.apps.updateNightEnd", Default: defaultNightEnd, Hidden: true},
			{Key: KeyAppsGuardMinutes, Type: settings.TypeInt, Scope: settings.ScopeGlobal, Label: "settings.apps.updateGuardMinutes", Default: int64(defaultGuardMinutes), Min: &zero, Max: &maxGuard, Hidden: true},
			{Key: KeyAppsAutostart, Type: settings.TypeString, Scope: settings.ScopeGlobal, Label: "settings.apps.autostartIds", Default: "", Hidden: true},
			{Key: KeyAppsOpenHost, Type: settings.TypeString, Scope: settings.ScopeGlobal, Label: "settings.apps.openHost", Default: "", Hidden: true},
			{Key: KeyAppsBootID, Type: settings.TypeString, Scope: settings.ScopeGlobal, Label: "settings.apps.autostartBoot", Default: "", Hidden: true},
		},
		Check: func(v settings.Values) map[string]settings.FieldError {
			errs := map[string]settings.FieldError{}
			if r := v.String(KeyAppsStorageRoot); r != "" && (!path.IsAbs(r) || path.Clean(r) != r || strings.ContainsAny(r, ":\n\r\x00")) {
				errs[KeyAppsStorageRoot] = settings.FieldError{Code: settings.ErrInvalidOption}
			}
			if h := v.String(KeyAppsOpenHost); h != "" && !validOpenHost(h) {
				errs[KeyAppsOpenHost] = settings.FieldError{Code: settings.ErrInvalidOption}
			}
			if mb := v.Int(KeyAppsMemoryMB); mb != 0 && mb < minCustomMemoryLimit {
				errs[KeyAppsMemoryMB] = settings.FieldError{Code: settings.ErrOutOfRange, Params: map[string]any{"min": minCustomMemoryLimit}}
			}
			for _, k := range []string{KeyAppsNightStart, KeyAppsNightEnd} {
				if _, err := updatewindow.ParseClock(v.String(k)); err != nil {
					errs[k] = settings.FieldError{Code: settings.ErrInvalidOption}
				}
			}
			return errs
		},
	}
}

// validOpenHost: a bare host name or IP address — no scheme, port or path
// (the app's own port is added to it).
func validOpenHost(h string) bool {
	return len(h) <= 253 && !strings.ContainsAny(h, " /:@?#\\\n\r\x00")
}

// appsConfig is the Docker settings as the hub uses them.
type appsConfig struct {
	StorageRoot      string // "" = the box's default
	AutostartDefault bool
	MemoryMB         int // 0 = automatic
	UpdateHours      int
	UpdateNotify     bool
	Autostart        []string
	BootID           string

	AutoContainers bool
	AutoPackages   bool
	NightOnly      bool
	NightStart     int // minutes since midnight
	NightEnd       int
	GuardMinutes   int
}

// guard is how long before a recording starts updates stay out of its way.
func (c appsConfig) guard() time.Duration { return time.Duration(c.GuardMinutes) * time.Minute }

// night is the optional "only between" window of the automatic updates.
func (c appsConfig) night() updatewindow.Night {
	return updatewindow.Night{Enabled: c.NightOnly, Start: c.NightStart, End: c.NightEnd}
}

func (h *Apps) config(ctx context.Context) appsConfig {
	c := appsConfig{AutostartDefault: defaultAutostartNew, UpdateHours: defaultUpdateHours, UpdateNotify: defaultUpdateNotify,
		NightStart: 2 * 60, NightEnd: 5 * 60, GuardMinutes: defaultGuardMinutes}
	if h.Settings == nil {
		return c
	}
	v, err := h.Settings.Values(ctx, h.User)
	if err != nil {
		return c
	}
	c.StorageRoot = v.String(KeyAppsStorageRoot)
	c.AutostartDefault = v.Bool(KeyAppsAutostartDefault)
	c.MemoryMB = int(v.Int(KeyAppsMemoryMB))
	if n := int(v.Int(KeyAppsUpdateHours)); n > 0 {
		c.UpdateHours = n
	}
	c.UpdateNotify = v.Bool(KeyAppsUpdateNotify)
	for _, id := range strings.Split(v.String(KeyAppsAutostart), ",") {
		if id = strings.TrimSpace(id); id != "" {
			c.Autostart = append(c.Autostart, id)
		}
	}
	c.BootID = v.String(KeyAppsBootID)
	c.AutoContainers = v.Bool(KeyAppsAutoContainers)
	c.AutoPackages = v.Bool(KeyAppsAutoPackages)
	c.NightOnly = v.Bool(KeyAppsNightOnly)
	if m, err := updatewindow.ParseClock(v.String(KeyAppsNightStart)); err == nil {
		c.NightStart = m
	}
	if m, err := updatewindow.ParseClock(v.String(KeyAppsNightEnd)); err == nil {
		c.NightEnd = m
	}
	c.GuardMinutes = int(v.Int(KeyAppsGuardMinutes))
	return c
}

// defaultMemoryMB is the limit for a container that states none.
func (c appsConfig) defaultMemoryMB() int {
	if c.MemoryMB > 0 {
		return c.MemoryMB
	}
	return defaultMemoryLimitMB
}

// defaultRoot is where a new app goes when the person doesn't choose: the
// configured location if it is still one of the places apps can be stored
// (a disk may have gone), else the box's own default.
func (h *Apps) defaultRoot(ctx context.Context) string {
	want := h.config(ctx).StorageRoot
	if want == "" || want == h.StorageRoot {
		return h.StorageRoot
	}
	for _, c := range h.rawStorageChoices(ctx) {
		if c.Path == want {
			return want
		}
	}
	return h.StorageRoot
}

// saveSetting writes one setting as the person making the request.
func (h *Apps) saveSetting(ctx context.Context, key string, value any) error {
	if h.Settings == nil {
		return errNoSettings
	}
	user, ok := httpx.User(ctx)
	if !ok {
		user = h.User
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return h.Settings.Update(ctx, user, map[string]json.RawMessage{key: raw})
}

var errNoSettings = errors.New("apps: no settings service")

// --- auto-start ---------------------------------------------------------------

// isAutostart reports whether the app starts with the box.
func (c appsConfig) isAutostart(appID string) bool { return slices.Contains(c.Autostart, appID) }

// setAutostart adds or removes an app from the auto-start list.
func (h *Apps) setAutostart(ctx context.Context, appID string, on bool) error {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	list := h.config(ctx).Autostart
	has := slices.Contains(list, appID)
	if has == on {
		return nil
	}
	if on {
		list = append(list, appID)
	} else {
		list = slices.DeleteFunc(list, func(s string) bool { return s == appID })
	}
	slices.Sort(list)
	var v any = strings.Join(list, ",")
	if len(list) == 0 {
		v = nil // back to the default (empty)
	}
	return h.saveSetting(ctx, KeyAppsAutostart, v)
}

// setAutostartBody is PUT /api/v1/apps/installed/{id}/autostart.
type setAutostartBody struct {
	Enabled *bool `json:"enabled"`
}

// putAutostart is a direct, reversible switch (no plan): like start and
// stop it is expected and low-risk, and the row says what it did.
func (h *Apps) putAutostart(w http.ResponseWriter, r *http.Request) {
	var body setAutostartBody
	if err := httpx.DecodeJSON(r, &body); err != nil || body.Enabled == nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "invalid_body", `expected {"enabled": true|false}`)
		return
	}
	if h.Docker == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "docker_unavailable", "the Docker helper isn't available")
		return
	}
	if h.Settings == nil {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, "settings_unavailable", "settings aren't available")
		return
	}
	containers, err := h.Docker.List(r.Context())
	if err != nil {
		writeDockerError(w, r, err, "the request failed")
		return
	}
	g, found := findApp(containers, r.PathValue("id"))
	if !found {
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "no such installed app")
		return
	}
	if err := h.setAutostart(r.Context(), g.appID, *body.Enabled); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "settings_failed", "the choice could not be saved")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"autostart": *body.Enabled})
}

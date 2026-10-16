package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// The DreamHub extension contract (docs/BUILDING_EXTENSIONS.md): GET
// /widgets, POST /actions/{id}/plan, POST /actions/{id}/apply — plain
// JSON, nothing imported from DreamHub's own code.

// Widget is one entry of GET /widgets (schema version 1).
type Widget struct {
	Type        string    `json:"type"`
	Label       string    `json:"label"`
	Value       string    `json:"value,omitempty"`
	Tone        string    `json:"tone,omitempty"`
	Rows        []ListRow `json:"rows,omitempty"`
	ActionID    string    `json:"actionId,omitempty"`
	ButtonLabel string    `json:"buttonLabel,omitempty"`
	// Tile (schema 3) puts this widget on DreamHub's start page too. The
	// extension decides what goes there; none flagged, no tile.
	Tile bool `json:"tile,omitempty"`
}

type ListRow struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type texts struct {
	on, off, turnOn, turnOff, noKey, badKey, notFound, found, readings, previewOn, previewOff string
	consumption, power, today, month, voltage, current                                        string
}

var lang = map[string]texts{
	"de": {on: "An", off: "Aus", turnOn: "Einschalten", turnOff: "Ausschalten",
		noKey: "Geräteschlüssel fehlt", badKey: "Status nicht lesbar — Geräteschlüssel prüfen", notFound: "Nicht im Netzwerk gefunden",
		found: "Gefunden — noch nicht eingerichtet", readings: "Messwerte",
		previewOn: "%s wird eingeschaltet.", previewOff: "%s wird ausgeschaltet.",
		consumption: "Verbrauch", power: "Leistung", today: "Heute", month: "Dieser Monat", voltage: "Spannung", current: "Strom"},
	"en": {on: "On", off: "Off", turnOn: "Turn on", turnOff: "Turn off",
		noKey: "Device key missing", badKey: "State unreadable — check the device key", notFound: "Not found on the network",
		found: "Found — not set up yet", readings: "Readings",
		previewOn: "%s will be switched on.", previewOff: "%s will be switched off.",
		consumption: "Consumption", power: "Power", today: "Today", month: "This month", voltage: "Voltage", current: "Current"},
}

type Server struct {
	cfg     *ConfigStore
	devices *Devices
	cloud   *Cloud
	energy  *EnergyStore
}

func (s *Server) t() texts { return lang[s.cfg.Get().Lang] }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /widgets", s.widgets)
	mux.HandleFunc("POST /actions/{action}/plan", s.plan)
	mux.HandleFunc("POST /actions/{action}/apply", s.apply)
	mux.HandleFunc("GET /settings", s.settings)
	mux.HandleFunc("PUT /settings", s.saveSettings)
	return mux
}

func actionID(id string) string { return "toggle-" + id }

func (s *Server) name(id string) string {
	if c, ok := s.cfg.Get().device(id); ok && c.Name != "" {
		return c.Name
	}
	return "eWeLink " + id
}

func (s *Server) widgets(w http.ResponseWriter, r *http.Request) {
	t := s.t()
	var out []Widget
	seen := map[string]bool{}
	for _, d := range s.devices.Snapshot() {
		seen[d.ID] = true
		name := s.name(d.ID)
		conf, configured := s.cfg.Get().device(d.ID)
		switch {
		case !configured:
			out = append(out, Widget{Type: "status", Label: name, Value: t.found, Tone: "warn"})
		case conf.Key == "" && d.State == nil:
			out = append(out, Widget{Type: "status", Label: name, Value: t.noKey, Tone: "bad"})
		case d.State == nil:
			out = append(out, Widget{Type: "status", Label: name, Value: t.badKey, Tone: "bad"})
		default:
			on, known := d.On()
			if !known {
				out = append(out, Widget{Type: "status", Label: name, Value: "?", Tone: "warn"})
				break
			}
			value, tone, button := t.off, "warn", t.turnOn
			if on {
				value, tone, button = t.on, "good", t.turnOff
			}
			out = append(out, Widget{Type: "status", Label: name, Value: value, Tone: tone, Tile: true})
			e, _ := s.energyOf(d.ID)
			if rows := s.consumptionRows(d.State, e); len(rows) > 0 {
				out = append(out, Widget{Type: "list", Label: name + " · " + t.consumption, Rows: rows, Tile: true})
			}
			if rows := s.readingRows(d.State, e); len(rows) > 0 {
				out = append(out, Widget{Type: "list", Label: name + " · " + t.readings, Rows: rows})
			}
			out = append(out, Widget{Type: "action", Label: name, ActionID: actionID(d.ID), ButtonLabel: button})
		}
	}
	for _, c := range s.cfg.Get().Devices {
		if !seen[c.ID] {
			out = append(out, Widget{Type: "status", Label: s.name(c.ID), Value: t.notFound, Tone: "bad"})
		}
	}
	if out == nil {
		out = []Widget{}
	}
	writeJSON(w, http.StatusOK, out)
}

// energyOf is the cloud's last look at a device, if there was one.
func (s *Server) energyOf(id string) (Energy, bool) {
	if s.energy == nil {
		return Energy{}, false
	}
	return s.energy.Get(id)
}

// liveValue prefers what the device announced on the LAN, then the cloud's.
func liveValue(state map[string]any, key string, cloud *float64) (float64, bool) {
	switch v := state[key].(type) {
	case float64:
		// The plug's own LAN announcement sends these as whole numbers in
		// hundredths (voltage 23442 is 234.42 V, power 901 is 9.01 W,
		// monthKwh 90 is 0.90 kWh) — confirmed on the box 2026-10-16 against
		// the plug's behaviour and SonoffLAN's UIID 190 scaling. A string
		// ("212.50", as the cloud and older plugs send) is already in units.
		return v * 0.01, true
	case string:
		if f, ok := number(v); ok {
			return f, true
		}
	}
	if cloud != nil {
		return *cloud, true
	}
	return 0, false
}

// consumptionRows is the part worth a glance: what the plug draws now,
// what it used today and this month. Rows with no data are left out.
func (s *Server) consumptionRows(state map[string]any, e Energy) []ListRow {
	t := s.t()
	var rows []ListRow
	if v, ok := liveValue(state, "power", e.PowerW); ok {
		rows = append(rows, ListRow{Label: t.power, Value: fmt.Sprintf("%.1f W", v)})
	}
	// What the plug announces on the LAN itself (dayKwh, monthKwh) comes
	// first; the cloud's history is for plugs that don't announce it.
	if v, ok := liveValue(state, "dayKwh", e.TodayKwh); ok {
		rows = append(rows, ListRow{Label: t.today, Value: kwh(v)})
	}
	if v, ok := liveValue(state, "monthKwh", e.MonthKwh); ok {
		rows = append(rows, ListRow{Label: t.month, Value: kwh(v)})
	}
	return rows
}

// readingRows are the remaining measurements, as received.
func (s *Server) readingRows(state map[string]any, e Energy) []ListRow {
	t := s.t()
	var rows []ListRow
	if v, ok := liveValue(state, "voltage", e.VoltageV); ok {
		rows = append(rows, ListRow{Label: t.voltage, Value: fmt.Sprintf("%.1f V", v)})
	}
	if v, ok := liveValue(state, "current", e.CurrentA); ok {
		rows = append(rows, ListRow{Label: t.current, Value: fmt.Sprintf("%.3f A", v)})
	}
	return rows
}

// target resolves an action id to a configured, readable device.
func (s *Server) target(action string) (Device, DeviceConfig, bool) {
	id, ok := strings.CutPrefix(action, "toggle-")
	if !ok {
		return Device{}, DeviceConfig{}, false
	}
	conf, configured := s.cfg.Get().device(id)
	d, seen := s.devices.Get(id)
	if !configured || !seen || d.State == nil {
		return Device{}, DeviceConfig{}, false
	}
	return d, conf, true
}

func (s *Server) plan(w http.ResponseWriter, r *http.Request) {
	_ = s.devices.Refresh(r.Context())
	d, conf, ok := s.target(r.PathValue("action"))
	if !ok {
		http.Error(w, "unknown or unreadable device", http.StatusNotFound)
		return
	}
	on, known := d.On()
	if !known {
		http.Error(w, "the device's state isn't known", http.StatusConflict)
		return
	}
	t := s.t()
	summary := fmt.Sprintf(t.previewOn, s.name(d.ID))
	if on {
		summary = fmt.Sprintf(t.previewOff, s.name(d.ID))
		if conf.OffWarning != "" {
			summary += " " + conf.OffWarning
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "fingerprint": d.Fingerprint()})
}

func (s *Server) apply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "expected {\"fingerprint\": ...}", http.StatusBadRequest)
		return
	}
	_ = s.devices.Refresh(r.Context())
	d, _, ok := s.target(r.PathValue("action"))
	if !ok {
		http.Error(w, "unknown or unreadable device", http.StatusNotFound)
		return
	}
	// Someone switched it from the app since the preview: refuse.
	if d.Fingerprint() != body.Fingerprint {
		http.Error(w, "the device changed since the preview", http.StatusConflict)
		return
	}
	on, _ := d.On()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.devices.Switch(ctx, d.ID, !on); err != nil {
		log.Printf("switching %s to %t over the LAN failed: %v", d.ID, !on, err)
		if err := s.switchViaCloud(ctx, d, !on); err != nil {
			log.Printf("switching %s to %t through the cloud failed: %v", d.ID, !on, err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		log.Printf("switched %s %s through the cloud", d.ID, map[bool]string{true: "on", false: "off"}[!on])
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	log.Printf("switched %s %s", d.ID, map[bool]string{true: "on", false: "off"}[!on])
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// --- settings (schema 2): GET/PUT /settings --------------------------------

type settingField struct {
	Key     string          `json:"key"`
	Label   string          `json:"label"`
	Type    string          `json:"type"`
	Help    string          `json:"help,omitempty"`
	Value   string          `json:"value,omitempty"`
	Set     bool            `json:"set,omitempty"`
	Options []settingOption `json:"options,omitempty"`
}

type settingOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// settingsText is the form's wording, German or English like the widgets.
func settingsText(l string) (language, name, key, keyHelp, warn, warnHelp string) {
	if l == "en" {
		return "Language of this card", "Name", "Device key", "Fetched once from your eWeLink account (dev.ewelink.cc); without it the device can be found but neither read nor switched.",
			"Warning before switching off", "Shown in the preview, e.g. what is plugged in and must be shut down first."
	}
	return "Sprache dieser Karte", "Name", "Geräteschlüssel", "Wird einmal aus deinem eWeLink-Konto geholt (dev.ewelink.cc); ohne ihn wird das Gerät gefunden, aber nicht gelesen oder geschaltet.",
		"Warnung vor dem Ausschalten", "Steht in der Vorschau, z. B. was angeschlossen ist und vorher heruntergefahren werden muss."
}

// settings lists a language and, per device (configured or just seen on
// the network), its name, key and switch-off warning.
func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Get()
	language, name, key, keyHelp, warn, warnHelp := settingsText(cfg.Lang)
	fields := []settingField{{Key: "lang", Label: language, Type: "select", Value: cfg.Lang,
		Options: []settingOption{{"de", "Deutsch"}, {"en", "English"}}}}
	fields = append(fields, s.cloudFields(cfg)...)
	ids := []string{}
	seen := map[string]bool{}
	for _, c := range cfg.Devices {
		ids, seen[c.ID] = append(ids, c.ID), true
	}
	for _, d := range s.devices.Snapshot() {
		if !seen[d.ID] {
			ids, seen[d.ID] = append(ids, d.ID), true
		}
	}
	for _, id := range ids {
		c, _ := cfg.device(id)
		fields = append(fields,
			settingField{Key: "name:" + id, Label: name + " · " + id, Type: "text", Value: c.Name},
			settingField{Key: "key:" + id, Label: key + " · " + id, Type: "secret", Set: c.Key != "", Help: keyHelp},
			settingField{Key: "offWarning:" + id, Label: warn + " · " + id, Type: "text", Value: c.OffWarning, Help: warnHelp},
		)
	}
	writeJSON(w, http.StatusOK, map[string]any{"fields": fields})
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Values map[string]string `json:"values"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expected {\"values\": {...}}"})
		return
	}
	code := strings.TrimSpace(body.Values["cloud:code"])
	delete(body.Values, "cloud:code")
	delete(body.Values, "cloud:loginUrl")
	delete(body.Values, "cloud:loginLink")
	err := s.cfg.Update(func(c *Config) error {
		for k, v := range body.Values {
			field, id, _ := strings.Cut(k, ":")
			if field == "cloud" {
				if err := s.setCloudField(c, id, v); err != nil {
					return err
				}
				continue
			}
			if field == "lang" {
				if v != "de" && v != "en" {
					return fmt.Errorf("language must be de or en")
				}
				c.Lang = v
				continue
			}
			if id == "" || len(id) > 64 {
				return fmt.Errorf("unknown setting %q", k)
			}
			i := -1
			for j := range c.Devices {
				if c.Devices[j].ID == id {
					i = j
				}
			}
			if i < 0 {
				c.Devices = append(c.Devices, DeviceConfig{ID: id})
				i = len(c.Devices) - 1
			}
			d := &c.Devices[i]
			switch field {
			case "name":
				d.Name = strings.TrimSpace(v)
			case "offWarning":
				d.OffWarning = strings.TrimSpace(v)
			case "key":
				// An empty secret means "keep", so a key can't be cleared
				// here — that is deliberate (remove the device in the file).
				if v = strings.TrimSpace(v); v != "" {
					d.Key = v
				}
			default:
				return fmt.Errorf("unknown setting %q", k)
			}
		}
		return nil
	})
	if err == nil && code != "" {
		err = s.logIn(r.Context(), code)
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.settings(w, r)
}

// --- eWeLink account: the one-time key fetch and the consumption login -----

type cloudTexts struct {
	appID, appIDHelp, secret, secretHelp, region, redirect, redirectHelp, login, loginHelp, code, codeHelp string
	needApp, loggedIn, noDevices, keysSet                                                                  string
}

func cloudText(l string) cloudTexts {
	if l == "en" {
		return cloudTexts{
			appID: "eWeLink app ID", appIDHelp: "From your own application at dev.ewelink.cc (Platform → App Management). Used to fetch the device keys once and to read the consumption.",
			secret: "eWeLink app secret", secretHelp: "From the same page. Stays on the box.",
			region: "eWeLink region", redirect: "Redirect address", redirectHelp: "Exactly as entered in the application at dev.ewelink.cc. The page behind it does not need to exist — you only copy its address.",
			login: "Log in to eWeLink", loginHelp: "Open this link, log in with your eWeLink account, then copy the whole address of the page you land on.",
			code: "Address after logging in", codeHelp: "Paste it here and save within 30 seconds — eWeLink's code expires quickly. The device keys are fetched and stored; the login is kept to read the consumption.",
			needApp: "Fill in the app ID, secret and redirect address, save, and the login link appears here.", loggedIn: "Logged in to eWeLink.",
			noDevices: "No device of this account is on the network yet.", keysSet: "Device keys stored for: %s.",
		}
	}
	return cloudTexts{
		appID: "eWeLink App-ID", appIDHelp: "Aus deiner eigenen Anwendung auf dev.ewelink.cc (Platform → App Management). Damit werden die Geräteschlüssel einmal geholt und der Verbrauch gelesen.",
		secret: "eWeLink App-Secret", secretHelp: "Von derselben Seite. Bleibt auf der Box.",
		region: "eWeLink-Region", redirect: "Weiterleitungsadresse", redirectHelp: "Genau so, wie sie in der Anwendung auf dev.ewelink.cc steht. Die Seite dahinter muss nicht existieren — du kopierst nur ihre Adresse.",
		login: "Bei eWeLink anmelden", loginHelp: "Diesen Link öffnen, mit dem eWeLink-Konto anmelden und dann die ganze Adresse der Seite kopieren, auf der du landest.",
		code: "Adresse nach der Anmeldung", codeHelp: "Hier einfügen und innerhalb von 30 Sekunden speichern — der Code von eWeLink läuft schnell ab. Die Geräteschlüssel werden geholt und gespeichert; die Anmeldung bleibt für den Verbrauch erhalten.",
		needApp: "App-ID, Secret und Weiterleitungsadresse eintragen, speichern — dann erscheint hier der Anmelde-Link.", loggedIn: "Bei eWeLink angemeldet.",
		noDevices: "Noch kein Gerät dieses Kontos im Netzwerk gefunden.", keysSet: "Geräteschlüssel gespeichert für: %s.",
	}
}

// cloudFields is the account part of the form, above the devices.
func (s *Server) cloudFields(cfg Config) []settingField {
	t := cloudText(cfg.Lang)
	cc := cfg.Cloud
	region := cc.region()
	fields := []settingField{
		{Key: "cloud:appId", Label: t.appID, Type: "text", Value: cc.AppID, Help: t.appIDHelp},
		{Key: "cloud:secret", Label: t.secret, Type: "secret", Set: cc.Secret != "", Help: t.secretHelp},
		{Key: "cloud:region", Label: t.region, Type: "select", Value: region, Options: []settingOption{
			{"eu", "Europe"}, {"us", "Americas"}, {"as", "Asia"}, {"cn", "China"}}},
		{Key: "cloud:redirectUrl", Label: t.redirect, Type: "text", Value: cc.RedirectURL, Help: t.redirectHelp},
	}
	if !cc.Ready() {
		return append(fields, settingField{Key: "cloud:loginUrl", Label: t.login, Type: "info", Value: t.needApp})
	}
	if cc.LoggedIn() {
		fields = append(fields, settingField{Key: "cloud:loginUrl", Label: t.login, Type: "info", Value: t.loggedIn})
	}
	return append(fields,
		settingField{Key: "cloud:loginLink", Label: t.login, Type: "link", Value: s.cloud.LoginURL(cc), Help: t.loginHelp},
		settingField{Key: "cloud:code", Label: t.code, Type: "text", Help: t.codeHelp},
	)
}

func (s *Server) setCloudField(c *Config, field, v string) error {
	v = strings.TrimSpace(v)
	switch field {
	case "appId":
		c.Cloud.AppID = v
	case "secret":
		if v != "" { // empty means "keep"
			c.Cloud.Secret = v
		}
	case "region":
		if v != "eu" && v != "us" && v != "as" && v != "cn" {
			return fmt.Errorf("region must be eu, us, as or cn")
		}
		c.Cloud.Region = v
	case "redirectUrl":
		if v != "" && !strings.HasPrefix(v, "https://") && !strings.HasPrefix(v, "http://") {
			return fmt.Errorf("the redirect address must start with https://")
		}
		c.Cloud.RedirectURL = v
	default:
		return fmt.Errorf("unknown setting %q", "cloud:"+field)
	}
	return nil
}

// logIn trades the pasted login address for tokens, stores the keys of the
// devices that are on the network (or already configured) and keeps the
// login for the consumption.
func (s *Server) logIn(ctx context.Context, pasted string) error {
	cfg := s.cfg.Get()
	if !cfg.Cloud.Ready() {
		return fmt.Errorf("fill in the app ID, secret and redirect address first")
	}
	code, region, err := ParseLoginRedirect(pasted)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cc, err := s.cloud.Exchange(ctx, cfg.Cloud, code, region)
	if err != nil {
		return err
	}
	things, err := s.cloud.Things(ctx, cc)
	if err != nil {
		return err
	}
	onLAN := map[string]bool{}
	for _, d := range s.devices.Snapshot() {
		onLAN[d.ID] = true
	}
	err = s.cfg.Update(func(c *Config) error {
		c.Cloud = cc
		for _, th := range things {
			_, known := c.device(th.ID)
			if !known && !onLAN[th.ID] {
				continue
			}
			i := -1
			for j := range c.Devices {
				if c.Devices[j].ID == th.ID {
					i = j
				}
			}
			if i < 0 {
				c.Devices = append(c.Devices, DeviceConfig{ID: th.ID})
				i = len(c.Devices) - 1
			}
			if th.Key != "" {
				c.Devices[i].Key = th.Key
			}
			if c.Devices[i].Name == "" {
				c.Devices[i].Name = th.Name
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	go energyOnce(context.WithoutCancel(ctx), s.cfg, s.cloud, s.energy)
	_ = s.devices.Refresh(ctx)
	return nil
}

// switchViaCloud is the fallback when the device refuses the LAN command
// (the S60TPF answers every encrypted LAN request with error 400, whatever
// the key — box check 2026-10-16): the same switch through eWeLink's cloud,
// when a login is stored. It waits until the device announces the new state.
func (s *Server) switchViaCloud(ctx context.Context, d Device, on bool) error {
	cfg := s.cfg.Get()
	if !cfg.Cloud.LoggedIn() {
		return fmt.Errorf("the device refused the LAN command and no eWeLink login is stored for the cloud")
	}
	fresh, changed, err := s.cloud.Fresh(ctx, cfg.Cloud)
	if err != nil {
		return err
	}
	if changed {
		_ = s.cfg.Update(func(c *Config) error { c.Cloud = fresh; return nil })
	}
	_, multi := d.State["switches"]
	if err := s.cloud.SetSwitch(ctx, fresh, d.ID, on, multi); err != nil {
		return err
	}
	for i := 0; i < 12; i++ {
		_ = s.devices.Refresh(ctx)
		if now, ok := s.devices.Get(d.ID); ok {
			if isOn, known := now.On(); known && isOn == on {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("the cloud accepted the command but the device didn't report the new state")
}

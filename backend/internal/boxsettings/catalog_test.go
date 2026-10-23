package boxsettings

import (
	"os"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

const fx = "../../testdata/fixtures/vti15_boxsettings/"

// solo2 builds the catalog from the answers recorded on a VU+ Solo2
// (VTi 15, 2026-10-23, expert level).
func solo2(t *testing.T, level string) *Catalog {
	t.Helper()
	open := func(name string) *os.File {
		f, err := os.Open(fx + name)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })
		return f
	}
	var sections []webcontrol.ConfigSection
	{
		// the section list is read the way the client reads it, from the
		// recorded answer's keys and titles
		names := map[string]string{}
		for _, k := range []string{"avsetup", "usage", "vti_movies", "autoshutdown", "vti_audio_setup", "vti_user_interface", "harddisk", "hdmi_cec", "vti_servicelist", "vti_system_setup", "vti_tv_radio", "keyboard", "vti_button_setup", "vti_timer", "vti_timeshift", "transcoding", "subtitlesetup", "timezone"} {
			names[k] = k + " title"
			sections = append(sections, webcontrol.ConfigSection{Key: k, Title: names[k]})
		}
	}
	items := map[string][]webcontrol.ConfigItem{}
	for _, s := range sections {
		list, err := webcontrol.DecodeConfigItems(open("sec_" + s.Key + ".json"))
		if err != nil {
			t.Fatal(err)
		}
		if level != "" {
			for i := range list {
				if list[i].Path == "config.usage.setup_level" {
					list[i].Value = level
				}
			}
		}
		items[s.Key] = list
	}
	setup, err := webcontrol.ParseSetupXML(open("setup.xml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"plugin_HDMICEC.xml", "plugin_AutoShutDown.xml"} {
		more, err := webcontrol.ParseSetupXML(open(p))
		if err != nil {
			t.Fatal(err)
		}
		setup = append(setup, more...)
	}
	return Build(sections, items, setup)
}

func TestBuild_SectionsAndBoxLevel(t *testing.T) {
	c := solo2(t, "")
	secs := c.Sections()
	if len(secs) != 18 {
		t.Fatalf("%d sections", len(secs))
	}
	counts := map[string]int{}
	for _, s := range secs {
		counts[s.Key] = s.Count
	}
	if counts["vti_movies"] != 62 || counts["usage"] != 36 || counts["hdmi_cec"] != 17 {
		t.Errorf("counts: %v", counts)
	}
	if c.BoxLevel != 2 || c.BoxLevelName != "Experte" {
		t.Errorf("box level %d %q", c.BoxLevel, c.BoxLevelName)
	}
	for _, s := range secs {
		if s.HiddenByLevel != 0 {
			t.Errorf("%s hides %d at expert level", s.Key, s.HiddenByLevel)
		}
	}
}

func TestBuild_HiddenByLevelAtASimpleBox(t *testing.T) {
	c := solo2(t, "simple")
	if c.BoxLevel != 0 {
		t.Fatalf("level %d", c.BoxLevel)
	}
	hidden := 0
	for _, s := range c.Sections() {
		hidden += s.HiddenByLevel
	}
	// only a count: the box itself would not list them, here the answer is
	// the expert one, so nothing is "hidden" except items it never lists
	if hidden == 0 {
		t.Error("a simple box shows nothing hidden")
	}
}

func TestBuild_ItemsCarryLevelRestartAndHelp(t *testing.T) {
	c := solo2(t, "")
	_, items, ok := c.Section("vti_tv_radio")
	if !ok {
		t.Fatal("no vti_tv_radio")
	}
	var zap Item
	for _, it := range items {
		if it.Path == "config.usage.channelzap_w_bouquet" {
			zap = it
		}
	}
	if !zap.GuiRestart || !zap.LevelKnown || zap.Level != 0 || zap.SetupText == "" {
		t.Errorf("zap item: %+v", zap)
	}
	_, items, _ = c.Section("subtitlesetup")
	var font Item
	for _, it := range items {
		if it.Path == "config.subtitles.subtitle_fontcolor" {
			font = it
		}
	}
	if font.Help != "Configure the color of the subtitles." {
		t.Errorf("help: %q", font.Help)
	}
	_, items, _ = c.Section("vti_movies")
	headings := 0
	for _, it := range items {
		if it.Kind == webcontrol.KindHeading {
			headings++
		}
	}
	if headings == 0 {
		t.Error("headings were dropped")
	}
}

func TestBuild_RiskMarks(t *testing.T) {
	c := solo2(t, "")
	_, av, _ := c.Section("avsetup")
	marked := 0
	for _, it := range av {
		if it.Kind == webcontrol.KindHeading {
			continue
		}
		if it.Risk != RiskLockout || it.RiskCategory != CatOutput {
			t.Errorf("%s: risk %q %q", it.Path, it.Risk, it.RiskCategory)
		}
		marked++
	}
	if marked == 0 {
		t.Error("no A/V items")
	}
	_, cec, _ := c.Section("hdmi_cec")
	for _, it := range cec {
		if it.Kind != webcontrol.KindHeading && (it.Risk != RiskCareful || it.RiskCategory != CatSideEffects) {
			t.Errorf("%s: %q %q", it.Path, it.Risk, it.RiskCategory)
		}
	}
	_, mv, _ := c.Section("vti_movies")
	for _, it := range mv {
		if it.Risk != RiskNone && !strings.HasPrefix(it.Path, "config.usage.timer_path") && !strings.HasPrefix(it.Path, "config.misc.epgcache") {
			t.Errorf("movie item %s marked %q", it.Path, it.Risk)
		}
	}
}

func TestRiskOf(t *testing.T) {
	for path, want := range map[string]struct {
		r Risk
		c string
	}{
		"config.av.videomode.HDMI":              {RiskLockout, CatOutput},
		"config.plugins.Webinterface.http.port": {RiskLockout, CatAccess},
		"config.network.wol_enabled":            {RiskCareful, CatPower},
		"config.network.ip":                     {RiskLockout, CatAccess},
		"config.Nims.0.dvbs.configMode":         {RiskCareful, CatTuner},
		"config.usage.timeshift_path":           {RiskCareful, CatStorage},
		"config.hdmicec.enabled":                {RiskCareful, CatSideEffects},
		"config.usage.show_infobar_on_zap":      {RiskNone, ""},
	} {
		if r, c := RiskOf(path); r != want.r || c != want.c {
			t.Errorf("%s: %q %q, want %q %q", path, r, c, want.r, want.c)
		}
	}
}

func TestIsSecret(t *testing.T) {
	for path, want := range map[string]bool{
		"config.ParentalControl.setuppin":        true,
		"config.plugins.serienRec.imap_password": true,
		"config.somenew.api_key":                 true,
		"config.OpenWebif.auth_password":         true,
		"config.plugins.Webinterface.http.auth":  false, // a plain yes/no
		"config.usage.ts_use_history_keys":       false,
		"config.keyboard.keymap":                 false,
		"config.recording.margin_before":         false,
		"config.misc.pinned":                     false,
	} {
		if IsSecret(path) != want {
			t.Errorf("%s: %v, want %v", path, !want, want)
		}
	}
}

func TestBuild_SecretsAreNeverCarried(t *testing.T) {
	sections := []webcontrol.ConfigSection{{Key: "x", Title: "X"}}
	items := map[string][]webcontrol.ConfigItem{"x": {
		{Path: "config.ParentalControl.setuppin", Label: "PIN", Kind: webcontrol.KindText, Value: "1234"},
		{Path: "config.ParentalControl.servicepin.0", Label: "empty", Kind: webcontrol.KindText, Value: ""},
	}}
	c := Build(sections, items, nil)
	_, got, _ := c.Section("x")
	if got[0].Value != nil || !got[0].Secret || !got[0].IsSet || got[1].IsSet || got[1].Value != nil {
		t.Errorf("secrets: %+v", got)
	}
}

func TestSearch(t *testing.T) {
	c := solo2(t, "")
	hits := c.Search("margin", 10)
	if len(hits) < 2 {
		t.Fatalf("margin: %d hits", len(hits))
	}
	if !strings.Contains(strings.ToLower(hits[0].Item.Label+hits[0].Item.SetupText+hits[0].Item.Path), "margin") {
		t.Errorf("first hit: %+v", hits[0])
	}
	// the box's own language and the setup file's English both find it
	if h := c.Search("vorlauf", 5); len(h) == 0 || h[0].Item.Path != "config.recording.margin_before" {
		t.Errorf("german label: %+v", h)
	}
	if h := c.Search("margin before", 5); len(h) == 0 || h[0].Item.Path != "config.recording.margin_before" {
		t.Errorf("two words: %+v", h)
	}
	if len(c.Search("zzzznothing", 5)) != 0 || len(c.Search("   ", 5)) != 0 {
		t.Error("search found nothing and blank must stay empty")
	}
	if len(c.Search("a", 3)) != 3 {
		t.Error("limit not applied")
	}
	for _, h := range c.Search("e", 500) {
		if h.Item.Kind == webcontrol.KindHeading {
			t.Fatal("a heading was found")
		}
	}
}

func TestSectionLookup(t *testing.T) {
	c := solo2(t, "")
	if _, _, ok := c.Section("nope"); ok {
		t.Error("unknown section found")
	}
}

func TestRuntimeState(t *testing.T) {
	for path, want := range map[string]bool{
		"config.misc.startCounter":            true,
		"config.tv.lastservice":               true,
		"config.plugins.epgrefresh.lastscan":  true,
		"config.gemininetcast.last_icon_path": true,
		"config.startupservice.lastroot":      true,
		"config.plugins.epgrefresh.interval":  false,
		"config.recording.margin_before":      false,
		"config.usage.show_infobar_on_zap":    false,
	} {
		if RuntimeState(path) != want {
			t.Errorf("%s: %v, want %v", path, !want, want)
		}
	}
}

func TestBuild_TreeSectionsKeepTheirGroupAndAPluginSaidSecret(t *testing.T) {
	sections := []webcontrol.ConfigSection{{Key: "tree.settings.plugins.x", Title: "plugins.x", Group: webcontrol.GroupTree}}
	items := map[string][]webcontrol.ConfigItem{sections[0].Key: {
		{Path: "config.plugins.x.mood", Label: "mood", Kind: webcontrol.KindText, Secret: true, IsSet: true},
		{Path: "config.plugins.x.on", Label: "on", Kind: webcontrol.KindBool, Value: true},
	}}
	c := Build(sections, items, nil)
	info, got, _ := c.Section(sections[0].Key)
	if info.Group != webcontrol.GroupTree || !got[0].Tree || !got[1].Tree {
		t.Errorf("group: %+v %+v", info, got)
	}
	// the plugin's word is enough: the name `mood` is on no list
	if !got[0].Secret || !got[0].IsSet || got[0].Value != nil || got[1].Secret {
		t.Errorf("secret: %+v", got)
	}
}

// Package boxsettings turns what a receiver's setup menus answer
// (webcontrol.BoxSettingsReader) into what DreamHub shows: each setting
// with its level, the risk and restart marks and the box's own help text,
// and a search across every section (docs/BOXSETTINGS_PLAN.md).
package boxsettings

import "strings"

// Risk is how much a wrong value can hurt (plan: "Settings that can make
// the box unusable"). Everything is shown; a marked setting needs an extra
// step before it can be changed (S3).
type Risk string

const (
	RiskNone Risk = ""
	// RiskLockout: a wrong value can leave the TV black or DreamHub unable
	// to reach the box.
	RiskLockout Risk = "A"
	// RiskCareful: a wrong value breaks recording, the tuner, the parental
	// lock or switches other devices — it needs the PIN step only.
	RiskCareful Risk = "B"
)

// Risk categories, the keys of the explanation shown with the mark.
const (
	CatOutput      = "output"      // picture and sound output
	CatAccess      = "access"      // the web interface, network, remote control
	CatTuner       = "tuner"       // tuner, satellite and CI configuration
	CatParental    = "parental"    // parental control and its PIN
	CatPower       = "power"       // standby and wake-up
	CatSideEffects = "sideeffects" // switches the TV or receiver (HDMI-CEC)
	CatStorage     = "storage"     // disks and recording paths
	CatStreaming   = "streaming"   // streaming and transcoding
	CatHardware    = "hardware"    // fans, front panel, picture enhancement
)

type riskRule struct {
	prefix   string
	risk     Risk
	category string
}

// riskRules are matched by key prefix, first match wins; the list grows as
// boxes teach us. Sources: BOXSETTINGS_PLAN.md, "The lists", and the
// notifier survey in ARCHITECTURE.md, "DreamOS config internals".
var riskRules = []riskRule{
	{"config.av.", RiskLockout, CatOutput},
	{"config.osd.", RiskLockout, CatOutput},
	{"config.rfmod.", RiskLockout, CatOutput},
	{"config.plugins.Webinterface.", RiskLockout, CatAccess},
	{"config.OpenWebif.", RiskLockout, CatAccess},
	{"config.network.wol_enabled", RiskCareful, CatPower},
	{"config.network.", RiskLockout, CatAccess},
	{"config.inputDevices.", RiskLockout, CatAccess},
	{"config.Nims.", RiskCareful, CatTuner},
	{"config.sat.", RiskCareful, CatTuner},
	{"config.ci.", RiskCareful, CatTuner},
	{"config.ParentalControl.", RiskCareful, CatParental},
	{"config.parental.", RiskCareful, CatParental},
	{"config.usage.inactivity_shutdown", RiskCareful, CatPower},
	{"config.usage.on_long_powerpress", RiskCareful, CatPower},
	{"config.usage.on_short_powerpress", RiskCareful, CatPower},
	{"config.usage.standby_zaptimer_wakeup", RiskCareful, CatPower},
	{"config.autoshutdown.", RiskCareful, CatPower},
	{"config.cec.", RiskCareful, CatSideEffects},
	{"config.hdmicec.", RiskCareful, CatSideEffects},
	{"config.storage.", RiskCareful, CatStorage},
	{"config.storage_options.", RiskCareful, CatStorage},
	{"config.usage.timer_path", RiskCareful, CatStorage},
	{"config.usage.timeshift_path", RiskCareful, CatStorage},
	{"config.misc.epgcache_filename", RiskCareful, CatStorage},
	{"config.streamservices.", RiskCareful, CatStreaming},
	{"config.plugins.transcodingsetup.", RiskCareful, CatStreaming},
	{"config.plugins.TransCodingSetup.", RiskCareful, CatStreaming},
	{"config.fans.", RiskCareful, CatHardware},
	{"config.pep.", RiskCareful, CatHardware},
}

// RiskOf says how risky a setting is and why; RiskNone and "" when it is
// not on the list.
func RiskOf(path string) (Risk, string) {
	for _, r := range riskRules {
		if strings.HasPrefix(path, r.prefix) {
			return r.risk, r.category
		}
	}
	return RiskNone, ""
}

// secrets are the settings whose value is a password, a PIN or a login —
// by their real names (ARCHITECTURE.md, "Plugin data survey"). The value is
// never sent to the browser; name patterns alone are too broad (a plain
// "key" matches remote-key options, "auth" matches the boolean
// Webinterface.*auth), so this is a list, not a regular expression.
var secrets = map[string]bool{
	"config.ParentalControl.setuppin":        true,
	"config.ParentalControl.servicepin.0":    true,
	"config.ParentalControl.servicepin.1":    true,
	"config.ParentalControl.servicepin.2":    true,
	"config.ParentalControl.storeservicepin": true,
	"config.plugins.serienRec.imap_login":    true,
	"config.plugins.serienRec.imap_password": true,
	"config.plugins.serienRec.imap_server":   true,
	"config.libporn.pincode":                 true,
	"config.gzerotier.apikey":                true,
	"config.serienstream.username":           true,
	"config.serienstream.passwd":             true,
	"config.tunein.username":                 true,
	"config.gemininetscan.samba_username":    true,
	"config.gemininetscan.samba_passwd":      true,
	"config.geminiserver.www_user_password":  true,
	"config.parental.setuppin":               true,
	"config.parental.servicepin":             true,
	"config.OpenWebif.auth_username":         true,
	"config.OpenWebif.auth_password":         true,
}

// secretFragments catch a secret the list does not name yet — whole
// words of the last path part only.
var secretFragments = []string{"password", "passwd", "pincode", "apikey", "api_key", "token", "secret"}

// IsSecret reports whether a setting's value must not be shown: it is on
// the list, or its name holds password, PIN code, token or secret.
func IsSecret(path string) bool {
	if secrets[path] {
		return true
	}
	last := strings.ToLower(path[strings.LastIndex(path, ".")+1:])
	for _, f := range secretFragments {
		if strings.Contains(last, f) {
			return true
		}
	}
	return false
}

// alwaysGuiRestart are settings that need a GUI restart on any image though
// a box's setup.xml says nothing (the skin and style plugins ask for it
// themselves); the VTi ones come from setup.xml (guirestart="yes").
var alwaysGuiRestart = []string{"config.skin.", "config.Styles."}

// NeedsGuiRestart is DreamHub's own list of restart-needing settings.
func NeedsGuiRestart(path string) bool {
	for _, p := range alwaysGuiRestart {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// stateKeys are settings that hold what the box does or did last, not what
// anybody chose (ARCHITECTURE.md, "Plugin data survey"): counters, the last
// channel, wake-up times. Shown, but marked, and left out of any copy.
var stateKeys = []string{
	"config.misc.startCounter", "config.misc.firstrun", "config.misc.defaultchosen",
	"config.misc.prev_wakeup", "config.misc.rcused", "config.misc.initialchannelselection",
	"config.misc.initialharddisknotification", "config.misc.missingdefaultstoragenotification",
	"config.misc.fsbl_update_never", "config.misc.graph_mepg",
	"config.startupservice.", "config.startupserviceleavingstandbymode.",
	"config.servicelist.lastmode", "config.tv.last", "config.radio.last", "config.movielist.last_",
	"config.timezone.version", "config.Webinterface.version",
}

// RuntimeState says whether a setting looks like state the box keeps itself:
// on the list, or a name whose last part starts with "last" (lastservice,
// lastscan, lastcheck, last_icon_path).
func RuntimeState(path string) bool {
	for _, k := range stateKeys {
		if strings.HasPrefix(path, k) {
			return true
		}
	}
	last := path[strings.LastIndex(path, ".")+1:]
	return strings.HasPrefix(strings.ToLower(last), "last")
}

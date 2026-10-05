package settings

import "strings"

// Core setting keys used outside this package.
const (
	KeyLanguage     = "general.language"
	KeyTimezoneMode = "general.timezoneMode"
	KeyTimezone     = "general.timezone"
	KeyAppearance   = "general.appearance"
	KeyNavLabels    = "general.navLabels"

	KeyAuthEnabled  = "auth.enabled"
	KeyAuthUsername = "auth.username"
	KeyAuthPassword = "auth.password"

	// KeyAllowedHosts is a comma-separated list of extra host names DreamHub
	// may be reached by (besides IP addresses, localhost and *.local), e.g.
	// a reverse-proxy name. See PRINCIPLES.md, "Security in the LAN".
	KeyAllowedHosts = "network.allowedHosts"
)

// AllowedHosts splits the network.allowedHosts setting.
func (v Values) AllowedHosts() []string {
	var out []string
	for _, h := range strings.Split(v.String(KeyAllowedHosts), ",") {
		if h = strings.TrimSpace(h); h != "" {
			out = append(out, h)
		}
	}
	return out
}

// CoreSections are the settings owned by the core itself. Hubs and
// connectors register their own sections alongside these.
func CoreSections() []Section {
	return []Section{
		{
			ID:    "general",
			Label: "settings.general",
			Tab:   TabGeneral,
			Fields: []Field{
				{Key: KeyLanguage, Type: TypeEnum, Scope: ScopeUser, Label: "settings.general.language", Default: "en", Options: []string{"de", "it", "en"}},
				{Key: KeyTimezoneMode, Type: TypeEnum, Scope: ScopeUser, Label: "settings.general.timezoneMode", Default: "box", Options: []string{"box", "browser", "fixed"}},
				{Key: KeyTimezone, Type: TypeTimezone, Scope: ScopeUser, Label: "settings.general.timezone", Default: ""},
				{Key: KeyAppearance, Type: TypeEnum, Scope: ScopeUser, Label: "settings.general.appearance", Default: "dark", Options: []string{"dark", "light", "system"}},
				{Key: KeyNavLabels, Type: TypeEnum, Scope: ScopeUser, Label: "settings.general.navLabels", Default: "descriptive", Options: []string{"descriptive", "hub"}},
			},
			Check: func(v Values) map[string]FieldError {
				if v.String(KeyTimezoneMode) == "fixed" && v.String(KeyTimezone) == "" {
					return map[string]FieldError{KeyTimezone: {Code: ErrRequired, Params: map[string]any{"when": KeyTimezoneMode + "=fixed"}}}
				}
				return nil
			},
		},
		{
			ID:    "auth",
			Label: "settings.auth",
			Tab:   TabAccess,
			Fields: []Field{
				{Key: KeyAuthEnabled, Type: TypeBool, Scope: ScopeGlobal, Label: "settings.auth.enabled", Default: false},
				{Key: KeyAuthUsername, Type: TypeString, Scope: ScopeGlobal, Label: "settings.auth.username", Default: ""},
				{Key: KeyAuthPassword, Type: TypePassword, Scope: ScopeGlobal, Label: "settings.auth.password", MinLength: 8},
			},
			// Enabling auth without credentials would lock everyone out.
			Check: func(v Values) map[string]FieldError {
				if !v.Bool(KeyAuthEnabled) {
					return nil
				}
				errs := map[string]FieldError{}
				if v.String(KeyAuthUsername) == "" {
					errs[KeyAuthUsername] = FieldError{Code: ErrRequired, Params: map[string]any{"when": KeyAuthEnabled}}
				}
				if !v.Bool(KeyAuthPassword) {
					errs[KeyAuthPassword] = FieldError{Code: ErrRequired, Params: map[string]any{"when": KeyAuthEnabled}}
				}
				return errs
			},
		},
		{
			ID:    "network",
			Label: "settings.network",
			Tab:   TabAccess,
			Fields: []Field{
				{Key: KeyAllowedHosts, Type: TypeString, Scope: ScopeGlobal, Label: "settings.network.allowedHosts", Default: ""},
			},
		},
	}
}

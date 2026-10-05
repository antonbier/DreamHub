// Package settings implements DreamHub's schema-driven settings: the core and
// every hub declare their settings as a schema, the core validates and stores
// them, and the UI is rendered from the same schema (PRINCIPLES.md,
// "Settings: one central menu, schema-driven, extended per hub").
package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
	_ "time/tzdata" // IANA zones available even if the device lacks /usr/share/zoneinfo
)

// Type is a setting's value type.
type Type string

const (
	TypeString   Type = "string"
	TypeBool     Type = "bool"
	TypeInt      Type = "int"
	TypeEnum     Type = "enum"
	TypeTimezone Type = "timezone" // IANA name, e.g. "Europe/Rome"; empty allowed
	TypeSecret   Type = "secret"   // retrievable (e.g. API keys), never returned in full
	TypePassword Type = "password" // stored as a bcrypt hash, never returned at all
)

// Scope decides whether a setting is shared or per user.
type Scope string

const (
	ScopeGlobal Scope = "global" // one value for everyone; changing it requires the admin role
	ScopeUser   Scope = "user"   // one value per user
)

// Field describes one setting. Labels are i18n keys, not text.
type Field struct {
	Key       string   `json:"key"`
	Type      Type     `json:"type"`
	Scope     Scope    `json:"scope"`
	Label     string   `json:"label"`
	Default   any      `json:"default"`
	Options   []string `json:"options,omitempty"`
	Min       *int64   `json:"min,omitempty"`
	Max       *int64   `json:"max,omitempty"`
	MinLength int      `json:"minLength,omitempty"`
	// Hidden fields are kept and validated like any other but not shown by
	// the generic settings form: a panel of their own edits them (Plex's
	// library choice) or nobody does (an identifier made once).
	Hidden bool `json:"hidden,omitempty"`
}

// Tabs of the settings menu, in the order the menu shows them. A section
// names its tab; the system tab holds only panels the frontend places
// itself (backup, diagnostics), no schema sections.
const (
	TabGeneral       = "general"
	TabRecordings    = "recordings"
	TabServices      = "services"
	TabNotifications = "notifications"
	TabHubs          = "hubs"
	TabAccess        = "access"
	TabSystem        = "system"
)

// Section groups fields in the settings menu (general, auth, one per hub…).
type Section struct {
	ID    string
	Label string
	// Tab is the settings tab the section shows on (Tab*); empty means
	// TabServices, where connectors live.
	Tab    string
	Fields []Field
	// Check validates rules spanning several fields against the values that
	// would be saved. It returns field key → error.
	Check func(v Values) map[string]FieldError
}

// Values are effective setting values: string, bool, int64, or — for
// password fields — a bool telling whether a password is set.
type Values map[string]any

func (v Values) String(k string) string { s, _ := v[k].(string); return s }
func (v Values) Bool(k string) bool     { b, _ := v[k].(bool); return b }
func (v Values) Int(k string) int64     { i, _ := v[k].(int64); return i }

// FieldError is a validation problem, expressed as a code (translated by the
// client) plus parameters — never as finished text.
type FieldError struct {
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}

// Validation error codes.
const (
	ErrUnknown         = "unknown"
	ErrForbidden       = "forbidden"
	ErrInvalidType     = "invalid_type"
	ErrInvalidOption   = "invalid_option"
	ErrOutOfRange      = "out_of_range"
	ErrTooShort        = "too_short"
	ErrInvalidTimezone = "invalid_timezone"
	ErrRequired        = "required"
	ErrInvalidURL      = "invalid_url"
)

// Registry holds all registered sections, in registration order.
type Registry struct {
	sections []Section
	fields   map[string]Field
	owner    map[string]string // field key → section ID
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{fields: map[string]Field{}, owner: map[string]string{}}
}

// Register adds a section. Field keys must be unique across all sections.
func (r *Registry) Register(s Section) error {
	for _, f := range s.Fields {
		if _, dup := r.fields[f.Key]; dup {
			return fmt.Errorf("settings: duplicate key %q", f.Key)
		}
		if f.Type == TypeEnum && len(f.Options) == 0 {
			return fmt.Errorf("settings: enum %q has no options", f.Key)
		}
	}
	for _, f := range s.Fields {
		r.fields[f.Key] = f
		r.owner[f.Key] = s.ID
	}
	r.sections = append(r.sections, s)
	return nil
}

// Field returns a registered field.
func (r *Registry) Field(key string) (Field, bool) {
	f, ok := r.fields[key]
	return f, ok
}

// Sections returns all registered sections.
func (r *Registry) Sections() []Section { return r.sections }

// parse validates a raw JSON value against a field. A JSON null means
// "reset to default" (or "clear" for secrets and passwords) and returns nil.
func parse(f Field, raw json.RawMessage) (any, *FieldError) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	switch f.Type {
	case TypeBool:
		var b bool
		if json.Unmarshal(raw, &b) != nil {
			return nil, &FieldError{Code: ErrInvalidType, Params: map[string]any{"expected": "bool"}}
		}
		return b, nil
	case TypeInt:
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var n json.Number
		if dec.Decode(&n) != nil {
			return nil, &FieldError{Code: ErrInvalidType, Params: map[string]any{"expected": "int"}}
		}
		i, err := n.Int64()
		if err != nil {
			return nil, &FieldError{Code: ErrInvalidType, Params: map[string]any{"expected": "int"}}
		}
		if (f.Min != nil && i < *f.Min) || (f.Max != nil && i > *f.Max) {
			return nil, &FieldError{Code: ErrOutOfRange, Params: map[string]any{"min": f.Min, "max": f.Max}}
		}
		return i, nil
	default:
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return nil, &FieldError{Code: ErrInvalidType, Params: map[string]any{"expected": "string"}}
		}
		switch f.Type {
		case TypeEnum:
			for _, o := range f.Options {
				if s == o {
					return s, nil
				}
			}
			return nil, &FieldError{Code: ErrInvalidOption, Params: map[string]any{"options": f.Options}}
		case TypeTimezone:
			if s == "" {
				return s, nil
			}
			if _, err := time.LoadLocation(s); err != nil || s == "Local" {
				return nil, &FieldError{Code: ErrInvalidTimezone}
			}
			return s, nil
		}
		if len([]rune(s)) < f.MinLength {
			return nil, &FieldError{Code: ErrTooShort, Params: map[string]any{"minLength": f.MinLength}}
		}
		return s, nil
	}
}

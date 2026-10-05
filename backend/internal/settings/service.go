package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"

	"github.com/antonbier/DreamHub/backend/internal/auth"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// ValidationError lists every invalid field of a rejected update. Nothing is
// saved when an update returns this error.
type ValidationError struct {
	Fields map[string]FieldError
}

func (e *ValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return "settings: invalid " + strings.Join(keys, ", ")
}

// Service reads, validates and writes settings.
type Service struct {
	store *store.Store
	reg   *Registry

	mu       sync.RWMutex
	authConf auth.Config
	version  uint64
}

// Version increases with every successful update, so caches derived from
// settings (e.g. connector states) know when to recompute.
func (s *Service) Version() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.version
}

// Values returns a user's effective values (their own plus global ones),
// including unmasked secrets. For use inside the core only — never send the
// result to a client; use View for that.
func (s *Service) Values(ctx context.Context, u store.User) (Values, error) {
	stored, err := s.stored(ctx, u)
	if err != nil {
		return nil, err
	}
	v, _ := s.effective(stored, u)
	return v, nil
}

// NewService returns a Service and loads the current auth configuration.
func NewService(ctx context.Context, st *store.Store, reg *Registry) (*Service, error) {
	s := &Service{store: st, reg: reg}
	if err := s.reloadAuth(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// FieldView is a field as shown to a client: the schema plus the effective
// value. Secrets are masked, passwords are never included.
type FieldView struct {
	Field
	Value     any  `json:"value"`
	HasValue  bool `json:"hasValue"`
	IsDefault bool `json:"isDefault"`
}

// SectionView is a section as shown to a client.
type SectionView struct {
	ID     string      `json:"id"`
	Label  string      `json:"label"`
	Tab    string      `json:"tab"`
	Fields []FieldView `json:"fields"`
}

func scopeID(f Field, u store.User) int64 {
	if f.Scope == ScopeGlobal {
		return store.GlobalScope
	}
	return u.ID
}

// stored loads the raw stored values relevant to a user (their own plus
// global ones).
func (s *Service) stored(ctx context.Context, u store.User) (map[int64]map[string]store.SettingValue, error) {
	global, err := s.store.Settings(ctx, store.GlobalScope)
	if err != nil {
		return nil, err
	}
	own, err := s.store.Settings(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return map[int64]map[string]store.SettingValue{store.GlobalScope: global, u.ID: own}, nil
}

func decodeStored(f Field, raw store.SettingValue) any {
	switch f.Type {
	case TypeBool:
		var b bool
		_ = json.Unmarshal(raw, &b)
		return b
	case TypeInt:
		var i int64
		_ = json.Unmarshal(raw, &i)
		return i
	default:
		var str string
		_ = json.Unmarshal(raw, &str)
		return str
	}
}

// effective returns the effective values for a user: stored or default.
// Password fields hold whether a password is set.
func (s *Service) effective(stored map[int64]map[string]store.SettingValue, u store.User) (Values, map[string]bool) {
	v := Values{}
	isSet := map[string]bool{}
	for _, sec := range s.reg.Sections() {
		for _, f := range sec.Fields {
			raw, ok := stored[scopeID(f, u)][f.Key]
			isSet[f.Key] = ok
			switch {
			case f.Type == TypePassword:
				v[f.Key] = ok
			case ok:
				v[f.Key] = decodeStored(f, raw)
			default:
				v[f.Key] = f.Default
			}
		}
	}
	return v, isSet
}

func mask(secret string) string {
	r := []rune(secret)
	if len(r) <= 4 {
		return "••••"
	}
	return "••••" + string(r[len(r)-4:])
}

// View returns all sections with the user's effective values.
func (s *Service) View(ctx context.Context, u store.User) ([]SectionView, error) {
	stored, err := s.stored(ctx, u)
	if err != nil {
		return nil, err
	}
	vals, isSet := s.effective(stored, u)
	out := []SectionView{}
	for _, sec := range s.reg.Sections() {
		tab := sec.Tab
		if tab == "" {
			tab = TabServices
		}
		sv := SectionView{ID: sec.ID, Label: sec.Label, Tab: tab, Fields: []FieldView{}}
		for _, f := range sec.Fields {
			fv := FieldView{Field: f, HasValue: isSet[f.Key], IsDefault: !isSet[f.Key]}
			switch f.Type {
			case TypePassword:
				fv.Value = nil
			case TypeSecret:
				if str, _ := vals[f.Key].(string); str != "" {
					fv.Value = mask(str)
				}
			default:
				fv.Value = vals[f.Key]
			}
			sv.Fields = append(sv.Fields, fv)
		}
		out = append(out, sv)
	}
	return out, nil
}

// Update validates and saves several settings at once. Either all changes
// are saved (and recorded in the change log) or none are.
func (s *Service) Update(ctx context.Context, u store.User, raw map[string]json.RawMessage) error {
	stored, err := s.stored(ctx, u)
	if err != nil {
		return err
	}
	vals, _ := s.effective(stored, u)

	errs := map[string]FieldError{}
	parsed := map[string]any{}
	for key, rv := range raw {
		f, ok := s.reg.Field(key)
		if !ok {
			errs[key] = FieldError{Code: ErrUnknown}
			continue
		}
		if f.Scope == ScopeGlobal && u.Role != store.RoleAdmin {
			errs[key] = FieldError{Code: ErrForbidden}
			continue
		}
		v, ferr := parse(f, rv)
		if ferr != nil {
			errs[key] = *ferr
			continue
		}
		parsed[key] = v
	}

	// Merge into the would-be-saved values, then run cross-field checks.
	for key, v := range parsed {
		f, _ := s.reg.Field(key)
		switch {
		case f.Type == TypePassword:
			vals[key] = v != nil
		case v == nil:
			vals[key] = f.Default
		default:
			vals[key] = v
		}
	}
	for _, sec := range s.reg.Sections() {
		if sec.Check == nil {
			continue
		}
		for k, e := range sec.Check(vals) {
			if _, already := errs[k]; !already {
				errs[k] = e
			}
		}
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}

	keys := make([]string, 0, len(parsed))
	for k := range parsed {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	type change struct {
		Key  string `json:"key"`
		From any    `json:"from"`
		To   any    `json:"to"`
	}
	var writes []store.SettingWrite
	var changes []change
	for _, key := range keys {
		f, _ := s.reg.Field(key)
		v := parsed[key]
		old, had := stored[scopeID(f, u)][key]

		w := store.SettingWrite{UserID: scopeID(f, u), Key: key}
		if v != nil {
			toStore := v
			if f.Type == TypePassword {
				hash, err := bcrypt.GenerateFromPassword([]byte(v.(string)), bcrypt.DefaultCost)
				if err != nil {
					return fmt.Errorf("settings: hashing password: %w", err)
				}
				toStore = string(hash)
			}
			enc, err := json.Marshal(toStore)
			if err != nil {
				return err
			}
			w.Value = enc
		}
		writes = append(writes, w)

		// The change log never contains secret values.
		c := change{Key: key}
		if f.Type == TypeSecret || f.Type == TypePassword {
			c.From = map[bool]string{true: "(set)", false: "(not set)"}[had]
			c.To = map[bool]string{true: "(changed)", false: "(cleared)"}[v != nil]
		} else {
			if had {
				c.From = decodeStored(f, old)
			} else {
				c.From = f.Default
			}
			if v != nil {
				c.To = v
			} else {
				c.To = f.Default
			}
		}
		changes = append(changes, c)
	}
	if len(writes) == 0 {
		return nil
	}

	detail, err := json.Marshal(map[string]any{"changes": changes})
	if err != nil {
		return err
	}
	entry := store.ChangeEntry{
		UserID:  u.ID,
		Action:  "settings.update",
		Summary: "Updated settings: " + strings.Join(keys, ", "),
		Detail:  detail,
	}
	if err := s.store.WriteSettings(ctx, writes, entry); err != nil {
		return err
	}
	s.mu.Lock()
	s.version++
	s.mu.Unlock()
	return s.reloadAuth(ctx)
}

// AuthConfig implements auth.ConfigSource.
func (s *Service) AuthConfig() auth.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authConf
}

func (s *Service) reloadAuth(ctx context.Context) error {
	global, err := s.store.Settings(ctx, store.GlobalScope)
	if err != nil {
		return err
	}
	var c auth.Config
	if raw, ok := global[KeyAuthEnabled]; ok {
		_ = json.Unmarshal(raw, &c.Enabled)
	}
	if raw, ok := global[KeyAuthUsername]; ok {
		_ = json.Unmarshal(raw, &c.Username)
	}
	if raw, ok := global[KeyAuthPassword]; ok {
		_ = json.Unmarshal(raw, &c.PasswordHash)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	c.Version = s.authConf.Version + 1
	s.authConf = c
	return nil
}

// IsValidation reports whether err is a *ValidationError.
func IsValidation(err error) (*ValidationError, bool) {
	var v *ValidationError
	ok := errors.As(err, &v)
	return v, ok
}

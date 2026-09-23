package settings

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/store"
)

// testSection adds a secret and a bounded int, which the core sections
// don't have yet (they arrive with the AI connector and hubs).
var testSection = Section{
	ID:    "test",
	Label: "settings.test",
	Fields: []Field{
		{Key: "test.apiKey", Type: TypeSecret, Scope: ScopeGlobal, Label: "k"},
		{Key: "test.limit", Type: TypeInt, Scope: ScopeGlobal, Label: "l", Default: int64(100), Min: ptr(0), Max: ptr(1000)},
	},
}

func ptr(v int64) *int64 { return &v }

func newService(t *testing.T) (*Service, *store.Store, store.User) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	u, err := st.EnsureDefaultUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry()
	for _, s := range append(CoreSections(), testSection) {
		if err := reg.Register(s); err != nil {
			t.Fatal(err)
		}
	}
	svc, err := NewService(ctx, st, reg)
	if err != nil {
		t.Fatal(err)
	}
	return svc, st, u
}

func vals(m map[string]string) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for k, v := range m {
		out[k] = json.RawMessage(v)
	}
	return out
}

func field(t *testing.T, view []SectionView, key string) FieldView {
	t.Helper()
	for _, s := range view {
		for _, f := range s.Fields {
			if f.Key == key {
				return f
			}
		}
	}
	t.Fatalf("field %s not in view", key)
	return FieldView{}
}

func codeOf(t *testing.T, err error, key string) string {
	t.Helper()
	v, ok := IsValidation(err)
	if !ok {
		t.Fatalf("expected validation error, got %v", err)
	}
	return v.Fields[key].Code
}

func TestRegister_RejectsDuplicateKeys(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(testSection); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(testSection); err == nil {
		t.Fatal("duplicate keys must be rejected")
	}
}

func TestValidation(t *testing.T) {
	svc, _, u := newService(t)
	ctx := context.Background()
	cases := []struct {
		key, value, code string
	}{
		{"general.appearance", `"neon"`, ErrInvalidOption},
		{"general.appearance", `true`, ErrInvalidType},
		{"general.timezone", `"Mars/Olympus"`, ErrInvalidTimezone},
		{"test.limit", `1001`, ErrOutOfRange},
		{"test.limit", `1.5`, ErrInvalidType},
		{"auth.password", `"short"`, ErrTooShort},
		{"does.not.exist", `1`, ErrUnknown},
	}
	for _, c := range cases {
		err := svc.Update(ctx, u, vals(map[string]string{c.key: c.value}))
		if got := codeOf(t, err, c.key); got != c.code {
			t.Errorf("%s=%s: expected %s, got %s", c.key, c.value, c.code, got)
		}
	}
}

func TestTimezone_FixedModeRequiresZone(t *testing.T) {
	svc, _, u := newService(t)
	ctx := context.Background()
	err := svc.Update(ctx, u, vals(map[string]string{"general.timezoneMode": `"fixed"`}))
	if codeOf(t, err, "general.timezone") != ErrRequired {
		t.Fatalf("fixed mode without a zone must be rejected, got %v", err)
	}
	if err := svc.Update(ctx, u, vals(map[string]string{"general.timezoneMode": `"fixed"`, "general.timezone": `"Europe/Rome"`})); err != nil {
		t.Fatalf("fixed mode with Europe/Rome must be accepted: %v", err)
	}
}

func TestSecret_IsMaskedAndLoggedWithoutValue(t *testing.T) {
	svc, st, u := newService(t)
	ctx := context.Background()
	if err := svc.Update(ctx, u, vals(map[string]string{"test.apiKey": `"sk-verysecret-ab12"`})); err != nil {
		t.Fatal(err)
	}
	view, _ := svc.View(ctx, u)
	f := field(t, view, "test.apiKey")
	if f.Value != "••••ab12" || !f.HasValue {
		t.Errorf("secret must be masked, got %+v", f)
	}
	changes, _ := st.Changes(ctx, 1)
	if strings.Contains(string(changes[0].Detail), "verysecret") {
		t.Errorf("change log must not contain secret values: %s", changes[0].Detail)
	}
}

func TestPassword_StoredAsHashNeverReturned(t *testing.T) {
	svc, st, u := newService(t)
	ctx := context.Background()
	if err := svc.Update(ctx, u, vals(map[string]string{"auth.password": `"correct horse"`})); err != nil {
		t.Fatal(err)
	}
	raw, err := st.Setting(ctx, store.GlobalScope, "auth.password")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "correct horse") || !strings.HasPrefix(strings.Trim(string(raw), `"`), "$2") {
		t.Errorf("password must be stored as a bcrypt hash, got %s", raw)
	}
	view, _ := svc.View(ctx, u)
	if f := field(t, view, "auth.password"); f.Value != nil || !f.HasValue {
		t.Errorf("password must not be returned, only hasValue: %+v", f)
	}
}

func TestAuth_CannotEnableWithoutCredentials(t *testing.T) {
	svc, _, u := newService(t)
	ctx := context.Background()
	err := svc.Update(ctx, u, vals(map[string]string{"auth.enabled": `true`}))
	if codeOf(t, err, "auth.username") != ErrRequired || codeOf(t, err, "auth.password") != ErrRequired {
		t.Fatalf("expected username and password to be required, got %v", err)
	}
	if svc.AuthConfig().Enabled {
		t.Fatal("auth must not be enabled after a rejected update")
	}
	if err := svc.Update(ctx, u, vals(map[string]string{"auth.enabled": `true`, "auth.username": `"armin"`, "auth.password": `"correct horse"`})); err != nil {
		t.Fatal(err)
	}
	c := svc.AuthConfig()
	if !c.Enabled || c.Username != "armin" || c.PasswordHash == "" {
		t.Errorf("auth config not reloaded: %+v", c)
	}
}

func TestGlobalSettings_RequireAdmin(t *testing.T) {
	svc, _, u := newService(t)
	u.Role = store.RoleUser
	err := svc.Update(context.Background(), u, vals(map[string]string{"auth.enabled": `false`}))
	if codeOf(t, err, "auth.enabled") != ErrForbidden {
		t.Fatalf("non-admins must not change global settings, got %v", err)
	}
}

func TestReset_NullRestoresDefault(t *testing.T) {
	svc, _, u := newService(t)
	ctx := context.Background()
	svc.Update(ctx, u, vals(map[string]string{"general.language": `"it"`}))
	if err := svc.Update(ctx, u, vals(map[string]string{"general.language": `null`})); err != nil {
		t.Fatal(err)
	}
	view, _ := svc.View(ctx, u)
	if f := field(t, view, "general.language"); f.Value != "en" || !f.IsDefault {
		t.Errorf("null must restore the default, got %+v", f)
	}
}

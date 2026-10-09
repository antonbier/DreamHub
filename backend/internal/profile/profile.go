// Package profile manages DreamHub's profiles and the sessions that pick
// one on a device (docs/archive/PROFILES_PLAN.md, P2). Profiles are DreamHub's own
// state, like settings, so changes are saved directly and written to the
// change log rather than going through preview and confirm.
//
// Two layers decide who a request is: the unchanged Basic Auth / API token
// "door" (internal/auth), then the profile. A household with a single
// profile and no PIN never sees a picker — DreamHub behaves as before
// profiles existed.
package profile

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/antonbier/DreamHub/backend/internal/perm"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// Errors.
var (
	ErrNotFound         = errors.New("profile: not found")
	ErrWrongPin         = errors.New("profile: wrong PIN")
	ErrLastAdmin        = errors.New("profile: the last admin can't be removed or demoted")
	ErrAdminPinRequired = errors.New("profile: admins need a PIN once there's more than one profile")
	ErrSelf             = errors.New("profile: you can't delete your own profile")
)

// LockedError: too many wrong PINs; try again after RetryAfter.
type LockedError struct{ RetryAfter time.Duration }

func (e *LockedError) Error() string { return "profile: locked for " + e.RetryAfter.String() }

// ValidationError lists invalid fields by name with a code.
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return fmt.Sprintf("profile: invalid fields %v", e.Fields) }

// Colors are the picker's color tokens (DESIGN.md accent palette).
var Colors = []string{"blue", "green", "amber", "red", "violet", "teal", "pink", "slate"}

// SessionIdle is how long an unused session stays valid ("remember on the
// device").
const SessionIdle = 180 * 24 * time.Hour

// touchEvery limits how often a session's last use is written.
const touchEvery = time.Hour

// Wrong-PIN lockout: free tries, then a doubling pause up to the cap.
const (
	freeTries = 5
	firstLock = 30 * time.Second
	maxLock   = 15 * time.Minute
)

// Profile is a profile as clients see it — never the PIN hash.
type Profile struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Role        store.Role      `json:"role"`
	Color       string          `json:"color"`
	HasPin      bool            `json:"hasPin"`
	Overrides   map[string]bool `json:"overrides"`
	Permissions []perm.Perm     `json:"permissions"`
}

// View turns a stored user into a Profile.
func View(u store.User) Profile {
	o := u.Overrides
	if o == nil {
		o = map[string]bool{}
	}
	return Profile{ID: u.ID, Name: u.Name, Role: u.Role, Color: u.Color, HasPin: u.HasPin(),
		Overrides: o, Permissions: perm.For(u).List()}
}

// Service manages profiles and sessions.
type Service struct {
	store *store.Store
	now   func() time.Time

	mu    sync.Mutex
	fails map[int64]*failState
}

type failState struct {
	count int
	until time.Time
}

// NewService returns a Service.
func NewService(st *store.Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: st, now: now, fails: map[int64]*failState{}}
}

// List returns every profile.
func (s *Service) List(ctx context.Context) ([]Profile, error) {
	users, err := s.store.Users(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Profile, 0, len(users))
	for _, u := range users {
		out = append(out, View(u))
	}
	return out, nil
}

// Single returns the household's only profile when no picker is needed:
// exactly one profile, without a PIN.
func (s *Service) Single(ctx context.Context) (store.User, bool, error) {
	users, err := s.store.Users(ctx)
	if err != nil {
		return store.User{}, false, err
	}
	if len(users) == 1 && !users[0].HasPin() {
		return users[0], true, nil
	}
	return store.User{}, false, nil
}

// StartSession picks a profile on a device, checking its PIN, and returns
// the session token the client sends from now on.
func (s *Service) StartSession(ctx context.Context, id int64, pin string) (string, store.User, error) {
	u, err := s.store.User(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return "", store.User{}, ErrNotFound
	}
	if err != nil {
		return "", store.User{}, err
	}
	if u.HasPin() {
		if err := s.checkPin(u, pin); err != nil {
			return "", store.User{}, err
		}
	}
	token := randomHex(32)
	_ = s.store.DeleteProfileSessionsBefore(ctx, s.now().Add(-SessionIdle))
	if err := s.store.CreateProfileSession(ctx, u.ID, hash(token)); err != nil {
		return "", store.User{}, err
	}
	return token, u, nil
}

func (s *Service) checkPin(u store.User, pin string) error {
	s.mu.Lock()
	f := s.fails[u.ID]
	if f != nil && s.now().Before(f.until) {
		wait := f.until.Sub(s.now())
		s.mu.Unlock()
		return &LockedError{RetryAfter: wait}
	}
	s.mu.Unlock()

	// bcrypt outside the lock: it's slow on the box's cores on purpose.
	ok := bcrypt.CompareHashAndPassword([]byte(u.PinHash), []byte(pin)) == nil

	s.mu.Lock()
	defer s.mu.Unlock()
	if ok {
		delete(s.fails, u.ID)
		return nil
	}
	f = s.fails[u.ID]
	if f == nil {
		f = &failState{}
		s.fails[u.ID] = f
	}
	f.count++
	if f.count >= freeTries {
		lock := firstLock << (f.count - freeTries)
		if lock > maxLock || lock <= 0 {
			lock = maxLock
		}
		f.until = s.now().Add(lock)
	}
	return ErrWrongPin
}

// Resolve returns the profile a session token belongs to.
func (s *Service) Resolve(ctx context.Context, token string) (store.User, error) {
	if token == "" {
		return store.User{}, ErrNotFound
	}
	h := hash(token)
	id, last, err := s.store.ProfileSessionUser(ctx, h)
	if err != nil {
		return store.User{}, ErrNotFound
	}
	now := s.now()
	if now.Sub(last) > SessionIdle {
		_ = s.store.DeleteProfileSession(ctx, h)
		return store.User{}, ErrNotFound
	}
	if now.Sub(last) > touchEvery {
		_ = s.store.TouchProfileSession(ctx, h)
	}
	u, err := s.store.User(ctx, id)
	if err != nil {
		return store.User{}, ErrNotFound
	}
	return u, nil
}

// EndSession signs a device out of its profile.
func (s *Service) EndSession(ctx context.Context, token string) error {
	return s.store.DeleteProfileSession(ctx, hash(token))
}

// Input is a profile to create, or the fields to change (nil: unchanged).
type Input struct {
	Name  *string     `json:"name"`
	Role  *store.Role `json:"role"`
	Color *string     `json:"color"`
	// Pin sets a new PIN; "" removes it.
	Pin       *string         `json:"pin"`
	Overrides map[string]bool `json:"overrides"`
}

// Create adds a profile.
func (s *Service) Create(ctx context.Context, actor store.User, in Input) (Profile, error) {
	u := store.User{Role: store.RoleUser, Color: Colors[0]}
	if in.Name == nil {
		empty := ""
		in.Name = &empty
	}
	if err := apply(&u, in); err != nil {
		return Profile{}, err
	}
	users, err := s.store.Users(ctx)
	if err != nil {
		return Profile{}, err
	}
	if err := checkInvariants(append(users, u)); err != nil {
		return Profile{}, err
	}
	created, err := s.store.CreateUserLike(ctx, u, actor.ID)
	if errors.Is(err, store.ErrNameTaken) {
		return Profile{}, &ValidationError{Fields: map[string]string{"name": "taken"}}
	}
	if err != nil {
		return Profile{}, err
	}
	s.log(ctx, actor, "profile.create", fmt.Sprintf("Created profile %q", created.Name), created)
	return View(created), nil
}

// Update changes a profile.
func (s *Service) Update(ctx context.Context, actor store.User, id int64, in Input) (Profile, error) {
	users, err := s.store.Users(ctx)
	if err != nil {
		return Profile{}, err
	}
	idx := -1
	for i, u := range users {
		if u.ID == id {
			idx = i
		}
	}
	if idx < 0 {
		return Profile{}, ErrNotFound
	}
	u := users[idx]
	if err := apply(&u, in); err != nil {
		return Profile{}, err
	}
	users[idx] = u
	if err := checkInvariants(users); err != nil {
		return Profile{}, err
	}
	if err := s.store.UpdateUser(ctx, u); err != nil {
		if errors.Is(err, store.ErrNameTaken) {
			return Profile{}, &ValidationError{Fields: map[string]string{"name": "taken"}}
		}
		return Profile{}, err
	}
	s.mu.Lock()
	delete(s.fails, u.ID) // a new PIN starts with a clean slate
	s.mu.Unlock()
	s.log(ctx, actor, "profile.update", fmt.Sprintf("Changed profile %q", u.Name), u)
	return View(u), nil
}

// Delete removes a profile.
func (s *Service) Delete(ctx context.Context, actor store.User, id int64) error {
	if id == actor.ID {
		return ErrSelf
	}
	users, err := s.store.Users(ctx)
	if err != nil {
		return err
	}
	var gone *store.User
	rest := []store.User{}
	for i := range users {
		if users[i].ID == id {
			gone = &users[i]
		} else {
			rest = append(rest, users[i])
		}
	}
	if gone == nil {
		return ErrNotFound
	}
	if err := checkInvariants(rest); err != nil {
		return err
	}
	if err := s.store.DeleteUser(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	s.log(ctx, actor, "profile.delete", fmt.Sprintf("Deleted profile %q", gone.Name), *gone)
	return nil
}

// apply validates in and copies it onto u.
func apply(u *store.User, in Input) error {
	errs := map[string]string{}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		switch {
		case name == "":
			errs["name"] = "required"
		case utf8.RuneCountInString(name) > 30:
			errs["name"] = "tooLong"
		default:
			u.Name = name
		}
	}
	if in.Role != nil {
		if perm.ValidRole(*in.Role) {
			u.Role = *in.Role
		} else {
			errs["role"] = "invalid"
		}
	}
	if in.Color != nil {
		if contains(Colors, *in.Color) {
			u.Color = *in.Color
		} else {
			errs["color"] = "invalid"
		}
	}
	if in.Pin != nil {
		switch pin := *in.Pin; {
		case pin == "":
			u.PinHash = ""
		case !validPin(pin):
			errs["pin"] = "format"
		default:
			h, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			u.PinHash = string(h)
		}
	}
	if in.Overrides != nil {
		o := map[string]bool{}
		for k, v := range in.Overrides {
			if !perm.IsSwitchable(k) {
				errs["overrides"] = "invalid"
				continue
			}
			o[k] = v
		}
		u.Overrides = o
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

// checkInvariants: at least one admin, and every admin has a PIN once
// there's more than one profile — otherwise anyone could pick the admin.
func checkInvariants(users []store.User) error {
	admins := 0
	for _, u := range users {
		if u.Role == store.RoleAdmin {
			admins++
			if len(users) > 1 && !u.HasPin() {
				return ErrAdminPinRequired
			}
		}
	}
	if admins == 0 {
		return ErrLastAdmin
	}
	return nil
}

// validPin: 4 to 8 digits.
func validPin(p string) bool {
	if len(p) < 4 || len(p) > 8 {
		return false
	}
	for _, c := range p {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (s *Service) log(ctx context.Context, actor store.User, action, summary string, u store.User) {
	detail, _ := json.Marshal(map[string]any{"id": u.ID, "name": u.Name, "role": u.Role, "hasPin": u.HasPin(), "overrides": u.Overrides})
	_ = s.store.AppendChange(ctx, store.ChangeEntry{UserID: actor.ID, Action: action, Summary: summary, Detail: detail})
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

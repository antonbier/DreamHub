package boxes

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/antonbier/DreamHub/backend/internal/box"
	"github.com/antonbier/DreamHub/backend/internal/peer"
	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// Errors of the box management.
var (
	ErrUnauthorized = errors.New("boxes: the box wants a login, or the one given is wrong")
	ErrUnreachable  = errors.New("boxes: the box does not answer")
	ErrMainBox      = errors.New("boxes: the first box can't be removed")
	ErrNoFactory    = errors.New("boxes: no way to connect to further boxes")
)

// ValidationError lists invalid fields by name with a code.
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "boxes: invalid fields" }

// Colors are the box colours, the same palette as the profiles'.
var Colors = []string{"blue", "green", "amber", "red", "violet", "teal", "pink", "slate"}

// TestResult is what asking a box about itself yields.
type TestResult struct {
	DeviceName string
	Tuners     int
}

// Input is what a client sends to add or change a box; nil keeps the
// current value.
type Input struct {
	Name     *string `json:"name"`
	Color    *string `json:"color"`
	URL      *string `json:"url"`
	Username *string `json:"username"`
	Password *string `json:"password"`
	// StreamBase: how that box reaches this DreamHub; "" = detect.
	StreamBase *string `json:"streamBase"`
	// Power-on method (M9): WakeMethod is "", "wol" or "extension".
	WakeMethod *string `json:"wakeMethod"`
	WakeMAC    *string `json:"wakeMac"`
	WakeExt    *string `json:"wakeExt"`
	WakeAction *string `json:"wakeAction"`
}

// Manager adds, changes and removes boxes while DreamHub runs
// (docs/MULTIDEVICE_PLAN.md, M3).
type Manager struct {
	Store    *store.Store
	Registry *Registry
	// Factory builds the connections to a box from its row.
	Factory func(store.Box) (Clients, error)
	// Peer pairs with the DreamHub on a box (M6); nil turns pairing off.
	Peer peer.Client
	// Timeout bounds a connection test; 8 s when zero.
	Timeout time.Duration
	// SendWOL sends the Wake-on-LAN packet; SendMagicPacket when nil (tests
	// and mock mode replace it, nothing should hit the network there).
	SendWOL func(mac string) error
	// OnWake is told when a box was just switched on, so it is looked at
	// more often for a while (the poller's back-off).
	OnWake func(id string)
}

// Test connects to the box described by b and reads its device info —
// what the "Test" button shows. ErrUnauthorized for a login problem,
// ErrUnreachable for everything else.
func (m *Manager) Test(ctx context.Context, b store.Box) (TestResult, error) {
	if m.Factory == nil {
		return TestResult{}, ErrNoFactory
	}
	c, err := m.Factory(b)
	if err != nil {
		return TestResult{}, ErrUnreachable
	}
	d := m.Timeout
	if d <= 0 {
		d = 8 * time.Second
	}
	ctx, cancel := context.WithTimeout(box.With(ctx, b.ID), d)
	defer cancel()
	info, err := c.WC.DeviceInfo(ctx)
	switch {
	case errors.Is(err, webcontrol.ErrUnauthorized):
		return TestResult{}, ErrUnauthorized
	case err != nil:
		return TestResult{}, ErrUnreachable
	}
	return TestResult{DeviceName: info.DeviceName, Tuners: info.Tuners()}, nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slug turns a name into a box ID that is not taken yet.
func (m *Manager) slug(name string) string {
	base := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if base == "" {
		base = "box"
	}
	id := base
	for n := 2; ; n++ {
		if _, taken := m.Registry.Get(id); !taken {
			return id
		}
		id = base + "-" + itoa(n)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// normalizeURL accepts "192.168.1.20" or "http://host:80/" and returns
// "http://host:port"; "" when it is not an address.
func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return ""
		}
		if (u.Scheme == "http" && n == 80) || (u.Scheme == "https" && n == 443) {
			return u.Scheme + "://" + host
		}
		return u.Scheme + "://" + host + ":" + p
	}
	return u.Scheme + "://" + host
}

// addressTaken reports whether another box already has this address: two
// entries for one box would poll it twice and split its data.
func (m *Manager) addressTaken(b store.Box) bool {
	for _, e := range m.Registry.List() {
		if e.ID != b.ID && !e.Local && e.URL != "" && e.URL == b.URL {
			return true
		}
	}
	return false
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// apply validates in and copies it onto b. main: the first box's address
// and login live in the configuration, so only its name and colour change.
func apply(b *store.Box, in Input, main bool) error {
	errs := map[string]string{}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		switch {
		case name == "":
			errs["name"] = "required"
		case utf8.RuneCountInString(name) > 30:
			errs["name"] = "tooLong"
		default:
			b.Name = name
		}
	}
	if in.Color != nil {
		if contains(Colors, *in.Color) {
			b.Color = *in.Color
		} else {
			errs["color"] = "invalid"
		}
	}
	if !main {
		if in.URL != nil {
			if u := normalizeURL(*in.URL); u != "" {
				b.URL = u
			} else {
				errs["url"] = "invalid"
			}
		}
		if in.Username != nil {
			b.Username = strings.TrimSpace(*in.Username)
		}
		if in.Password != nil {
			b.Password = *in.Password
		}
		if in.StreamBase != nil {
			if sb := strings.TrimSpace(*in.StreamBase); sb == "" {
				b.StreamBase = ""
			} else if u := normalizeURL(sb); u != "" {
				b.StreamBase = u
			} else {
				errs["streamBase"] = "invalid"
			}
		}
	}
	applyWake(b, in, errs)
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

// applyWake validates the power-on fields: a method needs what it works
// with, and switching to "none" forgets the rest.
func applyWake(b *store.Box, in Input, errs map[string]string) {
	if in.WakeMethod == nil && in.WakeMAC == nil && in.WakeExt == nil && in.WakeAction == nil {
		return
	}
	w := *b
	if in.WakeMethod != nil {
		w.WakeMethod = strings.TrimSpace(*in.WakeMethod)
	}
	if in.WakeMAC != nil {
		w.WakeMAC = strings.TrimSpace(*in.WakeMAC)
	}
	if in.WakeExt != nil {
		w.WakeExt = strings.TrimSpace(*in.WakeExt)
	}
	if in.WakeAction != nil {
		w.WakeAction = strings.TrimSpace(*in.WakeAction)
	}
	switch w.WakeMethod {
	case WakeNone:
		w.WakeMAC, w.WakeExt, w.WakeAction = "", "", ""
	case WakeWOL:
		w.WakeExt, w.WakeAction = "", ""
		if w.WakeMAC = ParseMAC(w.WakeMAC); w.WakeMAC == "" {
			errs["wakeMac"] = "invalid"
		}
	case WakeExtension:
		w.WakeMAC = ""
		if w.WakeExt == "" || w.WakeAction == "" {
			errs["wakeAction"] = "required"
		}
	default:
		errs["wakeMethod"] = "invalid"
	}
	if len(errs) == 0 || (errs["wakeMac"] == "" && errs["wakeAction"] == "" && errs["wakeMethod"] == "") {
		b.WakeMethod, b.WakeMAC, b.WakeExt, b.WakeAction = w.WakeMethod, w.WakeMAC, w.WakeExt, w.WakeAction
	}
}

// Candidate returns the box a form describes — without saving it — for
// testing before it is added. Only the fields given are validated.
func (m *Manager) Candidate(in Input) (store.Box, error) {
	b := store.Box{ID: "test"}
	if err := apply(&b, in, false); err != nil {
		return store.Box{}, err
	}
	if b.URL == "" {
		return store.Box{}, &ValidationError{Fields: map[string]string{"url": "required"}}
	}
	return b, nil
}

// Add tests and registers a new box. It must answer: an unreachable or
// unauthorised box is refused (ErrUnreachable, ErrUnauthorized).
func (m *Manager) Add(ctx context.Context, in Input) (Entry, error) {
	b := store.Box{Color: Colors[len(m.Registry.List())%len(Colors)]}
	if in.Name == nil {
		empty := ""
		in.Name = &empty
	}
	if err := apply(&b, in, false); err != nil {
		return Entry{}, err
	}
	if b.URL == "" {
		return Entry{}, &ValidationError{Fields: map[string]string{"url": "required"}}
	}
	b.ID = m.slug(b.Name)
	if m.addressTaken(b) {
		return Entry{}, &ValidationError{Fields: map[string]string{"url": "taken"}}
	}
	if _, err := m.Test(ctx, b); err != nil {
		return Entry{}, err
	}
	c, err := m.Factory(b)
	if err != nil {
		return Entry{}, ErrUnreachable
	}
	if err := m.Store.SaveBox(ctx, b); err != nil {
		return Entry{}, err
	}
	m.Registry.Add(b, c, false)
	e, _ := m.Registry.Get(b.ID)
	return e, nil
}

// Update changes a box. A changed address or login is tested first.
func (m *Manager) Update(ctx context.Context, id string, in Input) (Entry, error) {
	e, ok := m.Registry.Get(id)
	if !ok {
		return Entry{}, ErrUnknownBox
	}
	b := e.Box
	main := id == m.Registry.DefaultID()
	if err := apply(&b, in, main); err != nil {
		return Entry{}, err
	}
	if !main && b.URL != e.URL && m.addressTaken(b) {
		return Entry{}, &ValidationError{Fields: map[string]string{"url": "taken"}}
	}
	clients := e.Clients
	if !main && (b.URL != e.URL || b.Username != e.Username || b.Password != e.Password) {
		if _, err := m.Test(ctx, b); err != nil {
			return Entry{}, err
		}
		c, err := m.Factory(b)
		if err != nil {
			return Entry{}, ErrUnreachable
		}
		clients = c
	}
	if err := m.Store.SaveBox(ctx, b); err != nil {
		return Entry{}, err
	}
	m.Registry.Add(b, clients, e.Local)
	e, _ = m.Registry.Get(id)
	return e, nil
}

// Remove unregisters a box and deletes everything DreamHub keeps for it;
// the profiles' lists forget it. The first box stays (ErrMainBox).
func (m *Manager) Remove(ctx context.Context, id string) error {
	if id == m.Registry.DefaultID() {
		return ErrMainBox
	}
	if _, ok := m.Registry.Get(id); !ok {
		return ErrUnknownBox
	}
	if err := m.Store.DeleteBox(ctx, id); err != nil {
		return err
	}
	m.Registry.Remove(id)
	users, err := m.Store.Users(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		kept := u.Boxes[:0:0]
		for _, b := range u.Boxes {
			if b != id {
				kept = append(kept, b)
			}
		}
		if len(kept) != len(u.Boxes) {
			if len(kept) == 0 { // never widen a restriction to "all"
				kept = []string{m.Registry.DefaultID()}
			}
			u.Boxes = kept
			if err := m.Store.UpdateUser(ctx, u); err != nil {
				return err
			}
		}
	}
	return nil
}

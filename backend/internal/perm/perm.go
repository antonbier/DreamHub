// Package perm is DreamHub's permission model (docs/PROFILES_PLAN.md, P1):
// code asks "may this user do X?", never "is this user an admin?". A role
// is a named set of permissions defined here and nowhere else, so a new
// role (a kids role, later) is one more entry in roles below — no hub
// changes. A viewer profile can additionally have a few switches flipped
// by an admin (Switchable); everything else follows the role.
package perm

import "github.com/antonbier/DreamHub/backend/internal/store"

// Perm is one thing a user may do.
type Perm string

// The permissions. Reads of the everyday hubs need Watch; everything that
// changes something names what it changes.
const (
	// Watch: see the everyday hubs — live TV, recordings, guide, Plex,
	// series, radio — and the user's own dashboard, notifications and
	// personal settings.
	Watch Perm = "watch"
	// Control: the box's remote, switching channels, messages on the TV.
	Control Perm = "control"
	// PlexUse: play a Plex film on the box, mark it watched, sync.
	PlexUse Perm = "plex.use"
	// Discovery: ask the AI for recommendations (costs money; budgeted).
	Discovery Perm = "discovery"
	// TimersPlan: plan recordings, and delete or change the timers this
	// user made through DreamHub.
	TimersPlan Perm = "timers.plan"
	// TimersManage: delete or switch off any timer on the box.
	TimersManage Perm = "timers.manage"
	// RecordingsManage: rename, trash, restore and edit recordings.
	RecordingsManage Perm = "recordings.manage"
	// SettingsGlobal: settings shared by everyone, connectors, hub switches.
	SettingsGlobal Perm = "settings.global"
	// System: SystemHub, AdminHub, the change log, API tokens, diagnostics.
	System Perm = "system"
	// Profiles: create, change and delete profiles.
	Profiles Perm = "profiles"
)

// all lists every permission, in display order.
var all = []Perm{Watch, Control, PlexUse, Discovery, TimersPlan, TimersManage,
	RecordingsManage, SettingsGlobal, System, Profiles}

// All returns every permission.
func All() []Perm { return append([]Perm(nil), all...) }

// roles defines what each role may do. Admin may do everything.
var roles = map[store.Role][]Perm{
	store.RoleAdmin: all,
	store.RoleUser:  {Watch, Control, PlexUse, Discovery, TimersPlan},
}

// Switchable are the permissions an admin can grant or take away per
// viewer profile (docs/PROFILES_PLAN.md: "a few, not a matrix").
var Switchable = []Perm{TimersPlan, RecordingsManage}

// Set is a user's effective permissions.
type Set map[Perm]bool

// Has reports whether the set contains p.
func (s Set) Has(p Perm) bool { return s[p] }

// List returns the set's permissions in display order.
func (s Set) List() []Perm {
	out := []Perm{}
	for _, p := range all {
		if s[p] {
			out = append(out, p)
		}
	}
	return out
}

// ForRole returns a role's permissions; an unknown role gets none.
func ForRole(r store.Role) Set {
	s := Set{}
	for _, p := range roles[r] {
		s[p] = true
	}
	return s
}

// For returns a user's effective permissions: the role's, with the
// profile's switches applied. Admins ignore switches — an admin who can't
// manage something could lock everyone out of it.
func For(u store.User) Set {
	s := ForRole(u.Role)
	if u.Role == store.RoleAdmin {
		return s
	}
	for _, p := range Switchable {
		if on, ok := u.Overrides[string(p)]; ok {
			s[p] = on
		}
	}
	return s
}

// ValidRole reports whether r is a role profiles can have.
func ValidRole(r store.Role) bool { _, ok := roles[r]; return ok }

// IsSwitchable reports whether p can be switched per profile.
func IsSwitchable(p string) bool {
	for _, s := range Switchable {
		if string(s) == p {
			return true
		}
	}
	return false
}

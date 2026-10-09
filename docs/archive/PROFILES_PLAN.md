# Profiles, roles and the viewer UI — plan

Status: **built.** P1, P2, P4 and P5 on 2026-10-07 (released in 0.2.0); P3 finished on 2026-10-08 (everyday error texts with "Details", viewers' sidebar without setup state). Still open: the per-profile bouquet switch — it needs ChannelHub, the next topic — and the kids role ("Later" below). Tracks issue #39. Once a package lands, its status moves to `ROADMAP.md` like the `V1_PLAN.md` packages did.

## Why now

With StreamHub (live TV), PlexHub and the recordings player, DreamHub stopped being an admin tool only the owner uses: the rest of the household watches TV and films with it on their phones. They still see everything the admin sees — logs, diagnostics, settings, delete buttons, beta features. Profiles and roles were prepared in the data model from day one (`PRINCIPLES.md`, "Profiles, roles and dashboards"); this plan builds them, together with a deliberately simple interface for people who just want to watch.

## Decisions

### Two roles now, a third later

- **Admin**: everything, as today.
- **Viewer** ("Zuschauer"): watching and light everyday use. Agreed list:
  - *may*: live TV, Plex, watch recordings, TV guide, remote control, switch channel, **plan recordings** (still preview and confirm), delete timers *they created themselves through DreamHub*, their own dashboard and personal settings (language, appearance, display time zone)
  - *may not*: delete, rename or trash recordings; delete or change other timers; global settings; AdminHub, SystemHub, diagnostics, logs; experimental features
- **Kids** — later, not part of this plan's packages (see "Later" below). The permission model must make adding it a matter of one more role definition, nothing else.

People are **profiles**, not roles: "Anna" and "Ben" are two profiles with the viewer role. A role is a permission set; a profile is a person with a role.

### Permissions, not role comparisons

Every check asks "may this user do X?" (`perm.TimersPlan`, `perm.RecordingsDelete`, `perm.SettingsGlobal`, …), never "is this user an admin?". A role is a named set of permissions defined in one place in the backend. Today's `hub.Visible(min, role)` (a two-level rank) is replaced by a hub's required permission.

### Per-profile switches: a few, not a matrix

A full per-user permission grid was considered and rejected: twenty checkboxes per person become unexplainable within months. Instead, the role is a template and the admin can flip **a handful of switches per profile** that come up in real households:

- plan recordings (on/off)
- delete recordings (on/off)
- which hubs are shown (e.g. hide Plex for one profile)
- which bouquets are shown

A profile that differs from its role's template says so on the profile page, with "Reset to role defaults". Nothing else is configurable per profile.

### Signing in: a door and a profile

Two independent layers:

1. **The door** — the existing Basic Auth / API tokens (`PRINCIPLES.md`, "Authentication"). Off by default on the LAN, as today; on for access from outside (e.g. ZeroTier). Unchanged.
2. **The profile** — who is using DreamHub. A profile picker (name + color initial, no photos), Netflix-style:
   - a profile may have a **PIN**; **admin profiles must have one** as soon as a second profile exists
   - PINs are bcrypt-hashed like passwords and rate limited (5 wrong tries → growing lockout), since 4–6 digits are guessable otherwise
   - the choice is remembered on the device; switching is in the user menu
   - choosing a profile yields a profile session token the frontend sends as a header, like everything else it sends (`frontend/src/api/client.ts` already sends every request — images included — through `fetch` with headers), so no cookies and no new CSRF surface
   - the server enforces the profile's permissions on every request; hiding a button is never the protection

**Migration / single-user households**: the existing `admin` user stays. While it is the only profile and has no PIN, there is no picker and nothing changes — DreamHub behaves exactly as today. Creating the first second profile asks for an admin PIN.

This is also a security gain: today, with the door off, anyone on the LAN is admin; afterwards they're at most a viewer unless they know the admin PIN.

API tokens belong to a profile (existing ones to the admin). The Enigma2 plugin only uses public endpoints and is unaffected.

### Enforcement: default deny for changes

Every route declares its permission when a hub mounts it. **A mutating route without a declared permission is admin-only**, and a test walks all registered routes to make sure each mutating one has a permission — so a new hub can't accidentally hand viewers a delete button. Plans (`plan` → `apply`) check the permission of the plan's kind at both steps; plans already belong to their creator.

Timers have no owner on the box. DreamHub records which profile created a timer through it (service reference + begin + end) so "delete timers you created" works; timers made on the TV or by AutoTimer/SerienRecorder are nobody's and stay admin-only.

### Settings

The schema already splits settings into `global` (admin only) and `user` scope (`internal/settings/schema.go`). Viewers see only their own `user`-scope sections; the settings page renders from the schema, so this is a filter, not a second page.

### Dashboards: a template, freely adjustable, never by accident

- The admin designs a **viewer template** dashboard.
- A viewer's dashboard *is* the template until they change it; their first change makes a personal copy. **"Reset to template"** deletes the copy — a messed-up start page is one tap from fixed.
- Changing anything requires the explicit **"Customize start page"** mode that exists today; normal mode has no drag handles and no hide buttons. That is the "not by accident" requirement.

### New tiles

Priorities from the owner: recent recordings and "now on TV" yes, more and configurable Plex tiles yes, radio low.

- **Now on TV** (built-in): a large channel strip — picon, what's on, progress — tap to watch in the browser, long press / secondary action to switch the TV. Built on StreamHub's channel list.
- **Recent recordings** (built-in): newest recordings as poster/thumbnail cards, tap to watch in the browser. Today this is already possible as a custom list tile over the `recordings` view source; the built-in version gives it the visual treatment viewers expect.
- **Plex, configurable**: a new Plex **view source** for custom tiles (`internal/hub/view.go`), so the admin or a viewer can add as many Plex rows as they like: library or collection, "recently added", "unwatched", sort; plus a **poster row display** for custom tiles. "Continue watching" (`plex.continue`) stays as is.

### Error messages in plain words

Viewers see everyday wording ("The box doesn't answer right now", "The tuner is busy with a recording"); technical detail (error code, request ID) moves behind a "Details" disclosure — for everyone, but the admin gets it expanded. Many hubs already translate their error codes (e.g. `stream.error.noTuner`); this is a pass over the remaining ones, not new machinery.

## Packages

| # | Package | Contents | Done when |
|---|---|---|---|
| P1 | Permission model | permission constants, role definitions, route-level declarations, default-deny for mutating routes, route-walking test, hub visibility by permission | all existing routes declare a permission; admin behavior unchanged |
| P2 | Profiles and sessions | profile CRUD (admin), PINs with rate limiting, profile session tokens, picker, remember-on-device, switch in user menu, tokens per profile, single-user migration with no visible change | a second profile can sign in as viewer and is refused admin routes by the server |
| P3 | Viewer interface | navigation and search by permission, settings filtered to personal scope, delete/rename/system actions absent, plain-language error pass with "Details" | a viewer sees no admin, system or destructive UI anywhere |
| P4 | Start page | viewer template, copy-on-customize, reset to template; tiles "Now on TV", "Recent recordings"; Plex view source and poster-row display | a viewer's start page works out of the box and survives "customize" mishaps |
| P5 | Per-profile switches and timer ownership | the four switches, "differs from role" indicator, timer ownership records | a viewer can delete their own timers but not others' |

P1 and P2 come first (P2 depends on P1). P3–P5 can follow in any order; P4 is where most of the visible value for the household is.

## Later (not in these packages)

- **Kids role.** Feasible parts: allowed bouquets/channels (in the guide, live TV, remote zapping through DreamHub), Plex with a maximum content rating (Plex carries one per title, e.g. `de/12`), no recording planning. Not reliable: age ratings for broadcasts and recordings — DVB carries an optional parental-rating field in the EIT, but many German broadcasters don't fill it, and it's unconfirmed whether the box's `/web/*` API passes it on (a recording's `.eit` file may have it — to be checked read-only on the box). A filter that has to guess for every broadcast without a rating is either leaky or empty, and the TV remote has no such lock anyway. Parental controls as a feature stay out of scope (`PROJECT_PLAN.md`); this is the honest subset.
- **Plex watched state per profile** (Plex Home users). Until then all profiles share the one Plex account's watched state and "continue watching".
- **Native/Capacitor app and HTTPS via an own domain** — parked until this plan is through (owner's decision 2026-10-07). Its trigger: whether HD H.264 channels stutter on phones over plain `http://`, where WebCodecs isn't available.

## Answered by the owner (2026-10-07)

1. Viewers may delete timers they created themselves through DreamHub — yes.
2. The chosen profile is remembered on the device; switching is in the user menu.
3. Viewers may switch the TV's channel any time, recording or not: the box itself refuses switches the tuner can't serve, so nothing breaks.

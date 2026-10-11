# AdminHub — plan

Status: **decided 2026-10-11, not yet built.** Tracks issue #47; the terminal piece was already tracked separately as #36 and keeps that number. Once a package lands, its status moves to `ROADMAP.md`.

## Why

The box's native WebAdmin already does package/feed management, script upload and a terminal — but with none of DreamHub's safety net: no preview, no confirm, no record of what happened, and a flat list of hundreds of cryptically-named packages with no way to tell which one you actually need. The goal isn't to out-feature WebAdmin; it's to bring the one everyday admin job that still has no safe mobile interface up to DreamHub's usual standard — the same motivation `CHANNELHUB_PLAN.md` states for bouquets. Terminal stays the deliberate exception to preview-and-confirm (`PRINCIPLES.md`); everything else here gets it.

## Decisions

### Categories come from apt's own metadata, never a bundled lookup

Checked live on the reference box (read-only, 2026-10-11): every installed package already carries a real `Section` field from its feed maintainer (`plugin-skincomponents`, `console/network`, `devel/python`, `base`, …). Grouping the package list by this field needs no DreamHub-side category database to keep in sync — a brand-new package with a `Section` DreamHub has never seen just becomes a new group on the fly, never an unknown/broken state.

For the handful of packages DreamHub specifically cares about (its own package, Extension Hub packages, picon sets), apt's generic `Section` isn't distinctive enough (`dreamhub` itself is just `Section: web`). These get a custom control-file field instead (`X-DreamHub-Category: self|extension|picons`) — set only on packages DreamHub or an extension author controls, so it's never a source of truth DreamHub has to maintain for packages it doesn't own. A package without the field simply falls back to its plain apt `Section`.

### The full package list, filterable, not a curated subset

All system packages are shown (not just DreamHub-related ones) — hiding packages the box happens to need would just send the owner back to WebAdmin for anything outside DreamHub's curated view, defeating the point. Filters: free text, category (from the rule above), status (installed / upgradable / not installed), feed/origin, and a quick filter "needed by a DreamHub hub" (reads the same `requirements` a hub already declares in the hub contract — ffmpeg, BouquetEditor, …).

### Install/remove as a plan, with a real preview

`apt-get install -s` / `apt-get remove -s` (simulate) can list exactly what else an action pulls in or drops before anything happens — that answer becomes the plan's preview text, through the same plan → apply flow as everywhere else in DreamHub. A package action that would touch a running service enigma2 depends on reuses the existing safety check DreamHub's own `postinst` already does for its own updates (never restart such a service while recording/playing) — generalized here to any package, not just DreamHub's.

Because `apt` upgrades aren't atomically reversible like a BouquetEditor restore, DreamHub records the exact previous version before an upgrade (not just a change-log line) so a downgrade, if ever needed, is a concrete prepared step instead of guesswork.

### Feeds, with a trust signal

Feeds get the same honesty upgrade as packages: name, purpose, package count, and a visible distinction between known feeds (the device's own, DreamHub's, the Extension Hub's) and a feed someone added by hand — flagged, since adding a feed on a box with root access is a real supply-chain decision, not a casual toggle. Enabling/disabling a feed is a plan like any other change.

### Scripts stay in AdminHub; AutomationHub reuses them, doesn't re-build them

A script someone uploads and runs by hand is an admin action (manual, confirmed, admin-only) — not automation. AdminHub owns the library: upload, store, run with a required confirmation and a change-log entry (name, who, when, exit code). `AutomationHub` (#35, later) can wire the same stored script as a rule's action ("if disk > 90%, run cleanup.sh") instead of needing its own script storage — one library, two ways to trigger it.

### Update notifications, not a badge you have to go find

Pending package updates surface through the core's existing notifications inbox (the same mechanism behind "disk above 90%", a timer conflict, a hub going unavailable) — found out about, not hunted for, which is the one thing WebAdmin categorically can't do.

### A package change re-checks only the hubs it could affect

Re-running the self-test after every package action would be noise (most of the box's ~800 installed packages have nothing to do with DreamHub). Instead: if the package just touched is one a hub's own `requirements` names, re-check that hub's status only. Everything else triggers nothing.

### AI explains a package, on request

Reusing the existing AI-provider connector (already used by DiscoveryHub) for an on-demand "what does this package do?" button — apt descriptions for feed packages are often terse or assume context (`gp4-picon-hotbird13e`). Opt-in like DiscoveryHub's own AI use, nothing sent unless asked.

### SystemHub and AdminHub merge into one nav entry — "Gerät"

Both are about *this device* — SystemHub (status, diagnostics, change log, everyone) and AdminHub (actions that change the device, admin only) are closer to each other than either is to Settings (configuration, a different mental model, and already its own well-working role-gated-tabs entry — not merged further). The merged entry uses the same pattern Settings already proves: one nav position, tabs that differ by role. Descriptive label **"Gerät"**, deliberately not "System" (too close to "Settings"/"Einstellungen" to tell apart at a glance); fixed hub name `DeviceHub` (not `SystemHub`, which stays retired to avoid confusion with the old name in old links/docs).

Tabs: *Übersicht & Diagnose* (today's SystemHub content, everyone), *Administration* (today's AdminHub + packages/feeds/scripts/terminal, admin-only — hidden entirely for anyone else, same as AdminHub's role gate today).

**When this merge is actually built, don't forget:** the existing `/system?tab=diagnostics` link from `SettingsView.vue`; any other in-app link to `/system` or an AdminHub route; the old-hash-link redirect pattern `SettingsView.vue` already uses for `/settings#tmdb` — the same treatment for `/system`/`/admin`; every mention of `SystemHub`/`AdminHub` by name in `CONCEPT.md` and `ROADMAP.md`.

### Extensions reach the dashboard through the existing tile mechanism, not a nav slot each

Settled while planning the Extension Hub (`docs/EXTENSIONHUB_PLAN.md`): an installed extension doesn't get its own top-level nav entry. It can contribute a dashboard tile (the hub contract's existing tile mechanism) and react to events — the same two extension points a hub already has. A later, more granular option — a hub declaring **named extension points** of its own (a specific button slot in a specific list, SAP UI5-style) — is worth keeping in mind but not designing yet; it would need real extensions in use first to know what's actually wanted.

### Who may

Package/feed/script actions need `system` (the same permission that already gates AdminHub and Settings → System today) — no new permission. Visibility of the Administration tab is binary (admin or not); no partial "can see but not act" tier — decided against on purpose, to keep the permission model from growing a second axis nobody asked for.

## Packages

| # | Package | Contents |
|---|---|---|
| A1 | DeviceHub merge | SystemHub + AdminHub → one nav entry, role-gated tabs, the redirect/link cleanup above |
| A2 | Packages & feeds | full package list (apt-Section categories, filters, AI explain), install/remove as a plan with simulate-preview and the generalized "don't restart a service while recording" check, feed list with a trust signal, update notifications, targeted self-test re-check |
| A3 | Scripts | upload, store, run with confirm + change-log entry |
| A4 | Terminal (#36) | `shellinaboxd` proxied behind DreamHub's auth, off by default, re-auth required, session start/end logged |

A1 can be built independently of A2–A4 (it's a navigation change, not new capability). A2's categorization and filtering can be built and shown against a mock package list; the install/remove plan and the "don't restart while recording" check need the box to confirm `apt`'s simulate output and service-restart behavior match what's assumed here.

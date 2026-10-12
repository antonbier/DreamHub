# Building a DreamHub extension

This is for anyone who wants to add something to DreamHub that isn't already there — a bridge to another system (a smart switch, a sensor, another box), or a self-hosted app you'd like to see in AppsHub's catalog. It explains the two different ways to do that, and walks through building and packaging a real extension.

## Which one do you want?

DreamHub has three ways something new can show up in it, and they're not interchangeable:

| You want to… | Build | Where |
| --- | --- | --- |
| Show a status, a number, a short list, or one button that does something — for a device or service DreamHub doesn't know about | **An extension** | AppsHub → Extensions tab |
| Run a self-hosted app (your own, or one you'd like added) as a container | **Nothing — it's curated** | AppsHub → Apps tab |
| Add a genuinely new area of DreamHub itself (a new hub) | **A core contribution** | talk to the maintainers first |

**Extensions** are what the rest of this document covers: a separate process you write and run, registering with DreamHub and describing a small widget-based UI. This is the real, open plugin mechanism (`docs/EXTENSIONHUB_PLAN.md`, #46) — nobody has to ask permission to write one, and it never touches DreamHub's own code.

**AppsHub's curated catalog** (`docs/APPSHUB_PLAN.md`, App3) is *not* an extension mechanism, on purpose — see "AI is always optional" in `PRINCIPLES.md`: the list (`backend/internal/appcatalog`) is plain, hand-maintained project data, deliberately lean, with no submission API. If you want your own self-hosted app added to it, that's a pull request adding an entry to `appcatalog.Apps` with its Docker Hub image, confirmed architectures, container port and config path — not a separate process to build. Free search in that same tab already lets anyone check whether *any* Docker Hub image would run on a given box's processor; only installing one is still curated-only today.

A genuinely new **hub** (its own nav entry, its own page, its own backend package) is a much bigger thing — it means contributing to DreamHub's own core, following `CONCEPT.md`'s hub contract and `PRINCIPLES.md` throughout. Open an issue and talk it through before writing code; this document doesn't cover that path.

The rest of this guide is about the first one: **building an extension.**

## What an extension actually is

A separate OS process — your own binary, your own small HTTP server, bound to `127.0.0.1` only, never reachable from the LAN or the internet. DreamHub's own backend is the only thing that ever calls it; it never calls out anywhere else DreamHub proxies for it. **An extension never ships any frontend code** — `ExtensionsHubView.vue` is the one generic renderer every extension's widgets go through, the same component for all of them.

At a high level:

1. On startup, it registers itself with DreamHub once.
2. From then on, DreamHub's backend calls it, never the other way around: "what do you look like right now" (polled fresh on every page load, no push/webhook mechanism today) and "do this" (through the same preview-then-confirm flow every built-in hub's own actions use).
3. If your process isn't running, or answers something DreamHub doesn't understand, its card shows "unavailable" with a reason — it doesn't break anything else.

## 1. Register

```
POST http://127.0.0.1:8080/api/v1/extensions/register
Content-Type: application/json

{
  "id": "my-extension",
  "name": "My Extension",
  "icon": "control",
  "admin": false,
  "schemaVersion": 1,
  "baseUrl": "http://127.0.0.1:9100"
}
```

- `id` — stable and unique across extensions, e.g. `"ewelink"`.
- `name` — shown next to your widgets, literal text (an extension owns no DreamHub locale file, unlike a built-in hub's label — there's no i18n key to localize this into).
- `icon` — one of the names `frontend/src/components/Icon.vue` recognizes; anything else falls back to a generic extension icon.
- `admin` — `true` if only admins should see your card (the same binary on/off choice a built-in hub's own permission makes — there's no finer-grained permission an extension can ask for).
- `schemaVersion` — the widget schema version you're writing against; `1` today (see "Versioning" below).
- `baseUrl` — where DreamHub should reach *you*, e.g. `http://127.0.0.1:9100`. Must itself be a loopback address — DreamHub checks this and the request's own origin, and refuses anything else, the same loopback-only check `PlexHub`'s own box-side media handoff uses.

The call answers `200 {"registered": true, "coreSchemaVersion": 1}` on success. There's no DreamHub login involved — the handshake is public but gated entirely by the fact that only a process already running on this device can reach `127.0.0.1`.

Registration isn't persisted by DreamHub across a restart (the same as your own process restarting loses nothing either way) — register again on startup, every time.

## 2. Answer DreamHub's three calls

All under your own `baseUrl`:

### `GET /widgets` → `200`, a JSON array

Called fresh on every `GET /api/v1/extensions` — no caching on DreamHub's side, so answer quickly (DreamHub itself gives up after 5 seconds and shows your card unreachable). Each element is one widget, one of four shapes:

| Type | Fields used | What it shows |
| --- | --- | --- |
| `"status"` | `label`, `value`, `tone` (`"good"`, `"warn"`, `"bad"`) | A one-line colored status, e.g. "Switch: On" |
| `"meter"` | `label`, `current`, `max`, `unit` | A bar/gauge, e.g. battery or signal level |
| `"list"` | `label`, `rows: [{label, value, icon?}]` | A short table of rows |
| `"action"` | `label`, `actionId`, `buttonLabel` | A button; pressing it starts the preview→confirm flow below, calling your `actionId` |

```json
[
  {"type": "status", "label": "Switch", "value": "On", "tone": "good"},
  {"type": "action", "label": "Switch", "actionId": "toggle", "buttonLabel": "Turn off"}
]
```

Every field not used by a given `type` is simply omitted — the renderer only looks at the ones its type defines, and it skips a widget of a `type` it doesn't recognize (a newer extension against an older DreamHub) rather than showing it broken.

### `POST /actions/{actionId}/plan` → `200`, an `ActionPreview`

Called when someone presses your action button, *before* anything happens — this is the "preview" half of preview-and-confirm. Return what pressing it would actually do, in words, plus a fingerprint of your current state:

```json
{"summary": "Turn the switch off.", "fingerprint": "on-7"}
```

`fingerprint` is yours to define (a revision counter, a hash of your state — anything that changes whenever the thing this action would affect changes). It's shown to the user as part of the preview and sent back unchanged on apply.

### `POST /actions/{actionId}/apply` with `{"fingerprint": "..."}` → `200`, or `409`

Called only after the person confirms. **Check the fingerprint against your current state before doing anything** — if it doesn't match what you'd hand out right now, your own state moved since the preview (someone else flipped the switch from its own app, say), and the honest answer is `409` — DreamHub shows this as "that changed since you looked, try again," never silently reapplying a now-stale preview. Apply the change and return `200` only once it's actually done.

```go
// Pseudocode for the toggle example above
func applyToggle(fingerprint string) (status int) {
    if fingerprint != currentFingerprint() {
        return 409 // stale — refuse, don't guess
    }
    flipTheSwitch()
    return 200
}
```

## Versioning: additive only

`schemaVersion` only ever grows when DreamHub's core adds a new, *optional* widget type or field — it never removes or redefines one. Write your extension against the version you need; it keeps working against every newer DreamHub forever, because a newer core still understands the older shape, it just also understands more now. If you ever see DreamHub advertise a `coreSchemaVersion` higher than what you wrote against, there's nothing to do — your extension still works exactly as before, there's just more available if you want it.

## A minimal worked example

[`extensions/ewelink/`](../extensions/ewelink/) is a complete, real extension — its own Go module, its own `.deb` built from the template, discovery, encryption, a fingerprint-checked switch action; start there. `backend/internal/extension/mock/mock.go` in this repo is a smaller, in-process implementation of exactly this contract — a one-socket "smart switch" with a status widget and a toggle action, fingerprint-checked apply included. Reading it is the fastest way to see the whole contract in ~70 lines of Go; the shape translates directly to any language, since it's all just JSON over HTTP.

## 3. Package it (optional, but the convention for distribution)

`packaging/extension-template/` has everything to build your extension as its own installable `.deb`, matching how DreamHub packages itself:

- `control.template` — the `.deb`'s own control file (fill in your id, version, architecture, maintainer).
- `extension.service` — a systemd unit that runs your process as its **own dedicated system user** (created by `postinst.template`, removed only on purge), hardened (`NoNewPrivileges`, a read-only view of the system outside its own state directory, every Linux capability dropped, network restricted to loopback only via `RestrictAddressFamilies`/`IPAddressAllow=localhost`). This isn't optional boilerplate — it's what makes "an extension can only ever reach `127.0.0.1`" actually true at the OS level, not just a convention your code happens to follow.
- `postinst.template` / `prerm.template` / `postrm.template` — install/remove lifecycle, matching DreamHub's own packaging conventions (a plain removal keeps your extension's own state; only a purge deletes it).

See `packaging/extension-template/README.md` for the exact placeholders and how to fill each one in — it's the authoritative reference for packaging, this document only summarizes it.

Extensions are meant to eventually live on their own apt feed (`dreamhub-extensions-feed`, already recognized by DreamHub's own feed list and package browser via the `X-DreamHub-Category` control-file field) — standing up that feed is an operational step, not something your own `.deb` needs to do.

## 4. Try it against a mock DreamHub

You don't need a real Dreambox to develop against — run DreamHub itself in mock mode (`DREAMHUB_MODE=mock`, see `backend/README.md`) and point your extension's registration call at it, the same `127.0.0.1:8080` it would use on a real box. The core doesn't distinguish a mock-mode session from a real one for anything extension-related.

## What this doesn't cover yet

- **No extension-contributed dashboard tiles.** E1's own original scope named this; still deferred — eWeLink (E4) exists now and is the candidate to prove it against.
- **Nothing pushes.** DreamHub polls `GET /widgets` on each page load; there's no way for your extension to proactively tell DreamHub something changed. If that turns out to matter, it'd be a later, additive addition to the schema (SSE or similar), not something you need to work around today.
- **Permissions are binary.** `admin: true/false` is the whole vocabulary — an extension can't ask for one of DreamHub's finer-grained permissions (`channels.manage`, say). If your extension genuinely needs that kind of integration, it's a sign you might actually want the "core contribution" path from the table above instead.

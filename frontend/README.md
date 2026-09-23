# frontend/

The web UI: Vue 3 (Composition API, `<script setup>`), TypeScript, Vue Router, Pinia, Tailwind CSS and vue-i18n. Built as static files and embedded into the Go backend via `go:embed` — see [`../backend/README.md`](../backend/README.md#frontend) and [`../backend/internal/webui`](../backend/internal/webui).

Mirrors the hub structure (`../CONCEPT.md`): a core shell (sidebar, top bar, dashboard, settings) plus one route per hub, built out as each hub itself is built (`../ROADMAP.md`). Visual and interaction decisions — colors, type, spacing, breakpoints, components — follow [`../DESIGN.md`](../DESIGN.md); that file is the source of truth, not this code.

## Running

```sh
npm install
npm run dev
```

Opens on `http://localhost:5173` and proxies `/api` to a backend running separately on port 8080 (`go run ./cmd/dreamhub` in `../backend`) — see `vite.config.ts`. This gives hot reload; nothing here works standalone without a backend, mock or real, to talk to.

```sh
npm run build
```

Type-checks (`vue-tsc -b`) and builds straight into `../backend/internal/webui/dist` (see `vite.config.ts`, `outDir`) — that's what `go build`/`go run` in the backend then embeds and serves. `npm run build:skip-check` skips the type-check for a faster iteration loop; CI and releases should use the full `build`.

## Conventions

- **No external requests at runtime** (`../PRINCIPLES.md`): fonts (Source Sans 3, Source Code Pro — `@fontsource/*`) and every script/stylesheet are bundled, nothing loads from a CDN or Google Fonts. Verify with the browser's network tab against an offline network if in doubt.
- **i18n everywhere user-facing**: no hardcoded UI strings. Locale files are `src/locales/{de,it,en}.json`; `en` is the fallback (`src/i18n/index.ts`). Backend status/problem codes (`internal/status`, `internal/hub`) are translated by `src/composables/useProblem.ts` — the backend's `Fix` values already carry their own `fix.` prefix, `Code` values don't (they're nested under `problem.` in the locale files); see that file's comments before changing either.
- **RTL-ready**: Tailwind's logical-property utilities only (`ps-`/`pe-`, `ms-`/`me-`, `start-`/`end-`, `text-start`/`text-end`, `border-s`/`border-e`) — never `left`/`right`, `ml-`/`mr-`, etc.
- **Design tokens** are CSS custom properties in `src/style.css` (`--dh-*`), mapped into Tailwind's theme in `tailwind.config.js`. Appearance (dark/light/system) is applied as `data-theme` on `<html>` by `src/stores/preferences.ts`, not Tailwind's `dark:` media-query variant.
- **Custom breakpoints** (`below-lg`/`below-md`/`below-sm`, i.e. `max-width` queries) match `../DESIGN.md`'s layout breakpoints exactly (1180/900/640px) — see `tailwind.config.js`. They replace Tailwind's default `sm`/`md`/`lg`/… scale on purpose, so a component doesn't reach for those by habit.
- **A hub view without a backend implementation yet is a real placeholder, not a mock**: `HubView.vue` shows that hub's actual live status and problems from `GET /api/v1/navigation`, translated, with a fix hint — it just doesn't have the hub's own screens yet. Don't invent fake data to make a screen look more finished than the backend behind it.
- **A section's settings label and its fields' labels collide as i18n keys** (`settings.general` vs. `settings.general.language`) — see `src/composables/useSettingsText.ts`'s comment before adding a section or changing how labels are looked up.
- **Settings and the dashboard save directly** (no preview/apply — PRINCIPLES.md's exception for DreamHub's own state), but saving `general.language`/`appearance`/`navLabels` from the settings page must also update the live `preferences` store, or the UI would only pick up the change after a reload; see `SettingsSectionCard.vue`'s `syncPreferences`.

## Layout

- `src/main.ts` — mounts the app with Pinia, Vue Router and vue-i18n.
- `src/App.vue` — the core shell: sidebar + top bar around the routed view; loads preferences and navigation once at startup.
- `src/router/` — one route per hub path (mirrors `backend/internal/hub/v1.go`), `/` (dashboard) and `/settings`.
- `src/stores/` — Pinia stores: `preferences` (language/appearance/navLabels, backed by `GET`/`PATCH /api/v1/settings`), `navigation` (sidebar contents), `ui` (transient state — currently just the mobile drawer's open/closed).
- `src/api/` — a small typed fetch wrapper (`client.ts`, CSRF header on mutating requests, the backend's error envelope) and hand-written types mirroring the backend's JSON shapes (`types.ts`).
- `src/composables/` — `useProblem.ts` (translates `status.Problem` values), `useSettingsText.ts` (section/field/option/error-code labels for the settings menu), `useSettingsForm.ts` (drafts, dirty tracking, per-section save against `GET`/`PATCH /api/v1/settings`), `useDashboard.ts` (loads and persists the dashboard layout against `GET`/`PUT /api/v1/dashboard`).
- `src/components/` — `AppSidebar.vue`, `AppTopBar.vue`, `DropdownMenu.vue`, `Icon.vue` (a small self-drawn icon set — no icon font/CDN); `settings/` (`SettingsSectionCard.vue`, `SettingsFieldInput.vue` — the generic, schema-driven form); `dashboard/` (`DashboardTileFrame.vue` — chrome shared by every tile; `TimersUpcomingTile.vue`, `HubsStatusTile.vue` — the v1 built-in tiles, reusing `GET /api/v1/timers` and `GET /api/v1/hubs`).
- `src/views/` — `DashboardView.vue` (tile grid, edit mode with native drag-and-drop reorder), `HubView.vue` (the shared per-hub placeholder), `SettingsView.vue` (renders every section `GET /api/v1/settings` returns — no per-field list to keep in sync here), `NotFoundView.vue`.
- `src/locales/` — `de.json`, `it.json`, `en.json`.

## Known issue

`npm audit` flags a moderate advisory in `esbuild` (via Vite's dev server only — `npm run dev`'s local dev server could accept requests from an arbitrary website in a victim's browser and read the response; the production build this ships is unaffected). Fixing it needs a Vite 8 upgrade; left as-is for now since `npm run dev` only ever runs on a developer's own machine, not on the box.

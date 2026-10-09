# enigma2-plugin/

The Enigma2 launcher plugin (docs/archive/V1_PLAN.md, WP8). Python 2.7 — the only interpreter present on-device (see `ARCHITECTURE.md`). **Verified live on the reference box** (2026-09-25, CHANGELOG.md): installed through the `.deb`, it appears in the Plugin Browser with its translated description, and the screen renders the QR code and the backend status ("Backend läuft (v…)") — checked through the box's own screenshot endpoint and simulated remote keys.

**v1 scope — a launcher, not a second UI** (same division of labor as the WebControl plugin: basic settings on the TV, everything else in the web UI):
- Shows a **QR code** that opens DreamHub on a phone — no IP address to type. Rendered by the backend (`GET /api/v1/qr.png`, public/unauthenticated like `/api/v1/health` — the plugin needs it before any credentials exist), so the plugin itself needs no QR library and holds no logic.
- Shows whether the backend is running and its version (`GET /api/v1/health`), with a mismatch message if the plugin and backend versions differ.
- A few basic settings, reachable via the green button on the launcher screen: which port to call on `127.0.0.1`, and whether the backend has its own auth switched on. These are the plugin's *own* settings for reaching the backend — it never changes DreamHub's real settings itself (that stays the web UI's job, `PRINCIPLES.md`'s "one central menu").

**Later**: AI recommendations and notifications shown directly on the TV.

## Layout

```
src/                        # becomes Extensions/DreamHub/ at install time
  __init__.py                # gettext wiring + config.plugins.dreamhub (port, authEnabled)
  plugin.py                  # PluginDescriptor registration only
  DreamHubScreen.py          # the launcher screen: QR code + backend status
  DreamHubSetup.py           # the settings screen (port, authEnabled)
  backend.py                 # the two HTTP calls to 127.0.0.1, via Twisted's getPage (non-blocking)
  webplay.py                 # /dreamhub/play on the box's web server: opens the movie player (PlexHub's films)
  locale/<lang>/LC_MESSAGES/DreamHub.po   # translation sources; de and it done, en is gettext's own source text
```

Calls `127.0.0.1:<port>` (the local Go backend, default port 8080) only. No business logic of its own. One addition since 0.1.7: at session start it registers `/dreamhub/play` on the box's own web server (WebInterface's `addExternalChild`, like AutoTimer's API, same login as `/web/*`) — it opens Enigma2's movie player for a reference. `/web/zap` only does that for a file path starting with `/`, so a Plex stream URL would otherwise play like a channel, without seeking. Follows DreamOS's configured system language (`config.osd.language.value`, format `de_DE`) via gettext, confirmed against `PROJECT_PLAN.md`'s researched convention: `PluginLanguageDomain`/`PluginLanguagePath`, `gettext.bindtextdomain()`, re-initialized on every `language.addCallback()` — no separate language setting for the user to maintain.

## Building and installing by hand (the `.deb` does this for you)

`.po` files compile to `.mo` via `msgfmt`, into the same `locale/<lang>/LC_MESSAGES/` layout:

```sh
msgfmt src/locale/de/LC_MESSAGES/DreamHub.po -o src/locale/de/LC_MESSAGES/DreamHub.mo
msgfmt src/locale/it/LC_MESSAGES/DreamHub.po -o src/locale/it/LC_MESSAGES/DreamHub.mo
```

Then copy `src/` (including the compiled `.mo` files, not the `.po` sources) to `/usr/lib/enigma2/python/Plugins/Extensions/DreamHub/` on the box and restart Enigma2 (`init 4 && init 3`, or the GUI restart from the menu) to pick it up. `.deb` packaging with this wired into `postinst` is WP9.

## Known gaps

- Plugin icon: `src/plugin.png` (100×40 with transparency, the Plugin Browser's classic size), drawn from `art/plugin-icon.svg` in DreamHub's own colors and font — a hub of five nodes around a core, next to the name. Regenerate with `art/render-icon.mjs` (headless Chromium via `playwright-core`). Not yet seen in the box's own Plugin Browser: skins that expect a different size scale it.
- Checked on one skin only (the reference box's Gemini/AIO image); it doesn't appear in that image's own "Gemini BluePanel" quick launcher, which keeps its own category list.

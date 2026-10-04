# packaging/

Builds `dreamhub_<version>_<arch>.deb` (docs/V1_PLAN.md, WP9): the backend binary, its systemd unit, an avahi mDNS service file, and the Enigma2 launcher plugin (`../enigma2-plugin/`), in one installable package. Built automatically on every push by `.github/workflows/ci.yml` (both architectures, uploaded as the workflow artifact `dreamhub-deb`); released packages are attached to the GitHub releases.

```sh
# from the repo root
sh packaging/build-deb.sh                # dist/dreamhub_<version>_arm64.deb
ARCH=armhf sh packaging/build-deb.sh     # dist/dreamhub_<version>_armhf.deb
```

| `ARCH` | Boxes | Status |
| --- | --- | --- |
| `arm64` (default) | Dreambox One, Two | tested on a Dreambox One (DreamOS 2.6) |
| `armhf` | DM900 UHD, DM920 UHD | built; smoke-tested on the One's 32-bit compatibility, not on such a box |
| — | DM7080, DM820, DM520, DM525 (`mipsel`) | can't be built: `modernc.org/sqlite` has no `linux/mipsle` port |

The version comes from `backend/internal/api/router.go` (`Version`); the TV plugin's `PluginVersion` (`enigma2-plugin/src/__init__.py`) must match it, or the plugin reports a version mismatch.

Requires `go`, `msgfmt` (Debian/Ubuntu: `apt install gettext`) and `dpkg-deb`/`fakeroot` (already present on most Debian-family systems, including the CI image). Build the frontend first (`npm run build` in `../frontend`) — the binary embeds whatever is in `backend/internal/webui/dist`.

## What it installs

| File | Purpose |
| --- | --- |
| `/usr/bin/dreamhub` | the backend binary (frontend embedded via `go:embed`) |
| `/lib/systemd/system/dreamhub.service` | the systemd unit, enabled and started by `postinst` |
| `/etc/avahi/services/dreamhub.service` | mDNS advertisement (`<box>.local:8080`) |
| `/usr/lib/enigma2/python/Plugins/Extensions/DreamHub/` | the TV launcher plugin, `.po` files pre-compiled to `.mo` |

`postinst` restarts `enigma2` only when the launcher plugin changed (it keeps a checksum), so the plugin shows up without a manual GUI restart — the same thing installing any other Extensions plugin through the Addon Manager does. A recording running at that moment would be cut: install when nothing records. `postrm` only removes `/data/dreamhub` (the SQLite database and its settings) on `purge`, never on a plain removal, so reinstalling later picks up where the user left off. Install and in-place upgrades are confirmed on the reference box (ARCHITECTURE.md).

## Making a release

1. Set `Version` in `backend/internal/api/router.go` and `PluginVersion` in `enigma2-plugin/src/__init__.py`; add the release to `CHANGELOG.md`.
2. `npm run build` in `frontend/`, then `ARCH=arm64` and `ARCH=armhf` builds.
3. Install the arm64 package on the reference box and run the self-test (System → Diagnostics) — all green before releasing.
4. Tag `v<version>` and create the GitHub release with both packages attached.

## What's still open (ROADMAP.md, "v1 — release & packaging")

- An actual apt feed to publish to (decision D4 in `docs/V1_PLAN.md` — the package format doesn't depend on it)
- Uninstall/purge on the box (not run: the box keeps DreamHub installed)
- MIPS packages (see above)

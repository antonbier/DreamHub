# packaging/

Builds `dreamhub_<version>_arm64.deb` (docs/V1_PLAN.md, WP9): the backend binary, its systemd unit, an avahi mDNS service file, and the Enigma2 launcher plugin (`../enigma2-plugin/`), in one installable package. Built automatically on every push by `.github/workflows/ci.yml` and uploaded as a workflow artifact.

```sh
# from the repo root
sh packaging/build-deb.sh          # writes dist/dreamhub_<version>_arm64.deb
```

Requires `go`, `msgfmt` (Debian/Ubuntu: `apt install gettext`) and `dpkg-deb`/`fakeroot` (already present on most Debian-family systems, including the CI image).

## What it installs

| File | Purpose |
| --- | --- |
| `/usr/bin/dreamhub` | the backend binary (frontend embedded via `go:embed`) |
| `/lib/systemd/system/dreamhub.service` | the systemd unit, enabled and started by `postinst` |
| `/etc/avahi/services/dreamhub.service` | mDNS advertisement (`<box>.local:8080`) |
| `/usr/lib/enigma2/python/Plugins/Extensions/DreamHub/` | the TV launcher plugin, `.po` files pre-compiled to `.mo` |

`postinst` also restarts `enigma2` if it's running, so the plugin shows up without a manual GUI restart — the same thing installing any other Extensions plugin through the Addon Manager already does, not something DreamHub invented. `postrm` only removes `/data/dreamhub` (the SQLite database and its settings) on `purge`, never on a plain removal, so reinstalling later picks up where the user left off — this was verified against a real `dpkg-deb --build` + `--info`/`--contents` locally; live install/upgrade/remove on the reference box itself is still open (ROADMAP.md).

## What's still open (ROADMAP.md, "v1 — release & packaging")

- An actual apt feed to publish to (decision D4 in `docs/V1_PLAN.md` — package format itself doesn't depend on this)
- Live install/upgrade/uninstall verified on the reference box
- User docs: installing via WebAdmin's "Upload new `*-feed.list`" page

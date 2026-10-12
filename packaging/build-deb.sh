#!/bin/sh
# Builds dreamhub_<version>_<arch>.deb: cross-compiles the backend, compiles
# the Enigma2 plugin's .po files to .mo, assembles the file tree dpkg
# expects, and runs dpkg-deb. See ROADMAP.md, "v1 — release & packaging"
# and docs/archive/V1_PLAN.md, WP9.
#
# Usage: [ARCH=arm64|armhf] packaging/build-deb.sh [output-dir]
#
# ARCH is the Debian architecture of the DreamOS box (default arm64):
#   arm64   Dreambox One, Two     — tested on a Dreambox One
#   armhf   DM900 UHD, DM920 UHD  — built, not tested on a box yet
# The MIPS DreamOS boxes (DM7080, DM820, DM520, DM525 — mipsel) can't be
# built yet: the pure-Go SQLite driver (modernc.org/sqlite) has no
# linux/mipsle port, and a cgo driver would need a MIPS C toolchain.
# Older Dreamboxes (DM800se, DM7020HD, DM8000, …) don't run DreamOS — no
# dpkg, no systemd — and aren't supported.
# Requires: go, msgfmt (package gettext), dpkg-deb. All present in this
# repo's own CI image; on a Debian/Ubuntu dev machine: apt install gettext.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-$ROOT/dist}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

VERSION="$(sed -n 's/^const Version = "\(.*\)"$/\1/p' "$ROOT/backend/internal/api/router.go")"
if [ -z "$VERSION" ]; then
    echo "build-deb.sh: could not read Version from backend/internal/api/router.go" >&2
    exit 1
fi
# The TV plugin reports a version mismatch unless its version is the same.
PLUGIN_VERSION="$(sed -n 's/^PluginVersion = "\(.*\)"$/\1/p' "$ROOT/enigma2-plugin/src/__init__.py")"
if [ "$PLUGIN_VERSION" != "$VERSION" ]; then
    echo "build-deb.sh: PluginVersion ($PLUGIN_VERSION, enigma2-plugin/src/__init__.py) differs from Version ($VERSION)" >&2
    exit 1
fi

# The binary embeds whatever is in backend/internal/webui/dist. An index.html
# naming asset files that aren't there (a stale or placeholder index.html
# next to a newer build — happened on 2026-10-06) gives a package whose web
# UI is a blank page. Run `npm run build` in frontend/ right before this.
DIST="$ROOT/backend/internal/webui/dist"
ASSETS="$(grep -o '/assets/[^"]*' "$DIST/index.html" || true)"
if [ -z "$ASSETS" ]; then
    echo "build-deb.sh: $DIST/index.html names no assets — run 'npm run build' in frontend/ first" >&2
    exit 1
fi
for a in $ASSETS; do
    if [ ! -f "$DIST$a" ]; then
        echo "build-deb.sh: $DIST/index.html names $a, which doesn't exist — run 'npm run build' in frontend/ right before this" >&2
        exit 1
    fi
done

ARCH="${ARCH:-arm64}"
case "$ARCH" in
    arm64)  GOARCH=arm64;  GOARM=;  GOMIPS= ;;
    armhf)  GOARCH=arm;    GOARM=7; GOMIPS= ;;
    *) echo "build-deb.sh: unknown ARCH '$ARCH' (arm64, armhf)" >&2; exit 1 ;;
esac
echo "Building dreamhub $VERSION for $ARCH..."

PKG="$WORK/pkg"
mkdir -p "$PKG/DEBIAN" \
         "$PKG/usr/bin" \
         "$PKG/lib/systemd/system" \
         "$PKG/etc/avahi/services" \
         "$PKG/usr/lib/enigma2/python/Plugins/Extensions/DreamHub/locale"

# --- backend binary ---
echo "Cross-compiling the backend..."
(cd "$ROOT/backend" && CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" GOARM="$GOARM" GOMIPS="$GOMIPS" go build -trimpath -ldflags="-s -w" \
    -o "$PKG/usr/bin/dreamhub" ./cmd/dreamhub)
# AppsHub's Docker helper (docs/APPSHUB_PLAN.md, App2): the only process
# that holds the Docker socket; its unit stays disabled until "Set up
# Docker" in the Apps tab enables it.
(cd "$ROOT/backend" && CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" GOARM="$GOARM" GOMIPS="$GOMIPS" go build -trimpath -ldflags="-s -w" \
    -o "$PKG/usr/bin/dreamhub-dockerd-helper" ./cmd/dreamhub-dockerd-helper)

# --- systemd unit + avahi service ---
cp "$ROOT/packaging/dreamhub.service" "$PKG/lib/systemd/system/dreamhub.service"
cp "$ROOT/packaging/dreamhub-docker.service" "$PKG/lib/systemd/system/dreamhub-docker.service"
cp "$ROOT/packaging/dreamhub-dockerd-helper.service" "$PKG/lib/systemd/system/dreamhub-dockerd-helper.service"
cp "$ROOT/packaging/dreamhub.avahi.service" "$PKG/etc/avahi/services/dreamhub.service"

# --- Enigma2 plugin ---
echo "Compiling plugin translations..."
for po in "$ROOT"/enigma2-plugin/src/locale/*/LC_MESSAGES/DreamHub.po; do
    lang="$(basename "$(dirname "$(dirname "$po")")")"
    mkdir -p "$PKG/usr/lib/enigma2/python/Plugins/Extensions/DreamHub/locale/$lang/LC_MESSAGES"
    msgfmt "$ROOT/enigma2-plugin/src/locale/$lang/LC_MESSAGES/DreamHub.po" \
        -o "$PKG/usr/lib/enigma2/python/Plugins/Extensions/DreamHub/locale/$lang/LC_MESSAGES/DreamHub.mo"
done
# Every .py: a new module can't be forgotten here.
for f in "$ROOT"/enigma2-plugin/src/*.py; do
    cp "$f" "$PKG/usr/lib/enigma2/python/Plugins/Extensions/DreamHub/"
done
cp "$ROOT/enigma2-plugin/src/plugin.png" "$PKG/usr/lib/enigma2/python/Plugins/Extensions/DreamHub/plugin.png"

# --- control files ---
sed -e "s/@VERSION@/$VERSION/" -e "s/@ARCH@/$ARCH/" "$ROOT/packaging/control.in" > "$PKG/DEBIAN/control"
INSTALLED_SIZE="$(du -sk "$PKG" --exclude=DEBIAN | cut -f1)"
echo "Installed-Size: $INSTALLED_SIZE" >> "$PKG/DEBIAN/control"
for script in postinst prerm postrm; do
    cp "$ROOT/packaging/$script" "$PKG/DEBIAN/$script"
    chmod 755 "$PKG/DEBIAN/$script"
done

# --- build ---
mkdir -p "$OUT"
DEB="$OUT/dreamhub_${VERSION}_${ARCH}.deb"
fakeroot dpkg-deb --build --root-owner-group "$PKG" "$DEB"
echo "Built $DEB"
dpkg-deb --info "$DEB"
echo "---"
dpkg-deb --contents "$DEB"

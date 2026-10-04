#!/bin/sh
# Builds dreamhub_<version>_arm64.deb: cross-compiles the backend, compiles
# the Enigma2 plugin's .po files to .mo, assembles the file tree dpkg
# expects, and runs dpkg-deb. See ROADMAP.md, "v1 — release & packaging"
# and docs/V1_PLAN.md, WP9.
#
# Usage: packaging/build-deb.sh [output-dir]
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
echo "Building dreamhub $VERSION for arm64..."

PKG="$WORK/pkg"
mkdir -p "$PKG/DEBIAN" \
         "$PKG/usr/bin" \
         "$PKG/lib/systemd/system" \
         "$PKG/etc/avahi/services" \
         "$PKG/usr/lib/enigma2/python/Plugins/Extensions/DreamHub/locale/de/LC_MESSAGES" \
         "$PKG/usr/lib/enigma2/python/Plugins/Extensions/DreamHub/locale/it/LC_MESSAGES"

# --- backend binary ---
echo "Cross-compiling the backend..."
(cd "$ROOT/backend" && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" \
    -o "$PKG/usr/bin/dreamhub" ./cmd/dreamhub)

# --- systemd unit + avahi service ---
cp "$ROOT/packaging/dreamhub.service" "$PKG/lib/systemd/system/dreamhub.service"
cp "$ROOT/packaging/dreamhub.avahi.service" "$PKG/etc/avahi/services/dreamhub.service"

# --- Enigma2 plugin ---
echo "Compiling plugin translations..."
for lang in de it; do
    msgfmt "$ROOT/enigma2-plugin/src/locale/$lang/LC_MESSAGES/DreamHub.po" \
        -o "$PKG/usr/lib/enigma2/python/Plugins/Extensions/DreamHub/locale/$lang/LC_MESSAGES/DreamHub.mo"
done
for f in __init__.py backend.py plugin.py DreamHubScreen.py DreamHubSetup.py; do
    cp "$ROOT/enigma2-plugin/src/$f" "$PKG/usr/lib/enigma2/python/Plugins/Extensions/DreamHub/$f"
done
cp "$ROOT/enigma2-plugin/src/plugin.png" "$PKG/usr/lib/enigma2/python/Plugins/Extensions/DreamHub/plugin.png"

# --- control files ---
sed "s/@VERSION@/$VERSION/" "$ROOT/packaging/control.in" > "$PKG/DEBIAN/control"
INSTALLED_SIZE="$(du -sk "$PKG" --exclude=DEBIAN | cut -f1)"
echo "Installed-Size: $INSTALLED_SIZE" >> "$PKG/DEBIAN/control"
for script in postinst prerm postrm; do
    cp "$ROOT/packaging/$script" "$PKG/DEBIAN/$script"
    chmod 755 "$PKG/DEBIAN/$script"
done

# --- build ---
mkdir -p "$OUT"
DEB="$OUT/dreamhub_${VERSION}_arm64.deb"
fakeroot dpkg-deb --build --root-owner-group "$PKG" "$DEB"
echo "Built $DEB"
dpkg-deb --info "$DEB"
echo "---"
dpkg-deb --contents "$DEB"

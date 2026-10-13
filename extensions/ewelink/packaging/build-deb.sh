#!/bin/sh
# Builds dreamhub-ext-ewelink's own .deb from DreamHub's extension
# template (packaging/extension-template, E3) — the template's first real
# user. Usage: ARCH=arm64|armhf sh extensions/ewelink/packaging/build-deb.sh
set -eu

HERE=$(cd "$(dirname "$0")" && pwd)
EXT=$(cd "$HERE/.." && pwd)
ROOT=$(cd "$EXT/../.." && pwd)
TPL="$ROOT/packaging/extension-template"
ARCH=${ARCH:-arm64}
VERSION=${VERSION:-0.2.0}
ID=dreamhub-ext-ewelink
case "$ARCH" in
    arm64) GOARCH=arm64; GOARM= ;;
    armhf) GOARCH=arm; GOARM=7 ;;
    *) echo "unknown ARCH $ARCH" >&2; exit 1 ;;
esac

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
PKG="$WORK/pkg"
mkdir -p "$PKG/DEBIAN" "$PKG/usr/bin" "$PKG/lib/systemd/system" "$PKG/usr/share/doc/$ID"

(cd "$EXT" && CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" GOARM="$GOARM" go build -trimpath -ldflags="-s -w" -o "$PKG/usr/bin/$ID" .)

subst() {
    sed -e "s#@EXT_ID@#$ID#g" -e "s#@EXT_USER@#$ID#g" -e "s#@VERSION@#$VERSION#g" -e "s#@ARCH@#$ARCH#g" \
        -e "s#@MIN_DREAMHUB_VERSION@#0.6.0#g" -e "s#@EXT_MAINTAINER@#DreamHub <noreply@github.com>#g" \
        -e "s#@EXT_HOMEPAGE@#https://github.com/antonbier/DreamHub/tree/main/extensions/ewelink#g" \
        -e "s#@EXT_DESCRIPTION@#DreamHub extension: eWeLink/Sonoff devices on the local network#g" \
        -e "s#@EXT_DESCRIPTION_LONG@#Reads and switches eWeLink/Sonoff devices with LAN control on, without the eWeLink cloud at run time.#g" \
        -e "s#@EXT_EXEC@#/usr/bin/$ID#g" "$1"
}
subst "$TPL/control.template" | grep -v '^#' > "$PKG/DEBIAN/control"
subst "$TPL/postinst.template" > "$PKG/DEBIAN/postinst"
subst "$TPL/prerm.template" > "$PKG/DEBIAN/prerm"
subst "$TPL/postrm.template" > "$PKG/DEBIAN/postrm"
chmod 755 "$PKG/DEBIAN/postinst" "$PKG/DEBIAN/prerm" "$PKG/DEBIAN/postrm"

# The template's unit, with one deliberate change: this extension must
# reach the LAN (mDNS multicast to find devices, each device's own
# :8081), so the loopback-only address allowance is widened to private
# ranges and mDNS. (On systemd 232 that directive is ignored anyway —
# packaging/extension-template/README.md.)
subst "$TPL/extension.service" | sed \
    -e 's#^IPAddressAllow=localhost#IPAddressAllow=localhost 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 224.0.0.251#' \
    > "$PKG/lib/systemd/system/$ID.service"
cp "$EXT/README.md" "$PKG/usr/share/doc/$ID/README.md"

mkdir -p "$ROOT/dist"
dpkg-deb --root-owner-group --build "$PKG" "$ROOT/dist/${ID}_${VERSION}_${ARCH}.deb" >/dev/null
echo "$ROOT/dist/${ID}_${VERSION}_${ARCH}.deb"

#!/bin/sh
# ChannelHub (#26, docs/CHANNELHUB_PLAN.md "To confirm on the box"):
# everything that can be learned before the first write, in one read-only
# run. It only calls listing endpoints and copies the BouquetEditor
# plugin's Python source to /tmp, so its write semantics (position from 0
# or 1, sRefBefore at the end, where backup() writes, whether restore()
# reloads) can be read instead of tried.
#
#   sh channelhub.sh > /tmp/channelhub-check.txt 2>&1
#   then fetch /tmp/channelhub-check.txt and /tmp/bouqueteditor-src.tgz

WC=http://127.0.0.1
section() { printf '\n===== %s\n' "$1"; }
enc() { python -c 'import sys, urllib; print(urllib.quote(sys.argv[1], safe=""))' "$1"; }
get() { wget -q -O - "$WC$1" 2>&1; }
count() { grep -c '<e2servicereference>'; }

# The references the backend uses (internal/webcontrol/channels.go, radio.go).
TV_TYPES='(type == 1) || (type == 17) || (type == 22) || (type == 25) || (type == 31) || (type == 134) || (type == 195)'
TV_ALL="1:7:1:0:0:0:0:0:0:0:$TV_TYPES ORDER BY name"
TV_PROVIDERS="1:7:1:0:0:0:0:0:0:0:$TV_TYPES FROM PROVIDERS ORDER BY name"
RADIO_ALL='1:7:2:0:0:0:0:0:0:0:(type == 2) || (type == 10) ORDER BY name'
RADIO_PROVIDERS='1:7:2:0:0:0:0:0:0:0:(type == 2) || (type == 10) FROM PROVIDERS ORDER BY name'
TV_ROOT='1:7:1:0:0:0:0:0:0:0:FROM BOUQUET "bouquets.tv" ORDER BY bouquet'
RADIO_ROOT='1:7:2:0:0:0:0:0:0:0:FROM BOUQUET "bouquets.radio" ORDER BY bouquet'

section "WebControl: counts per reference"
for name in TV_ALL TV_PROVIDERS RADIO_ALL RADIO_PROVIDERS TV_ROOT RADIO_ROOT; do
    eval ref=\$$name
    n=$(get "/web/getservices?sRef=$(enc "$ref")" | count)
    echo "$name: $n"
done

section "WebControl: TV providers (first 10) and the first provider's channels (first 5)"
providers=$(get "/web/getservices?sRef=$(enc "$TV_PROVIDERS")")
echo "$providers" | grep -e '<e2servicereference>' -e '<e2servicename>' | head -20
first=$(echo "$providers" | grep -o '<e2servicereference>[^<]*' | head -1 | sed 's/<e2servicereference>//')
[ -n "$first" ] && get "/web/getservices?sRef=$(enc "$first")" | grep -e '<e2servicereference>' -e '<e2servicename>' | head -10

section "WebControl: first TV bouquet (first 15 entries, markers included)"
bouquet=$(get "/web/getservices?sRef=$(enc "$TV_ROOT")" | grep -o '<e2servicereference>[^<]*' | head -1 | sed 's/<e2servicereference>//')
echo "bouquet: $bouquet"
[ -n "$bouquet" ] && get "/web/getservices?sRef=$(enc "$bouquet")" | grep -e '<e2servicereference>' -e '<e2servicename>' | head -30

section "BouquetEditor: its own listing of the same bouquet"
[ -n "$bouquet" ] && get "/bouqueteditor/web/getservices?sRef=$(enc "$bouquet")" | head -40

section "BouquetEditor plugin: files"
BE=$(ls -d /usr/lib/enigma2/python/Plugins/Extensions/WebBouquetEditor 2>/dev/null)
echo "${BE:-not installed}"
if [ -n "$BE" ]; then
    find "$BE" -type f | head -60
    dpkg -S "$BE" 2>/dev/null | head -2
    # The Python sources (where the logic is), the web/*.xml (the endpoints).
    find "$BE" /usr/lib/enigma2/python/Plugins/Extensions/WebInterface/WebComponents/Sources \
        -name '*.py' -o -name '*.xml' 2>/dev/null |
        grep -i -e bouquet -e "$BE" | tar czf /tmp/bouqueteditor-src.tgz -T - 2>/dev/null
    ls -l /tmp/bouqueteditor-src.tgz
    section "BouquetEditor: positions, backup, restore (quick look)"
    grep -rn -e 'def moveService' -e 'def moveBouquet' -e 'position' -e 'def backup' -e 'def restore' \
        -e 'reload' -e 'servicelist' -e 'BACKUP' -e '\.tar' "$BE" --include=*.py | head -60
fi

section "channel list reload endpoint"
ls /usr/lib/enigma2/python/Plugins/Extensions/WebInterface/web/ 2>/dev/null | grep -i -e reload -e servicelist

section "picons: sets on disk and in the feed"
ls /data/piconpool 2>/dev/null
for d in /data/piconpool/*/; do [ -d "$d" ] && echo "$d $(ls "$d" | wc -l)"; done
apt-cache search picon 2>/dev/null | head -40
dpkg -l 2>/dev/null | grep -i picon

section "done"

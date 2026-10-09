#!/bin/sh
# AdScan phase 0 (docs/ADSCAN_PLAN.md, #44): collect the device facts in one
# run. Read-only: nothing on the box is written except the report in /tmp
# and FFmpeg's throwaway frames on a pipe. Everything runs under nice/ionice.
#
#   sh phase0.sh /media/hdd/movie/<recording>.ts > /tmp/adscan-phase0.txt 2>&1
#
# Pick a recording of an ad-financed channel (RTL, VOX, ProSieben, ...) of
# about an hour, that is neither being recorded nor played.

REC="$1"
if [ -z "$REC" ] || [ ! -f "$REC" ]; then
    echo "usage: sh phase0.sh <recording.ts>" >&2
    exit 2
fi
LOW="nice -n 19"
command -v ionice >/dev/null 2>&1 && LOW="$LOW ionice -c3"

section() { printf '\n===== %s\n' "$1"; }

# Wall-clock milliseconds; busybox date may lack %N, python2 is always there.
now_ms() { python -c 'import time; print(int(time.time()*1000))'; }
timed() {
    t0=$(now_ms)
    "$@" >/dev/null 2>&1
    rc=$?
    echo "  rc=$rc  $(( $(now_ms) - t0 )) ms: $*"
}

section "box"
uname -a
nproc
free -m
uptime
df -h "$(dirname "$REC")"

section "recording running? (e2state 1/2 = running, the script only reads)"
wget -q -O - http://127.0.0.1/web/timerlist 2>/dev/null | grep -c '<e2state>[12]</e2state>'

section "ffmpeg"
ffmpeg -hide_banner -version | head -1
ffmpeg -hide_banner -buildconf 2>/dev/null | tr ' ' '\n' | grep -i -e chromaprint -e enable-gpl -e enable-libzvbi
ffmpeg -hide_banner -filters 2>/dev/null | grep -w -e showinfo -e blackdetect -e silencedetect -e crop -e scale
# 4.1 has -vsync, -fps_mode came in 5.1.
ffmpeg -hide_banner -h full 2>/dev/null | grep -c -e '-fps_mode' | sed 's/^/fps_mode options: /'

section "recording files"
ls -l "${REC%.ts}".*
head -4 "$REC.meta" 2>/dev/null

section ".ts.ap (expected: 16-byte entries, 64-bit BE byte offset + 64-bit BE PTS)"
if [ -f "$REC.ap" ]; then
    size=$(wc -c < "$REC.ap")
    echo "size $size, /16 = $((size / 16)), /16 remainder $((size % 16))"
    od -A d -t x1 -N 96 "$REC.ap"
    echo "last entries:"
    tail -c 48 "$REC.ap" | od -A d -t x1
fi

section ".ts.cuts (expected: 12-byte entries, 64-bit BE PTS + 32-bit BE type)"
if [ -f "$REC.cuts" ]; then
    size=$(wc -c < "$REC.cuts")
    echo "size $size, /12 = $((size / 12)), /12 remainder $((size % 12))"
    od -A d -t x1 "$REC.cuts" | head -20
fi
echo "cuts files by size on this disk (how common are marks):"
find "$(dirname "$REC")" -maxdepth 1 -name '*.ts.cuts' -exec wc -c {} + 2>/dev/null | sort -n | awk '{print $1}' | uniq -c | head -15

section ".ts.sc (structure file, size only)"
[ -f "$REC.sc" ] && wc -c < "$REC.sc"

section "Enigma2: cut list handling and settings"
GEN=/usr/lib/enigma2/python/Screens/InfoBarGenerics
ls -l $GEN.py $GEN.pyo 2>/dev/null
if [ -f $GEN.py ]; then
    grep -n -e CUT_TYPE -e cutlist -e jumpNextMark -e uncut -e seekTo $GEN.py | head -60
elif [ -f $GEN.pyo ]; then
    strings $GEN.pyo 2>/dev/null | grep -i -e cut_type -e cutlist -e uncut | sort -u | head -40
fi
grep -i -e cut -e seek -e resume -e playback /etc/enigma2/settings

section "AdvancedMovieSelection: seen status"
AMS=/usr/lib/enigma2/python/Plugins/Extensions/AdvancedMovieSelection
if [ -d $AMS ]; then
    grep -rn -i -e 'cuts' -e 'seen' -e 'LAST' $AMS --include=*.py | grep -v -i -e 'self.session' | head -60
    grep -i advancedmovieselection /etc/enigma2/settings | grep -i -e seen -e mark -e cut -e percent
else
    echo "not installed"
fi

section "timing: decode keyframes only, first 10 min, grayscale crop pipe"
VF="crop=iw/4:ih/4:0:0,scale=96:54,format=gray"
timed $LOW ffmpeg -nostdin -v error -skip_frame nokey -i "$REC" -t 600 -map 0:v:0 -an -sn -vf "$VF" -vsync passthrough -f rawvideo -y /dev/null
echo "keyframes in those 10 min:"
$LOW ffmpeg -nostdin -v info -skip_frame nokey -i "$REC" -t 600 -map 0:v:0 -an -sn -vf showinfo -vsync passthrough -f null - 2>&1 | grep -c pts_time

section "timing: one keyframe from a 1.5 MB slice over stdin, 5 places"
size=$(wc -c < "$REC")
for pct in 10 30 50 70 90; do
    # 8192 TS packets of 188 bytes = 1.5 MB, starting on a packet boundary.
    skip=$(( size / 188 * pct / 100 ))
    t0=$(now_ms)
    out=$(dd if="$REC" bs=188 skip=$skip count=8192 2>/dev/null |
        $LOW ffmpeg -v info -skip_frame nokey -copyts -i - -map 0:v:0 -an -sn -vf "$VF,showinfo" -frames:v 1 -vsync passthrough -f rawvideo -y /dev/null 2>&1 |
        grep -o 'pts_time:[0-9.]*' | head -1)
    echo "  $pct %: $out  $(( $(now_ms) - t0 )) ms"
done

section "timing: ffmpeg start alone"
timed ffmpeg -hide_banner -version

section "timing: audio only, first 10 min, mono 11 kHz PCM (fingerprint layer)"
timed $LOW ffmpeg -nostdin -v error -i "$REC" -t 600 -map 0:a:0 -vn -sn -ac 1 -ar 11025 -f s16le -y /dev/null

section "timing: read 500 MB sequentially (disk speed for whole-file layers)"
timed $LOW dd if="$REC" of=/dev/null bs=1M count=500

section "done"

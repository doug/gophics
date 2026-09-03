#!/usr/bin/env bash
# Install the twin on the connected emulator/device, launch it, inject a
# flick with `adb shell input swipe`, and pull the trace it writes.
#
#   ./run.sh [out.json] [duration_ms]
#
# The injected swipe is a straight line at constant speed — not a human
# finger — which is fine for a reference of the *decay*: the finger phase is
# recorded and replayed identically either way, and only what the platform
# does after release is being compared. Reproducible, and needs nobody.
set -euo pipefail
out="${1:-android-flick.json}"; case "$out" in /*) ;; *) out="$PWD/$out" ;; esac
cd "$(dirname "$0")"
SDK="${ANDROID_SDK_ROOT:-$HOME/Library/Android/sdk}"
ADB="$SDK/platform-tools/adb"
dur="${2:-100}"
[ -f build/twin.apk ] || ./build.sh
"$ADB" wait-for-device
"$ADB" shell rm -f /sdcard/Android/data/com.gophics.twin/files/trace.json 2>/dev/null || true
"$ADB" install -r -g build/twin.apk >/dev/null
"$ADB" logcat -c
"$ADB" shell am start -n com.gophics.twin/.MainActivity >/dev/null
sleep 2
read -r w h < <("$ADB" shell wm size | sed -E 's/.*: ([0-9]+)x([0-9]+).*/\1 \2/' | tr -d '\r')
x=$((w / 2)); y1=$((h * 7 / 10)); y2=$((h * 3 / 10))
echo "swipe ${x},${y1} → ${x},${y2} over ${dur}ms"
"$ADB" shell input swipe "$x" "$y1" "$x" "$y2" "$dur"
for i in $(seq 1 60); do
	"$ADB" logcat -d -s twin:I 2>/dev/null | grep -q 'TRACE-DONE' && break
	sleep 0.5
done
"$ADB" logcat -d -s twin:I | grep -E 'wrote|failed' | sed 's/^.*twin *: //' || true
"$ADB" pull /sdcard/Android/data/com.gophics.twin/files/trace.json "$out" >/dev/null
echo "wrote $out"

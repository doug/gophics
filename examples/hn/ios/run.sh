#!/usr/bin/env bash
# One command: build the Go side into an xcframework, generate + build the
# Xcode project for the iOS Simulator, then install + launch.
#
#   ./run.sh              # HN app
#   ./run.sh --verify     # GPU bring-up scene (docs/mobile-gpu-bringup.md)
#   ./run.sh --build      # build only, don't install/launch
#
# The Simulator can't create a GPU surface, so the app renders via the CPU
# fallback (Hnmobile.GpuActive() is false there) — a faithful, interactive
# build for layout/logic/touch. GPU still runs on a real device.
#
# Prereqs: full Xcode (not just Command Line Tools), gomobile, xcodegen.
set -euo pipefail

cd "$(dirname "$0")"
REPO_ROOT=$(cd ../../.. && pwd)

VERIFY=0
RUN=1
for arg in "$@"; do
  case "$arg" in
    --verify) VERIFY=1 ;;
    --build)  RUN=0 ;;
    -h|--help) sed -n '2,10p' "$0"; exit 0 ;;
    *) echo "unknown flag: $arg" >&2; exit 2 ;;
  esac
done

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing tool: $1 — $2" >&2; exit 1; }; }
need gomobile "go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init"
need xcodegen "brew install xcodegen"
case "$(xcode-select -p)" in
  *Xcode*) ;;
  *) echo "full Xcode required (gomobile bind needs it). Run: sudo xcode-select -s /Applications/Xcode.app/Contents/Developer" >&2; exit 1 ;;
esac

# Pick a simulator: a booted one if any, else the first available iPhone.
udid_of() { grep -oE '[0-9A-Fa-f-]{36}' | head -1; }
UDID=$(xcrun simctl list devices booted | grep -i iphone | udid_of || true)
[ -n "$UDID" ] || UDID=$(xcrun simctl list devices available | grep -i iphone | udid_of || true)
[ -n "$UDID" ] || { echo "no iPhone simulator available (open Xcode > Settings > Platforms)" >&2; exit 1; }
NAME=$(xcrun simctl list devices | grep -i "$UDID" | sed -E 's/ *\(.*//; s/^ *//')
echo "→ simulator: $NAME ($UDID)"

echo "→ gomobile bind (Go → Hnmobile.xcframework)"
gomobile bind -target=ios,iossimulator -o Hnmobile.xcframework "$REPO_ROOT/examples/hn/mobile"

echo "→ xcodegen generate"
xcodegen generate >/dev/null

BUILD_ARGS=(-project GossamerHN.xcodeproj -scheme GossamerHN
  -sdk iphonesimulator -configuration Debug
  -destination "id=$UDID" -derivedDataPath build)
[ "$VERIFY" = 1 ] && BUILD_ARGS+=("SWIFT_ACTIVE_COMPILATION_CONDITIONS=DEBUG VERIFY")
echo "→ xcodebuild"
xcodebuild "${BUILD_ARGS[@]}" build >/dev/null

APP=build/Build/Products/Debug-iphonesimulator/GossamerHN.app
echo "→ built $APP"

if [ "$RUN" = 1 ]; then
  echo "→ boot + install + launch"
  xcrun simctl bootstatus "$UDID" -b >/dev/null 2>&1 || xcrun simctl boot "$UDID" || true
  open -a Simulator
  xcrun simctl install "$UDID" "$APP"
  xcrun simctl terminate "$UDID" dev.gossamer.hn 2>/dev/null || true
  echo "→ launching dev.gossamer.hn (Ctrl-C to stop the log stream)"
  exec xcrun simctl launch --console-pty "$UDID" dev.gossamer.hn
fi

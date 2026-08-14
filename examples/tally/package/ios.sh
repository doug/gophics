#!/usr/bin/env bash
# ios.sh — bind the Go side, generate the Xcode project, build for the simulator.
#
#   ./package/ios.sh          # build
#   ./package/ios.sh --run    # build, install and launch on a booted simulator
set -euo pipefail
cd "$(dirname "$0")/.."

echo "binding Go → xcframework…"
gomobile bind -target=ios,iossimulator -o ios/Tallymobile.xcframework ./mobile

echo "generating Xcode project…"
(cd ios && xcodegen generate)

echo "building for the simulator…"
xcodebuild -project ios/Tally.xcodeproj -scheme Tally \
  -sdk iphonesimulator -configuration Debug \
  -derivedDataPath ios/build build | tail -3

APP="ios/build/Build/Products/Debug-iphonesimulator/Tally.app"
echo "built $APP"

if [ "${1:-}" = "--run" ]; then
  DEV=$(xcrun simctl list devices booted | grep -oE '\([-0-9A-F]{36}\)' | head -1 | tr -d '()')
  if [ -z "$DEV" ]; then
    DEV=$(xcrun simctl list devices available | grep -E 'iPhone 1[5-7]' | head -1 | grep -oE '\([-0-9A-F]{36}\)' | tr -d '()')
    xcrun simctl boot "$DEV"
  fi
  xcrun simctl install "$DEV" "$APP"
  xcrun simctl launch "$DEV" com.gophics.tally
  open -a Simulator
fi

#!/usr/bin/env bash
# Build the iOS twin for the booted Simulator, install it, launch it with its
# console attached, and save the trace it prints. No Xcode project: one Swift
# file, a hand-written Info.plist, and simctl.
#
#   ./run.sh [out.json]      then flick upward once in the Simulator window
set -euo pipefail
cd "$(dirname "$0")"
out="${1:-ios-flick.json}"
arch="$(uname -m)"; [ "$arch" = "arm64" ] || arch=x86_64
rm -rf twin.app && mkdir -p twin.app
xcrun -sdk iphonesimulator swiftc -O -target "${arch}-apple-ios17.0-simulator" \
	-o twin.app/twin main.swift -framework UIKit
cp Info.plist twin.app/Info.plist
xcrun simctl install booted twin.app
echo "launched — flick upward once in the Simulator window"
# --console blocks until the app exits, streaming its stdout here.
xcrun simctl launch --console booted com.gophics.twin | tee twin.log
awk '/^TRACE-BEGIN$/{f=1;next}/^TRACE-END$/{f=0}f' twin.log > "$out"
[ -s "$out" ] && echo "wrote $out" || { echo "no trace captured (see twin.log)"; exit 1; }

#!/usr/bin/env bash
# Build the macOS twin with the toolchain that ships with Xcode's command-line
# tools. No project file: one source, one binary.
set -euo pipefail
cd "$(dirname "$0")"
swiftc -O -o twin main.swift -framework Cocoa -framework CoreVideo
echo "built ./twin — run it, then flick upward on the trackpad once"

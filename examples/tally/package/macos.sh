#!/usr/bin/env bash
#
# macos.sh — build Tally.app, a real macOS application bundle.
#
#   ./package/macos.sh                 # build into build/Tally.app
#   ./package/macos.sh --open          # build and launch it
#   CODESIGN_ID="Developer ID Application: You (TEAMID)" ./package/macos.sh
#
# Signing is optional here and required for distribution. Without an identity the
# bundle runs locally but Gatekeeper will refuse it on another machine, and the
# Mac App Store needs a "3rd Party Mac Developer Application" identity plus a
# provisioning profile — see package/README.md.
set -euo pipefail
cd "$(dirname "$0")/.."

APP_NAME="Tally"
BUNDLE_ID="${BUNDLE_ID:-com.dougfritz.tally}"
VERSION="${VERSION:-0.1.0}"
BUILD="${BUILD:-1}"
OUT="build/${APP_NAME}.app"

rm -rf "$OUT"
mkdir -p "$OUT/Contents/MacOS" "$OUT/Contents/Resources"

# A universal binary, so one bundle runs on both Apple silicon and Intel — the
# store expects that rather than two uploads.
echo "building universal binary…"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o build/tally-arm64 .
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o build/tally-amd64 .
lipo -create -output "$OUT/Contents/MacOS/$APP_NAME" build/tally-arm64 build/tally-amd64
rm -f build/tally-arm64 build/tally-amd64

cp Tally.icns "$OUT/Contents/Resources/$APP_NAME.icns"

cat > "$OUT/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>${APP_NAME}</string>
  <key>CFBundleDisplayName</key><string>${APP_NAME}</string>
  <key>CFBundleIdentifier</key><string>${BUNDLE_ID}</string>
  <key>CFBundleVersion</key><string>${BUILD}</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleExecutable</key><string>${APP_NAME}</string>
  <key>CFBundleIconFile</key><string>${APP_NAME}</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>NSHighResolutionCapable</key><true/>
  <!-- Tally draws its own UI, so it is not a document-based app in the AppKit
       sense; it still declares the beancount type so Finder can open ledgers
       with it and the Open panel filters sensibly. -->
  <key>CFBundleDocumentTypes</key>
  <array>
    <dict>
      <key>CFBundleTypeName</key><string>Beancount Ledger</string>
      <key>CFBundleTypeRole</key><string>Editor</string>
      <key>LSHandlerRank</key><string>Alternate</string>
      <key>CFBundleTypeExtensions</key>
      <array><string>beancount</string><string>bean</string></array>
    </dict>
  </array>
</dict>
</plist>
PLIST

if [ -n "${CODESIGN_ID:-}" ]; then
  echo "signing as ${CODESIGN_ID}…"
  # The hardened runtime is required for notarization; entitlements stay minimal
  # because the app talks to nothing but the file the user chose.
  codesign --force --deep --options runtime \
    ${ENTITLEMENTS:+--entitlements "$ENTITLEMENTS"} \
    --sign "$CODESIGN_ID" "$OUT"
  codesign --verify --strict --verbose=2 "$OUT"
else
  echo "unsigned (set CODESIGN_ID to sign; required for distribution)"
fi

echo "built $OUT ($(du -sh "$OUT" | cut -f1))"
[ "${1:-}" = "--open" ] && open "$OUT"
exit 0

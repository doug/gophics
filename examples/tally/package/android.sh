#!/usr/bin/env bash
# android.sh — bind the Go side and assemble the APK.
#
#   ./package/android.sh            # debug APK
#   ./package/android.sh --release  # release APK (needs a keystore; see README)
set -euo pipefail
cd "$(dirname "$0")/.."

# Gradle 8.9 cannot parse classes from a JDK newer than 21 ("Unsupported class
# file major version"), and macOS may default to a much newer one. Pin to 21 when
# it is installed rather than failing three minutes into a build.
if [ -z "${JAVA_HOME:-}" ] && /usr/libexec/java_home -v 21 >/dev/null 2>&1; then
  JAVA_HOME="$(/usr/libexec/java_home -v 21)"
  export JAVA_HOME
fi
# The SDK location: gradle reads local.properties, which is machine-specific and
# not committed.
if [ ! -f android/local.properties ] && [ -d "$HOME/Library/Android/sdk" ]; then
  echo "sdk.dir=$HOME/Library/Android/sdk" > android/local.properties
fi

mkdir -p android/app/libs
echo "binding Go → aar…"
gomobile bind -target=android -androidapi 24 -o android/app/libs/tallymobile.aar ./mobile

TASK=assembleDebug
[ "${1:-}" = "--release" ] && TASK=assembleRelease
echo "gradle $TASK…"
(cd android && ./gradlew --no-daemon "$TASK")

find android/app/build/outputs/apk -name '*.apk' -print

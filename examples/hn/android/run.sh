#!/usr/bin/env bash
# One command: build the Go side, build the APK (incl. the native surface shim),
# then install + launch on a connected device/emulator.
#
#   ./run.sh              # HN app
#   ./run.sh --verify     # GPU bring-up scene (docs/mobile-gpu-bringup.md)
#   ./run.sh --build      # build only, don't install/launch
#
# Prereqs it will check (and auto-install the SDK bits it can): Android SDK,
# JDK 17+, gradle, gomobile. Set ANDROID_HOME if the SDK isn't in the default
# location.
set -euo pipefail

cd "$(dirname "$0")"
REPO_ROOT=$(cd ../../.. && pwd)

VERIFY=0
RUN=1
for arg in "$@"; do
  case "$arg" in
    --verify) VERIFY=1 ;;
    --build)  RUN=0 ;;
    -h|--help) sed -n '2,12p' "$0"; exit 0 ;;
    *) echo "unknown flag: $arg" >&2; exit 2 ;;
  esac
done

export ANDROID_HOME=${ANDROID_HOME:-${ANDROID_SDK_ROOT:-$HOME/Library/Android/sdk}}
[ -d "$ANDROID_HOME" ] || { echo "ANDROID_HOME not found ($ANDROID_HOME); install the Android SDK or set ANDROID_HOME." >&2; exit 1; }
ADB="$ANDROID_HOME/platform-tools/adb"
SDKMANAGER=$(command -v sdkmanager || echo "$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager")
NDK_VER=26.1.10909125
CMAKE_VER=3.22.1

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing tool: $1 — $2" >&2; exit 1; }; }
need gomobile "go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init"

# AGP 8.5 needs JDK 17–21; the machine default may be newer. Prefer an
# explicit JAVA_HOME in that range, else Android Studio's bundled JBR, else a
# JDK 21/17 from java_home. (Gradle itself is pinned via ./gradlew → 8.9.)
java_ok() { [ -x "$1/bin/java" ] && "$1/bin/java" -version 2>&1 | grep -qE '"(17|21)\.'; }
if ! { [ -n "${JAVA_HOME:-}" ] && java_ok "$JAVA_HOME"; }; then
  for cand in \
    "/Applications/Android Studio.app/Contents/jbr/Contents/Home" \
    "$(/usr/libexec/java_home -v 21 2>/dev/null)" \
    "$(/usr/libexec/java_home -v 17 2>/dev/null)"; do
    if java_ok "$cand"; then export JAVA_HOME="$cand"; break; fi
  done
fi
[ -n "${JAVA_HOME:-}" ] && java_ok "$JAVA_HOME" || { echo "need a JDK 17–21 (set JAVA_HOME); found: $(java -version 2>&1 | head -1)" >&2; exit 1; }
echo "→ JDK: $("$JAVA_HOME/bin/java" -version 2>&1 | head -1)"

# Ensure the NDK + CMake the native surface shim needs are installed.
ensure_sdk() { # <package> <install-dir-under-ANDROID_HOME>
  [ -d "$ANDROID_HOME/$2" ] && return 0
  echo "→ installing $1 (one-time)"
  [ -x "$SDKMANAGER" ] || { echo "sdkmanager not found; install \"$1\" via Android Studio > SDK Manager." >&2; exit 1; }
  # yes|sdkmanager races SIGPIPE against pipefail; run it detached from -e/pipefail
  # and confirm by the install dir instead of the pipeline's exit status.
  # --sdk_root pins the target: a Homebrew sdkmanager otherwise installs into
  # its own SDK, not the one gradle uses.
  (set +o pipefail +e; yes | "$SDKMANAGER" --sdk_root="$ANDROID_HOME" "$1" >/dev/null 2>&1) || true
  [ -d "$ANDROID_HOME/$2" ] || { echo "failed to install $1" >&2; exit 1; }
}
ensure_sdk "ndk;$NDK_VER"       "ndk/$NDK_VER"
ensure_sdk "cmake;$CMAKE_VER"   "cmake/$CMAKE_VER"

echo "→ gomobile bind (Go → app/libs/hnmobile.aar)"
(cd "$REPO_ROOT" && gomobile bind -target=android -androidapi 24 \
  -o examples/hn/android/app/libs/hnmobile.aar ./examples/hn/mobile)

GRADLE_ARGS=(:app:assembleDebug)
[ "$VERIFY" = 1 ] && GRADLE_ARGS+=(-Pverify)
echo "→ ./gradlew ${GRADLE_ARGS[*]}"
./gradlew "${GRADLE_ARGS[@]}"

APK=app/build/outputs/apk/debug/app-debug.apk
echo "→ built $APK"
echo "  packaged native libs:"; unzip -l "$APK" | grep -E '\.so$' | awk '{print "    "$4}' || echo "    (none — native build FAILED)"

if [ "$RUN" = 1 ]; then
  if [ -x "$ADB" ] && [ -n "$("$ADB" devices | sed '1d' | grep -w device || true)" ]; then
    echo "→ installing + launching"
    "$ADB" install -r "$APK"
    "$ADB" shell am start -n dev.gossamer.hn/.MainActivity
    echo "→ logs: $ADB logcat -s gossamer:* AndroidRuntime:E"
  else
    echo "→ no device/emulator connected; skipping install. Start one, then:"
    echo "    $ADB install -r $APK && $ADB shell am start -n dev.gossamer.hn/.MainActivity"
  fi
fi

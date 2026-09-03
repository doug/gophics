#!/usr/bin/env bash
# Build the Android twin with four SDK tools and no Gradle: aapt2 links the
# manifest, javac compiles against android.jar, d8 dexes, apksigner signs with
# the debug key. --release 17 keeps the class files where d8 can read them
# regardless of how new the host JDK is.
set -euo pipefail
cd "$(dirname "$0")"
SDK="${ANDROID_SDK_ROOT:-$HOME/Library/Android/sdk}"
BT="$SDK/build-tools/$(ls "$SDK/build-tools" | sort -V | tail -1)"
PLAT="$SDK/platforms/$(ls "$SDK/platforms" | sort -V | tail -1)/android.jar"
rm -rf build && mkdir -p build/classes build/dex
"$BT/aapt2" link -o build/base.apk -I "$PLAT" --manifest AndroidManifest.xml
javac --release 17 -cp "$PLAT" -d build/classes src/com/gophics/twin/MainActivity.java
"$BT/d8" --release --lib "$PLAT" --output build/dex build/classes/com/gophics/twin/*.class
cp build/base.apk build/twin-unsigned.apk
(cd build/dex && zip -q -u ../twin-unsigned.apk classes.dex)
"$BT/zipalign" -f 4 build/twin-unsigned.apk build/twin-aligned.apk
KS="$HOME/.android/debug.keystore"
if [ ! -f "$KS" ]; then
	keytool -genkeypair -v -keystore "$KS" -storepass android -alias androiddebugkey -keypass android \
		-keyalg RSA -keysize 2048 -validity 10000 -dname "CN=Android Debug,O=Android,C=US" >/dev/null 2>&1
fi
"$BT/apksigner" sign --ks "$KS" --ks-pass pass:android --key-pass pass:android \
	--ks-key-alias androiddebugkey --out build/twin.apk build/twin-aligned.apk
echo "built build/twin.apk"

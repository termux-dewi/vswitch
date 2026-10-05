#!/bin/bash
# Build the vSwitch Android APK.
# Requires: Android SDK (ANDROID_HOME set), JDK 17+.
# Usage: cd android && ./build.sh

set -e
cd "$(dirname "$0")"

if [ -z "$ANDROID_HOME" ]; then
    echo "ERROR: ANDROID_HOME is not set. Install Android SDK and export ANDROID_HOME."
    exit 1
fi

# Ensure local.properties points to SDK
echo "sdk.dir=$ANDROID_HOME" > local.properties

# Use gradle wrapper if available, otherwise system gradle
if [ -f "./gradlew" ]; then
    ./gradlew assembleDebug
else
    gradle assembleDebug
fi

echo ""
echo "APK: app/build/outputs/apk/debug/app-debug.apk"

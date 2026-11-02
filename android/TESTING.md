# Android test setup

The Android app is a WebView interface backed by the native Go core, Media3, ML Kit and Whisper.cpp. It does not use Compose, Hilt, Room or a remote PC server. Tests use the existing plain JUnit4/AndroidX instrumentation runner; a new dependency-injection framework is unnecessary for these process-level checks.

Tests run only on an isolated, headless emulator with audio disabled. No physical Samsung device, Windows app, saved Windows library or account is used. Generated silent 24-bit WAV/FLAC fixtures verify metadata, import deduplication, Unicode lyric persistence, FFmpeg/yt-dlp executable startup, native pause/seek, and WebView startup with all nine themes/104 styles. The timing matcher must reject mismatched text and mark valid token candidates for review.

Run with an Android SDK/NDK, Go 1.26 and JDK17:

```powershell
./build-core.ps1 -Abi x86_64
gradle -PnawaAbis=x86_64 :app:assembleDebug :app:assembleDebugAndroidTest
adb -s emulator-5580 install -r app/build/outputs/apk/debug/app-debug.apk
adb -s emulator-5580 install -r app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk
adb -s emulator-5580 shell am instrument -w dev.nawa.music.debug.test/androidx.test.runner.AndroidJUnitRunner
```

Runtime validation results are recorded in `VALIDATION.json` when the release is packaged. Emulator tests cannot establish physical Samsung output sample rate, licensed Dolby availability, battery drain, or sustained native 120Hz rendering. Remote source availability is separate from the local test results.

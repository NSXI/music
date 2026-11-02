package dev.nawa.music

// Instrumentation-only foreground host: avoids a WebView changing player state
// and satisfies Android 15's real audio-focus eligibility requirement.
class NativePlaybackTestActivity : android.app.Activity() {
    override fun onCreate(state: android.os.Bundle?) {
        super.onCreate(state)
        setContentView(android.widget.TextView(this).apply {text="Nawa native playback verification"})
    }
}

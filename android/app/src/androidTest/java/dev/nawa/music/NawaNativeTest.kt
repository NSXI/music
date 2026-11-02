package dev.nawa.music

import android.content.Intent
import androidx.test.core.app.ApplicationProvider
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

@RunWith(AndroidJUnit4::class)
class NawaNativeTest {
    @Test fun pairedUpdateVersionMatchesInstalledAndroidPackage() {
        val app=app();val info=app.packageManager.getPackageInfo(app.packageName,0)
        val status=CoreBridge.call("GetUpdateStatus") as JSONObject
        assertEquals("android-arm64",status.getString("platform"));assertEquals(info.longVersionCode,status.getLong("currentCode"));assertEquals(info.versionName,status.getString("currentVersion"))
        assertEquals("https://github.com/NSXI/music/releases/latest/download/nawa-update.json",status.getString("feed"))
    }
    private fun app(): NawaApp = (ApplicationProvider.getApplicationContext<android.content.Context>() as NawaApp).also {
        assertTrue("Core startup timeout",it.ready.await(120,TimeUnit.SECONDS));assertNull("Core startup",it.startupError)
    }
    private fun fixture(app: NawaApp): File {
        val file=File(app.root,"native-test.wav")
        val rate=48000;val count=rate*2;val b=ByteBuffer.allocate(44+count*3*2).order(ByteOrder.LITTLE_ENDIAN)
        b.put("RIFF".toByteArray()).putInt(b.capacity()-8).put("WAVEfmt ".toByteArray()).putInt(16).putShort(1).putShort(2).putInt(rate).putInt(rate*6).putShort(6).putShort(24).put("data".toByteArray()).putInt(count*6)
        // Silence generated for tests; no user audio is opened or played.
        file.writeBytes(b.array());return file
    }
    @Test fun coreUnicodePersistenceAndTrueQuality() {
        val app=app();val wav=fixture(app)
        val probe=NativeWorkers(app).probe(wav.absolutePath).getJSONArray("streams").getJSONObject(0)
        assertEquals(24,probe.getInt("bits_per_sample"));assertEquals("48000",probe.getString("sample_rate"))
        val flac=File(app.root,"native-test.flac");app.ffmpeg("-v","error","-y","-i",wav.absolutePath,"-c:a","flac",flac.absolutePath)
        val fp=NativeWorkers(app).probe(flac.absolutePath).getJSONArray("streams").getJSONObject(0)
        assertEquals("flac",fp.getString("codec_name"));assertEquals(24,fp.getInt("bits_per_sample"))
        val paths=JSONArray().put(JSONArray().put(flac.absolutePath));CoreBridge.call("MobileImport",paths)
        assertEquals(0,(CoreBridge.call("MobileImport",paths) as Number).toInt())
        val state=CoreBridge.call("GetState") as JSONObject
        val jobs=state.getJSONArray("jobs");assertTrue(jobs.length()>0)
        val track=(0 until jobs.length()).map {jobs.getJSONObject(it)}.first {it.getString("path")==flac.absolutePath}
        val text="[00:00.10]نَوى 音楽 Музыка Music\n[00:01.00]صوت جديد"
        CoreBridge.call("SaveLyricsText",JSONArray().put(track.getString("id")).put(text).put(true))
        val disk=File(app.root,"data/library.json");assertTrue(disk.exists());assertTrue(disk.readText().contains("native-test"))
        val result=CoreBridge.call("GetLyrics",JSONArray().put(track.getString("id")).put(false)).toString()
        assertTrue(result.contains("نَوى"));assertTrue(result.contains("音楽"));assertTrue(result.contains("Музыка"))
    }
    @Test fun nativeToolsAreExecutableOnAndroid() {
        val app=app();assertTrue(app.ffmpeg("-version").contains("ffmpeg version"))
        val p=ProcessBuilder("${app.applicationInfo.nativeLibraryDir}/libpython.so","${app.tools}/yt-dlp/yt-dlp","--version").redirectErrorStream(true)
        p.environment().putAll(app.processEnvironment);val process=p.start();val out=process.inputStream.bufferedReader().readText();assertEquals(out,0,process.waitFor());assertTrue(out.trim().matches(Regex("\\d{4}\\.\\d{2}\\.\\d{2}.*")))
    }
    @Test fun timingRejectsMismatchInsteadOfInventingWords() {
        val app=app();val engine=TimingEngine(app)
        val raw=JSONObject().put("language","en").put("tokens",JSONArray().put(JSONObject().put("hex","206d75736963").put("start",1.0).put("end",1.5).put("probability",.9)))
        val matched=engine.match("Music!",raw);assertTrue(matched.getJSONObject("report").getBoolean("needsReview"));assertFalse(matched.getJSONObject("report").getBoolean("refined"))
        try {engine.match("Different words",raw);fail("Mismatch must be rejected")}catch(_: IllegalStateException){}
    }
    @Test fun webViewBootAndAllThemesRender() {
        app()
        ActivityScenario.launch(MainActivity::class.java).use { scenario ->
            val deadline=System.currentTimeMillis()+60000;var result=""
            while(System.currentTimeMillis()<deadline) {
                val done=CountDownLatch(1)
                scenario.onActivity { activity -> if(runCatching {activity.web}.isSuccess)activity.web.evaluateJavascript("JSON.stringify({ready:typeof ui!=='undefined',themes:typeof themes!=='undefined'?themes.length:0,styles:typeof lyricStyles!=='undefined'?lyricStyles.length:0,bridge:!!window.NawaMobile,overflow:document.documentElement.scrollWidth>innerWidth})") { result=it;done.countDown() } else done.countDown() }
                done.await(5,TimeUnit.SECONDS)
                if(result.contains("themes")&&result.contains("107"))break
                Thread.sleep(300)
            }
            assertTrue(result,result.contains("107"));assertTrue(result,result.contains("themes\\\":9"));assertTrue(result,result.contains("bridge\\\":true"));assertFalse(result,result.contains("overflow\\\":true"))
            scenario.onActivity { it.web.evaluateJavascript("ui.motion=false;applyAppearance()",null) }
            for(theme in listOf("glass","orchard","pulse","cloud","prism","noir","bloodborne","kingdom","reload")) {
                val done=CountDownLatch(1);var selected=""
                scenario.onActivity { a -> a.web.evaluateJavascript("ui.theme='$theme';applyAppearance();document.body.dataset.theme") { selected=it;done.countDown() } }
                assertTrue("Theme callback: $theme",done.await(15,TimeUnit.SECONDS));assertEquals("\"$theme\"",selected)
            }
        }
    }
    @Test fun silentNativePlaybackPauseAndSeek() {
        val app=app();fixture(app)
        val instrument=InstrumentationRegistry.getInstrumentation()
        instrument.runOnMainSync {app.startService(Intent(app,PlaybackService::class.java))}
        val deadline=System.currentTimeMillis()+10000;while(PlaybackService.instance==null&&System.currentTimeMillis()<deadline)Thread.sleep(100)
        assertNotNull(PlaybackService.instance)
        instrument.runOnMainSync {
            val s=PlaybackService.instance!!
            s.command("fx",JSONObject().put("volume",0).put("speed",1.0).put("preservePitch",true))
            s.command("load",JSONObject().put("id","test-silence").put("url",File(app.root,"native-test.wav").toURI().toString()).put("title","Silent native test"))
            s.command("play",JSONObject())
        }
        Thread.sleep(600)
        instrument.runOnMainSync {
            val s=PlaybackService.instance!!;assertNull(s.player.playerError);assertTrue(s.player.duration>=1900)
            s.command("pause",JSONObject());s.command("seek",JSONObject().put("position",1.1));assertEquals(1100,s.player.currentPosition);assertFalse(s.player.playWhenReady)
        }
    }
}

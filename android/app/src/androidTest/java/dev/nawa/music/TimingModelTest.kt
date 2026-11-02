package dev.nawa.music

import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Test
import org.junit.Assume.assumeTrue
import org.junit.Assert.*
import org.junit.runner.RunWith
import org.json.JSONObject
import java.io.File
import java.util.concurrent.TimeUnit

@RunWith(AndroidJUnit4::class)
class TimingModelTest {
    @Test fun optionalGeneratedSpeechInference() {
        val app=ApplicationProvider.getApplicationContext<NawaApp>();assertTrue(app.ready.await(120,TimeUnit.SECONDS));assertNull(app.startupError)
        val model=File(app.root,"models/ggml-base.bin");val audio=File(app.root,"test-speech.wav")
        assumeTrue("Install the verified model and generated original test-speech.wav fixture",model.isFile&&audio.isFile)
        val pcm=File(app.cacheDir,"timing-test.f32")
        app.ffmpeg("-v","error","-y","-i",audio.absolutePath,"-ac","1","-ar","16000","-c:a","pcm_f32le","-f","f32le",pcm.absolutePath)
        val engine=TimingEngine(app)
        val method=TimingEngine::class.java.getDeclaredMethod("nativeTokens",ByteArray::class.java,ByteArray::class.java,ByteArray::class.java,ByteArray::class.java,Int::class.javaPrimitiveType)
        method.isAccessible=true
        val text="This is a test of music timing.\nEvery word follows the sound."
        val started=System.currentTimeMillis()
        val raw=JSONObject(String(method.invoke(engine,model.absolutePath.toByteArray(),pcm.absolutePath.toByteArray(),"en".toByteArray(),text.toByteArray(),4) as ByteArray,Charsets.UTF_8))
        assertFalse(raw.toString(),raw.has("error"));assertTrue(raw.getJSONArray("tokens").length()>0)
        File(app.cacheDir,"generated-speech-inference.json").writeText(raw.put("elapsedMs",System.currentTimeMillis()-started).toString())
        val matched=engine.match(text,raw)
        assertEquals(12,matched.getJSONObject("report").getInt("words"))
        assertTrue(matched.getJSONObject("report").getBoolean("textMatched"))
        File(app.cacheDir,"generated-speech-inference.json").writeText(raw.put("elapsedMs",System.currentTimeMillis()-started).toString())
        pcm.delete()
    }
}

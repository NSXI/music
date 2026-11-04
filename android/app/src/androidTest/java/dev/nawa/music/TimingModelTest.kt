package dev.nawa.music

import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Test
import org.junit.Assume.assumeTrue
import org.junit.Assert.*
import org.junit.runner.RunWith
import org.json.JSONObject
import org.json.JSONArray
import java.io.File
import java.util.concurrent.TimeUnit

@RunWith(AndroidJUnit4::class)
class TimingModelTest {
    @Test fun optionalRangeInferencePreservesOutsideTimingUntilReview() {
        val app=ApplicationProvider.getApplicationContext<NawaApp>();assertTrue(app.ready.await(120,TimeUnit.SECONDS));assertNull(app.startupError)
        val model=File(app.root,"models/ggml-base.bin");val speech=File(app.root,"test-speech.wav")
        assumeTrue("Verified local model and generated speech required",model.isFile&&speech.isFile)
        val extended=File(app.root,"v2-range-generated-speech.wav")
        app.ffmpeg("-v","error","-y","-i",speech.absolutePath,"-af","apad=pad_dur=4","-ac","1","-ar","16000","-c:a","pcm_s16le",extended.absolutePath)
        CoreBridge.call("MobileImport",JSONArray().put(JSONArray().put(extended.absolutePath)))
        val state=CoreBridge.call("GetState") as JSONObject;val prefs=state.getJSONObject("preferences");val oldAuto=prefs.optBoolean("autoTranslate");prefs.put("autoTranslate",false);CoreBridge.call("SetPreferences",JSONArray().put(prefs))
        try {
            val jobs=state.getJSONArray("jobs");val task=(0 until jobs.length()).map {jobs.getJSONObject(it)}.first {it.optString("path")==extended.absolutePath};val id=task.getString("id");val duration=task.getJSONObject("song").getDouble("duration");val end=duration-4+.05;val outside=end+1.5
            fun lrc(at:Double):String {val ms=(at*1000).toLong();return "[%02d:%02d.%03d]".format(ms/60000,(ms/1000)%60,ms%1000)}
            val text=lrc(0.0)+"This is a test of music timing. Every word follows the sound.\n"+lrc(outside)+"Original outside fixture line"
            CoreBridge.call("SaveLyricsText",JSONArray().put(id).put(text).put(true));val before=CoreBridge.call("GetLyricDocument",JSONArray().put(id)) as JSONObject
            CoreBridge.call("StartRangeAlignment",JSONArray().put(id).put(0).put(end).put("en"))
            val deadline=System.currentTimeMillis()+360000;var result=JSONObject()
            while(System.currentTimeMillis()<deadline){result=(CoreBridge.call("GetState") as JSONObject).getJSONObject("alignment");if(!result.optBoolean("running"))break;Thread.sleep(350)}
            assertFalse("Range inference deadline: $result",result.optBoolean("running"));assertEquals(result.toString(),"",result.optString("error"));assertTrue(result.getBoolean("range"))
            val unchanged=CoreBridge.call("GetLyricDocument",JSONArray().put(id)) as JSONObject;assertEquals(before.getString("revision"),unchanged.getString("revision"))
            val preview=CoreBridge.call("GetRangeAlignmentPreview",JSONArray().put(id)) as JSONObject;val lines=preview.getJSONArray("lines");var words=0;for(i in 0 until lines.length())words+=lines.getJSONObject(i).getJSONArray("words").length();assertEquals(12,words)
            val applied=CoreBridge.call("ApplyRangeAlignment",JSONArray().put(id)) as JSONObject;val saved=applied.getJSONArray("lines");assertEquals(outside,saved.getJSONObject(saved.length()-1).getDouble("start"),.002);assertEquals("Original outside fixture line",saved.getJSONObject(saved.length()-1).getString("text"))
            File(app.cacheDir,"v2-range-inference-proof.json").writeText(JSONObject().put("words",words).put("reviewRequired",true).put("outsidePreserved",true).toString())
        } finally {prefs.put("autoTranslate",oldAuto);CoreBridge.call("SetPreferences",JSONArray().put(prefs))}
    }
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

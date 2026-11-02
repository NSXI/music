package dev.nawa.music

import android.content.Intent
import androidx.test.core.app.ApplicationProvider
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.Before
import org.junit.After
import org.junit.runner.RunWith
import java.io.File
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.net.ServerSocket
import java.net.InetAddress
import java.util.concurrent.TimeUnit
import kotlin.math.sin

@androidx.annotation.OptIn(androidx.media3.common.util.UnstableApi::class)
@RunWith(AndroidJUnit4::class)
class PlaybackRegressionTest {
    private lateinit var host: ActivityScenario<NativePlaybackTestActivity>
    @Before fun foregroundAudioHost() { host=ActivityScenario.launch(NativePlaybackTestActivity::class.java) }
    @After fun closeAudioHost() { main {PlaybackService.instance?.command("pause",JSONObject())};host.close() }
    private val instrumentation get()=InstrumentationRegistry.getInstrumentation()
    private fun main(action:()->Unit)=instrumentation.runOnMainSync(action)
    private fun app(): NawaApp {
        val app=ApplicationProvider.getApplicationContext<android.content.Context>() as NawaApp
        assertTrue(app.ready.await(120,TimeUnit.SECONDS));assertNull(app.startupError)
        main {app.startService(Intent(app,PlaybackService::class.java))}
        val until=System.currentTimeMillis()+10000
        while(PlaybackService.instance==null&&System.currentTimeMillis()<until)Thread.sleep(100)
        assertNotNull(PlaybackService.instance);return app
    }
    private fun tone(app: NawaApp): File {
        val rate=48000;val samples=rate*8;val data=ByteBuffer.allocate(44+samples*4).order(ByteOrder.LITTLE_ENDIAN)
        data.put("RIFF".toByteArray()).putInt(data.capacity()-8).put("WAVEfmt ".toByteArray()).putInt(16).putShort(1).putShort(2).putInt(rate).putInt(rate*4).putShort(4).putShort(16).put("data".toByteArray()).putInt(samples*4)
        for(i in 0 until samples){val value=(sin(i*2.0*Math.PI*440/rate)*5000).toInt().toShort();data.putShort(value).putShort(value)}
        return File(app.root,"playback-regression.wav").apply {writeBytes(data.array())}
    }
    private fun waitForPlaying(): JSONObject {
        var state=JSONObject();val until=System.currentTimeMillis()+20000
        while(System.currentTimeMillis()<until){main {state=PlaybackService.instance!!.state()};assertEquals(state.toString(),"",state.optString("error"));if(state.optBoolean("playing")&&state.optDouble("position")>.25)return state;Thread.sleep(100)}
        fail("Audio output failed to advance: $state");return state
    }
    @Test fun libraryHttpFormatsRenderRealBuffersAndResumeDoesNotRestart() {
        val app=app();val wav=tone(app)
        val fixtures=listOf("wav" to wav,"flac" to File(app.root,"playback-regression.flac"),"mp3" to File(app.root,"playback-regression.mp3"),"m4a" to File(app.root,"playback-regression.m4a"),"opus" to File(app.root,"playback-regression.opus"),"float" to File(app.root,"playback-regression-float.wav"))
        for((format,file) in fixtures){
            if(format!="wav") {val codec=when(format){"flac"->"flac";"mp3"->"libmp3lame";"m4a"->"aac";"opus"->"libopus";else->"pcm_f32le"};app.ffmpeg("-v","error","-y","-i",wav.absolutePath,"-c:a",codec,file.absolutePath)}
            CoreBridge.call("MobileImport",JSONArray().put(JSONArray().put(file.absolutePath)))
            val jobs=(CoreBridge.call("GetState") as JSONObject).getJSONArray("jobs")
            val job=(0 until jobs.length()).map {jobs.getJSONObject(it)}.first {it.optString("path")==file.absolutePath}
            val id=job.getString("id");val playback=CoreBridge.call("PreparePlayback",JSONArray().put(id)) as JSONObject
            val item=JSONObject().put("id",id).put("title","Original generated test $format").put("url",playback.getString("url")).put("duration",8)
            main {val s=PlaybackService.instance!!;assertEquals("Playback session must be registered without an external controller",1,s.sessions.size);s.command("output",JSONObject().put("mode","compatible"));s.command("fx",JSONObject().put("volume",.7).put("speed",1).put("preamp",0));s.command("load",item);s.command("queue",JSONObject().put("items",JSONArray().put(item)));s.command("play",JSONObject())}
            val state=waitForPlaying();assertEquals("compatible",state.getString("effectiveOutput"));assertTrue(state.getDouble("volume")>.6);assertEquals(8.0,state.getDouble("duration"),.15)
            main {val s=PlaybackService.instance!!;val counters=s.player.audioDecoderCounters;assertNotNull("Decoder counters for $format",counters);counters!!.ensureUpdated();assertTrue("No rendered audio buffers: $format",counters.renderedOutputBufferCount>0);s.command("pause",JSONObject());s.command("seek",JSONObject().put("position",3).put("_seekToken",100));assertEquals(3000,s.player.currentPosition);s.command("queue",JSONObject().put("items",JSONArray().put(item)));assertEquals("Resume queue changed position",3000,s.player.currentPosition);s.command("play",JSONObject())}
            val resumed=waitForPlaying();assertTrue(resumed.getDouble("position")>=3)
            main {PlaybackService.instance!!.command("pause",JSONObject())}
        }
    }
    @Test fun staleCommandsCannotReplaceNewLoadAndLazyEffectsStayOff() {
        val app=app();val file=tone(app)
        main {
            val s=PlaybackService.instance!!;val token=s.state().getLong("loadToken")+10
            s.command("fx",JSONObject().put("volume",.7).put("speed",1).put("reverb",0).put("width",1))
            s.command("load",JSONObject().put("id","new").put("url",file.toURI().toString()).put("_loadToken",token))
            s.command("load",JSONObject().put("id","old").put("url",file.toURI().toString()).put("_loadToken",token-1))
            s.command("play",JSONObject().put("_loadToken",token-1));assertEquals("new",s.player.currentMediaItem?.mediaId);assertFalse(s.player.playWhenReady)
            s.command("seek",JSONObject().put("position",4).put("_seekToken",5).put("_loadToken",token));s.command("seek",JSONObject().put("position",1).put("_seekToken",4).put("_loadToken",token));assertEquals(4000,s.player.currentPosition)
        }
    }
    @Test fun unseekableStreamingUsesDeclaredDurationAndSoftwareSpeed() {
        val app=app();val file=tone(app)
        main {val s=PlaybackService.instance!!;s.command("load",JSONObject().put("id","stream").put("url",file.toURI().toString()).put("duration",8).put("seekable",false));s.command("fx",JSONObject().put("volume",.7).put("speed",1.25).put("preservePitch",true));s.command("play",JSONObject())}
        val state=waitForPlaying();assertEquals(8.0,state.getDouble("duration"),.01);assertFalse(state.getBoolean("seekable"));assertEquals(1.25,state.getDouble("speed"),.001)
        main {val s=PlaybackService.instance!!;s.command("seek",JSONObject().put("position",6));assertTrue("Disabled stream seek moved",s.player.currentPosition<4000);s.command("pause",JSONObject())}
    }
    @Test fun searchPreviewProxyAndDecodedStreamRenderAudioWithoutDownload() {
        val app=app();val bytes=tone(app).readBytes()
        ServerSocket(0,8,InetAddress.getByName("127.0.0.1")).use {server->
            val thread=Thread {
                while(!server.isClosed)runCatching {server.accept().use {socket->
                    socket.soTimeout=5000
                    val input=socket.getInputStream().bufferedReader();val request=input.readLine() ?: return@use
                    var range="";while(true){val line=input.readLine() ?: break;if(line.isEmpty())break;if(line.startsWith("Range:",true))range=line.substringAfter(':').trim()}
                    val start=Regex("bytes=(\\d+)-").find(range)?.groupValues?.get(1)?.toIntOrNull()?.coerceIn(0,bytes.size-1) ?: 0
                    val code=if(start>0)"206 Partial Content" else "200 OK"
                    val contentRange=if(start>0)"Content-Range: bytes $start-${bytes.size-1}/${bytes.size}\r\n" else ""
                    val output=socket.getOutputStream();output.write(("HTTP/1.1 $code\r\nContent-Type: audio/wav\r\nAccept-Ranges: bytes\r\nContent-Length: ${bytes.size-start}\r\n${contentRange}Connection: close\r\n\r\n").toByteArray())
                    if(!request.startsWith("HEAD"))output.write(bytes,start,bytes.size-start);output.flush()
                }}
            }.apply {isDaemon=true;start()}
            val result=CoreBridge.call("Search",JSONArray().put("http://127.0.0.1:${server.localPort}/original-tone.wav").put("direct")) as JSONObject
            val song=result.getJSONArray("tracks").getJSONObject(0)
            for(decoded in listOf(false,true)){
                val preview=CoreBridge.call("PreparePreview",JSONArray().put(song.getString("id")).put(decoded)) as JSONObject
                assertTrue(preview.getString("url").contains("/stream/"))
                main {val s=PlaybackService.instance!!;s.command("fx",JSONObject().put("volume",.7).put("speed",1).put("preamp",0));s.command("load",JSONObject().put("id",song.getString("id")).put("url",preview.getString("url")).put("duration",8).put("seekable",!decoded));s.command("play",JSONObject())}
                val state=waitForPlaying();assertEquals("Preview duration decoded=$decoded",8.0,state.getDouble("duration"),.15)
                main {val s=PlaybackService.instance!!;val counters=s.player.audioDecoderCounters!!;counters.ensureUpdated();assertTrue(counters.renderedOutputBufferCount>0);s.command("pause",JSONObject())}
                if(decoded)main {val s=PlaybackService.instance!!;s.command("load",JSONObject().put("id","unknown-stream").put("url",preview.getString("url")).put("duration",0).put("seekable",false));assertEquals("Unknown stream is not an enormous WAV length",0.0,s.state().getDouble("duration"),.01)}
            }
            server.close();thread.join(2000)
        }
    }
}

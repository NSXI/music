package dev.nawa.music

import android.content.Intent
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.json.JSONArray
import org.json.JSONObject
import org.junit.After
import org.junit.Before
import org.junit.Test
import org.junit.Assert.*
import org.junit.runner.RunWith
import java.io.File
import java.util.concurrent.TimeUnit

@androidx.annotation.OptIn(androidx.media3.common.util.UnstableApi::class)
@RunWith(AndroidJUnit4::class)
class V2RegressionTest {
    private lateinit var host: ActivityScenario<NativePlaybackTestActivity>
    private lateinit var app: NawaApp
    private fun main(action:()->Unit)=InstrumentationRegistry.getInstrumentation().runOnMainSync(action)
    @Before fun setup(){
        host=ActivityScenario.launch(NativePlaybackTestActivity::class.java)
        app=ApplicationProvider.getApplicationContext<android.content.Context>() as NawaApp
        assertTrue(app.ready.await(120,TimeUnit.SECONDS));assertNull(app.startupError)
        main {app.startService(Intent(app,PlaybackService::class.java))}
        val deadline=System.currentTimeMillis()+10000
        while(PlaybackService.instance==null&&System.currentTimeMillis()<deadline)Thread.sleep(50)
        assertNotNull(PlaybackService.instance)
        main {val s=PlaybackService.instance!!;s.command("sleep",JSONObject().put("deadline",0).put("endOfTrack",false));s.command("output",JSONObject().put("mode","compatible"));s.command("fx",JSONObject().put("volume",.5).put("preamp",0).put("speed",1).put("pitch",0).put("crossfade",0))}
    }
    @After fun cleanup(){main {PlaybackService.instance?.command("sleep",JSONObject().put("deadline",0));PlaybackService.instance?.command("pause",JSONObject())};CoreBridge.call("StopDeviceLink");host.close()}
    private fun generatedTone(): File=File(app.root,"v2-original-tone.wav").also {if(!it.exists())app.ffmpeg("-v","error","-y","-f","lavfi","-i","sine=frequency=317:sample_rate=48000:duration=8","-ac","2",it.absolutePath)}
    private fun state(): JSONObject {var out=JSONObject();main {out=PlaybackService.instance!!.state()};return out}
    private fun awaitState(timeout:Long=15000,predicate:(JSONObject)->Boolean): JSONObject{val deadline=System.currentTimeMillis()+timeout;var s=JSONObject();while(System.currentTimeMillis()<deadline){s=state();assertEquals(s.toString(),"",s.optString("error"));if(predicate(s))return s;Thread.sleep(50)};fail("State deadline: $s");return s}
    private fun item(id:String,file:File)=JSONObject().put("id",id).put("title","Original v2 test").put("url",file.toURI().toString()).put("duration",8)
    @Test fun schedulerUsesDeviceTimezoneAndSystemOnlyBinding(){
        val info=app.packageManager.getServiceInfo(android.content.ComponentName(app,DownloadJobService::class.java),0)
        assertTrue(info.exported);assertEquals("android.permission.BIND_JOB_SERVICE",info.permission)
        assertEquals(android.content.pm.PackageManager.PERMISSION_GRANTED,app.checkSelfPermission("android.permission.RECEIVE_BOOT_COMPLETED"))
        try {CoreBridge.call("SetTransferTimezoneOffset",JSONArray().put(19800));val status=CoreBridge.call("GetTransferStatus") as JSONObject
            val hour=java.time.Instant.now().atOffset(java.time.ZoneOffset.ofTotalSeconds(19800)).hour;assertEquals(hour,status.getInt("hour"))
            CoreBridge.call("HoldTransferQueue");assertTrue((CoreBridge.call("GetTransferStatus") as JSONObject).getBoolean("held"))
            CoreBridge.call("WakeTransferQueue");assertFalse((CoreBridge.call("GetTransferStatus") as JSONObject).getBoolean("held"))
        }finally{CoreBridge.call("SetTransferTimezoneOffset",JSONArray().put(java.util.TimeZone.getDefault().getOffset(System.currentTimeMillis())/1000));CoreBridge.call("WakeTransferQueue")}
    }
    @Test fun nativeCrossfadePromotesQueuedAudioAndKeepsRegisteredSession(){
        val file=generatedTone();val first=item("v2-first",file);val second=item("v2-second",file)
        main {val s=PlaybackService.instance!!;s.command("load",first);s.command("queue",JSONObject().put("items",JSONArray().put(first).put(second)));s.command("fx",JSONObject().put("volume",.5).put("preamp",0).put("speed",1).put("pitch",0).put("crossfade",2));s.command("play",JSONObject())}
        awaitState {it.optBoolean("playing")&&it.optDouble("position")>.2}
        main {PlaybackService.instance!!.command("seek",JSONObject().put("position",4))}
        val fading=awaitState {it.optString("id")=="v2-second"&&it.optBoolean("crossfadeActive")&&it.optBoolean("playing")}
        assertTrue(fading.getDouble("position")<2.5)
        val after=awaitState {it.optString("id")=="v2-second"&&!it.optBoolean("crossfadeActive")&&it.optDouble("position")>1}
        assertTrue(after.getDouble("position")<4)
        main {assertEquals(1,PlaybackService.instance!!.sessions.size);val c=PlaybackService.instance!!.player.audioDecoderCounters;assertNotNull(c);c!!.ensureUpdated();assertTrue(c.renderedOutputBufferCount>0)}
    }
    @Test fun nativeIndependentPitchAndBothSleepModes(){
        val file=generatedTone();val first=item("v2-sleep-first",file);val second=item("v2-sleep-second",file)
        main {val s=PlaybackService.instance!!;s.command("load",first);s.command("queue",JSONObject().put("items",JSONArray().put(first).put(second)));s.command("fx",JSONObject().put("volume",.5).put("preamp",0).put("speed",1.25).put("preservePitch",true).put("pitch",3));s.command("play",JSONObject())}
        val playing=awaitState {it.optBoolean("playing")};assertEquals(1.25,playing.getDouble("speed"),.001);assertEquals(Math.pow(2.0,3.0/12),playing.getDouble("pitch"),.01)
        main {PlaybackService.instance!!.command("sleep",JSONObject().put("deadline",System.currentTimeMillis()+600))}
        awaitState {it.optBoolean("paused")&&it.optLong("sleepDeadline")==0L}
        main {val s=PlaybackService.instance!!;s.command("fx",JSONObject().put("volume",.5).put("preamp",0).put("speed",1).put("pitch",0));s.command("sleep",JSONObject().put("endOfTrack",true));s.command("seek",JSONObject().put("position",7.5));s.command("play",JSONObject())}
        val stopped=awaitState {it.optBoolean("paused")&&!it.optBoolean("sleepAtEnd")};assertEquals("v2-sleep-first",stopped.getString("id"));assertTrue(stopped.getDouble("position")>=7.5)
    }
    @Test fun sharedEditorWaveformAndLocalPairingUseActualAndroidCore(){
        val file=generatedTone();CoreBridge.call("MobileImport",JSONArray().put(JSONArray().put(file.absolutePath)))
        val jobs=(CoreBridge.call("GetState") as JSONObject).getJSONArray("jobs");val task=(0 until jobs.length()).map {jobs.getJSONObject(it)}.first {it.optString("path")==file.absolutePath};val id=task.getString("id")
        CoreBridge.call("SaveLyricsText",JSONArray().put(id).put("[00:00.100]Music in time\n[00:04.000]نَوى 音楽").put(true))
        val doc=CoreBridge.call("GetLyricDocument",JSONArray().put(id)) as JSONObject
        val line=JSONObject().put("text","Music in time").put("start",.1).put("end",3.0).put("words",JSONArray().put(JSONObject().put("text","Music").put("start",.1).put("end",1).put("probability",0)).put(JSONObject().put("text","in").put("start",1.1).put("end",1.5).put("probability",0)).put(JSONObject().put("text","time").put("start",1.7).put("end",3).put("probability",0)))
        doc.put("lines",JSONArray().put(line).put(JSONObject().put("text","نَوى 音楽").put("start",4).put("end",7).put("words",JSONArray())))
        val saved=CoreBridge.call("SaveLyricDocument",JSONArray().put(doc)) as JSONObject;assertEquals(3,saved.getJSONArray("lines").getJSONObject(0).getJSONArray("words").length())
        val waveform=CoreBridge.call("GetWaveform",JSONArray().put(id)) as JSONObject;assertTrue(waveform.getJSONArray("peaks").length()>0)
        val loudness=CoreBridge.call("AnalyzeLoudness",JSONArray().put(id)) as JSONObject;assertEquals(-18.0,loudness.getDouble("targetLUFS"),.001)
        val link=CoreBridge.call("StartDeviceLink",JSONArray().put(JSONArray().put(id))) as JSONObject;assertTrue(link.getBoolean("running"));val address=link.getJSONArray("urls").getString(0)
        val peer=CoreBridge.call("PreviewDeviceLibrary",JSONArray().put(address)) as JSONObject;assertEquals(1,peer.getJSONArray("tracks").length())
        CoreBridge.call("SendDeviceCommand",JSONArray().put(address).put("seek").put(2))
        val commands=CoreBridge.call("PollDeviceCommands",JSONArray().put("Fixture").put("Tests").put(0).put(8).put(true)) as JSONArray;assertEquals(1,commands.length());assertEquals("seek",commands.getJSONObject(0).getString("action"))
    }
}

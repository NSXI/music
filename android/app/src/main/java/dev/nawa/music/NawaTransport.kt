package dev.nawa.music

import android.media.audiofx.AudioEffect
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import androidx.media3.common.C
import androidx.media3.common.Player
import androidx.media3.exoplayer.ExoPlayer
import org.json.JSONObject

/** Native queue transitions and sleep timers do not depend on WebView frames. */
@androidx.annotation.OptIn(androidx.media3.common.util.UnstableApi::class)
class NawaTransport(private val service: PlaybackService) {
    private val handler=Handler(Looper.getMainLooper())
    private var warm: ExoPlayer?=null
    private var warmFrom=""
    private var outgoing: ExoPlayer?=null
    private var outgoingEffects: List<AudioEffect> = emptyList()
    private var fadeElapsed=0L
    private var fadeLength=1L
    private var lastTick=0L
    private var baseVolume=.7f
    private var outgoingVolume=.7f
    private var seconds=0.0
    private var sleepDeadline=0L
    private var sleepAtEnd=false
    private var sleepTrack=""
    private var stopped=false
    fun configure(data: JSONObject,volume: Float) {
        seconds=data.optDouble("crossfade",0.0).takeIf {it.isFinite()}?.coerceIn(0.0,12.0) ?: 0.0
        baseVolume=volume
        if(seconds==0.0)cancelFade() else applyVolumes()
    }
    fun setSleep(data: JSONObject) {
        val deadline=data.optLong("deadline",0L)
        require(deadline==0L||deadline in System.currentTimeMillis()..System.currentTimeMillis()+24*3600*1000L) {"Invalid sleep deadline"}
        sleepDeadline=deadline;sleepAtEnd=data.optBoolean("endOfTrack");sleepTrack=if(sleepAtEnd)service.player.currentMediaItem?.mediaId ?: "" else ""
        if(sleepAtEnd)cancelFade()
    }
    fun onAutomaticTransition(player: ExoPlayer,reason: Int) {
        if(reason!=Player.MEDIA_ITEM_TRANSITION_REASON_AUTO||!sleepAtEnd||service.player!==player)return
        sleepAtEnd=false;player.pause();cancelFade()
        val index=(0 until player.mediaItemCount).firstOrNull {player.getMediaItemAt(it).mediaId==sleepTrack}
        if(index!=null)player.seekTo(index,service.knownDuration(sleepTrack))
        sleepTrack=""
    }
    fun cancelFade() {
        warm?.release();warm=null;warmFrom=""
        outgoing?.release();outgoing=null
        outgoingEffects.forEach {runCatching {it.release()}};outgoingEffects=emptyList()
        if(!stopped)service.player.volume=baseVolume.coerceIn(0f,1f)
    }
    fun close(){stopped=true;handler.removeCallbacks(tick);cancelFade()}
    fun appendState(out: JSONObject): JSONObject = out.put("crossfadeActive",outgoing!=null).put("sleepDeadline",sleepDeadline).put("sleepAtEnd",sleepAtEnd)
    private fun applyVolumes(){
        val old=outgoing ?: return
        val fraction=(fadeElapsed.toDouble()/fadeLength).coerceIn(0.0,1.0).toFloat()
        old.volume=(outgoingVolume*(1-fraction)).coerceIn(0f,1f)
        service.player.volume=(baseVolume*fraction).coerceIn(0f,1f)
    }
    private val tick=object: Runnable {
        override fun run(){
            if(stopped)return
            val now=SystemClock.elapsedRealtime();val delta=if(lastTick==0L)0L else (now-lastTick).coerceIn(0,250);lastTick=now
            try {
                val player=service.player
                PlaybackWidget.refresh(service)
                if(sleepDeadline>0&&System.currentTimeMillis()>=sleepDeadline){sleepDeadline=0;player.pause();cancelFade()}
                val old=outgoing
                if(old!=null){
                    if(!player.playWhenReady){cancelFade()}
                    else {if(player.isPlaying)fadeElapsed+=delta;applyVolumes();if(fadeElapsed>=fadeLength||old.playbackState==Player.STATE_ENDED)cancelFade()}
                }else if(seconds>0&&!sleepAtEnd&&player.isPlaying&&player.duration>0&&player.isCurrentMediaItemSeekable){
                    val id=player.currentMediaItem?.mediaId ?: ""
                    if(warm!=null&&warmFrom!=id){warm?.release();warm=null}
                    val remaining=(player.duration-player.currentPosition).toDouble()/player.playbackParameters.speed
                    val nextIndex=player.nextMediaItemIndex
                    if(nextIndex!=C.INDEX_UNSET&&remaining>0&&remaining<seconds*1000+3500){
                        if(warm==null){
                            warmFrom=id
                            warm=service.makeCompanionPlayer().apply {
                                volume=0f;playbackParameters=player.playbackParameters
                                setMediaItems((0 until player.mediaItemCount).map {player.getMediaItemAt(it)},nextIndex,0)
                                repeatMode=player.repeatMode;shuffleModeEnabled=player.shuffleModeEnabled;prepare()
                            }
                        }
                        val next=warm
                        if(next!=null&&next.playbackState==Player.STATE_READY&&remaining<=seconds*1000){
                            outgoingVolume=player.volume;outgoing=player;fadeLength=remaining.toLong().coerceAtLeast(100);fadeElapsed=0
                            outgoingEffects=service.adoptCrossfadePlayer(next);warm=null;warmFrom=""
                            next.volume=0f;next.play()
                        }
                    }
                }
            } catch(_: Exception){cancelFade()}
            handler.postDelayed(this,if(outgoing!=null)50 else if(service.player.isPlaying)250 else 500)
        }
    }
    init {handler.post(tick)}

}

package dev.nawa.music

import android.app.PendingIntent
import android.content.Intent
import android.media.AudioManager
import android.media.audiofx.Equalizer
import android.media.audiofx.PresetReverb
import android.media.audiofx.Virtualizer
import android.os.SystemClock
import androidx.media3.common.AudioAttributes
import androidx.media3.common.AuxEffectInfo
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.MediaMetadata
import androidx.media3.common.PlaybackException
import androidx.media3.common.PlaybackParameters
import androidx.media3.common.Player
import androidx.media3.datasource.DefaultDataSource
import androidx.media3.datasource.DefaultHttpDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.DefaultRenderersFactory
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.session.MediaSession
import androidx.media3.session.MediaSessionService
import org.json.JSONObject
import kotlin.math.pow

@androidx.annotation.OptIn(androidx.media3.common.util.UnstableApi::class)
class PlaybackService : MediaSessionService() {
    lateinit var player: ExoPlayer
    private lateinit var session: MediaSession
    private var eq: Equalizer?=null
    private var reverb: PresetReverb?=null
    private var spatial: Virtualizer?=null
    private var effectSession=0
    private var fx=JSONObject()
    private var loadToken=0L
    private var seekToken=0L
    private var timelineRevision=0L
    private var floatOutput=false
    private var floatFailed=false
    private val durations=mutableMapOf<String,Double>()
    private val unseekable=mutableSetOf<String>()
    companion object { var instance: PlaybackService?=null }

    override fun onCreate() {
        super.onCreate()
        player=makePlayer(false)
        session=MediaSession.Builder(this,player).setSessionActivity(PendingIntent.getActivity(this,0,Intent(this,MainActivity::class.java),PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)).build()
        // This UI calls the player in-process rather than binding a MediaController.
        // onGetSession is therefore not the registration path for initial playback.
        addSession(session)
        applyFX(fx)
        instance=this
    }
    private fun makePlayer(useFloat: Boolean): ExoPlayer {
        val http=DefaultHttpDataSource.Factory().setDefaultRequestProperties(mapOf("X-Nawa-Token" to CoreBridge.token)).setConnectTimeoutMs(15000).setReadTimeoutMs(30000).setAllowCrossProtocolRedirects(false)
        val renderer=DefaultRenderersFactory(this).setEnableDecoderFallback(true)
            .setEnableAudioFloatOutput(useFloat).setEnableAudioTrackPlaybackParams(false)
        val result=ExoPlayer.Builder(this,renderer).setMediaSourceFactory(DefaultMediaSourceFactory(DefaultDataSource.Factory(this,http))).build()
        result.setAudioAttributes(AudioAttributes.Builder().setUsage(C.USAGE_MEDIA).setContentType(C.AUDIO_CONTENT_TYPE_MUSIC).build(),true)
        result.setHandleAudioBecomingNoisy(true)
        result.setWakeMode(C.WAKE_MODE_LOCAL)
        result.addListener(object: Player.Listener {
            override fun onPositionDiscontinuity(oldPosition: Player.PositionInfo,newPosition: Player.PositionInfo,reason: Int) { if(player===result)timelineRevision++ }
            override fun onAudioSessionIdChanged(id: Int) { if(player===result){releaseEffects();effectSession=id;applyEffects()} }
            override fun onPlayerError(error: PlaybackException) {
                // One bounded sink fallback, separate from the frontend's codec decode fallback.
                if(player===result&&floatOutput&&error.errorCode in listOf(PlaybackException.ERROR_CODE_AUDIO_TRACK_INIT_FAILED,PlaybackException.ERROR_CODE_AUDIO_TRACK_WRITE_FAILED)) {
                    floatFailed=true;rebuild(false)
                }
            }
        })
        return result
    }
    private fun rebuild(useFloat: Boolean) {
        if(floatOutput==useFloat)return
        val old=player
        val items=(0 until old.mediaItemCount).map { old.getMediaItemAt(it) }
        val index=old.currentMediaItemIndex.coerceAtLeast(0);val pos=old.currentPosition.coerceAtLeast(0);val playing=old.playWhenReady
        val repeat=old.repeatMode;val shuffle=old.shuffleModeEnabled
        releaseEffects();floatOutput=useFloat;player=makePlayer(useFloat)
        session.setPlayer(player);old.release()
        if(items.isNotEmpty())player.setMediaItems(items,index,pos)
        player.repeatMode=repeat;player.shuffleModeEnabled=shuffle
        applyFX(fx,false)
        if(items.isNotEmpty())player.prepare()
        player.playWhenReady=playing
    }
    override fun onGetSession(controllerInfo: MediaSession.ControllerInfo): MediaSession = session
    override fun onDestroy() { instance=null;releaseEffects();session.release();player.release();super.onDestroy() }
    private fun releaseEffects() {
        runCatching { eq?.release() };runCatching { reverb?.release() };runCatching { spatial?.release() }
        eq=null;reverb=null;spatial=null;effectSession=0
    }
    private fun media(data: JSONObject): MediaItem {
        val id=data.optString("id");durations[id]=data.optDouble("duration",0.0).takeIf {it.isFinite()&&it>0} ?: 0.0
        if(data.optBoolean("seekable",true))unseekable.remove(id) else unseekable.add(id)
        val art=data.optString("artwork")
        return MediaItem.Builder().setMediaId(id).setUri(CoreBridge.mediaUrl(data.getString("url")))
            .setMediaMetadata(MediaMetadata.Builder().setTitle(data.optString("title")).setArtist(data.optString("artist")).setAlbumTitle(data.optString("album"))
                .apply { if(art.isNotBlank())setArtworkUri(android.net.Uri.parse(CoreBridge.mediaUrl(art))) }.build()).build()
    }
    fun command(name: String,data: JSONObject) {
        val token=data.optLong("_loadToken",loadToken)
        if(name!="load"&&token!=loadToken)return
        when(name) {
            "load" -> {
                if(token<loadToken)return
                loadToken=token;seekToken=0
                player.pause();player.setMediaItem(media(data))
                // Prepare once after the queue is installed, rather than preparing twice.
            }
            "queue" -> {
                val list=data.getJSONArray("items")
                val unchanged=list.length()==player.mediaItemCount&&(0 until list.length()).all {i->val old=player.getMediaItemAt(i);val entry=list.getJSONObject(i);old.mediaId==entry.optString("id")&&old.localConfiguration?.uri?.toString()==CoreBridge.mediaUrl(entry.getString("url"))}
                if(unchanged)return
                val items=(0 until list.length()).map { media(list.getJSONObject(it)) }
                val index=items.indexOfFirst {it.mediaId==player.currentMediaItem?.mediaId}
                if(index>=0){val pos=player.currentPosition;val playing=player.playWhenReady;player.setMediaItems(items,index,pos);if(playing)player.prepare();player.playWhenReady=playing}
            }
            "play" -> {if(player.playbackState==Player.STATE_ENDED)player.seekToDefaultPosition();if(player.playbackState==Player.STATE_IDLE||player.playerError!=null)player.prepare();player.play()}
            "pause" -> player.pause()
            "seek" -> {
                val tokenSeek=data.optLong("_seekToken",seekToken+1);if(tokenSeek<seekToken)return;seekToken=tokenSeek
                val target=data.optDouble("position");require(target.isFinite()){"Invalid playback position"}
                if(player.currentMediaItem?.mediaId !in unseekable)player.seekTo((target.coerceAtLeast(0.0)*1000).toLong())
            }
            "repeat" -> player.repeatMode=if(data.optBoolean("repeat"))Player.REPEAT_MODE_ALL else Player.REPEAT_MODE_OFF
            "fx" -> applyFX(data)
            "output" -> {
                val mode=data.optString("mode");require(mode in listOf("compatible","float"))
                getSharedPreferences("nawa-mobile",MODE_PRIVATE).edit().putString("audioOutput",mode).apply();floatFailed=false;applyFX(fx)
            }
        }
    }
    private fun value(data: JSONObject,key: String,default: Double): Double = data.optDouble(key,default).takeIf {it.isFinite()} ?: default
    private fun applyFX(data: JSONObject,allowRebuild: Boolean=true) {
        fx=data
        val bypass=data.optBoolean("bypass")
        val speed=if(bypass)1f else value(data,"speed",1.0).toFloat().coerceIn(.25f,3f)
        val pitch=if(data.optBoolean("preservePitch")||bypass)1f else speed
        val requested=getSharedPreferences("nawa-mobile",MODE_PRIVATE).getString("audioOutput","compatible")=="float"
        // Float excludes Media3's software speed/pitch processors. Standard output
        // is required for those effects; source files and quality labels stay intact.
        val useFloat=requested&&!floatFailed&&speed==1f&&pitch==1f
        if(allowRebuild&&useFloat!=floatOutput){rebuild(useFloat);return}
        val params=PlaybackParameters(speed,pitch)
        if(player.playbackParameters!=params)player.playbackParameters=params
        val gain=if(bypass)1.0 else 10.0.pow(value(data,"preamp",-3.0).coerceIn(-60.0,24.0)/20)
        player.volume=(value(data,"volume",.7)*gain).toFloat().coerceIn(0f,1f)
        applyEffects()
        player.shuffleModeEnabled=data.optBoolean("shuffle")
        player.repeatMode=if(data.optBoolean("repeat"))Player.REPEAT_MODE_ALL else Player.REPEAT_MODE_OFF
    }
    private fun applyEffects() {
        if(effectSession<=0)return
        val bypass=fx.optBoolean("bypass");val values=fx.optJSONArray("eq")
        val equalizing=!bypass&&(0 until (values?.length() ?: 0)).any {kotlin.math.abs(values!!.optDouble(it,0.0))>.001}
        if(equalizing&&eq==null)eq=runCatching {Equalizer(0,effectSession)}.getOrNull()
        eq?.let {effect->runCatching {
            effect.enabled=equalizing
            if(equalizing){val freqs=doubleArrayOf(31.0,62.0,125.0,250.0,500.0,1000.0,2000.0,4000.0,8000.0,16000.0)
                for(b in 0 until effect.numberOfBands){val center=effect.getCenterFreq(b.toShort())/1000.0;val idx=freqs.indices.minByOrNull {kotlin.math.abs(kotlin.math.ln(freqs[it]/center))} ?: 0
                    val level=((values?.optDouble(idx,0.0) ?: 0.0)*100).toInt().coerceIn(effect.bandLevelRange[0].toInt(),effect.bandLevelRange[1].toInt());effect.setBandLevel(b.toShort(),level.toShort())}
            }
        }}
        val wet=if(bypass)0f else value(fx,"reverb",0.0).toFloat().coerceIn(0f,1f)
        if(wet>0&&reverb==null)reverb=runCatching {PresetReverb(0,0).apply {preset=PresetReverb.PRESET_LARGEROOM}}.getOrNull()
        reverb?.let {effect->runCatching {effect.enabled=wet>0;player.setAuxEffectInfo(AuxEffectInfo(effect.id,wet))}}
        val width=if(bypass)1.0 else value(fx,"width",1.0)
        if(width>1&&spatial==null)spatial=runCatching {Virtualizer(0,effectSession)}.getOrNull()
        spatial?.let {effect->runCatching {effect.enabled=width>1;if(effect.strengthSupported)effect.setStrength(((width-1)*1000).toInt().coerceIn(0,1000).toShort())}}
    }
    fun state(): JSONObject {
        val known=durations[player.currentMediaItem?.mediaId] ?: 0.0
        val native=if(player.duration>0)player.duration/1000.0 else 0.0
        val canSeek=player.isCurrentMediaItemSeekable&&player.currentMediaItem?.mediaId !in unseekable
        // A piped WAV has an indefinite header, not a multi-hour song length.
        val duration=if(player.currentMediaItem?.mediaId in unseekable)known else if(!canSeek&&known>0)known else native
        val audio=getSystemService(AUDIO_SERVICE) as AudioManager
        return JSONObject().put("position",player.currentPosition.coerceAtLeast(0)/1000.0).put("duration",duration)
            .put("paused",!player.playWhenReady||player.playbackState==Player.STATE_ENDED).put("playing",player.isPlaying).put("speed",player.playbackParameters.speed)
            .put("id",player.currentMediaItem?.mediaId ?: "").put("url",player.currentMediaItem?.localConfiguration?.uri?.toString() ?: "").put("loadToken",loadToken).put("seekToken",seekToken).put("timelineRevision",timelineRevision).put("sampledAt",SystemClock.elapsedRealtime())
            .put("buffering",player.playbackState==Player.STATE_BUFFERING).put("ended",player.playbackState==Player.STATE_ENDED)
            .put("seekable",canSeek).put("buffered",player.bufferedPosition/1000.0)
            .put("error",player.playerError?.errorCodeName ?: "").put("eqBands",eq?.numberOfBands?.toInt() ?: 0)
            .put("volume",player.volume).put("systemMuted",audio.getStreamVolume(AudioManager.STREAM_MUSIC)==0)
            .put("audioOutput",getSharedPreferences("nawa-mobile",MODE_PRIVATE).getString("audioOutput","compatible"))
            .put("effectiveOutput",if(floatOutput)"float" else "compatible").put("floatFallback",floatFailed)
    }
}

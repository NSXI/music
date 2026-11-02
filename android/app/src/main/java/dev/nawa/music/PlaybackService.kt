package dev.nawa.music

import android.app.PendingIntent
import android.content.Intent
import android.media.audiofx.Equalizer
import android.media.audiofx.PresetReverb
import android.media.audiofx.Virtualizer
import androidx.media3.common.AudioAttributes
import androidx.media3.common.AuxEffectInfo
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.MediaMetadata
import androidx.media3.common.PlaybackParameters
import androidx.media3.common.Player
import androidx.media3.datasource.DefaultDataSource
import androidx.media3.datasource.DefaultHttpDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.DefaultRenderersFactory
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.session.MediaSession
import androidx.media3.session.MediaSessionService
import org.json.JSONArray
import org.json.JSONObject
import kotlin.math.pow

@androidx.annotation.OptIn(androidx.media3.common.util.UnstableApi::class)
class PlaybackService : MediaSessionService() {
    lateinit var player: ExoPlayer
    private lateinit var session: MediaSession
    private var eq: Equalizer?=null
    private var reverb: PresetReverb?=null
    private var spatial: Virtualizer?=null
    private var fx=JSONObject()
    private var recovering=false
    private val decoded=mutableSetOf<String>()
    companion object { var instance: PlaybackService?=null }
    override fun onCreate() {
        super.onCreate()
        val http=DefaultHttpDataSource.Factory().setDefaultRequestProperties(mapOf("X-Nawa-Token" to CoreBridge.token)).setAllowCrossProtocolRedirects(false)
        player=ExoPlayer.Builder(this,DefaultRenderersFactory(this).setEnableAudioFloatOutput(true).setEnableAudioTrackPlaybackParams(true))
            .setMediaSourceFactory(DefaultMediaSourceFactory(DefaultDataSource.Factory(this,http))).build()
        player.setAudioAttributes(AudioAttributes.Builder().setUsage(C.USAGE_MEDIA).setContentType(C.AUDIO_CONTENT_TYPE_MUSIC).build(),true)
        player.setHandleAudioBecomingNoisy(true)
        player.setWakeMode(C.WAKE_MODE_LOCAL)
        player.addListener(object: Player.Listener {
            override fun onAudioSessionIdChanged(audioSessionId: Int) { createEffects(audioSessionId) }
            override fun onPlayerError(error: androidx.media3.common.PlaybackException) {
                val item=player.currentMediaItem ?: return
                if(item.localConfiguration?.uri?.path?.startsWith("/media/")!=true||!decoded.add(item.mediaId))return
                recovering=true
                val pos=player.currentPosition;val playing=player.playWhenReady
                (application as NawaApp).io.execute {
                    val playback=runCatching { CoreBridge.call("DecodePlayback",JSONArray().put(item.mediaId)) as JSONObject }
                    android.os.Handler(mainLooper).post {
                        if(player.currentMediaItem?.mediaId==item.mediaId&&playback.isSuccess) {
                            val replacement=item.buildUpon().setUri(CoreBridge.mediaUrl(playback.getOrThrow().getString("url"))).build()
                            player.replaceMediaItem(player.currentMediaItemIndex,replacement);player.prepare();player.seekTo(pos);player.playWhenReady=playing
                        }
                        recovering=false
                    }
                }
            }
        })
        session=MediaSession.Builder(this,player).setSessionActivity(PendingIntent.getActivity(this,0,Intent(this,MainActivity::class.java),PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)).build()
        instance=this
    }
    override fun onGetSession(controllerInfo: MediaSession.ControllerInfo): MediaSession = session
    override fun onDestroy() { instance=null;eq?.release();reverb?.release();spatial?.release();session.release();player.release();super.onDestroy() }
    private fun createEffects(id: Int) {
        eq?.release();reverb?.release();spatial?.release()
        eq=runCatching { Equalizer(0,id) }.getOrNull()
        reverb=runCatching { PresetReverb(0,0).apply { preset=PresetReverb.PRESET_LARGEROOM } }.getOrNull()
        spatial=runCatching { Virtualizer(0,id) }.getOrNull()
        applyFX(fx)
    }
    private fun media(data: JSONObject): MediaItem {
        val art=data.optString("artwork")
        return MediaItem.Builder().setMediaId(data.optString("id")).setUri(CoreBridge.mediaUrl(data.getString("url")))
            .setMediaMetadata(MediaMetadata.Builder().setTitle(data.optString("title")).setArtist(data.optString("artist")).setAlbumTitle(data.optString("album"))
                .apply { if(art.isNotBlank()) setArtworkUri(android.net.Uri.parse(CoreBridge.mediaUrl(art))) }.build()).build()
    }
    fun command(name: String, data: JSONObject) {
        when(name) {
            "load" -> { player.setMediaItem(media(data));player.prepare() }
            "queue" -> {
                val list=data.getJSONArray("items");val items=(0 until list.length()).map { media(list.getJSONObject(it)) }
                val id=player.currentMediaItem?.mediaId
                val index=items.indexOfFirst { it.mediaId==id }
                if(index>=0) { val pos=player.currentPosition;val playing=player.playWhenReady;player.setMediaItems(items,index,pos);player.prepare();player.playWhenReady=playing }
            }
            "play" -> player.play()
            "pause" -> player.pause()
            "seek" -> player.seekTo((data.optDouble("position")*1000).toLong().coerceAtLeast(0))
            "fx" -> applyFX(data)
        }
    }
    private fun applyFX(data: JSONObject) {
        fx=data
        val bypass=data.optBoolean("bypass")
        val speed=if(bypass)1f else data.optDouble("speed",1.0).toFloat().coerceIn(.25f,3f)
        val pitch=if(data.optBoolean("preservePitch")||bypass)1f else speed
        player.playbackParameters=PlaybackParameters(speed,pitch)
        val gain=if(bypass)1.0 else 10.0.pow(data.optDouble("preamp",-3.0)/20)
        player.volume=(data.optDouble("volume",.7)*gain).toFloat().coerceIn(0f,1f)
        eq?.let { effect -> runCatching {
            effect.enabled=!bypass
            val values=data.optJSONArray("eq") ?: JSONArray();val freqs=doubleArrayOf(31.0,62.0,125.0,250.0,500.0,1000.0,2000.0,4000.0,8000.0,16000.0)
            for(b in 0 until effect.numberOfBands) {
                val center=effect.getCenterFreq(b.toShort())/1000.0
                val idx=freqs.indices.minByOrNull { kotlin.math.abs(kotlin.math.ln(freqs[it]/center)) } ?: 0
                val level=(values.optDouble(idx,0.0)*100).toInt().coerceIn(effect.bandLevelRange[0].toInt(),effect.bandLevelRange[1].toInt())
                effect.setBandLevel(b.toShort(),level.toShort())
            }
        } }
        reverb?.let { effect -> runCatching { val wet=if(bypass)0f else data.optDouble("reverb",0.0).toFloat().coerceIn(0f,1f);effect.enabled=wet>0;player.setAuxEffectInfo(AuxEffectInfo(effect.id,wet)) } }
        spatial?.let { effect -> runCatching { val width=if(bypass)1.0 else data.optDouble("width",1.0);effect.enabled=width>1; if(effect.strengthSupported) effect.setStrength(((width-1)*1000).toInt().coerceIn(0,1000).toShort()) } }
        player.shuffleModeEnabled=data.optBoolean("shuffle")
        player.repeatMode=if(data.optBoolean("repeat"))Player.REPEAT_MODE_ALL else Player.REPEAT_MODE_OFF
    }
    fun state(): JSONObject = JSONObject().put("position",player.currentPosition/1000.0)
        .put("duration",if(player.duration>0)player.duration/1000.0 else 0.0).put("paused",!player.isPlaying)
        .put("speed",player.playbackParameters.speed).put("id",player.currentMediaItem?.mediaId ?: "")
        .put("buffering",player.playbackState==Player.STATE_BUFFERING).put("ended",player.playbackState==Player.STATE_ENDED)
        .put("error",if(recovering)"" else player.playerError?.errorCodeName ?: "").put("eqBands",eq?.numberOfBands?.toInt() ?: 0)
}

package dev.nawa.music

import android.media.MediaExtractor
import android.media.MediaFormat
import android.media.MediaMetadataRetriever
import com.google.android.gms.tasks.Tasks
import com.google.mlkit.common.model.DownloadConditions
import com.google.mlkit.nl.languageid.LanguageIdentification
import com.google.mlkit.nl.translate.TranslateLanguage
import com.google.mlkit.nl.translate.Translation
import com.google.mlkit.nl.translate.TranslatorOptions
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.io.RandomAccessFile
import java.util.concurrent.TimeUnit
import android.net.Uri
import androidx.documentfile.provider.DocumentFile

class NativeWorkers(private val app: NawaApp) {
    fun run(kind: String, input: JSONObject): Any = when(kind) {
        "probe" -> probe(input.getString("path"))
        "translation" -> translate(input)
        "alignment" -> TimingEngine(app).align(input)
        "export" -> export(input)
        "openurl" -> { app.openUrl(input.getString("url")); JSONObject() }
        else -> error("Unknown native task: $kind")
    }
    private fun export(input: JSONObject): JSONObject {
        val uri=app.getSharedPreferences("nawa-mobile",android.content.Context.MODE_PRIVATE).getString("exportTree",null) ?: return JSONObject()
        val target=DocumentFile.fromTreeUri(app,Uri.parse(uri)) ?: error("Export folder unavailable")
        val paths=input.getJSONArray("paths");val id=input.getString("id")
        for(i in 0 until paths.length()) {
            val file=File(paths.getString(i));if(!file.isFile)continue
            val name="${id.take(8)}-${file.name}"
            if(target.findFile(name)!=null)continue
            val mime=when(file.extension.lowercase()){ "jpg","jpeg"->"image/jpeg";"png"->"image/png";"lrc","txt"->"text/plain";else->"audio/${file.extension}" }
            val doc=target.createFile(mime,name) ?: error("Cannot create exported file")
            try { app.contentResolver.openOutputStream(doc.uri)!!.use { out -> file.inputStream().use { it.copyTo(out) } } }
            catch(e: Exception){doc.delete();throw e}
        }
        return JSONObject()
    }
    private fun progress(kind: String, value: Double, phase: String) { CoreBridge.call("MobileProgress",JSONArray().put(kind).put(value).put(phase)) }
    private fun translate(input: JSONObject): JSONObject {
        val lines=input.getJSONArray("lines");val texts=(0 until lines.length()).map { lines.getString(it) }
        var source=input.optString("source","auto")
        val target=input.getString("target")
        if(source=="auto") {
            val identifier=LanguageIdentification.getClient()
            try { source=Tasks.await(identifier.identifyLanguage(texts.joinToString("\n").take(10000)),45,TimeUnit.SECONDS) } finally { identifier.close() }
        }
        check(TranslateLanguage.fromLanguageTag(source)!=null) { "تعذر تحديد لغة الكلمات؛ اختر لغة الأصل يدويًا" }
        if(source==target) return JSONObject().put("source",source).put("target",target).put("sameLanguage",true).put("lines",JSONArray(texts)).put("engine","ML Kit on-device")
        val translator=Translation.getClient(TranslatorOptions.Builder().setSourceLanguage(source).setTargetLanguage(target).build())
        try {
            progress("translation",.05,"تنزيل نموذج اللغة عند الحاجة / Preparing language model")
            Tasks.await(translator.downloadModelIfNeeded(DownloadConditions.Builder().build()),15,TimeUnit.MINUTES)
            val output=JSONArray()
            texts.forEachIndexed { i,text ->
                check(CoreBridge.call("MobileWorkActive",JSONArray().put(input.optString("_workID")))==true){"Translation cancelled"}
                val translated=if(text.isBlank()) text else Tasks.await(translator.translate(text),90,TimeUnit.SECONDS).replace('\n',' ').trim()
                output.put(translated);progress("translation",.1+.85*(i+1)/texts.size,"${i+1} / ${texts.size}")
            }
            return JSONObject().put("source",source).put("target",target).put("sameLanguage",false).put("lines",output).put("engine","ML Kit on-device")
        } finally { translator.close() }
    }
    fun probe(path: String): JSONObject {
        val retriever=MediaMetadataRetriever();val streams=JSONArray();val tags=JSONObject();var dur=0.0;var bitrate=0L
        try {
            if(path.startsWith("http")) retriever.setDataSource(path,emptyMap()) else retriever.setDataSource(path)
            fun tag(key: Int,name: String) { retriever.extractMetadata(key)?.let { tags.put(name,it) } }
            tag(MediaMetadataRetriever.METADATA_KEY_TITLE,"title");tag(MediaMetadataRetriever.METADATA_KEY_ARTIST,"artist");tag(MediaMetadataRetriever.METADATA_KEY_ALBUM,"album");tag(MediaMetadataRetriever.METADATA_KEY_GENRE,"genre");tag(MediaMetadataRetriever.METADATA_KEY_YEAR,"date")
            dur=(retriever.extractMetadata(MediaMetadataRetriever.METADATA_KEY_DURATION)?.toDoubleOrNull() ?: 0.0)/1000
            bitrate=retriever.extractMetadata(MediaMetadataRetriever.METADATA_KEY_BITRATE)?.toLongOrNull() ?: 0
        } catch(_: Exception) {} finally { retriever.release() }
        val extractor=MediaExtractor()
        try {
            extractor.setDataSource(path)
            for(i in 0 until extractor.trackCount) {
                val f=extractor.getTrackFormat(i);val mime=f.getString(MediaFormat.KEY_MIME) ?: continue
                if(!mime.startsWith("audio/")) continue
                fun int(key: String)=if(f.containsKey(key)) f.getInteger(key) else 0
                if(dur<=0 && f.containsKey(MediaFormat.KEY_DURATION)) dur=f.getLong(MediaFormat.KEY_DURATION)/1000000.0
                val flacHeader=runCatching { java.io.FileInputStream(path).use { val b=ByteArray(4);it.read(b);String(b,Charsets.US_ASCII)=="fLaC" } }.getOrDefault(false)
                val codec=if(flacHeader)"flac" else mapOf("audio/flac" to "flac","audio/mpeg" to "mp3","audio/mp4a-latm" to "aac","audio/opus" to "opus","audio/vorbis" to "vorbis","audio/raw" to "pcm_s16le","audio/alac" to "alac")[mime] ?: mime.substringAfter('/')
                val bits=sourceBits(path).takeIf { it>0 } ?: when(int("pcm-encoding")) { 2->16;3->8;4->32;21->24;22->32;else->0 }
                val streamBitrate=if(codec.startsWith("pcm_")&&bits>0)int(MediaFormat.KEY_SAMPLE_RATE)*int(MediaFormat.KEY_CHANNEL_COUNT)*bits else int(MediaFormat.KEY_BIT_RATE)
                streams.put(JSONObject().put("codec_type","audio").put("codec_name",if(codec.startsWith("pcm_")&&bits>0) "pcm_s${bits}le" else codec).put("bits_per_sample",bits).put("bits_per_raw_sample",bits.toString()).put("sample_rate",int(MediaFormat.KEY_SAMPLE_RATE).toString()).put("channels",int(MediaFormat.KEY_CHANNEL_COUNT)).put("bit_rate",streamBitrate.toString()).put("duration",dur.toString()).put("tags",tags))
            }
        } catch(_: Exception) {} finally { extractor.release() }
        // FFmpeg verifies/decode-probes containers unavailable to Android's extractor.
        if(streams.length()==0) {
            val log=app.ffmpeg("-hide_banner","-i",path,"-t","0","-f","null","-")
            val m=Regex("Audio: ([^ ,]+).*?(\\d+) Hz, ([^,]+), ([^,\\n]+)").find(log) ?: error("Unsupported or damaged audio")
            val codec=m.groupValues[1];val bits=Regex("[sf](\\d+)").find(m.groupValues[4])?.groupValues?.get(1)?.toIntOrNull() ?: 0
            val duration=Regex("Duration: (\\d+):(\\d+):([0-9.]+)").find(log)
            if(dur<=0&&duration!=null) dur=duration.groupValues[1].toDouble()*3600+duration.groupValues[2].toDouble()*60+duration.groupValues[3].toDouble()
            // Sample format may be decoder precision; only exact PCM/FLAC headers establish source bit depth.
            val verifiedBits=sourceBits(path).takeIf { it>0 } ?: if(codec.startsWith("pcm_")) bits else 0
            streams.put(JSONObject().put("codec_type","audio").put("codec_name",codec).put("bits_per_sample",verifiedBits).put("sample_rate",m.groupValues[2]).put("channels",if(m.groupValues[3].contains("mono"))1 else if(m.groupValues[3].contains("stereo"))2 else 0).put("duration",dur.toString()).put("tags",tags))
        }
        return JSONObject().put("streams",streams).put("format",JSONObject().put("duration",dur.toString()).put("bit_rate",bitrate.toString()).put("tags",tags))
    }
    private fun sourceBits(path: String): Int = try {
        RandomAccessFile(File(path),"r").use { f ->
            val sig=ByteArray(4);f.readFully(sig)
            when(String(sig,Charsets.US_ASCII)) {
                "fLaC" -> { val h=ByteArray(4);f.readFully(h);if(h[0].toInt() and 127 != 0) 0 else { val b=ByteArray(18);f.readFully(b);((b[12].toInt() and 1) shl 4 or ((b[13].toInt() and 240) shr 4))+1 } }
                "RIFF" -> { f.seek(12);var answer=0;while(f.filePointer+8<f.length() && f.filePointer<1048576){f.readFully(sig);val length=Integer.reverseBytes(f.readInt()).toLong() and 0xffffffffL;if(String(sig,Charsets.US_ASCII)=="fmt "){f.skipBytes(14);answer=java.lang.Short.reverseBytes(f.readShort()).toInt();break};f.seek(f.filePointer+length+(length and 1))};answer }
                else -> 0
            }
        }
    } catch(_: Exception) { 0 }
}

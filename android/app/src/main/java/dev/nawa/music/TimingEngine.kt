package dev.nawa.music

import org.json.JSONArray
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.io.File
import java.text.Normalizer
import java.text.BreakIterator
import java.util.Locale

class TimingEngine(private val app: NawaApp) {
    companion object { init { System.loadLibrary("nawatiming") } }
    private external fun nativeTokens(model: ByteArray, pcm: ByteArray, language: ByteArray, prompt: ByteArray, threads: Int): ByteArray
    private external fun nativeCancel()
    fun cancel()=nativeCancel()
    fun onProgress(value: Int) { progress(.12+.7*value.coerceIn(0,100)/100,"مطابقة الكلام / Recognizing speech · $value%") }
    private data class Point(val start: Double,val end: Double,val probability: Double,val center: Double)
    fun align(input: JSONObject): JSONObject {
        val model=File(app.root,"models/ggml-base.bin")
        check(model.isFile) { "نزّل نموذج التوقيت من إعدادات أندرويد أولًا" }
        val pcm=File.createTempFile("alignment-",".f32",app.cacheDir)
        try {
            progress(.02,"فك الصوت / Decoding audio")
            app.ffmpeg("-v","error","-nostdin","-y","-i",input.getString("audio"),"-vn","-ac","1","-ar","16000","-c:a","pcm_f32le","-f","f32le",pcm.absolutePath)
            progress(.12,"مطابقة الكلام محليًا / Recognizing speech")
            check(CoreBridge.call("MobileWorkActive",JSONArray().put(input.optString("_workID")))==true){"Alignment cancelled"}
            val state=CoreBridge.call("GetState") as JSONObject
            val cores=when(state.optJSONObject("preferences")?.optString("resourceMode")) { "economy"->2;"smooth"->6;else->4 }
            val text=input.getString("text")
            val raw=JSONObject(String(nativeTokens(model.absolutePath.toByteArray(),pcm.absolutePath.toByteArray(),input.optString("language","auto").toByteArray(),text.take(3000).toByteArray(),cores.coerceAtMost(Runtime.getRuntime().availableProcessors()).coerceAtLeast(1)),Charsets.UTF_8))
            check(!raw.has("error")) { raw.optString("error") }
            progress(.88,"التحقق من النص والتوقيت / Verifying alignment")
            return match(text,raw)
        } finally { pcm.delete() }
    }
    private fun progress(value: Double,phase: String) { CoreBridge.call("MobileProgress",JSONArray().put("alignment").put(value).put(phase)) }
    private fun identity(s: String): String = Normalizer.normalize(s,Normalizer.Form.NFKC).lowercase(Locale.ROOT).filter { it.isLetterOrDigit() }
    fun match(text: String, raw: JSONObject): JSONObject {
        val tokens=raw.getJSONArray("tokens");val speech=StringBuilder();val points=mutableListOf<Point>();val bytes=ByteArrayOutputStream()
        var groupStart=0.0;var groupEnd=0.0;var groupProbability=1.0;var groupCenter=-1.0
        for(i in 0 until tokens.length()) {
            val t=tokens.getJSONObject(i);val hex=t.getString("hex")
            if(bytes.size()==0) { groupStart=t.getDouble("start");groupProbability=t.optDouble("probability",0.0);groupCenter=t.optDouble("center",-1.0) }
            groupEnd=t.getDouble("end");groupProbability=minOf(groupProbability,t.optDouble("probability",0.0))
            bytes.write(hex.chunked(2).map { it.toInt(16).toByte() }.toByteArray())
            val decoded=bytes.toByteArray().toString(Charsets.UTF_8)
            if(decoded.contains('\uFFFD')) continue
            val key=identity(decoded)
            if(key.isNotEmpty()) { speech.append(key);repeat(key.length){ points.add(Point(groupStart,groupEnd,groupProbability,groupCenter)) } }
            bytes.reset()
        }
        val original=identity(text)
        check(original.isNotBlank()&&original==speech.toString()) { "لم يطابق التعرف كل الكلمات بدقة. بقي التوقيت الأصلي محفوظًا؛ جرّب لغة الأصل أو ملف كلمات أوضح. / Speech did not match all lyrics; original timing preserved." }
        val output=JSONArray();val lrc=StringBuilder();var cursor=0;var total=0;var confidence=0.0;var previous=-1.0
        val rows=text.lines().map { it.trim() }.filter { it.isNotEmpty() }
        for(row in rows) {
            val wordList=mutableListOf<String>();val iterator=BreakIterator.getWordInstance(Locale.ROOT);iterator.setText(row)
            var from=iterator.first();var to=iterator.next()
            while(to!=BreakIterator.DONE) { val part=row.substring(from,to);if(part.any { it.isLetterOrDigit() })wordList.add(part) else if(wordList.isNotEmpty())wordList[wordList.lastIndex]+=part else if(part.isNotBlank())wordList.add(part);from=to;to=iterator.next() }
            // Attach punctuation-only prefixes rather than assign them independent timestamps.
            if(wordList.size>1&&identity(wordList[0]).isEmpty()){wordList[1]=wordList[0]+wordList[1];wordList.removeAt(0)}
            val words=JSONArray();var start=Double.NaN;var end=0.0
            for(word in wordList) {
                val n=identity(word).length;if(n==0)continue
                val p=points.subList(cursor,cursor+n);cursor+=n
                val dtw=p.all { it.center>=0&&it.center.isFinite() }
                val a=if(dtw)p.minOf { it.center } else p.minOf { it.start }
                var b=p.maxOf { it.end }
                if(dtw) {
                    val next=points.getOrNull(cursor)?.center?.takeIf { it>=0 } ?: raw.optDouble("duration",Double.NaN)
                    check(next.isFinite()&&next>a){"حدود DTW متداخلة؛ يلزم توقيت مصدر أو مراجعة / Ambiguous DTW word boundary"}
                    b=if(b>a)minOf(b,next) else next
                }
                val probability=p.map { it.probability }.average()
                check(a.isFinite()&&b.isFinite()&&a>=0&&b>a&&a>=previous) { "توقيت غير موثوق؛ لم يُطبّق على الكلمات / Invalid token timing" }
                previous=a;if(start.isNaN())start=a;end=maxOf(end,b);confidence+=probability;total++
                words.put(JSONObject().put("text",word).put("start",a).put("end",b).put("probability",probability.coerceIn(0.0,1.0)))
            }
            check(words.length()>0) { "Empty recognized line" }
            output.put(JSONObject().put("text",row).put("start",start).put("end",end).put("words",words))
            val centis=(start*100).toLong();lrc.append("[%02d:%02d.%02d]%s\n".format(Locale.ROOT,centis/6000,centis/100%60,centis%100,row))
        }
        check(cursor==points.size) { "Incomplete timing coverage" }
        return JSONObject().put("lrc",lrc.toString()).put("lines",output).put("report",JSONObject()
            .put("coverage",1.0).put("meanProbability",confidence/total).put("words",total).put("validWords",total)
            .put("needsReview",true).put("language",raw.optString("language")).put("model","Android Whisper")
            .put("refined",false).put("resolutionSeconds",.02).put("textMatched",true).put("characterTiming","visual interpolation")
            .put("note","بدايات الكلمات مشتقة من انتباه Whisper DTW، والنهايات مقيدة بالكلمة التالية ومدة الصوت. تحتاج مراجعة؛ تحريك الحروف بصري وليس قياسًا صوتيًا مستقلًا."))
    }
}

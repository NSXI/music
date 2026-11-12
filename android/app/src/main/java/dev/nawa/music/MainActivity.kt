package dev.nawa.music

import android.Manifest
import android.annotation.SuppressLint
import android.content.Intent
import android.content.res.Configuration
import android.graphics.Color
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.provider.Settings
import android.webkit.JavascriptInterface
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.TextView
import androidx.activity.ComponentActivity
import androidx.activity.OnBackPressedCallback
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.view.ViewCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.documentfile.provider.DocumentFile
import androidx.webkit.WebViewAssetLoader
import org.json.JSONArray
import org.json.JSONObject
import java.io.ByteArrayInputStream
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import java.security.MessageDigest
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicBoolean

@SuppressLint("SetJavaScriptEnabled")
class MainActivity : ComponentActivity() {
    lateinit var web: WebView
    private val app get()=application as NawaApp
    private val main=Handler(Looper.getMainLooper())
    private val responses=ConcurrentHashMap<String,ByteArray>()
    private var selection: ((List<Uri>)->Unit)?=null
    private var fullscreen=false
    private var resumed=false
    private var stateInFlight=false
    private var updateSignature=""
    private val cancelled=AtomicBoolean(false)
    private val pickFiles=registerForActivityResult(ActivityResultContracts.OpenMultipleDocuments()) { uris -> selection?.invoke(uris);selection=null }
    private val pickTree=registerForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri -> selection?.invoke(listOfNotNull(uri));selection=null }
    private val notifications=registerForActivityResult(ActivityResultContracts.RequestPermission()) {}
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        WindowCompat.setDecorFitsSystemWindows(window,false)
        setContentView(TextView(this).apply { text="نَوى · Nawa\nPreparing your music space…";textSize=22f;setTextColor(Color.WHITE);setPadding(32,100,32,32);setBackgroundColor(Color.rgb(8,11,18)) })
        app.io.execute {
            app.ready.await()
            runOnUiThread {
                if(app.startupError!=null) { setContentView(TextView(this).apply { text="Nawa could not start:\n${app.startupError}";setTextColor(Color.WHITE);setPadding(32,100,32,32) });return@runOnUiThread }
                createWeb()
                startService(Intent(this,PlaybackService::class.java))
                if(Build.VERSION.SDK_INT>=33) notifications.launch(Manifest.permission.POST_NOTIFICATIONS)
            }
        }
        onBackPressedDispatcher.addCallback(this,object: OnBackPressedCallback(true) {
            override fun handleOnBackPressed() {
                if(!::web.isInitialized) { moveTaskToBack(true);return }
                web.evaluateJavascript("window.NawaMobile?.back() || false") { value -> if(value!="true")moveTaskToBack(true) }
            }
        })
    }
    private fun createWeb() {
        web=WebView(this);web.setBackgroundColor(Color.TRANSPARENT)
        web.settings.apply {
            javaScriptEnabled=true;domStorageEnabled=true;allowFileAccess=false;allowContentAccess=false
            javaScriptCanOpenWindowsAutomatically=false;setSupportMultipleWindows(false);mediaPlaybackRequiresUserGesture=true
            mixedContentMode=android.webkit.WebSettings.MIXED_CONTENT_NEVER_ALLOW
        }
        if(applicationInfo.flags and android.content.pm.ApplicationInfo.FLAG_DEBUGGABLE != 0) WebView.setWebContentsDebuggingEnabled(true)
        web.addJavascriptInterface(Bridge(),"NawaAndroid")
        val loader=WebViewAssetLoader.Builder().addPathHandler("/",WebViewAssetLoader.AssetsPathHandler(this)).build()
        web.webViewClient=object: WebViewClient() {
            override fun shouldOverrideUrlLoading(view: WebView,request: WebResourceRequest): Boolean {
                if(request.url.scheme=="https"&&request.url.host=="appassets.androidplatform.net")return false
                if(request.isForMainFrame&&request.url.scheme in listOf("http","https"))runCatching { app.openUrl(request.url.toString()) }
                return true
            }
            override fun shouldInterceptRequest(view: WebView,request: WebResourceRequest): WebResourceResponse? {
                val uri=request.url
                if(uri.host!="appassets.androidplatform.net"||uri.scheme!="https")return null
                val path=uri.path ?: "/"
                if(path.startsWith("/native-response/")) {
                    val bytes=responses.remove(path.substringAfterLast('/')) ?: "{\"ok\":false,\"error\":\"Expired response\"}".toByteArray()
                    return WebResourceResponse("application/json","UTF-8",ByteArrayInputStream(bytes))
                }
                if(path.startsWith("/fonts/")) {
                    val name=path.substringAfterLast('/');if(!Regex("[a-f0-9]{32}\\.(ttf|otf)").matches(name))return blocked()
                    val f=File(app.root,"fonts/$name");return if(f.isFile)WebResourceResponse("font/ttf",null,f.inputStream()) else blocked()
                }
                if(path.startsWith("/art/")||path.startsWith("/media/")||path.startsWith("/artwork/")||path.startsWith("/decoded/")) {
                    return try {
                        val conn=URL(CoreBridge.address+path+(uri.encodedQuery?.let { "?$it" } ?: "")).openConnection() as HttpURLConnection
                        conn.setRequestProperty("X-Nawa-Token",CoreBridge.token);request.requestHeaders["Range"]?.let { conn.setRequestProperty("Range",it) }
                        conn.connectTimeout=20000;conn.readTimeout=60000
                        val code=conn.responseCode;val headers=conn.headerFields.filterKeys { it!=null }.mapValues { it.value.joinToString(",") }
                        WebResourceResponse(conn.contentType?.substringBefore(';') ?: "application/octet-stream",null,code,conn.responseMessage ?: "OK",headers,object: java.io.FilterInputStream(if(code<400)conn.inputStream else conn.errorStream ?: ByteArrayInputStream(byteArrayOf())) { override fun close(){super.close();conn.disconnect()} })
                    } catch(_: Exception) { blocked() }
                }
                return loader.shouldInterceptRequest(if(path=="/")Uri.parse("https://appassets.androidplatform.net/index.html") else uri)
            }
            override fun onPageFinished(view: WebView,url: String) { if(url.startsWith("https://appassets.androidplatform.net/")) web.evaluateJavascript("window.NawaMobile?.resume()",null) }
        }
        val container=android.widget.FrameLayout(this)
        container.addView(web,android.widget.FrameLayout.LayoutParams(-1,-1))
        container.setBackgroundColor(Color.rgb(8,11,18))
        setContentView(container)
        ViewCompat.setOnApplyWindowInsetsListener(container) { view,insets ->
            val safe=insets.getInsets(WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.displayCutout() or WindowInsetsCompat.Type.ime())
            view.setPadding(safe.left,safe.top,safe.right,safe.bottom);insets
        }
        ViewCompat.requestApplyInsets(container)
        web.loadUrl("https://appassets.androidplatform.net/index.html")
        main.post(tick)
    }
    private fun blocked()=WebResourceResponse("text/plain","UTF-8",403,"Forbidden",emptyMap(),ByteArrayInputStream(byteArrayOf()))
    private val tick=object: Runnable {
        override fun run() {
            if(isDestroyed)return
            if(resumed&&::web.isInitialized&&!stateInFlight) PlaybackService.instance?.let { service ->
                stateInFlight=true
                web.evaluateJavascript("window.NawaMobile?.state(${service.state()})") { stateInFlight=false }
            }
            if(resumed&&::web.isInitialized){val update=app.updater.snapshot().toString();if(update!=updateSignature){updateSignature=update;web.evaluateJavascript("window.nawaUpdates?.androidState($update)",null)}}
            main.postDelayed(this,if(resumed&&PlaybackService.instance?.player?.isPlaying==true)100 else 500)
        }
    }
    override fun onResume() { super.onResume();resumed=true;if(::web.isInitialized){web.onResume();web.evaluateJavascript("window.NawaMobile?.resume()",null)};app.updater.onResume(this);DownloadScheduler.updateClock(app,true) }
    override fun onPause() { resumed=false;if(::web.isInitialized){web.evaluateJavascript("window.NawaMobile?.suspend()",null);web.onPause()};super.onPause() }
    override fun onConfigurationChanged(newConfig: Configuration) { super.onConfigurationChanged(newConfig);if(::web.isInitialized)ViewCompat.requestApplyInsets(web) }
    override fun onDestroy() { main.removeCallbacks(tick);if(::web.isInitialized){web.removeJavascriptInterface("NawaAndroid");web.destroy()};super.onDestroy() }
    private fun reply(id: String,value: Any?=null,error: String?=null,raw: ByteArray?=null) {
        if(isDestroyed)return
        responses[id]=raw ?: (if(error==null)JSONObject().put("ok",true).put("result",value ?: JSONObject.NULL) else JSONObject().put("ok",false).put("error",error)).toString().toByteArray()
        main.post { if(!isDestroyed)web.evaluateJavascript("window.NawaMobile?.reply(${JSONObject.quote(id)})",null) }
        main.postDelayed({responses.remove(id)},120000)
    }
    private fun launchPicker(id: String,tree: Boolean,mimes: Array<String> = arrayOf("audio/*"),block: (List<Uri>)->Any?) {
        main.post {
            if(selection!=null){reply(id,error="أغلق نافذة اختيار الملفات أولًا");return@post}
            selection={ uris ->
                if(uris.isEmpty())reply(id,null)
                else app.io.execute { try { reply(id,block(uris)) }catch(e:Exception){reply(id,error=e.message)} }
            }
            if(tree)pickTree.launch(null) else pickFiles.launch(mimes)
        }
    }
    inner class Bridge {
        @JavascriptInterface fun dispatch(id: String,method: String,argsJson: String) {
            if(!Regex("[a-zA-Z0-9-]{1,64}").matches(id))return
            val executor=when(method){"Search","PreparePreview","PreparePlayback","DecodePlayback"->app.interactive;"GetState","Enqueue","Control","GetUpdateStatus","GetAppUpdateState","AndroidStatus","CancelSearch","CancelAlignment","CancelTranslation","CancelFolderImport"->app.quick;else->app.io}
            executor.execute {
                try {
                    val args=JSONArray(argsJson)
                    if(method in listOf("Enqueue","StartAlignment","StartRangeAlignment","StartFullLyricReview","StartOfflinePack","FinishLyricClip","StartDevicePull","StartTranslation","CacheLibraryLyrics","ImportFiles","AddLibraryFolder","RescanLibraryFolders","DownloadTimingModel","DownloadAppUpdate","OpenUpdatePage"))main.post { runCatching { androidx.core.content.ContextCompat.startForegroundService(this@MainActivity,WorkService.intent(this@MainActivity)) } }
                    if(method=="CancelAlignment")TimingEngine(app).cancel()
                    when(method) {
                        "ImportFiles" -> launchPicker(id,false,arrayOf("audio/*","application/ogg","application/octet-stream")) { uris ->
                            cancelled.set(false);val paths=uris.map { copyUri(it,File(app.root,"Imported")) };CoreBridge.call("MobileImport",JSONArray().put(JSONArray(paths)))
                        }
                        "AddLibraryFolder" -> launchPicker(id,true) { uris -> importTree(uris.first()) }
                        "RescanLibraryFolders" -> {
                            cancelled.set(false);val mapping=folderMappings();val prefs=(CoreBridge.call("GetState") as JSONObject).getJSONObject("preferences").optJSONArray("libraryFolders") ?: JSONArray()
                            for(i in 0 until prefs.length())mapping.optString(prefs.getString(i)).takeIf { it.isNotBlank() }?.let { copyTree(Uri.parse(it),File(prefs.getString(i))) }
                            reply(id,CoreBridge.call(method,args))
                        }
                        "CancelFolderImport" -> { cancelled.set(true);reply(id,CoreBridge.call(method,args)) }
                        "LoadLyricsFile" -> launchPicker(id,false,arrayOf("text/*","application/ttml+xml","application/xml","application/octet-stream")) { uris ->
                            val text=contentResolver.openInputStream(uris.first())!!.bufferedReader(Charsets.UTF_8).use { reader ->
                                val result=StringBuilder();val buffer=CharArray(4096)
                                while(true){val n=reader.read(buffer);if(n<0)break;check(result.length+n<=2000000){"ملف الكلمات كبير جدًا"};result.append(buffer,0,n)}
                                result.toString()
                            }
                            val extension=DocumentFile.fromSingleUri(this@MainActivity,uris.first())?.name?.substringAfterLast('.',"")?.lowercase() ?: "txt"
                            CoreBridge.call("SaveImportedLyrics",JSONArray().put(args.getString(0)).put(text).put(extension));null
                        }
                        "ExportTextDocument" -> launchPicker(id,true) { uris ->
                            val name=args.getString(0);val text=args.getString(1)
                            check(name.isNotBlank()&&name.length<180&&!name.any {it=='/'||it=='\\'||it==':'}) {"اسم الملف غير صالح"}
                            val bytes=text.toByteArray(Charsets.UTF_8);check(bytes.size<=64*1024*1024) {"الملف أكبر من 64 MB"}
                            val folder=DocumentFile.fromTreeUri(this@MainActivity,uris.first()) ?: error("المجلد غير متاح")
                            val base=name.substringBeforeLast('.',name);val extension=name.substringAfterLast('.',"json")
                            val output=folder.createFile(if(extension=="json")"application/json" else "text/plain","$base-${System.currentTimeMillis()}.$extension") ?: error("تعذر إنشاء الملف")
                            contentResolver.openOutputStream(output.uri,"w")!!.use {it.write(bytes)};output.name
                        }
                        "SaveLyricClip" -> launchPicker(id,true) { uris ->
                            val path=CoreBridge.call("LyricClipPath",JSONArray().put(args.getString(0))) as String
                            val file=File(path).canonicalFile
                            val clipRoot=File(app.root,"data/lyric-clips").canonicalFile
                            check(file.parentFile==clipRoot&&file.isFile) {"الفيديو غير متاح"}
                            val folder=DocumentFile.fromTreeUri(this@MainActivity,uris.first()) ?: error("المجلد غير متاح")
                            val output=folder.createFile("video/mp4","Nawa-lyrics-${System.currentTimeMillis()}.mp4") ?: error("تعذر إنشاء الفيديو")
                            try {
                                contentResolver.openOutputStream(output.uri,"w")!!.use {stream -> file.inputStream().use {it.copyTo(stream)} }
                            } catch(e:Exception) {
                                output.delete()
                                throw e
                            }
                            file.delete()
                            output.name
                        }
                        "ImportTextDocument" -> launchPicker(id,false,arrayOf("application/json","text/*","application/octet-stream")) { uris ->
                            contentResolver.openInputStream(uris.first())!!.bufferedReader(Charsets.UTF_8).use {reader ->
                                val result=StringBuilder();val buffer=CharArray(8192)
                                while(true){val n=reader.read(buffer);if(n<0)break;check(result.length+n<=32*1024*1024) {"الملف كبير جدًا"};result.append(buffer,0,n)}
                                result.toString()
                            }
                        }
                        "GetFonts" -> reply(id,fonts())
                        "ImportFont" -> launchPicker(id,false,arrayOf("font/*","application/octet-stream")) { uris ->
                            val path=copyUri(uris.first(),File(app.root,"font-import"));val original=File(path)
                            val ext=original.extension.lowercase();check(ext in listOf("ttf","otf")){"اختر TTF أو OTF"}
                            android.graphics.Typeface.createFromFile(original)
                            val dest=File(app.root,"fonts/${hash(original.readBytes())}.$ext");dest.parentFile!!.mkdirs();original.copyTo(dest,true);dest.name
                        }
                        "ChooseFolder" -> launchPicker(id,true) { uris ->
                            persist(uris.first());getSharedPreferences("nawa-mobile",MODE_PRIVATE).edit().putString("exportTree",uris.first().toString()).apply();JSONObject().put("folder","Android document folder selected")
                        }
                        "OpenFolder","ExportLibrary" -> { main.post { android.app.AlertDialog.Builder(this@MainActivity).setTitle("نَوى / Nawa").setMessage("الملفات محفوظة داخل التطبيق. اختر مجلدًا لتصدير نسخة من موسيقاك وكلماتها وأغلفتها.\nFiles are stored inside the app. Export a copy to a folder you choose.").setPositiveButton("Export / تصدير") { _,_ -> launchPicker(id,true) { uris -> exportLibrary(uris.first()) } }.setNegativeButton("إلغاء / Cancel") { _,_ -> reply(id,null) }.setOnCancelListener { reply(id,null) }.show() } }
                        "SetLyricsFullscreen" -> { main.post { setFullscreen(args.optBoolean(0));reply(id,null) };CoreBridge.call(method,args) }
                        "SpatialSettings" -> { main.post { runCatching { startActivity(Intent(Settings.ACTION_SOUND_SETTINGS)) };reply(id,null) } }
                        "SetRefreshRate" -> { main.post {
                            val display=windowManager.defaultDisplay
                            val currentMode=display.mode
                            val requested=args.optString(0,"auto").toFloatOrNull() ?: 240f
                            val modes=display.supportedModes.filter { it.physicalWidth==currentMode.physicalWidth&&it.physicalHeight==currentMode.physicalHeight }.sortedBy {it.refreshRate}
                            val chosen=modes.firstOrNull {it.refreshRate>=requested} ?: modes.lastOrNull() ?: currentMode
                            window.attributes=window.attributes.apply {preferredDisplayModeId=chosen.modeId;preferredRefreshRate=chosen.refreshRate}
                            reply(id,chosen.refreshRate)
                        } }
                        "OpenURL" -> {app.openUrl(args.getString(0));reply(id,null)}
                        "GetAppUpdateState" -> reply(id,app.updater.snapshot())
                        "OpenUpdatePage","DownloadAppUpdate" -> reply(id,app.updater.startDownload())
                        "CancelAppUpdate" -> reply(id,app.updater.cancel())
                        "InstallAppUpdate" -> { val uri=app.updater.prepareInstall();main.post {try{app.updater.launchInstaller(this@MainActivity,uri);reply(id,app.updater.snapshot())}catch(error:Exception){reply(id,error=error.message)}} }
                        "Window" -> reply(id,null)
                        "DownloadTimingModel" -> { downloadModel(id) }
                        "AndroidStatus" -> main.post { reply(id,JSONObject().put("modelReady",File(app.root,"models/ggml-base.bin").isFile).put("modelBytes",147951465).put("sdk",Build.VERSION.SDK_INT).put("abi",Build.SUPPORTED_ABIS.first()).put("eqBands",PlaybackService.instance?.state()?.optInt("eqBands") ?: 0).put("audioOutput",getSharedPreferences("nawa-mobile",MODE_PRIVATE).getString("audioOutput","compatible"))) }
                        else -> {
                            reply(id,raw=CoreBridge.raw(method,args))
                            if(method in listOf("SaveWorkspace","Enqueue","Control","QueueTrackUpgrade","ApplyTrackUpgrade")){CoreBridge.call("WakeTransferQueue");DownloadScheduler.schedule(app)}
                        }
                    }
                } catch(e:Exception) { reply(id,error=e.message ?: "Android operation failed") }
            }
        }
        @JavascriptInterface fun audio(command: String,data: String) {
            val payload=runCatching {JSONObject(data)}.getOrNull() ?: return
            val request=payload.optString("_request")
            if(!Regex("a-[0-9]{1,18}").matches(request))return
            main.post { executeAudio(command,payload,request,0) }
        }
    }
    private fun executeAudio(command: String,data: JSONObject,request: String,attempt: Int) {
        if(isDestroyed||!::web.isInitialized)return
        val service=PlaybackService.instance
        if(service==null&&attempt<40){
            if(attempt==0)runCatching {startService(Intent(this,PlaybackService::class.java))}
            main.postDelayed({executeAudio(command,data,request,attempt+1)},100);return
        }
        val result=runCatching {checkNotNull(service){"Audio player is starting / مشغل الصوت لم يجهز"};service.command(command,data);service.state()}
        val state=result.getOrNull()?.toString() ?: "null"
        val error=result.exceptionOrNull()?.let {JSONObject.quote(it.message ?: "Playback failed / تعذر التشغيل")} ?: "null"
        web.evaluateJavascript("window.NawaMobile?.audioReply(${JSONObject.quote(request)},$state,$error)",null)
    }
    private fun setFullscreen(enabled: Boolean) {
        fullscreen=enabled
        val controller=WindowInsetsControllerCompat(window,web)
        controller.systemBarsBehavior=WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        if(enabled)controller.hide(WindowInsetsCompat.Type.systemBars()) else controller.show(WindowInsetsCompat.Type.systemBars())
        window.addFlags(android.view.WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON.takeIf { enabled } ?: 0)
        if(!enabled)window.clearFlags(android.view.WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
    }
    private fun hash(bytes: ByteArray)=MessageDigest.getInstance("SHA-256").digest(bytes).take(16).joinToString(""){"%02x".format(it)}
    private fun persist(uri: Uri) { runCatching {contentResolver.takePersistableUriPermission(uri,Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION)} }
    private fun copyUri(uri: Uri,dir: File,preferred: String?=null): String {
        check(!cancelled.get()){"أُلغي الاستيراد / Import cancelled"};dir.mkdirs()
        val doc=DocumentFile.fromSingleUri(this,uri);val name=(preferred ?: doc?.name ?: "audio").replace(Regex("[\\\\/:*?\"<>|\\p{Cntrl}]"),"_").take(180)
        val dest=File(dir,"${hash(uri.toString().toByteArray()).take(12)}-$name")
        val modified=doc?.lastModified() ?: 0;val length=doc?.length() ?: -1
        if(dest.exists()&&length>0&&dest.length()==length&&modified>0&&dest.lastModified()==modified)return dest.absolutePath
        check(length<0||length<dir.usableSpace-16777216){"لا توجد مساحة كافية / Not enough storage"}
        val part=File(dir,dest.name+".partial")
        try { contentResolver.openInputStream(uri)!!.use { input -> part.outputStream().use { out -> val buffer=ByteArray(262144);while(true){check(!cancelled.get()){"Import cancelled"};val n=input.read(buffer);if(n<0)break;out.write(buffer,0,n)} } };check(part.renameTo(dest)){"Cannot save imported file"};if(modified>0)dest.setLastModified(modified) }
        finally {part.delete()}
        return dest.absolutePath
    }
    private fun folderMappings(): JSONObject { val f=File(app.root,"folder-sources.json");return if(f.isFile)JSONObject(f.readText())else JSONObject() }
    private fun importTree(uri: Uri): Any? {
        cancelled.set(false);persist(uri)
        val label=(DocumentFile.fromTreeUri(this,uri)?.name ?: "Music").replace(Regex("[^\\p{L}\\p{N} _-]"),"_").take(60)
        val dir=File(app.root,"Folders/$label-${hash(uri.toString().toByteArray())}").apply { mkdirs() }
        val mapping=folderMappings().put(dir.absolutePath,uri.toString());File(app.root,"folder-sources.json").writeText(mapping.toString())
        copyTree(uri,dir)
        return CoreBridge.call("ImportLibraryFolders",JSONArray().put(JSONArray().put(dir.absolutePath)))
    }
    private fun copyTree(uri: Uri,dest: File) {
        val root=DocumentFile.fromTreeUri(this,uri) ?: error("المجلد غير متاح")
        val extensions=setOf("flac","mp3","m4a","aac","wav","ogg","opus","aiff","aif","wma","ape","wv","dsf","dff","ac3","eac3","mka","mp4","alac","tta","webm","amr","m4b","tak","lrc","txt","ttml","yrc","jpg","jpeg","png")
        val files=mutableListOf<Pair<DocumentFile,File>>()
        fun walk(doc: DocumentFile,dir: File,depth: Int) {
            check(depth<64){"Too many nested folders"};check(!cancelled.get()){"Import cancelled"}
            for(child in doc.listFiles()) {
                check(!cancelled.get()){"Import cancelled"}
                if(child.isDirectory)walk(child,File(dir,hash(child.uri.toString().toByteArray()).take(12)),depth+1)
                else if(child.name?.substringAfterLast('.',"")?.lowercase() in extensions)files.add(child to dir)
            }
        }
        try {
            CoreBridge.call("MobileCopyState",JSONArray().put(true).put(root.name).put(0).put(0))
            walk(root,dest,0)
            files.forEachIndexed { index,(doc,dir) ->
                CoreBridge.call("MobileCopyState",JSONArray().put(true).put(doc.name).put(index).put(files.size))
                copyUri(doc.uri,dir,doc.name)
            }
        } finally { CoreBridge.call("MobileCopyState",JSONArray().put(false).put("").put(files.size).put(files.size)) }
    }
    private fun exportLibrary(uri: Uri): Int {
        persist(uri);val target=DocumentFile.fromTreeUri(this,uri) ?: error("Folder unavailable")
        val jobs=(CoreBridge.call("GetState") as JSONObject).getJSONArray("jobs");var count=0
        for(i in 0 until jobs.length()) {
            val job=jobs.getJSONObject(i);if(job.optString("status")!="done")continue
            for(key in listOf("path","cover","lyrics")) {
                val f=File(job.optString(key));if(!f.isFile)continue
                // Existing exported files are preserved. New names do not overwrite user documents.
                if(target.findFile(f.name)!=null)continue
                val out=target.createFile(when(f.extension.lowercase()){ "jpg","jpeg"->"image/jpeg";"png"->"image/png";"lrc","txt"->"text/plain";else->"audio/${f.extension}" },f.name) ?: error("Cannot create export")
                contentResolver.openOutputStream(out.uri)!!.use { output -> f.inputStream().use { it.copyTo(output) } };count++
            }
        }
        return count
    }
    private fun fonts(): JSONArray {
        val families=linkedSetOf("sans-serif","serif","monospace","sans-serif-condensed","cursive","sans-serif-light","sans-serif-medium")
        runCatching { Regex("<family[^>]*name=\"([^\"]+)\"").findAll(File("/system/etc/fonts.xml").readText()).forEach { families.add(it.groupValues[1]) } }
        File(app.root,"fonts").listFiles()?.filter { it.extension in listOf("ttf","otf") }?.forEach { families.add("NawaFont-${it.nameWithoutExtension}") }
        return JSONArray(families.toList())
    }
    private fun downloadModel(id: String) {
        check(app.modelDownloading.compareAndSet(false,true)){"تنزيل النموذج يعمل بالفعل / Model download already running"}
        val dest=File(app.root,"models/ggml-base.bin");dest.parentFile!!.mkdirs();val part=File(dest.parentFile,"ggml-base.partial")
        try {
            val conn=URL("https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin").openConnection() as HttpURLConnection
            conn.connectTimeout=30000;conn.readTimeout=60000
            check(conn.responseCode==200){"Model download failed"};val digest=MessageDigest.getInstance("SHA-256");var read=0L;var last=0L
            conn.inputStream.use { input -> part.outputStream().use { out -> val buffer=ByteArray(262144);while(true){val n=input.read(buffer);if(n<0)break;out.write(buffer,0,n);digest.update(buffer,0,n);read+=n;if(read-last>2097152){last=read;val percent=(read*100/147951465);main.post {web.evaluateJavascript("window.NawaMobile?.modelProgress($percent)",null)}}} } };conn.disconnect()
            check(read==147951465L&&digest.digest().joinToString(""){"%02x".format(it)}=="60ed5bc3dd14eea856493d334349b405782ddcaf0028d4b5df4088345fba2efe"){"Model checksum mismatch"}
            check(part.renameTo(dest)){"Cannot install model"};reply(id,true)
        } finally {part.delete();app.modelDownloading.set(false)}
    }
}

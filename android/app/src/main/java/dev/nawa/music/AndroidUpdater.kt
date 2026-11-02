package dev.nawa.music

import android.app.Activity
import android.content.ClipData
import android.content.Intent
import android.content.pm.PackageInfo
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.SystemClock
import android.provider.Settings
import androidx.core.content.FileProvider
import org.json.JSONObject
import java.io.File
import java.io.RandomAccessFile
import java.net.HttpURLConnection
import java.net.URL
import java.security.MessageDigest
import java.util.concurrent.atomic.AtomicBoolean
import java.util.zip.ZipFile

/** Download and validate updates in app-private cache; Android confirms installation. */
class AndroidUpdater internal constructor(
    private val app: NawaApp,
    private val releaseStatus: ()->JSONObject = { CoreBridge.call("GetUpdateStatus") as JSONObject },
    private val connect: (URL)->HttpURLConnection = { it.openConnection() as HttpURLConnection },
    private val installed: ()->PackageInfo = { app.packageManager.getPackageInfo(app.packageName,PackageManager.GET_SIGNING_CERTIFICATES) },
    private val directory: File=File(app.cacheDir,"updates")
) {
    val active=AtomicBoolean(false)
    private val cancelled=AtomicBoolean(false)
    private val metadata=File(directory,"session.json")
    private var state=JSONObject().put("status","idle").put("progress",0)
    @Volatile private var liveConnection: HttpURLConnection?=null
    @Volatile private var permissionPending=false
    @Volatile private var canInstallCached=app.packageManager.canRequestPackageInstalls()
    init {
        directory.mkdirs()
        runCatching {
            val old=JSONObject(metadata.readText())
            if(old.optString("sha256").matches(Regex("[a-f0-9]{64}"))&&old.optLong("versionCode")>0) {
                state=old
                if(old.optLong("versionCode")<=installed().longVersionCode)state.put("status","installed")
                if(old.optString("status") in listOf("downloading","verifying","installing","permission"))state.put("status",if(readyFile().isFile)"ready" else "paused")
                if(state.optString("status")=="ready"&&!readyFile().isFile)state.put("status","idle")
            }
        }
    }
    @Synchronized fun snapshot(): JSONObject = JSONObject(state.toString()).put("active",active.get())
        .put("canInstall",canInstallCached)
    @Synchronized private fun update(persist: Boolean=false,block: (JSONObject)->Unit) {
        block(state)
        if(persist) runCatching { val part=File(directory,"session.json.partial");part.writeText(state.toString());check(part.renameTo(metadata)){"Cannot save update state"} }.onFailure { state.put("persistenceWarning","تعذر حفظ حالة التحديث / Cannot persist update state") }
    }
    private fun readyFile(): File {
        val s=synchronized(this){JSONObject(state.toString())}
        val code=s.optLong("versionCode");val hash=s.optString("sha256")
        check(code>0&&hash.matches(Regex("[a-f0-9]{64}"))){"هوية ملف التحديث غير صالحة / Invalid update identity"}
        return File(directory,"nawa-$code-$hash.apk")
    }
    fun startDownload(): JSONObject {
        if(active.get())return snapshot()
        val release=releaseStatus()
        check(release.optBoolean("available")){"افحص وجود تحديث جديد أولًا / Check for an available update first"}
        val address=release.getString("downloadUrl");val hash=release.getString("sha256").lowercase();val size=release.getLong("bytes")
        requireHttps(address);check(hash.matches(Regex("[a-f0-9]{64}"))&&size in 1..536870912L){"حجم أو بصمة التحديث غير صالح / Invalid update size or checksum"}
        check(release.getLong("versionCode")>installed().longVersionCode){"هذا الإصدار ليس أحدث من المثبّت / This version is already installed"}
        if(!active.compareAndSet(false,true))return snapshot()
        cancelled.set(false)
        update(true) { s -> s.put("status","downloading").put("url",address).put("sha256",hash).put("total",size).put("versionCode",release.getLong("versionCode")).put("version",release.getString("version")).put("error","").put("downloaded",0).put("progress",0) }
        app.io.execute {
            try { download();update(true){it.put("status","ready").put("progress",1.0).put("error","")} }
            catch(e: Exception) { update(true){it.put("status",if(cancelled.get())"paused" else "error").put("error",if(cancelled.get())"" else readableError(e))} }
            finally { liveConnection?.disconnect();liveConnection=null;active.set(false) }
        }
        return snapshot()
    }
    fun cancel(): JSONObject { cancelled.set(true);liveConnection?.disconnect();return snapshot() }
    private fun requireHttps(address: String) { val url=URL(address);check(url.protocol=="https"&&url.host.isNotBlank()&&url.userInfo==null){"رابط تحديث HTTPS مطلوب / An HTTPS update URL is required"} }
    private fun open(address: String,offset: Long): HttpURLConnection {
        var url=address
        repeat(6) {
            requireHttps(url)
            val connection=connect(URL(url));liveConnection=connection;connection.instanceFollowRedirects=false;connection.connectTimeout=30000;connection.readTimeout=30000
            connection.setRequestProperty("Accept-Encoding","identity");connection.setRequestProperty("User-Agent","Nawa-Android")
            if(offset>0)connection.setRequestProperty("Range","bytes=$offset-")
            val code=connection.responseCode
            if(code in listOf(301,302,303,307,308)) { val location=connection.getHeaderField("Location") ?: error("رابط تحويل غير صالح / Invalid update redirect");url=URL(URL(url),location).toString();connection.disconnect() }
            else return connection
        }
        error("تحويلات تحديث كثيرة / Too many update redirects")
    }
    private fun download() {
        val s=snapshot();val file=readyFile();val part=File(directory,file.name+".part");val total=s.getLong("total")
        if(file.isFile){try{validate(file,s);return}catch(_:Exception){check(file.delete()){"Cannot reset cached update"}}}
        if(part.length()>total)check(part.delete()){"Cannot reset partial update"}
        var offset=part.length()
        if(offset==total){check(part.renameTo(file)){"Cannot save APK"};try{validate(file,s)}catch(e:Exception){file.delete();throw e};return}
        val connection=open(s.getString("url"),offset)
        val code=connection.responseCode
        check(code==200||code==206){"تعذر تنزيل التحديث: HTTP $code / Update download failed"}
        if(code==200)offset=0
        else {
            val range=Regex("bytes (\\d+)-(\\d+)/(\\d+)").matchEntire(connection.getHeaderField("Content-Range") ?: "") ?: error("بيانات استئناف غير صالحة / Invalid resume response")
            check(range.groupValues[1].toLong()==offset&&range.groupValues[3].toLong()==total){"الملف تغير أثناء الاستئناف / Update changed during resume"}
        }
        var downloaded=offset;var last=0L
        connection.inputStream.use { input -> RandomAccessFile(part,"rw").use { output ->
            if(offset==0L)output.setLength(0)
            output.seek(offset)
            val buffer=ByteArray(262144)
            while(true) {
                check(!cancelled.get()){"Update paused"}
                val count=input.read(buffer);if(count<0)break
                downloaded+=count;check(downloaded<=total){"حجم التحديث أكبر من المعلن / Update exceeds declared size"};output.write(buffer,0,count)
                val now=SystemClock.elapsedRealtime()
                if(now-last>250){last=now;update{it.put("downloaded",downloaded).put("progress",downloaded.toDouble()/total)}}
            }
            output.fd.sync()
        } }
        check(downloaded==total){"التنزيل غير مكتمل؛ يمكنك استئنافه / Incomplete download; retry to resume"}
        update(true){it.put("status","verifying").put("downloaded",downloaded).put("progress",1.0)}
        check(part.renameTo(file)){"تعذر حفظ APK / Cannot save APK"}
        try { validate(file,s) } catch(e:Exception){file.delete();throw e}
    }
    internal fun validate(file: File,candidate: JSONObject) {
        check(file.length()==candidate.getLong("total")){"حجم APK لا يطابق الإصدار / APK size mismatch"}
        val hash=MessageDigest.getInstance("SHA-256");file.inputStream().use { input -> val b=ByteArray(262144);while(true){val n=input.read(b);if(n<0)break;hash.update(b,0,n)} }
        check(hash.digest().joinToString(""){"%02x".format(it)}==candidate.getString("sha256")){"بصمة APK غير مطابقة؛ لم يُثبّت / APK checksum mismatch"}
        val info=app.packageManager.getPackageArchiveInfo(file.absolutePath,PackageManager.GET_SIGNING_CERTIFICATES) ?: error("APK غير مقروء / Invalid APK")
        val current=installed()
        check(info.packageName==app.packageName){"هذه الحزمة لتطبيق مختلف / Wrong application package"}
        check(info.longVersionCode==candidate.getLong("versionCode")&&info.longVersionCode>current.longVersionCode){"إصدار APK لا يطابق التحديث / APK version mismatch"}
        val trusted=current.signingInfo?.apkContentsSigners?.map { MessageDigest.getInstance("SHA-256").digest(it.toByteArray()).toList() }?.toSet() ?: emptySet()
        val signing=info.signingInfo ?: error("APK غير موقّع / Unsigned APK")
        val certificates=(if(signing.hasMultipleSigners())signing.apkContentsSigners else signing.signingCertificateHistory)?.map { MessageDigest.getInstance("SHA-256").digest(it.toByteArray()).toList() }?.toSet() ?: emptySet()
        val signingMatches=if(signing.hasMultipleSigners()||current.signingInfo?.hasMultipleSigners()==true)certificates==trusted else certificates.containsAll(trusted)
        check(trusted.isNotEmpty()&&signingMatches){"توقيع APK مختلف عن التطبيق؛ لم يُثبّت / APK signing certificate mismatch"}
        check((info.applicationInfo?.minSdkVersion ?: 28)<=Build.VERSION.SDK_INT){"التحديث يحتاج إصدار أندرويد أحدث / Android version unsupported"}
        ZipFile(file).use { zip -> check(Build.SUPPORTED_ABIS.any { zip.getEntry("lib/$it/libnawacore.so")!=null }){"APK غير متوافق مع معالج الجهاز / APK CPU architecture mismatch"} }
    }
    fun prepareInstall(): Uri {
        check(!active.get()){"انتظر اكتمال التنزيل / Wait for the download"}
        val s=snapshot();check(s.optString("status") in listOf("ready","permission","installing")){"نزّل التحديث أولًا / Download the update first"}
        val file=readyFile();validate(file,s)
        return FileProvider.getUriForFile(app,app.packageName+".updates",file)
    }
    fun launchInstaller(activity: Activity,uri: Uri) {
        canInstallCached=app.packageManager.canRequestPackageInstalls()
        if(!canInstallCached) {
            permissionPending=true;update(true){it.put("status","permission")}
            activity.startActivity(Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,Uri.parse("package:"+app.packageName)));return
        }
        permissionPending=false
        val intent=Intent(Intent.ACTION_VIEW).setDataAndType(uri,"application/vnd.android.package-archive").addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        intent.clipData=ClipData.newRawUri("Nawa update",uri)
        activity.startActivity(intent);update(true){it.put("status","installing")}
    }
    fun onResume(activity: Activity) {
        canInstallCached=app.packageManager.canRequestPackageInstalls()
        if(permissionPending) {
            permissionPending=false
            if(canInstallCached) {
                app.io.execute {
                    runCatching { prepareInstall() }
                        .onSuccess { uri -> activity.runOnUiThread { if(!activity.isFinishing&&!activity.isDestroyed)launchInstaller(activity,uri) } }
                        .onFailure { error -> update(true){it.put("status","error").put("error",readableError(error))} }
                }
            } else update(true){it.put("status","ready")}
        } else if(snapshot().optString("status")=="installing")update(true){it.put("status","ready")}
    }
    private fun readableError(error: Throwable): String {
        val message=error.message ?: "تعذر تحديث التطبيق / App update failed"
        return message.replace(Regex("https?://\\S+"),"[update server]").take(500)
    }
}

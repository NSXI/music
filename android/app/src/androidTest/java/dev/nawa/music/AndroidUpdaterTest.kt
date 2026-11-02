package dev.nawa.music

import android.content.pm.PackageInfo
import android.content.pm.PackageManager
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File
import java.io.FileInputStream
import java.io.FilterInputStream
import java.io.InputStream
import java.net.HttpURLConnection
import java.net.URL
import java.security.MessageDigest
import java.util.UUID
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.TimeUnit

@RunWith(AndroidJUnit4::class)
class AndroidUpdaterTest {
    private fun app()=ApplicationProvider.getApplicationContext<NawaApp>().also { assertTrue(it.ready.await(120,TimeUnit.SECONDS));assertNull(it.startupError) }
    private fun hash(file: File): String {val digest=MessageDigest.getInstance("SHA-256");file.inputStream().use {input->val buffer=ByteArray(262144);while(true){val n=input.read(buffer);if(n<0)break;digest.update(buffer,0,n)}};return digest.digest().joinToString(""){"%02x".format(it)}}
    private fun candidate(file: File): JSONObject {
        val context=app();val info=context.packageManager.getPackageInfo(context.packageName,0)
        return JSONObject().put("available",true).put("downloadUrl","https://updates.invalid/nawa.apk").put("sha256",hash(file)).put("bytes",file.length()).put("total",file.length()).put("versionCode",info.longVersionCode).put("version",info.versionName)
    }
    private fun previous(app: NawaApp): PackageInfo {
        val actual=app.packageManager.getPackageInfo(app.packageName,PackageManager.GET_SIGNING_CERTIFICATES)
        return PackageInfo().apply {packageName=actual.packageName;signingInfo=actual.signingInfo;applicationInfo=actual.applicationInfo;setLongVersionCode(actual.longVersionCode-1)}
    }
    private class SourceConnection(url: URL,private val source: File,private val ranges: MutableList<Long>,private val slow: Boolean): HttpURLConnection(url) {
        private var closed=false
        private fun offset()=getRequestProperty("Range")?.removePrefix("bytes=")?.substringBefore('-')?.toLongOrNull() ?: 0L
        override fun connect(){}
        override fun disconnect(){closed=true}
        override fun usingProxy()=false
        override fun getResponseCode()=if(offset()>0)206 else 200
        override fun getHeaderField(name: String?): String?=when(name){"Content-Range"->"bytes ${offset()}-${source.length()-1}/${source.length()}";else->null}
        override fun getInputStream(): InputStream {
            val start=offset();ranges.add(start);val input=FileInputStream(source);input.channel.position(start)
            return object: FilterInputStream(input) {override fun read(b: ByteArray,off: Int,len: Int): Int {if(closed)throw java.io.IOException("paused");if(slow)Thread.sleep(5);return super.read(b,off,len)}}
        }
    }
    private fun finish(manager: AndroidUpdater) {val end=System.currentTimeMillis()+45000;while(manager.active.get()&&System.currentTimeMillis()<end)Thread.sleep(30);assertFalse("Update task timed out",manager.active.get())}
    @Test fun downloadsChecksHashAndSigningAndCreatesOnlyAContentUri() {
        val app=app();val source=File(app.applicationInfo.sourceDir);val info=candidate(source);val dir=File(app.cacheDir,"updates/test-${UUID.randomUUID()}");val ranges=CopyOnWriteArrayList<Long>()
        val manager=AndroidUpdater(app,{info},{url->SourceConnection(url,source,ranges,false)},{previous(app)},dir)
        manager.startDownload();finish(manager);assertEquals(manager.snapshot().toString(),"ready",manager.snapshot().getString("status"));assertEquals(0L,ranges.first())
        val uri=manager.prepareInstall();assertEquals("content",uri.scheme);assertEquals(app.packageName+".updates",uri.authority)
        // No installer or settings activity is launched by validation or URI preparation.
        assertEquals("ready",manager.snapshot().getString("status"))
    }
    @Test fun pausedDownloadResumesFromTheExistingPrefix() {
        val app=app();val source=File(app.applicationInfo.sourceDir);val info=candidate(source);val dir=File(app.cacheDir,"updates/test-${UUID.randomUUID()}");val ranges=CopyOnWriteArrayList<Long>()
        val manager=AndroidUpdater(app,{info},{url->SourceConnection(url,source,ranges,true)},{previous(app)},dir)
        manager.startDownload();val part=File(dir,"nawa-"+info.getLong("versionCode")+"-"+info.getString("sha256")+".apk.part");val end=System.currentTimeMillis()+10000
        while(part.length()<1048576&&System.currentTimeMillis()<end)Thread.sleep(10)
        manager.cancel();finish(manager);assertEquals("paused",manager.snapshot().getString("status"));assertTrue(part.length()>0)
        manager.startDownload();finish(manager);assertEquals(manager.snapshot().toString(),"ready",manager.snapshot().getString("status"));assertTrue(ranges.any {it>0})
    }
    @Test fun badChecksumAndWrongVersionNeverBecomeInstallable() {
        val app=app();val source=File(app.applicationInfo.sourceDir)
        for(kind in listOf("checksum","version")) {
            val info=candidate(source);if(kind=="checksum")info.put("sha256","0".repeat(64))else info.put("versionCode",app.packageManager.getPackageInfo(app.packageName,0).longVersionCode+1)
            val dir=File(app.cacheDir,"updates/test-${UUID.randomUUID()}");val manager=AndroidUpdater(app,{info},{url->SourceConnection(url,source,CopyOnWriteArrayList(),false)},{previous(app)},dir)
            manager.startDownload();finish(manager);assertEquals("error",manager.snapshot().getString("status"))
            try{manager.prepareInstall();fail("Invalid APK was installable")}catch(_: IllegalStateException){}
        }
    }
    @Test fun foreignPackageAndMissingTrustedSignatureAreRejected() {
        val app=app();val source=File(app.applicationInfo.sourceDir);val info=candidate(source)
        val manager=AndroidUpdater(app,{info},{url->SourceConnection(url,source,CopyOnWriteArrayList(),false)},{previous(app).apply {signingInfo=null}},File(app.cacheDir,"updates/test-${UUID.randomUUID()}"))
        try{manager.validate(source,info);fail("Untrusted signature accepted")}catch(_: IllegalStateException){}
        val other=File(InstrumentationRegistry.getInstrumentation().context.applicationInfo.sourceDir)
        try{manager.validate(other,candidate(other));fail("Foreign package accepted")}catch(_: IllegalStateException){}
    }
}

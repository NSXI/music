package dev.nawa.music

import android.app.Application
import android.content.Intent
import android.net.Uri
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import com.yausername.youtubedl_android.YoutubeDL
import com.yausername.ffmpeg.FFmpeg
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.security.KeyStore
import java.security.SecureRandom
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

class NawaApp : Application() {
    val ready = CountDownLatch(1)
    var startupError: String? = null
    val modelDownloading = java.util.concurrent.atomic.AtomicBoolean(false)
    val updater by lazy { AndroidUpdater(this) }
    val io = Executors.newFixedThreadPool(4)
    private val workers = Executors.newFixedThreadPool(2)
    lateinit var root: File
    lateinit var tools: File
    lateinit var processEnvironment: Map<String, String>
    override fun onCreate() {
        super.onCreate()
        root = File(filesDir,"nawa").apply { mkdirs() }
        io.execute {
            try {
                YoutubeDL.init(this); FFmpeg.getInstance().init(this)
                tools = File(noBackupFilesDir,"youtubedl-android")
                val bin = applicationInfo.nativeLibraryDir
                processEnvironment = mapOf(
                    "LD_LIBRARY_PATH" to "${tools}/packages/python/usr/lib:${tools}/packages/ffmpeg/usr/lib",
                    "SSL_CERT_FILE" to "${tools}/packages/python/usr/etc/tls/cert.pem",
                    "PYTHONHOME" to "${tools}/packages/python/usr",
                    "TMPDIR" to cacheDir.absolutePath,
                    "PATH" to "${System.getenv("PATH")}:$bin"
                )
                val paths = JSONObject().put("python","$bin/libpython.so").put("ytdlpZip","$tools/yt-dlp/yt-dlp")
                    .put("quickjs","$bin/libqjs.so").put("ffmpeg","$bin/libffmpeg.so")
                    .put("alignmentModel",File(root,"models/ggml-base.bin").absolutePath)
                CoreBridge.start(root.absolutePath, JSONObject().put("paths",paths).put("env",JSONArray(processEnvironment.map { "${it.key}=${it.value}" })), masterKey())
                Thread({
                    while(true) {
                        val work = CoreBridge.poll()
                        if(work == null) { Thread.sleep(50); continue }
                        workers.execute {
                            try { CoreBridge.complete(work.getString("id"), NativeWorkers(this).run(work.getString("kind"),work.getJSONObject("input").put("_workID",work.getString("id")))) }
                            catch(e: Exception) { CoreBridge.complete(work.getString("id"),error=e.message ?: "Native task failed") }
                        }
                    }
                },"nawa-core-workers").start()
            } catch(e: Throwable) { startupError = e.message ?: e.javaClass.simpleName }
            finally { ready.countDown() }
        }
    }
    private fun masterKey(): String {
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        val alias = "nawa-vault-v1"
        val key = (store.getKey(alias,null) as? SecretKey) ?: KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES,"AndroidKeyStore").apply {
            init(KeyGenParameterSpec.Builder(alias,KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT).setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build())
        }.generateKey()
        val f = File(noBackupFilesDir,"vault-key.sealed")
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        val secret = if(f.exists()) {
            val b=f.readBytes(); cipher.init(Cipher.DECRYPT_MODE,key,GCMParameterSpec(128,b.copyOfRange(0,12)));cipher.doFinal(b.copyOfRange(12,b.size))
        } else {
            val b=ByteArray(32).apply { SecureRandom().nextBytes(this) };cipher.init(Cipher.ENCRYPT_MODE,key)
            f.writeBytes(cipher.iv+cipher.doFinal(b));b
        }
        return Base64.encodeToString(secret,Base64.NO_WRAP)
    }
    fun ffmpeg(vararg args: String): String {
        val p=ProcessBuilder(listOf("${applicationInfo.nativeLibraryDir}/libffmpeg.so")+args).redirectErrorStream(true)
        p.environment().putAll(processEnvironment)
        val run=p.start();val log=run.inputStream.bufferedReader().use { it.readText() }
        check(run.waitFor()==0) { log.takeLast(1200) };return log
    }
    fun openUrl(address: String) {
        val uri=Uri.parse(address)
        check(uri.scheme in listOf("http","https")) { "Unsupported address" }
        startActivity(Intent(Intent.ACTION_VIEW,uri).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
    }
}

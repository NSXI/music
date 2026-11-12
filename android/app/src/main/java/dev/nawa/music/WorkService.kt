package dev.nawa.music

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import androidx.core.app.NotificationCompat
import org.json.JSONObject

class WorkService : Service() {
    companion object { @Volatile var running=false;fun intent(context: Context)=Intent(context,WorkService::class.java) }
    private val handler=Handler(Looper.getMainLooper())
    private var started=0L
    override fun onCreate() {
        super.onCreate();running=true;started=System.currentTimeMillis()
        getSystemService(NotificationManager::class.java).createNotificationChannel(NotificationChannel("nawa-work","Nawa downloads and local processing",NotificationManager.IMPORTANCE_LOW))
        startForeground(60,NotificationCompat.Builder(this,"nawa-work").setSmallIcon(R.drawable.ic_nawa).setContentTitle("نَوى · Nawa").setContentText("Downloads / Local processing · تحميل ومعالجة محلية").setOngoing(true).setSilent(true).build())
        handler.post(check)
    }
    private val check=object: Runnable {
        override fun run() {
            (application as NawaApp).io.execute {
                val s=runCatching { CoreBridge.call("MobileSummary") as JSONObject }.getOrNull()
                val app=application as NawaApp
                val transfers=runCatching {CoreBridge.call("GetTransferStatus") as JSONObject}.getOrNull()
                val transferPending=(transfers?.optInt("queued") ?: 0)+(transfers?.optInt("working") ?: 0)+(transfers?.optInt("metadata") ?: 0)
                val v3Busy=runCatching {CoreBridge.call("HasV3BackgroundWork")==true}.getOrDefault(false)
                val busy=v3Busy||app.modelDownloading.get()||app.updater.active.get()||(s!=null&&(s.optInt("pending")>transferPending||s.optBoolean("alignment")||s.optBoolean("translation")||s.optBoolean("folder")))||(transfers?.optBoolean("readyNow") ?: false)
                if(!busy&&transferPending>0)DownloadScheduler.schedule(app)
                handler.post { if(!busy&&System.currentTimeMillis()-started>15000)stopSelf() else handler.postDelayed(this,2000) }
            }
        }
    }
    override fun onBind(intent: Intent?): IBinder?=null
    override fun onTimeout(startId: Int,fgsType: Int){(application as NawaApp).quick.execute {runCatching {CoreBridge.call("HoldTransferQueue");CoreBridge.call("CancelAlignment");CoreBridge.call("CancelTranslation");CoreBridge.call("CancelOfflinePack");CoreBridge.call("CancelLyricClip")}};stopSelf()}
    override fun onDestroy(){running=false;handler.removeCallbacks(check);DownloadScheduler.schedule(application as NawaApp);super.onDestroy()}
}

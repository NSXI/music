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
    companion object { fun intent(context: Context)=Intent(context,WorkService::class.java) }
    private val handler=Handler(Looper.getMainLooper())
    private var started=0L
    override fun onCreate() {
        super.onCreate();started=System.currentTimeMillis()
        getSystemService(NotificationManager::class.java).createNotificationChannel(NotificationChannel("nawa-work","Nawa downloads and local processing",NotificationManager.IMPORTANCE_LOW))
        startForeground(60,NotificationCompat.Builder(this,"nawa-work").setSmallIcon(R.drawable.ic_nawa).setContentTitle("نَوى · Nawa").setContentText("Downloads / Local processing · تحميل ومعالجة محلية").setOngoing(true).setSilent(true).build())
        handler.post(check)
    }
    private val check=object: Runnable {
        override fun run() {
            (application as NawaApp).io.execute {
                val s=runCatching { CoreBridge.call("MobileSummary") as JSONObject }.getOrNull()
                val app=application as NawaApp
                val busy=app.modelDownloading.get()||app.updater.active.get()||(s!=null&&(s.optInt("pending")>0||s.optBoolean("alignment")||s.optBoolean("translation")||s.optBoolean("folder")))
                handler.post { if(!busy&&System.currentTimeMillis()-started>15000)stopSelf() else handler.postDelayed(this,2000) }
            }
        }
    }
    override fun onBind(intent: Intent?): IBinder?=null
    override fun onDestroy(){handler.removeCallbacks(check);super.onDestroy()}
}

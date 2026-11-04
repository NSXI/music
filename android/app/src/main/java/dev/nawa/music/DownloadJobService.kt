package dev.nawa.music

import android.app.job.JobInfo
import android.app.job.JobParameters
import android.app.job.JobScheduler
import android.app.job.JobService
import android.content.ComponentName
import android.os.Handler
import android.os.Looper
import org.json.JSONArray
import org.json.JSONObject
import java.time.Duration
import java.time.ZonedDateTime
import java.util.TimeZone
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean

/** Persisted system jobs keep scheduled/resumable transfers independent of UI. */
object DownloadScheduler {
    const val JOB_ID=204207
    @Volatile var running=false
    fun schedule(app: NawaApp,retryDelay: Long=0) {
        app.quick.execute {runCatching {
            if(!app.ready.await(120,TimeUnit.SECONDS)||app.startupError!=null)return@runCatching
            val manager=app.getSystemService(JobScheduler::class.java)
            val state=CoreBridge.call("GetTransferStatus") as JSONObject
            val pending=state.optInt("queued")+state.optInt("working")+state.optInt("metadata")
            if(pending==0){if(!running)manager.cancel(JOB_ID);return@runCatching}
            if(running)return@runCatching
            val p=state.getJSONObject("policy");var delay=retryDelay
            val now=ZonedDateTime.now();val start=p.optInt("startHour");val end=p.optInt("endHour")
            val inWindow=!p.optBoolean("scheduled")||start==end||if(start<end)now.hour>=start&&now.hour<end else now.hour>=start||now.hour<end
            if(!state.optBoolean("readyNow")&&!inWindow&&state.optInt("metadata")==0){var next=now.withHour(start).withMinute(0).withSecond(0).withNano(0);if(!next.isAfter(now))next=next.plusDays(1);delay=maxOf(delay,Duration.between(now,next).toMillis())}
            if(!state.optBoolean("networkAllowed")&&inWindow)delay=maxOf(delay,5*60*1000)
            val job=JobInfo.Builder(JOB_ID,ComponentName(app,DownloadJobService::class.java))
                .setPersisted(true).setMinimumLatency(delay.coerceAtLeast(0))
                .setRequiredNetworkType(if(p.optBoolean("wifiOnly"))JobInfo.NETWORK_TYPE_UNMETERED else JobInfo.NETWORK_TYPE_ANY)
                .setBackoffCriteria(30000,JobInfo.BACKOFF_POLICY_EXPONENTIAL).build()
            manager.schedule(job)
        }}
    }
    fun updateClock(app: NawaApp,wake: Boolean=false){app.quick.execute {runCatching {if(!app.ready.await(120,TimeUnit.SECONDS))return@runCatching;CoreBridge.call("SetTransferTimezoneOffset",JSONArray().put(TimeZone.getDefault().getOffset(System.currentTimeMillis())/1000));if(wake)CoreBridge.call("WakeTransferQueue")};schedule(app)}}
}

class DownloadJobService: JobService() {
    private val handler=Handler(Looper.getMainLooper())
    private val stopped=AtomicBoolean(false)
    private var generation=0
    override fun onStartJob(params: JobParameters): Boolean {
        stopped.set(false);DownloadScheduler.running=true;val epoch=++generation;val app=application as NawaApp
        app.io.execute {runCatching {check(app.ready.await(120,TimeUnit.SECONDS));CoreBridge.call("SetTransferTimezoneOffset",JSONArray().put(TimeZone.getDefault().getOffset(System.currentTimeMillis())/1000));CoreBridge.call("WakeTransferQueue")};handler.post {check(params,epoch)}}
        return true
    }
    private fun check(params: JobParameters,epoch: Int){if(stopped.get()||epoch!=generation)return;val app=application as NawaApp
        app.quick.execute {val status=runCatching {CoreBridge.call("GetTransferStatus") as JSONObject}.getOrNull();handler.post {
            if(stopped.get()||epoch!=generation)return@post
            if(status==null){finish(params,app,true);return@post}
            val pending=status.optInt("queued")+status.optInt("working")+status.optInt("metadata")
            if(pending==0){finish(params,app,false)}else if(!status.optBoolean("readyNow")&&status.optInt("working")==0){finish(params,app,false);DownloadScheduler.schedule(app,60000)}else handler.postDelayed({check(params,epoch)},1500)
        }}
    }
    private fun finish(params:JobParameters,app:NawaApp,retry:Boolean){stopped.set(true);DownloadScheduler.running=false;jobFinished(params,retry)}
    override fun onStopJob(params: JobParameters): Boolean {stopped.set(true);generation++;DownloadScheduler.running=false;if(!WorkService.running)(application as NawaApp).quick.execute {runCatching {CoreBridge.call("HoldTransferQueue")}};return true}
    override fun onDestroy(){handler.removeCallbacksAndMessages(null);DownloadScheduler.running=false;super.onDestroy()}
}

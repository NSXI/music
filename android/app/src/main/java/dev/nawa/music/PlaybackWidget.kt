package dev.nawa.music

import android.app.PendingIntent
import android.appwidget.AppWidgetManager
import android.appwidget.AppWidgetProvider
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.widget.RemoteViews

class PlaybackWidget : AppWidgetProvider() {
    override fun onUpdate(context: Context, manager: AppWidgetManager, ids: IntArray) {
        render(context, manager, ids, PlaybackService.instance)
    }

    override fun onReceive(context: Context, intent: Intent) {
        super.onReceive(context, intent)
        val service = PlaybackService.instance ?: return
        val player = service.player
        when (intent.action) {
            toggleAction -> {
                if (player.playWhenReady) player.pause() else player.play()
            }
            nextAction -> if (player.hasNextMediaItem()) player.seekToNextMediaItem()
            else -> return
        }
        refresh(service)
    }

    companion object {
        private const val toggleAction = "dev.nawa.music.widget.TOGGLE"
        private const val nextAction = "dev.nawa.music.widget.NEXT"
        private var signature = ""

        fun refresh(service: PlaybackService) {
            val player = service.player
            val title = player.mediaMetadata.title?.toString().orEmpty()
            val next = if (player.hasNextMediaItem()) {
                player.getMediaItemAt(player.nextMediaItemIndex).mediaMetadata.title?.toString().orEmpty()
            } else ""
            val current = "${player.currentMediaItem?.mediaId}|$title|$next|${player.playWhenReady}"
            if (current == signature) return
            signature = current
            val manager = AppWidgetManager.getInstance(service)
            val ids = manager.getAppWidgetIds(ComponentName(service, PlaybackWidget::class.java))
            if (ids.isNotEmpty()) render(service, manager, ids, service)
        }

        fun disconnected(context: Context) {
            signature = ""
            val manager = AppWidgetManager.getInstance(context)
            val ids = manager.getAppWidgetIds(ComponentName(context, PlaybackWidget::class.java))
            if (ids.isNotEmpty()) render(context, manager, ids, null)
        }

        private fun render(context: Context, manager: AppWidgetManager, ids: IntArray, service: PlaybackService?) {
            val prefs = context.getSharedPreferences("nawa-widget", Context.MODE_PRIVATE)
            val player = service?.player
            val liveTitle = player?.mediaMetadata?.title?.toString().orEmpty()
            val liveNext = if (player?.hasNextMediaItem() == true) {
                player.getMediaItemAt(player.nextMediaItemIndex).mediaMetadata.title?.toString().orEmpty()
            } else ""
            if (liveTitle.isNotEmpty()) {
                prefs.edit().putString("title", liveTitle).putString("next", liveNext).apply()
            }
            val title = liveTitle.ifEmpty { prefs.getString("title", "").orEmpty() }
            val next = if (liveTitle.isNotEmpty()) liveNext else prefs.getString("next", "").orEmpty()
            val launch = PendingIntent.getActivity(
                context, 41, Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
            )
            val views = RemoteViews(context.packageName, R.layout.nawa_playback_widget)
            views.setTextViewText(R.id.nawa_widget_title, title.ifEmpty { context.getString(R.string.widget_title) })
            views.setTextViewText(R.id.nawa_widget_queue, if (next.isEmpty()) context.getString(R.string.widget_open) else context.getString(R.string.widget_next_track, next))
            views.setTextViewText(R.id.nawa_widget_play, if (player?.playWhenReady == true) "Ⅱ" else "▶")
            views.setOnClickPendingIntent(R.id.nawa_widget_content, launch)
            views.setOnClickPendingIntent(R.id.nawa_widget_play, if (player?.currentMediaItem != null) command(context, toggleAction, 42) else launch)
            views.setOnClickPendingIntent(R.id.nawa_widget_next, if (player?.hasNextMediaItem() == true) command(context, nextAction, 43) else launch)
            for (id in ids) manager.updateAppWidget(id, views)
        }

        private fun command(context: Context, action: String, code: Int): PendingIntent =
            PendingIntent.getBroadcast(
                context, code, Intent(context, PlaybackWidget::class.java).setAction(action),
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
            )
    }
}

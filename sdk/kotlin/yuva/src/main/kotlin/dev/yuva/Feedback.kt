package dev.yuva

import android.content.Context
import android.os.Build
import java.util.UUID

class YuvaFeedback(
    val categoryKey: String,
    val body: String,
    val subject: String? = null,
    val screenshots: List<YuvaUpload> = emptyList(),
    val allowEmail: Boolean = false,
    val email: String? = null,
    val screen: String? = null,
    val clientId: String = UUID.randomUUID().toString(),
) {
    constructor(
        category: YuvaFeedbackCategory,
        body: String,
        subject: String? = null,
        screenshots: List<YuvaUpload> = emptyList(),
        allowEmail: Boolean = false,
        email: String? = null,
        screen: String? = null,
        clientId: String = UUID.randomUUID().toString(),
    ) : this(category.value, body, subject, screenshots, allowEmail, email, screen, clientId)

    val category: YuvaFeedbackCategory get() = YuvaFeedbackCategory.fromKey(categoryKey)
}

data class YuvaDeviceInfo(
    val appVersion: String?,
    val build: String?,
    val os: String?,
    val osVersion: String?,
    val deviceModel: String?,
    val locale: String?,
    val screen: String? = null,
    val installationId: String?,
) {
    companion object {
        fun current(context: Context): YuvaDeviceInfo {
            val info = runCatching { context.packageManager.getPackageInfo(context.packageName, 0) }.getOrNull()
            val build = info?.let {
                if (Build.VERSION.SDK_INT >= 28) it.longVersionCode.toString() else @Suppress("DEPRECATION") it.versionCode.toString()
            }
            return YuvaDeviceInfo(
                appVersion = info?.versionName,
                build = build,
                os = "Android",
                osVersion = Build.VERSION.RELEASE,
                deviceModel = listOf(Build.MANUFACTURER, Build.MODEL).filter { it.isNotBlank() }.joinToString(" "),
                locale = context.resources.configuration.locales[0].toLanguageTag(),
                installationId = installationId(context),
            )
        }

        private fun installationId(context: Context): String {
            val prefs = context.getSharedPreferences("yuva", Context.MODE_PRIVATE)
            prefs.getString("installation_id", null)?.let { return it }
            val id = UUID.randomUUID().toString()
            prefs.edit().putString("installation_id", id).apply()
            return id
        }
    }
}

enum class YuvaFeedbackState {
    SENT, ANSWERED, RESOLVED, PLANNED, IN_PROGRESS, DONE, WONT_DO;

    val isProgress: Boolean get() = this == PLANNED || this == IN_PROGRESS || this == DONE || this == WONT_DO

    companion object {
        fun of(
            status: YuvaConversationStatus,
            lastAuthor: YuvaAuthorType?,
            progress: YuvaFeedbackProgress? = null,
        ): YuvaFeedbackState = when {
            progress == YuvaFeedbackProgress.PLANNED -> PLANNED
            progress == YuvaFeedbackProgress.IN_PROGRESS -> IN_PROGRESS
            progress == YuvaFeedbackProgress.DONE -> DONE
            progress == YuvaFeedbackProgress.WONT_DO -> WONT_DO
            status == YuvaConversationStatus.CLOSED -> RESOLVED
            lastAuthor?.isTeam == true -> ANSWERED
            else -> SENT
        }
    }
}

val YuvaConversation.isFeedback: Boolean get() = kind == YuvaConversationKind.FEEDBACK || feedback != null

val YuvaConversation.feedbackState: YuvaFeedbackState?
    get() = if (isFeedback) YuvaFeedbackState.of(status, lastMessage?.authorType, feedback?.progress) else null

internal fun YuvaConversation.withFeedbackProgress(progress: YuvaFeedbackProgress?): YuvaConversation =
    if (feedback == null) this else copy(feedback = feedback.copy(progress = progress))

package dev.yuva

import android.content.Context
import android.os.Build
import java.util.UUID

class YuvaFeedback(
    val category: YuvaFeedbackCategory,
    val body: String,
    val subject: String? = null,
    val screenshots: List<YuvaUpload> = emptyList(),
    val allowEmail: Boolean = false,
    val email: String? = null,
    val screen: String? = null,
    val clientId: String = UUID.randomUUID().toString(),
)

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

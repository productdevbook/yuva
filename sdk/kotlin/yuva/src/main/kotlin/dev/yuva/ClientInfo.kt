package dev.yuva

import android.content.Context
import android.content.res.Resources
import android.os.Build
import android.os.StatFs
import java.util.TimeZone
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

internal object ClientInfo {
    fun current(context: Context): JsonElement {
        val device = YuvaDeviceInfo.current(context)
        val metrics = context.resources.displayMetrics
        val freeMb = runCatching { StatFs(context.filesDir.path).availableBytes / (1024 * 1024) }.getOrNull()
        val fields = mapOf(
            "app_version" to device.appVersion,
            "build" to device.build,
            "sdk_name" to "yuva-kotlin",
            "sdk_version" to Yuva.VERSION,
            "platform" to "android",
            "os" to device.os,
            "os_version" to device.osVersion,
            "device_manufacturer" to Build.MANUFACTURER,
            "device_model" to device.deviceModel,
            "app_locale" to device.locale,
            "device_locale" to Resources.getSystem().configuration.locales[0]?.toLanguageTag(),
            "timezone" to TimeZone.getDefault().id,
            "screen" to "${metrics.widthPixels}x${metrics.heightPixels}",
        )
        val values = fields.mapNotNull { (key, value) -> value?.takeIf { it.isNotBlank() }?.let { key to JsonPrimitive(it) } }.toMap()
        return JsonObject(if (freeMb != null) values + ("free_storage_mb" to JsonPrimitive(freeMb)) else values)
    }
}

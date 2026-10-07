package dev.yuva

import android.content.Context
import android.util.Base64
import java.time.Instant
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

internal data class StoredSession(
    val token: String? = null,
    val expiresAt: Instant? = null,
    val subject: String? = null,
    val visitorId: String? = null,
)

internal class SessionStore(context: Context, channelKey: String) {
    private val prefs = context.getSharedPreferences("yuva.session.$channelKey", Context.MODE_PRIVATE)

    fun load() = StoredSession(
        token = prefs.getString("token", null),
        expiresAt = prefs.getString("expires_at", null)?.let(Instant::parse),
        subject = prefs.getString("subject", null),
        visitorId = prefs.getString("visitor_id", null),
    )

    fun save(stored: StoredSession) {
        prefs.edit()
            .putString("token", stored.token)
            .putString("expires_at", stored.expiresAt?.toString())
            .putString("subject", stored.subject)
            .putString("visitor_id", stored.visitorId)
            .apply()
    }

    fun clear() {
        prefs.edit().clear().apply()
    }
}

internal object IdentityToken {
    fun subject(token: String): String? {
        val parts = token.split(".")
        if (parts.size != 3) return null
        return runCatching {
            val payload = String(Base64.decode(parts[1], Base64.URL_SAFE or Base64.NO_PADDING or Base64.NO_WRAP))
            YuvaClient.json.parseToJsonElement(payload).jsonObject["sub"]?.jsonPrimitive?.contentOrNull
        }.getOrNull()
    }
}

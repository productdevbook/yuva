package dev.yuva

import android.app.Application
import androidx.compose.runtime.Composable
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

object Yuva {
    const val VERSION: String = BuildConfig.YUVA_VERSION

    fun enableShakeToFeedback(
        application: Application,
        client: YuvaClient,
        screen: (() -> String?)? = null,
        isEnabled: () -> Boolean = { true },
        theme: (@Composable (content: @Composable () -> Unit) -> Unit)? = null,
    ) = ShakeFeedback.enable(application, client, screen, isEnabled, theme)

    fun disableShakeToFeedback() = ShakeFeedback.disable()

    fun setPushToken(token: String) = PushRegistry.set(token)

    fun handleNotification(data: Map<String, String>): String? =
        data["yuva_conversation_id"]
            ?: data["yuva"]?.let { raw ->
                runCatching {
                    YuvaClient.json.parseToJsonElement(raw).jsonObject["conversation_id"]?.jsonPrimitive?.contentOrNull
                }.getOrNull()
            }
}

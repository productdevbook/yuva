package dev.yuva

import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

object Yuva {
    const val VERSION: String = BuildConfig.YUVA_VERSION

    fun handleNotification(data: Map<String, String>): String? =
        data["yuva_conversation_id"]
            ?: data["yuva"]?.let { raw ->
                runCatching {
                    YuvaClient.json.parseToJsonElement(raw).jsonObject["conversation_id"]?.jsonPrimitive?.contentOrNull
                }.getOrNull()
            }
}

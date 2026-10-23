package dev.yuva

import kotlinx.serialization.KSerializer
import kotlinx.serialization.Serializable
import kotlinx.serialization.descriptors.SerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder
import kotlinx.serialization.json.JsonDecoder
import kotlinx.serialization.json.JsonObject

sealed interface YuvaAttachmentRefusal {
    data class TooLarge(val maxBytes: Long) : YuvaAttachmentRefusal
    data object TypeNotAllowed : YuvaAttachmentRefusal

    val error: YuvaException
        get() = when (this) {
            is TooLarge -> YuvaException(413, "attachment_too_large", "an attachment is larger than the channel allows")
            TypeNotAllowed -> YuvaException(415, "attachment_type_not_allowed", "this type of file is not allowed")
        }
}

@Serializable
data class YuvaAttachmentLimits(
    val maxBytes: Long = Long.MAX_VALUE,
    val types: List<String> = listOf("*/*"),
) {
    val acceptsFiles: Boolean get() = types.isNotEmpty()

    val allowsImages: Boolean get() = allows("image/jpeg")

    fun allows(contentType: String): Boolean {
        val type = contentType.substringBefore(';').trim().lowercase()
        return types.any { entry ->
            val allowed = entry.lowercase()
            allowed == "*/*" || allowed == type || allowed.endsWith("/*") && type.startsWith(allowed.dropLast(1))
        }
    }

    fun refusal(upload: YuvaUpload): YuvaAttachmentRefusal? = when {
        !allows(upload.contentType) -> YuvaAttachmentRefusal.TypeNotAllowed
        upload.data.size.toLong() > maxBytes -> YuvaAttachmentRefusal.TooLarge(maxBytes)
        else -> null
    }
}

internal object LossyAttachmentLimitsSerializer : KSerializer<YuvaAttachmentLimits?> {
    private val inner = YuvaAttachmentLimits.serializer()
    override val descriptor: SerialDescriptor = inner.descriptor

    override fun serialize(encoder: Encoder, value: YuvaAttachmentLimits?) {
        if (value != null) encoder.encodeSerializableValue(inner, value)
    }

    override fun deserialize(decoder: Decoder): YuvaAttachmentLimits? {
        val input = decoder as? JsonDecoder ?: return decoder.decodeSerializableValue(inner)
        val element = input.decodeJsonElement()
        if (element !is JsonObject) return YuvaAttachmentLimits()
        return runCatching { input.json.decodeFromJsonElement(inner, element) }.getOrDefault(YuvaAttachmentLimits())
    }
}

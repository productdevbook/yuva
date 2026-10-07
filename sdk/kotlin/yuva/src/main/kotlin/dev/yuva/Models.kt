package dev.yuva

import java.time.Instant
import java.time.OffsetDateTime
import kotlinx.serialization.KSerializer
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.descriptors.PrimitiveKind
import kotlinx.serialization.descriptors.PrimitiveSerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder

internal object InstantSerializer : KSerializer<Instant> {
    override val descriptor = PrimitiveSerialDescriptor("Instant", PrimitiveKind.STRING)
    override fun serialize(encoder: Encoder, value: Instant) = encoder.encodeString(value.toString())
    override fun deserialize(decoder: Decoder): Instant = OffsetDateTime.parse(decoder.decodeString()).toInstant()
}

@Serializable
enum class YuvaConversationStatus {
    @SerialName("open") OPEN,
    @SerialName("pending") PENDING,
    @SerialName("snoozed") SNOOZED,
    @SerialName("closed") CLOSED,
}

@Serializable
enum class YuvaAuthorType {
    @SerialName("contact") CONTACT,
    @SerialName("member") MEMBER,
    @SerialName("system") SYSTEM,
}

@Serializable
enum class YuvaDirection {
    @SerialName("in") IN,
    @SerialName("out") OUT,
}

@Serializable
enum class YuvaInboxMode {
    @SerialName("live") LIVE,
    @SerialName("async") ASYNC,
}

@Serializable
enum class YuvaConversationKind {
    @SerialName("conversation") CONVERSATION,
    @SerialName("feedback") FEEDBACK,
}

@Serializable
enum class YuvaFeedbackCategory(val value: String) {
    @SerialName("bug") BUG("bug"),
    @SerialName("idea") IDEA("idea"),
    @SerialName("praise") PRAISE("praise"),
    @SerialName("other") OTHER("other"),
}

enum class YuvaOrder(val value: String) { ASC("asc"), DESC("desc") }

@Serializable
data class YuvaMember(val name: String, val initials: String)

@Serializable
data class YuvaPresence(val available: Boolean, val members: List<YuvaMember> = emptyList())

@Serializable
data class YuvaBranding(val color: String? = null, val logoUrl: String? = null, val greeting: String? = null)

@Serializable
data class YuvaInbox(
    val id: String,
    val name: String,
    val branding: YuvaBranding = YuvaBranding(),
    val defaultLocale: String = "en",
    val timezone: String = "UTC",
    val mode: YuvaInboxMode = YuvaInboxMode.ASYNC,
    val openNow: Boolean = true,
    val expectedReplyMinutes: Int? = null,
    val presence: YuvaPresence? = null,
    @SerialName("feedback_categories") internal val feedbackCategoryNames: List<String> = emptyList(),
) {
    val feedbackCategories: List<YuvaFeedbackCategory>
        get() = feedbackCategoryNames.mapNotNull { name -> YuvaFeedbackCategory.entries.firstOrNull { it.value == name } }
}

@Serializable
data class YuvaContact(
    val id: String,
    val name: String = "",
    val email: String? = null,
    val typedEmail: String? = null,
    val identified: Boolean = false,
)

@Serializable
data class YuvaSession(
    @Serializable(InstantSerializer::class) val expiresAt: Instant,
    val visitorId: String? = null,
    val contact: YuvaContact,
    val inbox: YuvaInbox,
)

@Serializable
internal data class SessionResponse(
    val token: String,
    @Serializable(InstantSerializer::class) val expiresAt: Instant,
    val visitorId: String? = null,
    val contact: YuvaContact,
    val inbox: YuvaInbox,
) {
    val session get() = YuvaSession(expiresAt, visitorId, contact, inbox)
}

@Serializable
data class YuvaMessagePreview(
    val id: String,
    val authorType: YuvaAuthorType,
    val text: String,
    @Serializable(InstantSerializer::class) val createdAt: Instant,
)

@Serializable
data class YuvaFeedbackInfo(
    val category: YuvaFeedbackCategory,
    val allowEmail: Boolean = false,
    val appVersion: String? = null,
    val build: String? = null,
    val os: String? = null,
    val osVersion: String? = null,
    val deviceModel: String? = null,
    val locale: String? = null,
    val screen: String? = null,
    val installationId: String? = null,
)

@Serializable
data class YuvaConversation(
    val id: String,
    val kind: YuvaConversationKind = YuvaConversationKind.CONVERSATION,
    val feedback: YuvaFeedbackInfo? = null,
    val subject: String = "",
    val status: YuvaConversationStatus,
    val lastMessage: YuvaMessagePreview? = null,
    @Serializable(InstantSerializer::class) val lastMessageAt: Instant? = null,
    val unread: Boolean = false,
    @Serializable(InstantSerializer::class) val lastReadByMemberAt: Instant? = null,
    @Serializable(InstantSerializer::class) val createdAt: Instant,
)

@Serializable
data class YuvaMessageAuthor(val type: YuvaAuthorType, val name: String? = null, val initials: String? = null)

@Serializable
data class YuvaAttachment(
    val id: String,
    val filename: String,
    val contentType: String,
    val size: Long,
    val contentId: String? = null,
    val inline: Boolean = false,
) {
    val isImage: Boolean get() = contentType.startsWith("image/")
}

@Serializable
data class YuvaMessage(
    val id: String,
    val conversationId: String,
    val direction: YuvaDirection,
    val author: YuvaMessageAuthor,
    val body: String = "",
    val html: String? = null,
    val clientId: String? = null,
    val attachments: List<YuvaAttachment> = emptyList(),
    @Serializable(InstantSerializer::class) val createdAt: Instant,
)

@Serializable
data class YuvaPage<T>(val items: List<T>, val nextCursor: String? = null)

@Serializable
data class YuvaConversationStarted(val conversation: YuvaConversation, val message: YuvaMessage)

@Serializable
data class YuvaReadState(val conversationId: String, val lastReadMessageId: String? = null, val unread: Boolean)

class YuvaUpload(val filename: String, val contentType: String, val data: ByteArray)

sealed interface YuvaEvent {
    data class ConversationCreated(val conversation: YuvaConversation) : YuvaEvent
    data class ConversationUpdated(val conversationId: String, val status: YuvaConversationStatus) : YuvaEvent
    data class MessageCreated(val message: YuvaMessage) : YuvaEvent
    data class MessageUpdated(val message: YuvaMessage) : YuvaEvent
    data class Read(val conversationId: String, val readAt: Instant) : YuvaEvent
    data class Typing(val conversationId: String, val typing: Boolean, val author: YuvaMessageAuthor) : YuvaEvent
    data class Presence(val presence: YuvaPresence) : YuvaEvent
    data class InboxUpdated(val inbox: YuvaInbox) : YuvaEvent
    data object ResyncRequired : YuvaEvent
    data class ConnectionChanged(val connected: Boolean) : YuvaEvent
}

class YuvaException(val status: Int, val code: String, val detail: String?) : Exception(detail ?: code)

@Serializable
internal data class Problem(val title: String? = null, val status: Int? = null, val code: String? = null, val detail: String? = null)

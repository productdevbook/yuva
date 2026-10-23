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
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonDecoder
import kotlinx.serialization.json.decodeFromJsonElement

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

@Serializable(with = AuthorTypeSerializer::class)
enum class YuvaAuthorType(internal val wire: String) {
    CONTACT("contact"),
    MEMBER("member"),
    BOT("bot"),
    SYSTEM("system");

    internal val isTeam: Boolean get() = this == MEMBER || this == BOT
}

internal object AuthorTypeSerializer : KSerializer<YuvaAuthorType> {
    override val descriptor = PrimitiveSerialDescriptor("YuvaAuthorType", PrimitiveKind.STRING)
    override fun serialize(encoder: Encoder, value: YuvaAuthorType) = encoder.encodeString(value.wire)
    override fun deserialize(decoder: Decoder): YuvaAuthorType {
        val raw = decoder.decodeString()
        return YuvaAuthorType.entries.firstOrNull { it.wire == raw } ?: YuvaAuthorType.SYSTEM
    }
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
enum class YuvaConversationKind(val value: String) {
    @SerialName("conversation") CONVERSATION("conversation"),
    @SerialName("feedback") FEEDBACK("feedback"),
}

@Serializable
enum class YuvaFeedbackCategory(val value: String) {
    @SerialName("bug") BUG("bug"),
    @SerialName("idea") IDEA("idea"),
    @SerialName("praise") PRAISE("praise"),
    @SerialName("other") OTHER("other");

    companion object {
        fun fromKey(key: String?): YuvaFeedbackCategory = entries.firstOrNull { it.value == key } ?: OTHER
    }
}

@Serializable
data class YuvaFeedbackCategoryItem(val key: String, val name: String = "", val icon: String? = null)

@Serializable(YuvaFeedbackProgressSerializer::class)
enum class YuvaFeedbackProgress(val value: String) {
    PLANNED("planned"),
    IN_PROGRESS("in_progress"),
    DONE("done"),
    WONT_DO("wont_do"),
}

internal object YuvaFeedbackProgressSerializer : KSerializer<YuvaFeedbackProgress?> {
    override val descriptor = PrimitiveSerialDescriptor("YuvaFeedbackProgress", PrimitiveKind.STRING)
    override fun serialize(encoder: Encoder, value: YuvaFeedbackProgress?) = encoder.encodeString(value?.value.orEmpty())
    override fun deserialize(decoder: Decoder): YuvaFeedbackProgress? {
        val raw = decoder.decodeString()
        return YuvaFeedbackProgress.entries.firstOrNull { it.value == raw }
    }
}

@Serializable
enum class YuvaRating(val value: String) {
    @SerialName("good") GOOD("good"),
    @SerialName("bad") BAD("bad"),
}

enum class YuvaOrder(val value: String) { ASC("asc"), DESC("desc") }

@Serializable
data class YuvaMember(val name: String, val initials: String)

@Serializable
data class YuvaPresence(val available: Boolean, val members: List<YuvaMember> = emptyList())

@Serializable
data class YuvaBranding(val color: String? = null, val logoUrl: String? = null, val greeting: String? = null)

internal open class WireEnumSerializer<T : Enum<T>>(
    name: String,
    private val entries: List<T>,
    private val wire: (T) -> String,
    private val fallback: T,
) : KSerializer<T> {
    override val descriptor = PrimitiveSerialDescriptor(name, PrimitiveKind.STRING)
    override fun serialize(encoder: Encoder, value: T) = encoder.encodeString(wire(value))
    override fun deserialize(decoder: Decoder): T {
        val raw = decoder.decodeString()
        return entries.firstOrNull { wire(it) == raw } ?: fallback
    }
}

@Serializable(YuvaStatusImpactSerializer::class)
enum class YuvaStatusImpact(internal val wire: String) {
    NONE("none"),
    MINOR("minor"),
    MAJOR("major"),
    CRITICAL("critical"),
}

internal object YuvaStatusImpactSerializer :
    WireEnumSerializer<YuvaStatusImpact>("YuvaStatusImpact", YuvaStatusImpact.entries, { it.wire }, YuvaStatusImpact.NONE)

@Serializable(YuvaStatusKindSerializer::class)
enum class YuvaStatusKind(internal val wire: String) {
    INCIDENT("incident"),
    MAINTENANCE("maintenance"),
}

internal object YuvaStatusKindSerializer :
    WireEnumSerializer<YuvaStatusKind>("YuvaStatusKind", YuvaStatusKind.entries, { it.wire }, YuvaStatusKind.INCIDENT)

@Serializable(YuvaStatusStateSerializer::class)
enum class YuvaStatusState(internal val wire: String) {
    INVESTIGATING("investigating"),
    IDENTIFIED("identified"),
    MONITORING("monitoring"),
    RESOLVED("resolved"),
    SCHEDULED("scheduled"),
    IN_PROGRESS("in_progress"),
    COMPLETED("completed"),
    UNKNOWN(""),
}

internal object YuvaStatusStateSerializer :
    WireEnumSerializer<YuvaStatusState>("YuvaStatusState", YuvaStatusState.entries, { it.wire }, YuvaStatusState.UNKNOWN)

@Serializable
data class YuvaStatusIncident(
    val id: String,
    val kind: YuvaStatusKind = YuvaStatusKind.INCIDENT,
    val title: String,
    val impact: YuvaStatusImpact = YuvaStatusImpact.NONE,
    val state: YuvaStatusState = YuvaStatusState.UNKNOWN,
    @Serializable(InstantSerializer::class) val updatedAt: Instant,
    val url: String = "",
)

internal object LossyIncidentsSerializer : KSerializer<List<YuvaStatusIncident>> {
    override val descriptor = PrimitiveSerialDescriptor("YuvaStatusIncidents", PrimitiveKind.STRING)
    override fun serialize(encoder: Encoder, value: List<YuvaStatusIncident>) = Unit
    override fun deserialize(decoder: Decoder): List<YuvaStatusIncident> {
        val input = decoder as JsonDecoder
        val array = input.decodeJsonElement() as? JsonArray ?: return emptyList()
        return array.mapNotNull { runCatching { input.json.decodeFromJsonElement<YuvaStatusIncident>(it) }.getOrNull() }
    }
}

@Serializable
data class YuvaInboxStatus(
    val url: String = "",
    @Serializable(LossyIncidentsSerializer::class) val open: List<YuvaStatusIncident> = emptyList(),
)

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
    val status: YuvaInboxStatus? = null,
    @SerialName("feedback_categories") internal val feedbackCategoryNames: List<String> = emptyList(),
    val feedbackCategoryItems: List<YuvaFeedbackCategoryItem> = emptyList(),
    val askForRating: Boolean = false,
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
    @SerialName("category") internal val categoryValue: String = "other",
    @SerialName("category_key") internal val categoryKeyValue: String? = null,
    val categoryName: String? = null,
    val allowEmail: Boolean = false,
    val text: String? = null,
    val attachmentCount: Int = 0,
    val appVersion: String? = null,
    val build: String? = null,
    val os: String? = null,
    val osVersion: String? = null,
    val deviceModel: String? = null,
    val locale: String? = null,
    val screen: String? = null,
    val installationId: String? = null,
    val progress: YuvaFeedbackProgress? = null,
) {
    val categoryKey: String get() = categoryKeyValue ?: categoryValue
    val category: YuvaFeedbackCategory get() = YuvaFeedbackCategory.fromKey(categoryKey)
}

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
    val canRate: Boolean = false,
    val rating: YuvaRating? = null,
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

@Serializable
enum class YuvaSubscriptionState(val value: String) {
    @SerialName("pending") PENDING("pending"),
    @SerialName("subscribed") SUBSCRIBED("subscribed"),
    @SerialName("unsubscribed") UNSUBSCRIBED("unsubscribed"),
}

@Serializable
data class YuvaMailingList(
    val id: String,
    val name: String,
    val description: String = "",
    val doubleOptIn: Boolean = false,
)

@Serializable
data class YuvaSubscription(val list: YuvaMailingList, val state: YuvaSubscriptionState? = null)

@Serializable
internal data class SubscriptionStatus(val state: YuvaSubscriptionState)

class YuvaUpload(val filename: String, val contentType: String, val data: ByteArray)

sealed interface YuvaEvent {
    data class ConversationCreated(val conversation: YuvaConversation) : YuvaEvent
    data class ConversationUpdated(
        val conversationId: String,
        val status: YuvaConversationStatus,
        val canRate: Boolean = false,
        val rating: YuvaRating? = null,
        val feedbackProgress: YuvaFeedbackProgress? = null,
    ) : YuvaEvent
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

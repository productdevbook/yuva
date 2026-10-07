package dev.yuva

import android.content.Context
import java.io.IOException
import java.time.Instant
import java.util.UUID
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import kotlin.math.min
import kotlin.math.pow
import kotlin.random.Random
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNamingStrategy
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull
import kotlinx.serialization.json.put
import okhttp3.Call
import okhttp3.Callback
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.MultipartBody
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener

class YuvaConfiguration(
    val serverUrl: String,
    val channelKey: String,
    val identityToken: (suspend () -> String?)? = null,
)

class YuvaClient(
    context: Context,
    val configuration: YuvaConfiguration,
    httpClient: OkHttpClient = OkHttpClient(),
) {
    internal val appContext: Context = context.applicationContext
    private val http = httpClient.newBuilder().pingInterval(0, TimeUnit.SECONDS).build()
    private val store = SessionStore(appContext, configuration.channelKey)
    private val base: HttpUrl = configuration.serverUrl.trimEnd('/').toHttpUrl()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val sessionLock = Mutex()
    private val shared = MutableSharedFlow<YuvaEvent>(extraBufferCapacity = 256, onBufferOverflow = BufferOverflow.DROP_OLDEST)
    private var realtime: Job? = null

    @Volatile private var token: String? = null
    @Volatile private var lastEventId = 0L

    @Volatile var session: YuvaSession? = null
        private set

    fun events(): SharedFlow<YuvaEvent> = shared.asSharedFlow()

    init {
        scope.launch {
            shared.subscriptionCount.map { it > 0 }.distinctUntilChanged().collect { active ->
                if (active) startRealtime() else stopRealtime()
            }
        }
    }

    suspend fun start(): YuvaSession {
        sessionToken()
        return session ?: refreshSession()
    }

    suspend fun refreshSession(): YuvaSession =
        request<YuvaSession>("GET", "client/v1/session").also { session = it }

    suspend fun identify(): YuvaSession {
        sessionToken(fresh = true)
        restartRealtime()
        return start()
    }

    suspend fun signOut() {
        stopRealtime()
        token?.let { current ->
            runCatching { execute(Request.Builder().url(url("client/v1/session")).delete().bearer(current).build()) }
        }
        token = null
        session = null
        lastEventId = 0
        store.clear()
        if (shared.subscriptionCount.value > 0) startRealtime()
    }

    suspend fun conversations(cursor: String? = null, limit: Int? = null): YuvaPage<YuvaConversation> =
        request("GET", "client/v1/conversations", query = paging(cursor, limit))

    suspend fun conversation(id: String): YuvaConversation = request("GET", "client/v1/conversations/$id")

    suspend fun messages(
        conversationId: String,
        order: YuvaOrder = YuvaOrder.DESC,
        cursor: String? = null,
        limit: Int? = null,
    ): YuvaPage<YuvaMessage> = request(
        "GET",
        "client/v1/conversations/$conversationId/messages",
        query = listOf("order" to order.value) + paging(cursor, limit),
    )

    suspend fun startConversation(
        body: String,
        subject: String? = null,
        attachments: List<YuvaUpload> = emptyList(),
        clientId: String = UUID.randomUUID().toString(),
    ): YuvaConversationStarted = request(
        "POST",
        "client/v1/conversations",
        body = Body(mapOf("body" to body, "client_id" to clientId, "subject" to subject), attachments),
    )

    suspend fun sendMessage(
        conversationId: String,
        body: String,
        attachments: List<YuvaUpload> = emptyList(),
        clientId: String = UUID.randomUUID().toString(),
    ): YuvaMessage = request(
        "POST",
        "client/v1/conversations/$conversationId/messages",
        body = Body(mapOf("body" to body, "client_id" to clientId), attachments),
    )

    suspend fun markRead(conversationId: String, messageId: String? = null): YuvaReadState =
        request("POST", "client/v1/conversations/$conversationId/read", body = Body(mapOf("message_id" to messageId)))

    suspend fun setTyping(conversationId: String, typing: Boolean) {
        requestRaw(
            "POST",
            "client/v1/conversations/$conversationId/typing",
            body = Body(json = buildJsonObject { put("typing", typing) }),
        )
    }

    suspend fun setContactEmail(email: String): YuvaContact =
        request("PUT", "client/v1/contact/email", body = Body(mapOf("email" to email)))

    suspend fun attachment(id: String): ByteArray = requestRaw("GET", "client/v1/attachments/$id")

    suspend fun sendFeedback(
        feedback: YuvaFeedback,
        device: YuvaDeviceInfo = YuvaDeviceInfo.current(appContext),
    ): YuvaConversationStarted {
        val fields = mapOf(
            "category" to feedback.category.value,
            "body" to feedback.body,
            "client_id" to feedback.clientId,
            "subject" to feedback.subject,
            "email" to feedback.email,
            "screen" to (feedback.screen ?: device.screen),
            "app_version" to device.appVersion,
            "build" to device.build,
            "os" to device.os,
            "os_version" to device.osVersion,
            "device_model" to device.deviceModel,
            "locale" to device.locale,
            "installation_id" to device.installationId,
        )
        val body = if (feedback.screenshots.isEmpty()) {
            Body(json = buildJsonObject {
                fields.forEach { (key, value) -> if (value != null) put(key, value) }
                put("allow_email", feedback.allowEmail)
            })
        } else {
            Body(fields + ("allow_email" to feedback.allowEmail.toString()), feedback.screenshots)
        }
        return request("POST", "client/v1/feedback", body = body)
    }

    // Session

    private suspend fun sessionToken(fresh: Boolean = false): String {
        if (!fresh) token?.let { return it }
        return sessionLock.withLock {
            if (!fresh) token?.let { return@withLock it }
            beginSession(fresh)
        }
    }

    private suspend fun beginSession(fresh: Boolean): String {
        val stored = store.load()
        val identity = configuration.identityToken?.invoke()
        val subject = identity?.let(IdentityToken::subject)
        val saved = stored.token
        if (!fresh && saved != null && stored.subject == subject &&
            (stored.expiresAt ?: Instant.EPOCH).isAfter(Instant.now().plusSeconds(60))
        ) {
            try {
                val info = decode<YuvaSession>(send("GET", "client/v1/session", emptyList(), null, saved))
                apply(info, saved, subject, stored)
                return saved
            } catch (error: YuvaException) {
                if (error.status != 401 && error.status != 403) throw error
            }
        }
        val created = decode<SessionResponse>(
            send(
                "POST",
                "client/v1/session",
                emptyList(),
                Body(mapOf("channel_key" to configuration.channelKey, "identity_token" to identity, "visitor_id" to stored.visitorId)),
                null,
            ),
        )
        apply(created.session, created.token, subject, stored)
        return created.token
    }

    private fun apply(info: YuvaSession, newToken: String, subject: String?, stored: StoredSession) {
        if (token != newToken) lastEventId = 0
        token = newToken
        session = info
        store.save(
            stored.copy(
                token = newToken,
                expiresAt = info.expiresAt,
                subject = subject,
                visitorId = info.visitorId ?: if (info.contact.identified) null else stored.visitorId,
            ),
        )
    }

    private fun dropSession() {
        token = null
        store.save(store.load().copy(token = null, expiresAt = null))
    }

    // HTTP

    internal class Body(
        val fields: Map<String, String?> = emptyMap(),
        val files: List<YuvaUpload> = emptyList(),
        val json: JsonElement? = null,
    ) {
        fun toRequestBody(): RequestBody {
            if (files.isEmpty()) {
                val element = json ?: JsonObject(fields.filterValues { it != null }.mapValues { JsonPrimitive(it.value) })
                return element.toString().toRequestBody(JSON_TYPE)
            }
            val builder = MultipartBody.Builder().setType(MultipartBody.FORM)
            fields.forEach { (name, value) -> if (value != null) builder.addFormDataPart(name, value) }
            files.forEach { file ->
                builder.addFormDataPart("files", file.filename, file.data.toRequestBody(file.contentType.toMediaType()))
            }
            return builder.build()
        }
    }

    private fun url(path: String, query: List<Pair<String, String>> = emptyList()): HttpUrl =
        base.newBuilder().addPathSegments(path).apply { query.forEach { (k, v) -> addQueryParameter(k, v) } }.build()

    private fun paging(cursor: String?, limit: Int?): List<Pair<String, String>> =
        listOfNotNull(cursor?.let { "cursor" to it }, limit?.let { "limit" to it.toString() })

    private suspend inline fun <reified T> request(
        method: String,
        path: String,
        query: List<Pair<String, String>> = emptyList(),
        body: Body? = null,
    ): T = decode(requestRaw(method, path, query, body))

    private suspend fun requestRaw(
        method: String,
        path: String,
        query: List<Pair<String, String>> = emptyList(),
        body: Body? = null,
    ): ByteArray {
        val current = sessionToken()
        return try {
            send(method, path, query, body, current)
        } catch (error: YuvaException) {
            if (error.status != 401) throw error
            if (token == current) dropSession()
            val renewed = sessionToken(fresh = true)
            restartRealtime()
            send(method, path, query, body, renewed)
        }
    }

    private suspend fun send(
        method: String,
        path: String,
        query: List<Pair<String, String>>,
        body: Body?,
        bearer: String?,
    ): ByteArray {
        val request = Request.Builder()
            .url(url(path, query))
            .header("Accept", "application/json")
            .method(method, body?.toRequestBody() ?: if (method == "GET" || method == "DELETE") null else "{}".toRequestBody(JSON_TYPE))
            .apply { if (bearer != null) bearer(bearer) }
            .build()
        return execute(request)
    }

    private suspend fun execute(request: Request): ByteArray {
        val response = http.newCall(request).await()
        return try {
            withContext(Dispatchers.IO) { read(response) }
        } catch (error: CancellationException) {
            response.close()
            throw error
        }
    }

    private fun read(response: Response): ByteArray {
        response.use {
            val bytes = it.body.bytes()
            if (!it.isSuccessful) {
                val problem = runCatching { json.decodeFromString<Problem>(bytes.decodeToString()) }.getOrNull()
                throw YuvaException(it.code, problem?.code ?: "http_${it.code}", problem?.detail ?: problem?.title)
            }
            return bytes
        }
    }

    private inline fun <reified T> decode(bytes: ByteArray): T = json.decodeFromString(bytes.decodeToString())

    private fun Request.Builder.bearer(value: String) = header("Authorization", "Bearer $value")

    // Realtime

    private fun startRealtime() {
        if (realtime?.isActive == true) return
        realtime = scope.launch { runRealtime() }
    }

    private fun stopRealtime() {
        realtime?.cancel()
        realtime = null
    }

    private fun restartRealtime() {
        if (realtime == null) return
        stopRealtime()
        startRealtime()
    }

    private suspend fun runRealtime() {
        val attempt = AtomicInteger(0)
        while (scope.isActive) {
            try {
                val current = sessionToken()
                val query = if (lastEventId > 0) listOf("last_event_id" to lastEventId.toString()) else emptyList()
                val request = Request.Builder()
                    .url(url("client/v1/realtime", query))
                    .header("Sec-WebSocket-Protocol", "yuva, yuva.token.$current")
                    .build()
                val closed = CompletableDeferred<Int>()
                val socket = http.newWebSocket(
                    request,
                    object : WebSocketListener() {
                        override fun onMessage(webSocket: WebSocket, text: String) {
                            if (handle(text)) attempt.set(0)
                        }

                        override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                            webSocket.close(1000, null)
                            closed.complete(code)
                        }

                        override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                            closed.complete(code)
                        }

                        override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                            closed.complete(-1)
                        }
                    },
                )
                val code = try {
                    closed.await()
                } finally {
                    socket.cancel()
                }
                shared.tryEmit(YuvaEvent.ConnectionChanged(false))
                if (code == 1008) {
                    lastEventId = 0
                    dropSession()
                    shared.tryEmit(YuvaEvent.ResyncRequired)
                }
            } catch (error: CancellationException) {
                throw error
            } catch (_: Exception) {
                shared.tryEmit(YuvaEvent.ConnectionChanged(false))
            }
            val wait = min(30.0, 2.0.pow(attempt.getAndIncrement().toDouble())) * Random.nextDouble(0.75, 1.25)
            delay((wait * 1000).toLong())
        }
    }

    private fun handle(text: String): Boolean {
        val message = runCatching { json.parseToJsonElement(text).jsonObject }.getOrNull() ?: return false
        message["id"]?.jsonPrimitive?.longOrNull?.let { lastEventId = maxOf(lastEventId, it) }
        val data = message["data"]
        fun <T> payload(decode: (JsonElement) -> T): T? = data?.let { runCatching { decode(it) }.getOrNull() }
        val event: YuvaEvent? = when (message["type"]?.jsonPrimitive?.contentOrNull) {
            "ready" -> {
                message["last_event_id"]?.jsonPrimitive?.longOrNull?.let { lastEventId = maxOf(lastEventId, it) }
                shared.tryEmit(YuvaEvent.ConnectionChanged(true))
                return true
            }
            "resync_required" -> YuvaEvent.ResyncRequired
            "conversation.created" -> payload { YuvaEvent.ConversationCreated(json.decodeFromJsonElement(it)) }
            "conversation.updated" -> payload {
                val status = json.decodeFromJsonElement<StatusData>(it)
                YuvaEvent.ConversationUpdated(status.id, status.status)
            }
            "message.created" -> payload { YuvaEvent.MessageCreated(json.decodeFromJsonElement(it)) }
            "message.updated" -> payload { YuvaEvent.MessageUpdated(json.decodeFromJsonElement(it)) }
            "read" -> payload {
                val read = json.decodeFromJsonElement<ReadData>(it)
                YuvaEvent.Read(read.conversationId, read.readAt)
            }
            "typing" -> payload {
                val typing = json.decodeFromJsonElement<TypingData>(it)
                YuvaEvent.Typing(typing.conversationId, typing.typing, typing.author)
            }
            "presence" -> payload { YuvaEvent.Presence(json.decodeFromJsonElement(it)) }
            else -> null
        }
        event?.let(shared::tryEmit)
        return false
    }

    @kotlinx.serialization.Serializable
    private data class StatusData(val id: String, val status: YuvaConversationStatus)

    @kotlinx.serialization.Serializable
    private data class ReadData(
        val conversationId: String,
        @kotlinx.serialization.Serializable(InstantSerializer::class) val readAt: Instant,
    )

    @kotlinx.serialization.Serializable
    private data class TypingData(val conversationId: String, val typing: Boolean, val author: YuvaMessageAuthor)

    internal companion object {
        val JSON_TYPE = "application/json".toMediaType()

        @OptIn(ExperimentalSerializationApi::class)
        val json = Json {
            ignoreUnknownKeys = true
            explicitNulls = false
            coerceInputValues = true
            namingStrategy = JsonNamingStrategy.SnakeCase
        }
    }
}

private suspend fun Call.await(): Response = suspendCancellableCoroutine { continuation ->
    enqueue(object : Callback {
        override fun onResponse(call: Call, response: Response) = continuation.resumeWith(Result.success(response))
        override fun onFailure(call: Call, e: IOException) = continuation.resumeWith(Result.failure(e))
    })
    continuation.invokeOnCancellation { cancel() }
}

package dev.yuva

import java.io.IOException
import java.time.Instant
import kotlinx.coroutines.async
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.Interceptor
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import okio.Buffer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

internal class MemoryStore(private var value: StoredSession = StoredSession()) : SessionStorage {
    @Synchronized override fun load() = value
    @Synchronized override fun save(stored: StoredSession) { value = stored }
    @Synchronized override fun clear() { value = StoredSession() }
}

internal data class Recorded(
    val method: String,
    val path: String,
    val bearer: String?,
    val identityToken: String?,
    val consentText: String? = null,
    val pushToken: String? = null,
    val platform: String? = null,
    val appId: String? = null,
)

internal class FakeServer : Interceptor {
    val requests = mutableListOf<Recorded>()
    @Volatile var offline = false
    @Volatile var inboxExtra = ""
    @Volatile var failPush = false
    private var issued = 0
    private val identifiedTokens = mutableSetOf<String>()

    @Synchronized override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        val text = request.body?.let { Buffer().also(it::writeTo).readUtf8() }.orEmpty()
        val fields = runCatching { YuvaClient.json.parseToJsonElement(text).jsonObject }.getOrNull()
        val identity = fields?.get("identity_token")?.jsonPrimitive?.contentOrNull
        requests += Recorded(
            request.method,
            request.url.encodedPath,
            request.header("Authorization")?.removePrefix("Bearer "),
            identity,
            fields?.get("consent_text")?.jsonPrimitive?.contentOrNull,
            fields?.get("token")?.jsonPrimitive?.contentOrNull,
            fields?.get("platform")?.jsonPrimitive?.contentOrNull,
            fields?.get("app_id")?.jsonPrimitive?.contentOrNull,
        )
        if (offline) throw IOException("offline")
        val path = request.url.encodedPath
        val subscription = path.startsWith("/client/v1/subscriptions/")
        val (code, body) = when {
            request.method == "DELETE" && path == "/client/v1/session" -> 204 to ""
            request.method == "POST" && path == "/client/v1/session" -> {
                val token = "t${++issued}"
                if (identity != null) identifiedTokens += token
                201 to session(token, identity != null)
            }
            request.method == "GET" && path == "/client/v1/session" -> 200 to session(null, false)
            request.method == "POST" && path == "/client/v1/push-devices" ->
                if (failPush) {
                    500 to ""
                } else {
                    val token = fields?.get("token")?.jsonPrimitive?.contentOrNull
                    200 to """{"id":"dev-$token","platform":"android","app_id":"dev.test","created_at":"2099-01-01T00:00:00Z","updated_at":"2099-01-01T00:00:00Z"}"""
                }
            request.method == "DELETE" && path.startsWith("/client/v1/push-devices/") -> 204 to ""
            request.method == "GET" && path == "/client/v1/subscriptions" -> 200 to SUBSCRIPTIONS
            request.method == "PUT" && subscription ->
                if (request.header("Authorization")?.removePrefix("Bearer ") in identifiedTokens) {
                    200 to """{"state":"${if (path.endsWith("/double")) "pending" else "subscribed"}"}"""
                } else {
                    403 to """{"status":403,"code":"identity_required","detail":"only signed-in users"}"""
                }
            request.method == "DELETE" && subscription -> 204 to ""
            else -> 404 to ""
        }
        return Response.Builder()
            .request(request)
            .protocol(Protocol.HTTP_1_1)
            .code(code)
            .message("")
            .body(body.toResponseBody(YuvaClient.JSON_TYPE))
            .build()
    }

    private fun session(token: String?, identified: Boolean): String {
        val tokenField = token?.let { "\"token\":\"$it\"," }.orEmpty()
        return """{$tokenField"expires_at":"2099-01-01T00:00:00Z",""" +
            """"contact":{"id":"c1","name":"Ada","identified":$identified},""" +
            """"inbox":{"id":"i1","name":"Acme","branding":{},"default_locale":"en","timezone":"UTC",""" +
            """"mode":"live","open_now":true,"feedback_categories":[]$inboxExtra}}"""
    }
}

private const val SUBSCRIPTIONS = """{"items":[""" +
    """{"list":{"id":"news","name":"News","description":"Monthly","double_opt_in":false},"state":"subscribed"},""" +
    """{"list":{"id":"double","name":"Beta","description":"","double_opt_in":true}}]}"""

internal const val IDENTITY_TOKEN = "e30.eyJzdWIiOiJ1MSJ9.sig"

internal fun client(
    server: FakeServer,
    store: MemoryStore,
    maxAttachmentSize: Long = YuvaConfiguration.DEFAULT_MAX_ATTACHMENT_SIZE,
    identity: (suspend () -> String?)? = null,
) = YuvaClient(
    YuvaConfiguration("https://yuva.test", "pk", identity, maxAttachmentSize),
    OkHttpClient.Builder().addInterceptor(server).build(),
    store,
    null,
    "dev.test",
)

private fun saved(token: String) = StoredSession(token = token, expiresAt = Instant.now().plusSeconds(3600), subject = "u1")

class SignOutTest {
    @Test
    fun revokesTheStoredTokenWhenItWasNeverLoaded() = runBlocking {
        val server = FakeServer()
        val store = MemoryStore(saved("old"))

        client(server, store) { IDENTITY_TOKEN }.signOut()

        assertEquals(listOf(Recorded("DELETE", "/client/v1/session", "old", null)), server.requests)
        assertNull(store.load().token)
        assertEquals(emptyList<String>(), store.load().pendingRevoke)
    }

    @Test
    fun keepsAPendingRevokeWhenOfflineAndRetriesBeforeTheNextSession() = runBlocking {
        val server = FakeServer()
        val store = MemoryStore(saved("old"))
        server.offline = true

        client(server, store) { IDENTITY_TOKEN }.signOut()

        assertNull(store.load().token)
        assertEquals(listOf("old"), store.load().pendingRevoke)

        server.offline = false
        val before = server.requests.size
        client(server, store).start()

        val after = server.requests.drop(before)
        assertEquals(listOf("DELETE", "POST"), after.map { it.method })
        assertEquals("old", after.first().bearer)
        assertNull(after.last().identityToken)
        assertEquals(emptyList<String>(), store.load().pendingRevoke)
        assertEquals("t1", store.load().token)
    }

    @Test
    fun aStartRacingSignOutLeavesNoIdentifiedSession() = runBlocking {
        val server = FakeServer()
        val store = MemoryStore()
        val yuva = client(server, store) {
            delay(200)
            IDENTITY_TOKEN
        }

        val starting = async { runCatching { yuva.start() } }
        delay(50)
        yuva.signOut()
        starting.await()

        val identified = server.requests.filter { it.method == "POST" && it.identityToken != null }
        assertEquals(1, identified.size)
        assertTrue(server.requests.any { it.method == "DELETE" && it.bearer == "t1" })
        assertNull(store.load().token)
        assertEquals(emptyList<String>(), store.load().pendingRevoke)

        yuva.start()
        assertNull(server.requests.last { it.method == "POST" }.identityToken)
        assertEquals(1, server.requests.count { it.method == "POST" && it.identityToken != null })
    }

    @Test
    fun refusesAnAttachmentLargerThanTheLimitBeforeUploading() = runBlocking {
        val server = FakeServer()
        val yuva = client(server, MemoryStore(), maxAttachmentSize = 10)
        try {
            yuva.sendMessage("c1", "", listOf(YuvaUpload("big.jpg", "image/jpeg", ByteArray(11))))
            fail("expected attachment_too_large")
        } catch (error: YuvaException) {
            assertEquals("attachment_too_large", error.code)
        }
        assertTrue(server.requests.isEmpty())
    }
}

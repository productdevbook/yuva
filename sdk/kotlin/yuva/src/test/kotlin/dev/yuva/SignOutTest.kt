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

private class MemoryStore(private var value: StoredSession = StoredSession()) : SessionStorage {
    @Synchronized override fun load() = value
    @Synchronized override fun save(stored: StoredSession) { value = stored }
    @Synchronized override fun clear() { value = StoredSession() }
}

private data class Recorded(val method: String, val path: String, val bearer: String?, val identityToken: String?)

private class FakeServer : Interceptor {
    val requests = mutableListOf<Recorded>()
    @Volatile var offline = false
    private var issued = 0

    @Synchronized override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        val text = request.body?.let { Buffer().also(it::writeTo).readUtf8() }.orEmpty()
        val identity = runCatching {
            YuvaClient.json.parseToJsonElement(text).jsonObject["identity_token"]?.jsonPrimitive?.contentOrNull
        }.getOrNull()
        requests += Recorded(
            request.method,
            request.url.encodedPath,
            request.header("Authorization")?.removePrefix("Bearer "),
            identity,
        )
        if (offline) throw IOException("offline")
        val (code, body) = when (request.method to request.url.encodedPath) {
            "DELETE" to "/client/v1/session" -> 204 to ""
            "POST" to "/client/v1/session" -> 201 to session("t${++issued}", identity != null)
            "GET" to "/client/v1/session" -> 200 to session(null, false)
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
            """"mode":"live","open_now":true,"feedback_categories":[]}}"""
    }
}

private const val IDENTITY_TOKEN = "e30.eyJzdWIiOiJ1MSJ9.sig"

private fun client(
    server: FakeServer,
    store: MemoryStore,
    maxAttachmentSize: Long = YuvaConfiguration.DEFAULT_MAX_ATTACHMENT_SIZE,
    identity: (suspend () -> String?)? = null,
) = YuvaClient(
    YuvaConfiguration("https://yuva.test", "pk", identity, maxAttachmentSize),
    OkHttpClient.Builder().addInterceptor(server).build(),
    store,
    null,
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

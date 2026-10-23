package dev.yuva

import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.async
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.decodeFromString
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class RedactedMessageTest {
    private val message = """{"id":"m1","conversation_id":"c1","direction":"out","author":{"type":"member","name":"Ada"},"body":"","attachments":[],"redacted":true,"created_at":"2026-01-01T10:00:00Z"}"""

    @Test
    fun aRedactedFrameBecomesAnEventWithTheRedactedMessage() = runBlocking {
        val client = client(FakeServer(), MemoryStore())
        val received = async(start = CoroutineStart.UNDISPATCHED) {
            withTimeout(5000) { client.events().first { it is YuvaEvent.MessageRedacted } }
        }
        client.handle("""{"type":"message.redacted","id":9,"conversation_id":"c1","data":$message}""")
        val event = received.await() as YuvaEvent.MessageRedacted
        assertEquals("m1", event.message.id)
        assertTrue(event.message.redacted)
        assertEquals("", event.message.body)
    }

    @Test
    fun redactedDefaultsToFalseWhenTheServerOmitsIt() {
        val decoded = YuvaClient.json.decodeFromString<YuvaMessage>(message.replace(""""redacted":true,""", ""))
        assertFalse(decoded.redacted)
    }

    @Test
    fun aPreviewCarriesTheRedactedFlag() {
        val preview = YuvaClient.json.decodeFromString<YuvaMessagePreview>(
            """{"id":"m1","author_type":"member","text":"","redacted":true,"created_at":"2026-01-01T10:00:00Z"}""",
        )
        assertTrue(preview.redacted)
    }
}

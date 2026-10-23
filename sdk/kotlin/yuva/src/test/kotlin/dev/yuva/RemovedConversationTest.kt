package dev.yuva

import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.async
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.Assert.assertEquals
import org.junit.Test

class RemovedConversationTest {
    private fun removed(frame: String): String? = runBlocking {
        val client = client(FakeServer(), MemoryStore())
        val received = async(start = CoroutineStart.UNDISPATCHED) {
            withTimeout(5000) { client.events().first { it is YuvaEvent.ConversationRemoved } }
        }
        client.handle(frame)
        (received.await() as YuvaEvent.ConversationRemoved).id
    }

    @Test
    fun theRemovedFrameBecomesAnEventWithTheConversationId() {
        assertEquals("c1", removed("""{"type":"conversation.removed","id":7,"conversation_id":"c1","data":{"id":"c1"}}"""))
    }

    @Test
    fun theEnvelopeConversationIdIsUsedWhenTheDataLacksOne() {
        assertEquals("c2", removed("""{"type":"conversation.removed","conversation_id":"c2","data":{}}"""))
    }
}

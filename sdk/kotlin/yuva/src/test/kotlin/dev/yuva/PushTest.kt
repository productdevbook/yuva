package dev.yuva

import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Test

private suspend fun waitFor(condition: () -> Boolean) {
    repeat(200) { if (!condition()) delay(25) }
}

class PushTest {
    @Test
    fun registersTheTokenForTheSessionAndStoresTheDevice() = runBlocking {
        val server = FakeServer()
        val store = MemoryStore()
        val client = client(server, store)

        client.setPushToken("ab01ff")
        waitFor { store.load().pushDeviceId != null }

        val post = synchronized(server) { server.requests.last { it.path == "/client/v1/push-devices" } }
        assertEquals("POST", post.method)
        assertEquals("ab01ff", post.pushToken)
        assertEquals("android", post.platform)
        assertEquals("dev.test", post.appId)
        assertEquals("t1", post.bearer)
        assertEquals("dev-ab01ff", store.load().pushDeviceId)
    }

    @Test
    fun aChangedTokenIsRegisteredAgain() = runBlocking {
        val server = FakeServer()
        val store = MemoryStore()
        val client = client(server, store)

        client.setPushToken("one")
        waitFor { store.load().pushDeviceId == "dev-one" }
        client.setPushToken("two")
        waitFor { store.load().pushDeviceId == "dev-two" }

        val tokens = synchronized(server) { server.requests.filter { it.path == "/client/v1/push-devices" }.map { it.pushToken } }
        assertEquals(listOf("one", "two"), tokens)
    }

    @Test
    fun signOutDeletesTheDeviceAndStaysQuietUntilANewToken() = runBlocking {
        val server = FakeServer()
        val store = MemoryStore()
        val client = client(server, store)
        client.setPushToken("one")
        waitFor { store.load().pushDeviceId != null }

        client.signOut()

        val delete = synchronized(server) { server.requests.last { it.path == "/client/v1/push-devices/dev-one" } }
        assertEquals("DELETE", delete.method)
        assertEquals("t1", delete.bearer)
        assertNull(store.load().pushDeviceId)
        val before = synchronized(server) { server.requests.size }
        client.start()
        delay(100)
        assertFalse(synchronized(server) { server.requests.drop(before).any { it.path == "/client/v1/push-devices" } })
    }

    @Test
    fun retriesAfterAServerFailureWithoutThrowing() = runBlocking {
        val server = FakeServer()
        val store = MemoryStore()
        val client = client(server, store)
        client.start()
        server.failPush = true

        client.setPushToken("one")
        waitFor { synchronized(server) { server.requests.any { it.path == "/client/v1/push-devices" } } }
        server.failPush = false
        waitFor { store.load().pushDeviceId != null }

        assertEquals("dev-one", store.load().pushDeviceId)
    }
}

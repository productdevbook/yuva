package dev.yuva

import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

class AttachmentLimitTest {
    private fun limits(json: String) = YuvaClient.json.decodeFromString<YuvaAttachmentLimits>(json)

    private fun upload(type: String, bytes: Int) = YuvaUpload("f", type, ByteArray(bytes))

    private fun inbox(extra: String) = YuvaClient.json.decodeFromString<YuvaInbox>(
        """{"id":"i1","name":"Acme","branding":{},"default_locale":"en","timezone":"UTC","mode":"live",""" +
            """"open_now":true,"feedback_categories":[]$extra}""",
    )

    @Test
    fun decodesTheEffectiveLimitsOfTheInbox() {
        val inbox = inbox(""","attachments":{"max_bytes":1048576,"types":["image/*","application/pdf"]}""")
        assertEquals(YuvaAttachmentLimits(1_048_576, listOf("image/*", "application/pdf")), inbox.attachments)
    }

    @Test
    fun anInboxWithoutLimitsOrWithOddOnesStillDecodes() {
        assertNull(inbox("").attachments)
        assertNull(inbox(""","attachments":"x"""").attachments?.refusal(upload("image/png", 5)))
    }

    @Test
    fun matchesExactTypesAndWildcardsCaseInsensitively() {
        val limits = limits("""{"max_bytes":100,"types":["image/*","Application/PDF"]}""")
        assertTrue(limits.allows("image/png"))
        assertTrue(limits.allows("IMAGE/JPEG; charset=binary"))
        assertTrue(limits.allows("application/pdf"))
        assertFalse(limits.allows("application/zip"))
        assertFalse(limits.allows("imagery/png"))
    }

    @Test
    fun refusesATypeBeforeTheSize() {
        val limits = limits("""{"max_bytes":10,"types":["image/png"]}""")
        assertEquals(YuvaAttachmentRefusal.TypeNotAllowed, limits.refusal(upload("image/jpeg", 50)))
        assertEquals(YuvaAttachmentRefusal.TooLarge(10), limits.refusal(upload("image/png", 11)))
        assertNull(limits.refusal(upload("image/png", 10)))
    }

    @Test
    fun noTypesMeansNoFilesAndNoPhotoPicker() {
        val limits = limits("""{"max_bytes":100,"types":[]}""")
        assertFalse(limits.acceptsFiles)
        assertFalse(limits.allowsImages)
        assertEquals(YuvaAttachmentRefusal.TypeNotAllowed, limits.refusal(upload("image/jpeg", 1)))
    }

    @Test
    fun theClientRefusesBeforeUploadingAndKeepsTheStatusCodes() = runBlocking {
        val server = FakeServer().apply { inboxExtra = ""","attachments":{"max_bytes":10,"types":["image/png"]}""" }
        val yuva = client(server, MemoryStore())
        yuva.start()
        val before = server.requests.size

        for ((file, status) in listOf(upload("image/png", 11) to 413, upload("image/jpeg", 1) to 415)) {
            try {
                yuva.sendMessage("c1", "", listOf(file))
                fail("expected $status")
            } catch (error: YuvaException) {
                assertEquals(status, error.status)
            }
        }
        assertEquals(before, server.requests.size)
    }
}

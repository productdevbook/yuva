package dev.yuva

import kotlinx.serialization.decodeFromString
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class FeedbackCategoryTest {
    private fun info(json: String) = YuvaClient.json.decodeFromString<YuvaFeedbackInfo>(json)

    @Test
    fun aCustomKeyKeepsItsNameAndMapsToOtherInTheEnum() {
        val feedback = info("""{"category":"other","category_key":"ux","category_name":"Tasarım"}""")

        assertEquals("ux", feedback.categoryKey)
        assertEquals("Tasarım", feedback.categoryName)
        assertEquals(YuvaFeedbackCategory.OTHER, feedback.category)
    }

    @Test
    fun aBuiltInKeyMapsToItsEntry() {
        val feedback = info("""{"category":"bug","category_key":"bug","category_name":"Hata"}""")

        assertEquals(YuvaFeedbackCategory.BUG, feedback.category)
    }

    @Test
    fun anUnknownKeyAndAMissingKeyNeverFailDecoding() {
        val unknown = info("""{"category":"galactic"}""")
        assertEquals("galactic", unknown.categoryKey)
        assertEquals(YuvaFeedbackCategory.OTHER, unknown.category)
        assertNull(unknown.categoryName)

        assertEquals("other", info("{}").categoryKey)
    }

    @Test
    fun theInboxListsItsCategoryItems() {
        val json = """{"id":"i1","name":"Acme","branding":{},"default_locale":"en","timezone":"UTC","mode":"live","open_now":true,
            "feedback_categories":["bug","other"],
            "feedback_category_items":[{"key":"bug","name":"Hata","icon":"bug"},{"key":"ux","name":"Tasarım","icon":"palette"}]}"""
        val inbox = YuvaClient.json.decodeFromString<YuvaInbox>(json)

        assertEquals(listOf("bug", "ux"), inbox.feedbackCategoryItems.map { it.key })
        assertEquals("palette", inbox.feedbackCategoryItems[1].icon)
        assertEquals(listOf(YuvaFeedbackCategory.BUG, YuvaFeedbackCategory.OTHER), inbox.feedbackCategories)
    }

    @Test
    fun aFeedbackPostsTheRealKey() {
        assertEquals("ux", YuvaFeedback(categoryKey = "ux", body = "x").categoryKey)
        assertEquals("idea", YuvaFeedback(category = YuvaFeedbackCategory.IDEA, body = "x").categoryKey)
    }
}

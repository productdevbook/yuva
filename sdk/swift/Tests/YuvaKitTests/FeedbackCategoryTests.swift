import Foundation
import Testing
@testable import YuvaKit

@Suite struct FeedbackCategoryTests {
    private func info(_ json: String) throws -> YuvaFeedbackInfo {
        try YuvaJSON.decoder.decode(YuvaFeedbackInfo.self, from: Data(json.utf8))
    }

    @Test func aCustomKeyKeepsItsNameAndMapsToOtherInTheEnum() throws {
        let feedback = try info(#"{"category":"other","category_key":"ux","category_name":"Tasarım","allow_email":false}"#)
        #expect(feedback.categoryKey == "ux")
        #expect(feedback.categoryName == "Tasarım")
        #expect(feedback.category == .other)
    }

    @Test func aBuiltInKeyMapsToItsCase() throws {
        let feedback = try info(#"{"category":"bug","category_key":"bug","category_name":"Hata","allow_email":true}"#)
        #expect(feedback.category == .bug)
    }

    @Test func anUnknownKeyAndAMissingKeyNeverFailDecoding() throws {
        let unknown = try info(#"{"category":"galactic","allow_email":false}"#)
        #expect(unknown.categoryKey == "galactic")
        #expect(unknown.category == .other)
        #expect(unknown.categoryName == nil)
        let legacy = try info(#"{"allow_email":false}"#)
        #expect(legacy.categoryKey == "other")
    }

    @Test func theInboxListsItsCategoryItems() throws {
        let json = """
            {"id":"i1","name":"Acme","branding":{},"default_locale":"en","timezone":"UTC","mode":"live","open_now":true,\
            "feedback_categories":["bug","other"],\
            "feedback_category_items":[{"key":"bug","name":"Hata","icon":"bug"},{"key":"ux","name":"Tasarım","icon":"palette"}]}
            """
        let inbox = try YuvaJSON.decoder.decode(YuvaInbox.self, from: Data(json.utf8))
        #expect(inbox.feedbackCategoryItems.map(\.key) == ["bug", "ux"])
        #expect(inbox.feedbackCategoryItems[1].icon == "palette")
        #expect(inbox.feedbackCategories == [.bug, .other])
    }

    @Test func aFeedbackPostsTheRealKey() {
        #expect(YuvaFeedback(categoryKey: "ux", body: "x").categoryKey == "ux")
        #expect(YuvaFeedback(category: .idea, body: "x").categoryKey == "idea")
    }
}

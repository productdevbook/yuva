import Foundation
import Testing
@testable import YuvaKit

@Suite struct AttachmentLimitTests {
    private func limits(_ json: String) throws -> YuvaAttachmentLimits {
        try YuvaJSON.decoder.decode(YuvaAttachmentLimits.self, from: Data(json.utf8))
    }

    private func upload(_ type: String, bytes: Int) -> YuvaUpload {
        YuvaUpload(filename: "f", contentType: type, data: Data(count: bytes))
    }

    @Test func decodesTheEffectiveLimitsOfTheInbox() throws {
        let json = """
            {"id":"i1","name":"Acme","branding":{},"default_locale":"en","timezone":"UTC","mode":"live",\
            "open_now":true,"feedback_categories":[],"attachments":{"max_bytes":1048576,"types":["image/*","application/pdf"]}}
            """
        let inbox = try YuvaJSON.decoder.decode(YuvaInbox.self, from: Data(json.utf8))
        #expect(inbox.attachments == YuvaAttachmentLimits(maxBytes: 1_048_576, types: ["image/*", "application/pdf"]))
    }

    @Test func anInboxWithoutLimitsOrWithOddOnesStillDecodes() throws {
        let base = """
            {"id":"i1","name":"Acme","branding":{},"default_locale":"en","timezone":"UTC","mode":"live",\
            "open_now":true,"feedback_categories":[]
            """
        let without = try YuvaJSON.decoder.decode(YuvaInbox.self, from: Data((base + "}").utf8))
        #expect(without.attachments == nil)
        let odd = try YuvaJSON.decoder.decode(YuvaInbox.self, from: Data((base + #","attachments":"x"}"#).utf8))
        #expect(odd.attachments?.refusal(for: upload("image/png", bytes: 5)) == nil)
    }

    @Test func matchesExactTypesAndWildcardsCaseInsensitively() throws {
        let limits = try limits(#"{"max_bytes":100,"types":["image/*","Application/PDF"]}"#)
        #expect(limits.allows(contentType: "image/png"))
        #expect(limits.allows(contentType: "IMAGE/JPEG; charset=binary"))
        #expect(limits.allows(contentType: "application/pdf"))
        #expect(!limits.allows(contentType: "application/zip"))
        #expect(!limits.allows(contentType: "imagery/png"))
    }

    @Test func refusesATypeBeforeTheSize() throws {
        let limits = try limits(#"{"max_bytes":10,"types":["image/png"]}"#)
        #expect(limits.refusal(for: upload("image/jpeg", bytes: 50)) == .typeNotAllowed)
        #expect(limits.refusal(for: upload("image/png", bytes: 11)) == .tooLarge(maxBytes: 10))
        #expect(limits.refusal(for: upload("image/png", bytes: 10)) == nil)
    }

    @Test func noTypesMeansNoFilesAndNoPhotoPicker() throws {
        let limits = try limits(#"{"max_bytes":100,"types":[]}"#)
        #expect(!limits.acceptsFiles)
        #expect(!limits.allowsImages)
        #expect(limits.refusal(for: upload("image/jpeg", bytes: 1)) == .typeNotAllowed)
    }

    @Test func theClientRefusesBeforeUploadingAndKeepsTheStatusCodes() async throws {
        let server = FakeServer()
        server.inboxExtra = #","attachments":{"max_bytes":10,"types":["image/png"]}"#
        let client = makeClient(server, store: MemoryStore())
        try await client.start()
        let before = server.requests.count

        await #expect {
            try await client.sendMessage(conversationId: "c1", body: "", attachments: [upload("image/png", bytes: 11)])
        } throws: { ($0 as? YuvaError)?.status == 413 }
        await #expect {
            try await client.sendMessage(conversationId: "c1", body: "", attachments: [upload("image/jpeg", bytes: 1)])
        } throws: { ($0 as? YuvaError)?.status == 415 }
        #expect(server.requests.count == before)
    }
}

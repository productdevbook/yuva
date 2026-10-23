import Foundation
import Testing
@testable import YuvaKit

@Suite struct RedactedMessageTests {
    private static let message = #"{"id":"m1","conversation_id":"c1","direction":"out","author":{"type":"member","name":"Ada"},"body":"","attachments":[],"redacted":true,"created_at":"2026-01-01T10:00:00Z"}"#

    @Test func aRedactedFrameBecomesAnEventWithTheRedactedMessage() async throws {
        let client = makeClient(FakeServer(), store: MemoryStore())
        let stream = client.events()
        try await Task.sleep(for: .milliseconds(100))

        _ = await client.handle(Data(#"{"type":"message.redacted","id":9,"conversation_id":"c1","data":\#(Self.message)}"#.utf8))

        for await event in stream {
            if case .messageRedacted(let message) = event {
                #expect(message.id == "m1")
                #expect(message.redacted)
                #expect(message.body.isEmpty)
                return
            }
        }
        Issue.record("expected messageRedacted")
    }

    @Test func redactedDefaultsToFalseWhenTheServerOmitsIt() throws {
        let json = Self.message.replacingOccurrences(of: #""redacted":true,"#, with: "")
        let message = try YuvaJSON.decoder.decode(YuvaMessage.self, from: Data(json.utf8))
        #expect(!message.redacted)
    }

    @Test func aPreviewCarriesTheRedactedFlag() throws {
        let json = #"{"id":"m1","author_type":"member","text":"","redacted":true,"created_at":"2026-01-01T10:00:00Z"}"#
        let preview = try YuvaJSON.decoder.decode(YuvaMessagePreview.self, from: Data(json.utf8))
        #expect(preview.redacted)
    }

    @Test @MainActor func anOpenThreadReplacesTheMessageInPlace() async throws {
        let client = makeClient(FakeServer(), store: MemoryStore())
        let model = ThreadModel(client: client, conversationId: "c1")
        let original = Self.message.replacingOccurrences(of: #""body":"","#, with: #""body":"secret","#)
            .replacingOccurrences(of: #""redacted":true,"#, with: "")
        let listening = Task { await model.listen() }
        try await Task.sleep(for: .milliseconds(100))
        _ = await client.handle(Data(#"{"type":"message.created","conversation_id":"c1","data":\#(original)}"#.utf8))
        try await Task.sleep(for: .milliseconds(100))
        #expect(model.items.first?.body == "secret")

        _ = await client.handle(Data(#"{"type":"message.redacted","conversation_id":"c1","data":\#(Self.message)}"#.utf8))
        try await Task.sleep(for: .milliseconds(100))
        listening.cancel()

        #expect(model.items.count == 1)
        #expect(model.items.first?.message?.redacted == true)
        #expect(model.items.first?.body == "")
    }
}

import Foundation
import Testing
@testable import YuvaKit

@Suite struct RemovedConversationTests {
    private func client() -> YuvaClient {
        makeClient(FakeServer(), store: MemoryStore())
    }

    private func next(_ stream: AsyncStream<YuvaEvent>) async -> YuvaEvent? {
        for await event in stream {
            if case .conversationRemoved = event { return event }
        }
        return nil
    }

    @Test func theRemovedFrameBecomesAnEventWithTheConversationId() async throws {
        let client = client()
        let stream = client.events()
        try await Task.sleep(for: .milliseconds(100))

        _ = await client.handle(Data(#"{"type":"conversation.removed","id":7,"conversation_id":"c1","data":{"id":"c1"}}"#.utf8))

        guard case .conversationRemoved(let id) = await next(stream) else {
            Issue.record("expected conversationRemoved")
            return
        }
        #expect(id == "c1")
    }

    @Test func theEnvelopeConversationIdIsUsedWhenTheDataLacksOne() async throws {
        let client = client()
        let stream = client.events()
        try await Task.sleep(for: .milliseconds(100))

        _ = await client.handle(Data(#"{"type":"conversation.removed","conversation_id":"c2","data":{}}"#.utf8))

        guard case .conversationRemoved(let id) = await next(stream) else {
            Issue.record("expected conversationRemoved")
            return
        }
        #expect(id == "c2")
    }

    @Test @MainActor func anOpenThreadStartsOverAndKeepsWhatWasNotSentYet() async throws {
        let client = client()
        let model = ThreadModel(client: client, conversationId: "c1")
        model.items = [ThreadItem(id: "local", body: "hi", uploads: [], state: .failed, createdAt: Date())]
        let listening = Task { await model.listen() }
        try await Task.sleep(for: .milliseconds(100))

        _ = await client.handle(Data(#"{"type":"conversation.removed","data":{"id":"other"}}"#.utf8))
        try await Task.sleep(for: .milliseconds(100))
        #expect(model.conversationId == "c1")
        #expect(!model.removed)

        _ = await client.handle(Data(#"{"type":"conversation.removed","data":{"id":"c1"}}"#.utf8))
        try await Task.sleep(for: .milliseconds(100))
        listening.cancel()

        #expect(model.conversationId == nil)
        #expect(model.removed)
        #expect(model.items.map(\.id) == ["local"])
    }
}

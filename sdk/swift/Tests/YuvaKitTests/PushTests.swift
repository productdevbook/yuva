import Foundation
import Testing
@testable import YuvaKit

private func waitFor(_ condition: @Sendable () -> Bool) async {
    for _ in 0..<200 where !condition() { try? await Task.sleep(for: .milliseconds(25)) }
}

@Suite struct PushTests {
    @Test func registersTheTokenForTheSessionAndStoresTheDevice() async {
        let server = FakeServer()
        let store = MemoryStore()
        let client = makeClient(server, store: store)

        await client.setPushToken(Data([0xAB, 0x01, 0xFF]))
        await waitFor { store.load().pushDeviceId != nil }

        let post = server.requests.last { $0.path == "/client/v1/push-devices" }
        #expect(post?.method == "POST")
        #expect(post?.body["token"] == "ab01ff")
        #expect(post?.body["platform"] == "ios")
        #expect(post?.body["app_id"]?.isEmpty == false)
        #expect(post?.bearer == "t1")
        #expect(store.load().pushDeviceId == "dev-ab01ff")
    }

    @Test func aChangedTokenIsRegisteredAgain() async {
        let server = FakeServer()
        let store = MemoryStore()
        let client = makeClient(server, store: store)

        await client.setPushToken("one")
        await waitFor { store.load().pushDeviceId == "dev-one" }
        await client.setPushToken("two")
        await waitFor { store.load().pushDeviceId == "dev-two" }

        let tokens = server.requests.filter { $0.path == "/client/v1/push-devices" }.compactMap { $0.body["token"] }
        #expect(tokens == ["one", "two"])
    }

    @Test func signOutDeletesTheDeviceAndStaysQuietUntilANewToken() async {
        let server = FakeServer()
        let store = MemoryStore()
        let client = makeClient(server, store: store)
        await client.setPushToken("one")
        await waitFor { store.load().pushDeviceId != nil }

        await client.signOut()

        let delete = server.requests.last { $0.path == "/client/v1/push-devices/dev-one" }
        #expect(delete?.method == "DELETE")
        #expect(delete?.bearer == "t1")
        #expect(store.load().pushDeviceId == nil)
        let before = server.requests.count
        _ = try? await client.start()
        try? await Task.sleep(for: .milliseconds(100))
        #expect(!server.requests.dropFirst(before).contains { $0.path == "/client/v1/push-devices" })
    }

    @Test func retriesAfterAServerFailureWithoutThrowing() async {
        let server = FakeServer()
        let store = MemoryStore()
        let client = makeClient(server, store: store)
        _ = try? await client.start()
        server.failPush = true

        await client.setPushToken("one")
        await waitFor { server.requests.contains { $0.path == "/client/v1/push-devices" } }
        server.failPush = false
        await waitFor { store.load().pushDeviceId != nil }

        #expect(store.load().pushDeviceId == "dev-one")
    }
}

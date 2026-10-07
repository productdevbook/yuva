import Foundation
import Testing
@testable import YuvaKit

final class MemoryStore: SessionStorage, @unchecked Sendable {
    private let lock = NSLock()
    private var value: StoredSession

    init(_ value: StoredSession = StoredSession()) {
        self.value = value
    }

    func load() -> StoredSession { lock.withLock { value } }
    func save(_ stored: StoredSession) { lock.withLock { value = stored } }
    func clear() { lock.withLock { value = StoredSession() } }
}

struct Recorded: Sendable {
    let method: String
    let path: String
    let bearer: String?
    let body: [String: String]
}

final class FakeServer: @unchecked Sendable {
    private let lock = NSLock()
    private var log: [Recorded] = []
    private var issued = 0
    private var offline = false

    var requests: [Recorded] { lock.withLock { log } }

    func setOffline(_ value: Bool) { lock.withLock { offline = value } }

    func respond(_ request: Recorded) throws -> (Int, Data) {
        try lock.withLock {
            log.append(request)
            if offline { throw URLError(.notConnectedToInternet) }
            switch (request.method, request.path) {
            case ("DELETE", "/client/v1/session"):
                return (204, Data())
            case ("POST", "/client/v1/session"):
                issued += 1
                return (201, Data(session(token: "t\(issued)", identified: request.body["identity_token"] != nil).utf8))
            case ("GET", "/client/v1/session"):
                return (200, Data(session(token: nil, identified: false).utf8))
            default:
                return (404, Data())
            }
        }
    }

    private func session(token: String?, identified: Bool) -> String {
        let tokenField = token.map { "\"token\":\"\($0)\"," } ?? ""
        return """
            {\(tokenField)"expires_at":"2099-01-01T00:00:00Z",\
            "contact":{"id":"c1","name":"Ada","identified":\(identified)},\
            "inbox":{"id":"i1","name":"Acme","branding":{},"default_locale":"en","timezone":"UTC",\
            "mode":"live","open_now":true,"feedback_categories":[]}}
            """
    }
}

final class StubProtocol: URLProtocol, @unchecked Sendable {
    private static let lock = NSLock()
    nonisolated(unsafe) private static var servers: [String: FakeServer] = [:]

    static func session(for server: FakeServer) -> URLSession {
        let id = UUID().uuidString
        lock.withLock { servers[id] = server }
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [StubProtocol.self]
        configuration.httpAdditionalHeaders = ["X-Stub": id]
        return URLSession(configuration: configuration)
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func stopLoading() {}

    override func startLoading() {
        let id = request.value(forHTTPHeaderField: "X-Stub") ?? ""
        guard let server = Self.lock.withLock({ Self.servers[id] }), let url = request.url else {
            client?.urlProtocol(self, didFailWithError: URLError(.badServerResponse))
            return
        }
        let recorded = Recorded(
            method: request.httpMethod ?? "GET", path: url.path,
            bearer: request.value(forHTTPHeaderField: "Authorization")?.replacingOccurrences(of: "Bearer ", with: ""),
            body: Self.fields(request))
        do {
            let (status, data) = try server.respond(recorded)
            let response = HTTPURLResponse(url: url, statusCode: status, httpVersion: nil, headerFields: nil)!
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    private static func fields(_ request: URLRequest) -> [String: String] {
        var data = request.httpBody ?? Data()
        if data.isEmpty, let stream = request.httpBodyStream {
            stream.open()
            defer { stream.close() }
            var buffer = [UInt8](repeating: 0, count: 4096)
            while stream.hasBytesAvailable {
                let count = stream.read(&buffer, maxLength: buffer.count)
                if count <= 0 { break }
                data.append(buffer, count: count)
            }
        }
        return (try? JSONSerialization.jsonObject(with: data) as? [String: String]) ?? [:]
    }
}

private let identityToken: String = {
    let payload = Data(#"{"sub":"u1"}"#.utf8).base64EncodedString()
        .replacingOccurrences(of: "=", with: "").replacingOccurrences(of: "+", with: "-")
        .replacingOccurrences(of: "/", with: "_")
    return "e30.\(payload).sig"
}()

private func makeClient(
    _ server: FakeServer, store: MemoryStore, identity: (@Sendable () async throws -> String?)? = nil,
    maxAttachmentSize: Int = YuvaConfiguration.defaultMaxAttachmentSize
) -> YuvaClient {
    let configuration = YuvaConfiguration(
        serverURL: URL(string: "https://yuva.test")!, channelKey: "pk", identityToken: identity,
        maxAttachmentSize: maxAttachmentSize)
    return YuvaClient(configuration: configuration, urlSession: StubProtocol.session(for: server), store: store)
}

private func savedSession(_ token: String) -> StoredSession {
    StoredSession(token: token, expiresAt: Date().addingTimeInterval(3600), subject: "u1")
}

@Suite struct SignOutTests {
    @Test func revokesTheStoredTokenWhenItWasNeverLoaded() async {
        let server = FakeServer()
        let store = MemoryStore(savedSession("old"))
        let client = makeClient(server, store: store, identity: { identityToken })

        await client.signOut()

        #expect(server.requests.map(\.method) == ["DELETE"])
        #expect(server.requests.first?.bearer == "old")
        #expect(store.load().token == nil)
        #expect(store.load().pendingRevoke == nil)
        #expect(await client.session == nil)
    }

    @Test func keepsAPendingRevokeWhenOfflineAndRetriesBeforeTheNextSession() async throws {
        let server = FakeServer()
        let store = MemoryStore(savedSession("old"))
        server.setOffline(true)

        await makeClient(server, store: store, identity: { identityToken }).signOut()

        #expect(store.load().token == nil)
        #expect(store.load().pendingRevoke == ["old"])

        server.setOffline(false)
        let before = server.requests.count
        try await makeClient(server, store: store).start()

        let after = Array(server.requests.dropFirst(before))
        #expect(after.map(\.method) == ["DELETE", "POST"])
        #expect(after.first?.bearer == "old")
        #expect(after.last?.body["identity_token"] == nil)
        #expect(store.load().pendingRevoke == nil)
        #expect(store.load().token == "t1")
    }

    @Test func aStartRacingSignOutLeavesNoIdentifiedSession() async throws {
        let server = FakeServer()
        let store = MemoryStore()
        let client = makeClient(server, store: store) {
            try await Task.sleep(for: .milliseconds(200))
            return identityToken
        }

        let starting = Task { try? await client.start() }
        try await Task.sleep(for: .milliseconds(50))
        await client.signOut()
        _ = await starting.value

        let identified = server.requests.filter { $0.method == "POST" && $0.body["identity_token"] != nil }
        #expect(identified.count == 1)
        #expect(server.requests.contains { $0.method == "DELETE" && $0.bearer == "t1" })
        #expect(store.load().token == nil)
        #expect(store.load().pendingRevoke == nil)

        try await client.start()
        let last = server.requests.last { $0.method == "POST" }
        #expect(last?.body["identity_token"] == nil)
        #expect(server.requests.filter { $0.method == "POST" && $0.body["identity_token"] != nil }.count == 1)
    }

    @Test func refusesAnAttachmentLargerThanTheLimitBeforeUploading() async {
        let server = FakeServer()
        let client = makeClient(server, store: MemoryStore(), maxAttachmentSize: 10)
        let upload = YuvaUpload(filename: "big.jpg", contentType: "image/jpeg", data: Data(count: 11))

        await #expect {
            try await client.sendMessage(conversationId: "c1", body: "", attachments: [upload])
        } throws: { error in
            (error as? YuvaError)?.code == "attachment_too_large"
        }
        #expect(server.requests.isEmpty)
    }
}

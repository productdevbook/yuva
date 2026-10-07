import Foundation

public struct YuvaConfiguration: Sendable {
    public var serverURL: URL
    public var channelKey: String
    public var identityToken: (@Sendable () async throws -> String?)?

    public init(serverURL: URL, channelKey: String, identityToken: (@Sendable () async throws -> String?)? = nil) {
        self.serverURL = serverURL
        self.channelKey = channelKey
        self.identityToken = identityToken
    }
}

public struct YuvaError: Error, Sendable, LocalizedError {
    public let status: Int
    public let code: String
    public let detail: String?

    public var errorDescription: String? { detail ?? code }
}

public enum YuvaOrder: String, Sendable {
    case asc, desc
}

public actor YuvaClient {
    public nonisolated let configuration: YuvaConfiguration
    private let urlSession: URLSession
    private let store: SessionStore
    private var token: String?
    private var starting: Task<String, Error>?
    private var subscribers: [UUID: AsyncStream<YuvaEvent>.Continuation] = [:]
    private var realtimeTask: Task<Void, Never>?
    private var lastEventId: Int64 = 0
    public private(set) var session: YuvaSession?

    public init(configuration: YuvaConfiguration, urlSession: URLSession = .shared) {
        self.configuration = configuration
        self.urlSession = urlSession
        store = SessionStore(channelKey: configuration.channelKey)
    }

    @discardableResult
    public func start() async throws -> YuvaSession {
        _ = try await sessionToken()
        if let session { return session }
        return try await refreshSession()
    }

    @discardableResult
    public func refreshSession() async throws -> YuvaSession {
        let info: YuvaSession = try await request("GET", "client/v1/session")
        session = info
        return info
    }

    @discardableResult
    public func identify() async throws -> YuvaSession {
        _ = try await sessionToken(fresh: true)
        restartRealtime()
        return try await start()
    }

    public func signOut() async {
        stopRealtime()
        if let token {
            var request = URLRequest(url: url("client/v1/session"))
            request.httpMethod = "DELETE"
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            _ = try? await urlSession.data(for: request)
        }
        token = nil
        session = nil
        lastEventId = 0
        store.clear()
        if !subscribers.isEmpty { startRealtime() }
    }

    public func conversations(cursor: String? = nil, limit: Int? = nil) async throws -> YuvaPage<YuvaConversation> {
        let page: Page<YuvaConversation> = try await request("GET", "client/v1/conversations", query: paging(cursor, limit))
        return YuvaPage(items: page.items, nextCursor: page.nextCursor)
    }

    public func conversation(id: String) async throws -> YuvaConversation {
        try await request("GET", "client/v1/conversations/\(id)")
    }

    public func messages(
        conversationId: String, order: YuvaOrder = .desc, cursor: String? = nil, limit: Int? = nil
    ) async throws -> YuvaPage<YuvaMessage> {
        let query = [URLQueryItem(name: "order", value: order.rawValue)] + paging(cursor, limit)
        let page: Page<YuvaMessage> = try await request(
            "GET", "client/v1/conversations/\(conversationId)/messages", query: query)
        return YuvaPage(items: page.items, nextCursor: page.nextCursor)
    }

    public func startConversation(
        body: String, subject: String? = nil, attachments: [YuvaUpload] = [], clientId: String = UUID().uuidString
    ) async throws -> YuvaConversationStarted {
        var fields = ["body": body, "client_id": clientId]
        fields["subject"] = subject
        return try await request("POST", "client/v1/conversations", body: Body(fields: fields, files: attachments))
    }

    public func sendMessage(
        conversationId: String, body: String, attachments: [YuvaUpload] = [], clientId: String = UUID().uuidString
    ) async throws -> YuvaMessage {
        try await request(
            "POST", "client/v1/conversations/\(conversationId)/messages",
            body: Body(fields: ["body": body, "client_id": clientId], files: attachments))
    }

    @discardableResult
    public func markRead(conversationId: String, messageId: String? = nil) async throws -> YuvaReadState {
        var fields: [String: String] = [:]
        fields["message_id"] = messageId
        return try await request(
            "POST", "client/v1/conversations/\(conversationId)/read", body: Body(fields: fields, files: []))
    }

    public func setTyping(conversationId: String, typing: Bool) async throws {
        let _: Empty = try await request(
            "POST", "client/v1/conversations/\(conversationId)/typing", body: Body(json: ["typing": typing]))
    }

    @discardableResult
    public func setContactEmail(_ email: String) async throws -> YuvaContact {
        try await request("PUT", "client/v1/contact/email", body: Body(fields: ["email": email], files: []))
    }

    public func attachment(id: String) async throws -> Data {
        try await raw("GET", "client/v1/attachments/\(id)", query: [], body: nil).0
    }

    func postFeedback(fields: [String: String], files: [YuvaUpload]) async throws -> YuvaConversationStarted {
        var body = Body(fields: fields, files: files)
        if files.isEmpty {
            var object: [String: Any] = fields
            object["allow_email"] = fields["allow_email"] == "true"
            body.json = try JSONSerialization.data(withJSONObject: object)
        }
        return try await request("POST", "client/v1/feedback", body: body)
    }

    public nonisolated func events() -> AsyncStream<YuvaEvent> {
        let id = UUID()
        let (stream, continuation) = AsyncStream<YuvaEvent>.makeStream(bufferingPolicy: .bufferingNewest(256))
        continuation.onTermination = { [weak self] _ in
            Task { await self?.unsubscribe(id) }
        }
        Task { await subscribe(id, continuation) }
        return stream
    }

    private func subscribe(_ id: UUID, _ continuation: AsyncStream<YuvaEvent>.Continuation) {
        subscribers[id] = continuation
        startRealtime()
    }

    private func unsubscribe(_ id: UUID) {
        subscribers[id] = nil
        if subscribers.isEmpty { stopRealtime() }
    }

    private func broadcast(_ event: YuvaEvent) {
        for continuation in subscribers.values { continuation.yield(event) }
    }

    // MARK: Session

    private func sessionToken(fresh: Bool = false) async throws -> String {
        if !fresh, let token { return token }
        if let starting { return try await starting.value }
        let task = Task { try await beginSession(fresh: fresh) }
        starting = task
        defer { starting = nil }
        return try await task.value
    }

    private func beginSession(fresh: Bool) async throws -> String {
        var stored = store.load()
        let identity = try await configuration.identityToken?()
        let subject = identity.flatMap(IdentityToken.subject(of:))
        if !fresh, let saved = stored.token, stored.subject == subject,
           let expires = stored.expiresAt, expires > Date().addingTimeInterval(60) {
            do {
                let info: YuvaSession = try await decode(raw("GET", "client/v1/session", query: [], body: nil, token: saved))
                apply(info, token: saved, subject: subject, stored: &stored)
                return saved
            } catch let error as YuvaError where error.status == 401 || error.status == 403 {}
        }
        var fields = ["channel_key": configuration.channelKey]
        fields["identity_token"] = identity
        fields["visitor_id"] = stored.visitorId
        let created: SessionResponse = try await decode(
            raw("POST", "client/v1/session", query: [], body: Body(json: fields), token: nil))
        apply(created.session, token: created.token, subject: subject, stored: &stored)
        return created.token
    }

    private func apply(_ info: YuvaSession, token: String, subject: String?, stored: inout StoredSession) {
        if self.token != token { lastEventId = 0 }
        self.token = token
        session = info
        stored.token = token
        stored.expiresAt = info.expiresAt
        stored.subject = subject
        if let visitor = info.visitorId {
            stored.visitorId = visitor
        } else if info.contact.identified {
            stored.visitorId = nil
        }
        store.save(stored)
    }

    private func dropSession() {
        token = nil
        var stored = store.load()
        stored.token = nil
        stored.expiresAt = nil
        store.save(stored)
    }

    // MARK: HTTP

    struct Body: Sendable {
        var json: Data?
        var fields: [String: String] = [:]
        var files: [YuvaUpload] = []

        init(json: some Encodable) {
            self.json = try? YuvaJSON.encoder.encode(json)
        }

        init(fields: [String: String], files: [YuvaUpload]) {
            self.fields = fields
            self.files = files
            if files.isEmpty { json = try? YuvaJSON.encoder.encode(fields) }
        }
    }

    struct Empty: Decodable {}

    private nonisolated func url(_ path: String, query: [URLQueryItem] = []) -> URL {
        var url = configuration.serverURL.appending(path: path)
        if !query.isEmpty { url.append(queryItems: query) }
        return url
    }

    private func paging(_ cursor: String?, _ limit: Int?) -> [URLQueryItem] {
        var items: [URLQueryItem] = []
        if let cursor { items.append(URLQueryItem(name: "cursor", value: cursor)) }
        if let limit { items.append(URLQueryItem(name: "limit", value: String(limit))) }
        return items
    }

    private func request<T: Decodable>(
        _ method: String, _ path: String, query: [URLQueryItem] = [], body: Body? = nil
    ) async throws -> T {
        try decode(try await raw(method, path, query: query, body: body))
    }

    private func raw(_ method: String, _ path: String, query: [URLQueryItem], body: Body?) async throws -> (Data, Int) {
        let token = try await sessionToken()
        do {
            return try await raw(method, path, query: query, body: body, token: token)
        } catch let error as YuvaError where error.status == 401 {
            if self.token == token { dropSession() }
            let renewed = try await sessionToken(fresh: true)
            restartRealtime()
            return try await raw(method, path, query: query, body: body, token: renewed)
        }
    }

    private func raw(
        _ method: String, _ path: String, query: [URLQueryItem], body: Body?, token: String?
    ) async throws -> (Data, Int) {
        var request = URLRequest(url: url(path, query: query))
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let token { request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization") }
        if let body {
            if body.files.isEmpty {
                request.setValue("application/json", forHTTPHeaderField: "Content-Type")
                request.httpBody = body.json
            } else {
                let boundary = "yuva-" + UUID().uuidString
                request.setValue("multipart/form-data; boundary=\(boundary)", forHTTPHeaderField: "Content-Type")
                request.httpBody = Self.multipart(body, boundary: boundary)
            }
        }
        let (data, response) = try await urlSession.data(for: request)
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        guard (200..<300).contains(status) else {
            let problem = try? YuvaJSON.decoder.decode(Problem.self, from: data)
            throw YuvaError(status: status, code: problem?.code ?? "http_\(status)", detail: problem?.detail ?? problem?.title)
        }
        return (data, status)
    }

    private func decode<T: Decodable>(_ response: (Data, Int)) throws -> T {
        if T.self == Empty.self { return Empty() as! T }
        return try YuvaJSON.decoder.decode(T.self, from: response.0)
    }

    private static func multipart(_ body: Body, boundary: String) -> Data {
        var data = Data()
        func line(_ text: String) { data.append(Data((text + "\r\n").utf8)) }
        for (name, value) in body.fields.sorted(by: { $0.key < $1.key }) {
            line("--\(boundary)")
            line("Content-Disposition: form-data; name=\"\(name)\"")
            line("")
            line(value)
        }
        for file in body.files {
            let filename = file.filename.replacingOccurrences(of: "\"", with: "")
            line("--\(boundary)")
            line("Content-Disposition: form-data; name=\"files\"; filename=\"\(filename)\"")
            line("Content-Type: \(file.contentType)")
            line("")
            data.append(file.data)
            line("")
        }
        line("--\(boundary)--")
        return data
    }

    // MARK: Realtime

    private func startRealtime() {
        guard realtimeTask == nil else { return }
        realtimeTask = Task { await runRealtime() }
    }

    private func stopRealtime() {
        realtimeTask?.cancel()
        realtimeTask = nil
    }

    private func restartRealtime() {
        guard realtimeTask != nil else { return }
        stopRealtime()
        startRealtime()
    }

    private func runRealtime() async {
        var attempt = 0
        while !Task.isCancelled {
            var socket: URLSessionWebSocketTask?
            do {
                let token = try await sessionToken()
                var query: [URLQueryItem] = []
                if lastEventId > 0 { query.append(URLQueryItem(name: "last_event_id", value: String(lastEventId))) }
                var components = URLComponents(url: url("client/v1/realtime", query: query), resolvingAgainstBaseURL: false)!
                components.scheme = components.scheme == "https" ? "wss" : "ws"
                let task = urlSession.webSocketTask(with: components.url!, protocols: ["yuva", "yuva.token.\(token)"])
                socket = task
                task.resume()
                try await withTaskCancellationHandler {
                    while true {
                        let message = try await task.receive()
                        if case .string(let text) = message, handle(Data(text.utf8)) { attempt = 0 }
                    }
                } onCancel: {
                    task.cancel(with: .normalClosure, reason: nil)
                }
            } catch {
                if Task.isCancelled { break }
                broadcast(.connectionChanged(connected: false))
                if socket?.closeCode.rawValue == 1008 {
                    lastEventId = 0
                    dropSession()
                    broadcast(.resyncRequired)
                }
            }
            if Task.isCancelled { break }
            let delay = min(30.0, pow(2.0, Double(attempt))) * Double.random(in: 0.75...1.25)
            attempt += 1
            try? await Task.sleep(for: .seconds(delay))
        }
    }

    private struct Envelope: Decodable {
        let type: String
        let id: Int64?
        let conversationId: String?
        let lastEventId: Int64?
    }

    private struct Payload<T: Decodable>: Decodable {
        let data: T
    }

    private struct StatusData: Decodable {
        let id: String
        let status: YuvaConversationStatus
    }

    private struct ReadData: Decodable {
        let conversationId: String
        let readAt: Date
    }

    private struct TypingData: Decodable {
        let conversationId: String
        let typing: Bool
        let author: YuvaMessageAuthor
    }

    private func handle(_ data: Data) -> Bool {
        let decoder = YuvaJSON.decoder
        guard let envelope = try? decoder.decode(Envelope.self, from: data) else { return false }
        if let id = envelope.id { lastEventId = max(lastEventId, id) }
        func payload<T: Decodable>(_ type: T.Type) -> T? {
            try? decoder.decode(Payload<T>.self, from: data).data
        }
        switch envelope.type {
        case "ready":
            lastEventId = max(lastEventId, envelope.lastEventId ?? 0)
            broadcast(.connectionChanged(connected: true))
            return true
        case "resync_required":
            broadcast(.resyncRequired)
        case "conversation.created":
            if let conversation = payload(YuvaConversation.self) { broadcast(.conversationCreated(conversation)) }
        case "conversation.updated":
            if let status = payload(StatusData.self) {
                broadcast(.conversationUpdated(conversationId: status.id, status: status.status))
            }
        case "message.created":
            if let message = payload(YuvaMessage.self) { broadcast(.messageCreated(message)) }
        case "message.updated":
            if let message = payload(YuvaMessage.self) { broadcast(.messageUpdated(message)) }
        case "read":
            if let read = payload(ReadData.self) {
                broadcast(.read(conversationId: read.conversationId, readAt: read.readAt))
            }
        case "typing":
            if let typing = payload(TypingData.self) {
                broadcast(.typing(conversationId: typing.conversationId, typing: typing.typing, author: typing.author))
            }
        case "presence":
            if let presence = payload(YuvaPresence.self) { broadcast(.presence(presence)) }
        default:
            break
        }
        return false
    }
}

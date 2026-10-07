import PhotosUI
import SwiftUI

struct ThreadItem: Identifiable {
    enum State { case sending, sent, failed }

    let id: String
    var message: YuvaMessage?
    var body: String
    var uploads: [YuvaUpload]
    var state: State
    var createdAt: Date

    var mine: Bool { message.map { $0.author.type == .contact } ?? true }
}

@MainActor
@Observable
final class ThreadModel {
    let client: YuvaClient
    var conversationId: String?
    var items: [ThreadItem] = []
    var conversation: YuvaConversation?
    var inbox: YuvaInbox?
    var memberReadAt: Date?
    var typingName: String?
    var connected = true
    var loading = false
    var failed = false
    private var olderCursor: String?
    private var loadingOlder = false
    private var typingClear: Task<Void, Never>?
    private var typingSentAt: Date?
    private var typingIdle: Task<Void, Never>?
    var visible = false

    init(client: YuvaClient, conversationId: String?) {
        self.client = client
        self.conversationId = conversationId
    }

    func load() async {
        loading = items.isEmpty && conversationId != nil
        defer { loading = false }
        do {
            inbox = try await client.start().inbox
            guard let id = conversationId else { return }
            async let conversation = client.conversation(id: id)
            let page = try await client.messages(conversationId: id, order: .desc, limit: 30)
            self.conversation = try await conversation
            memberReadAt = self.conversation?.lastReadByMemberAt
            let pending = items.filter { $0.message == nil }
            items = page.items.reversed().map(Self.item) + pending
            olderCursor = page.nextCursor
            failed = false
            await markRead()
        } catch {
            failed = items.isEmpty
        }
    }

    func loadOlder() async {
        guard let id = conversationId, let cursor = olderCursor, !loadingOlder else { return }
        loadingOlder = true
        defer { loadingOlder = false }
        guard let page = try? await client.messages(conversationId: id, order: .desc, cursor: cursor, limit: 30) else {
            return
        }
        let known = Set(items.map(\.id))
        items.insert(contentsOf: page.items.reversed().filter { !known.contains($0.id) }.map(Self.item), at: 0)
        olderCursor = page.nextCursor
    }

    var canLoadOlder: Bool { olderCursor != nil }

    func listen() async {
        for await event in client.events() {
            switch event {
            case .messageCreated(let message), .messageUpdated(let message):
                guard message.conversationId == conversationId else { continue }
                upsert(message)
                if message.author.type == .member {
                    typingName = nil
                    await markRead()
                }
            case .conversationUpdated(let id, _):
                if id == conversationId { conversation = try? await client.conversation(id: id) }
            case .read(let id, let readAt):
                if id == conversationId { memberReadAt = max(memberReadAt ?? readAt, readAt) }
            case .typing(let id, let typing, let author):
                guard id == conversationId else { continue }
                typingClear?.cancel()
                typingName = typing ? (author.name ?? "") : nil
                if typing {
                    typingClear = Task { [weak self] in
                        try? await Task.sleep(for: .seconds(8))
                        if !Task.isCancelled { self?.typingName = nil }
                    }
                }
            case .presence:
                inbox = try? await client.refreshSession().inbox
            case .inboxUpdated(let updated):
                inbox = updated
            case .connectionChanged(let connected):
                self.connected = connected
            case .resyncRequired:
                await load()
            default:
                break
            }
        }
    }

    func send(_ text: String, uploads: [YuvaUpload]) {
        let body = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !body.isEmpty || !uploads.isEmpty else { return }
        let clientId = UUID().uuidString
        items.append(ThreadItem(id: clientId, body: body, uploads: uploads, state: .sending, createdAt: Date()))
        stopTyping()
        Task { await deliver(clientId) }
    }

    func retry(_ clientId: String) {
        guard let index = items.firstIndex(where: { $0.id == clientId }) else { return }
        items[index].state = .sending
        Task { await deliver(clientId) }
    }

    private func deliver(_ clientId: String) async {
        guard let item = items.first(where: { $0.id == clientId }) else { return }
        do {
            let message: YuvaMessage
            if let id = conversationId {
                message = try await client.sendMessage(
                    conversationId: id, body: item.body, attachments: item.uploads, clientId: clientId)
            } else {
                let started = try await client.startConversation(
                    body: item.body, attachments: item.uploads, clientId: clientId)
                conversationId = started.conversation.id
                conversation = started.conversation
                message = started.message
            }
            upsert(message)
        } catch {
            if let index = items.firstIndex(where: { $0.id == clientId }) { items[index].state = .failed }
        }
    }

    private func upsert(_ message: YuvaMessage) {
        let item = Self.item(message)
        if let index = items.firstIndex(where: { $0.id == message.id || (message.clientId != nil && $0.id == message.clientId) }) {
            items[index] = item
        } else {
            items.append(item)
        }
    }

    private static func item(_ message: YuvaMessage) -> ThreadItem {
        ThreadItem(
            id: message.id, message: message, body: message.body, uploads: [], state: .sent, createdAt: message.createdAt)
    }

    func markRead() async {
        guard visible, let id = conversationId,
              let last = items.last(where: { $0.message?.author.type == .member })?.message
        else { return }
        _ = try? await client.markRead(conversationId: id, messageId: last.id)
    }

    func typing(_ text: String) {
        guard let id = conversationId, !text.isEmpty else { return }
        typingIdle?.cancel()
        typingIdle = Task { [weak self] in
            try? await Task.sleep(for: .seconds(4))
            if !Task.isCancelled { self?.stopTyping() }
        }
        if let sent = typingSentAt, Date().timeIntervalSince(sent) < 3 { return }
        typingSentAt = Date()
        Task { try? await client.setTyping(conversationId: id, typing: true) }
    }

    func stopTyping() {
        typingIdle?.cancel()
        guard typingSentAt != nil, let id = conversationId else { return }
        typingSentAt = nil
        Task { try? await client.setTyping(conversationId: id, typing: false) }
    }

    var seenMessageId: String? {
        guard let readAt = memberReadAt else { return nil }
        return items.last(where: { $0.mine && $0.message != nil && $0.createdAt <= readAt })?.id
    }
}

public struct YuvaThreadView: View {
    @State private var model: ThreadModel
    @State private var text = ""
    @State private var picked: [PhotosPickerItem] = []
    @State private var uploads: [YuvaUpload] = []

    public init(client: YuvaClient, conversationId: String?) {
        _model = State(initialValue: ThreadModel(client: client, conversationId: conversationId))
    }

    public var body: some View {
        VStack(spacing: 0) {
            if !model.connected {
                Text(L.t("connection.connecting"))
                    .font(.caption).foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity).padding(4).background(.bar)
            }
            messages
            composer
        }
        .navigationTitle(title)
        #if os(iOS)
        .navigationBarTitleDisplayMode(.inline)
        #endif
        .task { await model.load() }
        .task { await model.listen() }
        .onAppear {
            model.visible = true
            Task { await model.markRead() }
        }
        .onDisappear {
            model.visible = false
            model.stopTyping()
        }
    }

    private var title: String {
        if let subject = model.conversation?.subject, !subject.isEmpty { return subject }
        return model.inbox?.name ?? L.t("thread.title")
    }

    private var messages: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(spacing: 6) {
                    if model.canLoadOlder {
                        ProgressView().padding().onAppear { Task { await model.loadOlder() } }
                    }
                    if model.items.isEmpty && !model.loading {
                        greeting
                    }
                    ForEach(model.items) { item in
                        MessageRow(
                            item: item, client: model.client, seen: item.id == model.seenMessageId,
                            showStatus: item.mine && (item.id == model.items.last(where: \.mine)?.id || item.state != .sent)
                        ) {
                            model.retry(item.id)
                        }
                        .id(item.id)
                    }
                    if let name = model.typingName {
                        HStack {
                            Text(name.isEmpty ? L.t("thread.typingSomeone") : L.f("thread.typing", name))
                                .font(.caption).foregroundStyle(.secondary)
                            Spacer()
                        }
                        .padding(.horizontal)
                        .id("typing")
                    }
                }
                .padding(.vertical, 8)
            }
            .defaultScrollAnchor(.bottom)
            .overlay { if model.loading { ProgressView() } }
            .onChange(of: model.items.last?.id) { _, id in
                guard let id else { return }
                withAnimation { proxy.scrollTo(id, anchor: .bottom) }
            }
            .onChange(of: model.typingName) { _, name in
                if name != nil { withAnimation { proxy.scrollTo("typing", anchor: .bottom) } }
            }
        }
    }

    private var greeting: some View {
        VStack(spacing: 8) {
            if let inbox = model.inbox {
                PresenceHeader(inbox: inbox)
                if let greeting = inbox.branding.greeting, !greeting.isEmpty {
                    Text(greeting).font(.callout).multilineTextAlignment(.center).foregroundStyle(.secondary)
                }
            }
        }
        .padding(24)
    }

    private var composer: some View {
        VStack(spacing: 6) {
            if !uploads.isEmpty {
                ScrollView(.horizontal) {
                    HStack {
                        ForEach(Array(uploads.enumerated()), id: \.offset) { index, upload in
                            if let image = PlatformImage(data: upload.data) {
                                Image(platformImage: image).resizable().scaledToFill()
                                    .frame(width: 56, height: 56).clipShape(RoundedRectangle(cornerRadius: 8))
                                    .overlay(alignment: .topTrailing) {
                                        Button {
                                            uploads.remove(at: index)
                                        } label: {
                                            Image(systemName: "xmark.circle.fill").foregroundStyle(.white, .black.opacity(0.6))
                                        }
                                        .buttonStyle(.plain)
                                    }
                            }
                        }
                    }
                    .padding(.horizontal)
                }
            }
            HStack(alignment: .bottom, spacing: 8) {
                PhotosPicker(selection: $picked, maxSelectionCount: 10, matching: .images) {
                    Image(systemName: "photo").font(.title3).padding(.bottom, 6)
                }
                .accessibilityLabel(L.t("thread.attach"))
                TextField(L.t("thread.placeholder"), text: $text, axis: .vertical)
                    .lineLimit(1...5)
                    .textFieldStyle(.plain)
                    .padding(.horizontal, 12).padding(.vertical, 8)
                    .background(RoundedRectangle(cornerRadius: 18).fill(.quaternary))
                    .onChange(of: text) { _, value in model.typing(value) }
                    .accessibilityIdentifier("yuva.thread.input")
                Button {
                    model.send(text, uploads: uploads)
                    text = ""
                    uploads = []
                } label: {
                    Image(systemName: "arrow.up.circle.fill").font(.title)
                }
                .disabled(text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && uploads.isEmpty)
                .accessibilityLabel(L.t("thread.send"))
                .accessibilityIdentifier("yuva.thread.send")
            }
            .padding(.horizontal)
        }
        .padding(.vertical, 8)
        .background(.bar)
        .onChange(of: picked) { _, items in
            guard !items.isEmpty else { return }
            picked = []
            Task {
                for item in items {
                    if let data = try? await item.loadTransferable(type: Data.self), let upload = ImageUpload.make(from: data) {
                        uploads.append(upload)
                    }
                }
            }
        }
    }
}

struct MessageRow: View {
    let item: ThreadItem
    let client: YuvaClient
    let seen: Bool
    let showStatus: Bool
    let retry: () -> Void

    var body: some View {
        VStack(alignment: item.mine ? .trailing : .leading, spacing: 3) {
            HStack(alignment: .bottom, spacing: 6) {
                if item.mine { Spacer(minLength: 48) }
                if !item.mine, let author = item.message?.author, author.type == .member {
                    AvatarView(initials: author.initials ?? "")
                }
                VStack(alignment: item.mine ? .trailing : .leading, spacing: 4) {
                    if !item.mine, let name = item.message?.author.name, !name.isEmpty {
                        Text(name).font(.caption2).foregroundStyle(.secondary)
                    }
                    attachments
                    if !item.body.isEmpty {
                        Text(item.body)
                            .textSelection(.enabled)
                            .padding(.horizontal, 12).padding(.vertical, 8)
                            .foregroundStyle(item.mine ? Color.white : Color.primary)
                            .background(
                                RoundedRectangle(cornerRadius: 18)
                                    .fill(item.mine ? AnyShapeStyle(Color.accentColor) : AnyShapeStyle(.quaternary)))
                    }
                }
                .opacity(item.state == .sending ? 0.6 : 1)
                if !item.mine { Spacer(minLength: 48) }
            }
            if showStatus { status }
        }
        .padding(.horizontal)
    }

    @ViewBuilder
    private var attachments: some View {
        if let message = item.message {
            ForEach(message.attachments) { attachment in
                if attachment.isImage {
                    AttachmentImageView(client: client, attachment: attachment)
                } else {
                    AttachmentFileView(client: client, attachment: attachment)
                }
            }
        } else {
            ForEach(Array(item.uploads.enumerated()), id: \.offset) { _, upload in
                if let image = PlatformImage(data: upload.data) {
                    Image(platformImage: image).resizable().scaledToFit()
                        .frame(maxWidth: 220, maxHeight: 260).clipShape(RoundedRectangle(cornerRadius: 12))
                }
            }
        }
    }

    @ViewBuilder
    private var status: some View {
        switch item.state {
        case .sending:
            Text(L.t("thread.sending")).font(.caption2).foregroundStyle(.secondary)
        case .failed:
            Button(action: retry) {
                Label(L.t("thread.failed"), systemImage: "exclamationmark.circle")
            }
            .font(.caption2).foregroundStyle(.red).buttonStyle(.plain)
        case .sent:
            Text(seen ? L.t("thread.seen") : L.t("thread.sent")).font(.caption2).foregroundStyle(.secondary)
                .accessibilityIdentifier(seen ? "yuva.thread.seen" : "yuva.thread.sent")
        }
    }
}

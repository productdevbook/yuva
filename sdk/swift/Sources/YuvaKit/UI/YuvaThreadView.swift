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
    var failure = ""
    var ratingChoice: YuvaRating?
    var ratingSending = false
    var ratingError: String?
    var removed = false
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
            failure = L.message(for: error)
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
            case .messageCreated(let message), .messageUpdated(let message), .messageRedacted(let message):
                guard message.conversationId == conversationId else { continue }
                upsert(message)
                if message.author.type.isTeam {
                    typingName = nil
                    await markRead()
                }
            case .conversationRemoved(let id):
                guard id == conversationId else { continue }
                stopTyping()
                conversationId = nil
                conversation = nil
                olderCursor = nil
                memberReadAt = nil
                typingName = nil
                ratingChoice = nil
                items = items.filter { $0.message == nil }
                removed = true
            case .conversationUpdated(let id, _, _, _, let progress):
                if id == conversationId, let updated = try? await client.conversation(id: id) {
                    conversation = updated.settingFeedbackProgress(progress)
                }
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
                let asked = inbox?.askForRating
                inbox = updated
                if asked != nil, asked != updated.askForRating, let id = conversationId,
                   let refreshed = try? await client.conversation(id: id) {
                    conversation = refreshed
                }
            case .connectionChanged(let connected):
                self.connected = connected
            case .resyncRequired:
                await load()
            default:
                break
            }
        }
    }

    var showsRating: Bool {
        guard let conversation, !conversation.isFeedback, conversation.status == .closed else { return false }
        return conversation.canRate || conversation.rating != nil
    }

    var feedbackNoteState: YuvaFeedbackState? {
        guard let conversation, conversation.isFeedback else { return nil }
        if conversation.status == .closed { return .resolved }
        return feedbackState
    }

    var feedbackState: YuvaFeedbackState? {
        guard let conversation, conversation.isFeedback else { return nil }
        let last = items.last(where: { $0.message != nil })?.message?.author.type ?? conversation.lastMessage?.authorType
        return .of(status: conversation.status, lastAuthor: last, progress: conversation.feedback?.progress)
    }

    func rate(_ rating: YuvaRating, comment: String?) async {
        guard let id = conversationId, !ratingSending else { return }
        ratingSending = true
        ratingError = nil
        defer { ratingSending = false }
        do {
            conversation = try await client.rate(conversationId: id, rating: rating, comment: comment)
            ratingChoice = nil
        } catch let error as YuvaError where error.status == 409 {
            if let refreshed = try? await client.conversation(id: id) { conversation = refreshed }
            ratingChoice = nil
        } catch {
            ratingError = L.message(for: error)
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
            removed = false
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
              let last = items.last(where: { $0.message?.author.type.isTeam == true })?.message
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
    @State private var attachNotice: String?
    @State private var viewing: YuvaAttachment?
    @State private var bottomY: CGFloat = 0
    @State private var viewport: CGFloat = 0
    @State private var unseen = false
    @State private var sent = 0

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
            if let state = model.feedbackState {
                FeedbackHeader(feedback: model.conversation?.feedback, state: state)
            }
            messages
        }
        .bottomBar {
            VStack(spacing: 0) {
                YuvaStatusLine(status: model.inbox?.status)
                composer
            }
        }
        .sensoryFeedback(.impact(weight: .light), trigger: sent)
        #if os(iOS)
        .fullScreenCover(item: $viewing) { ImageViewer(client: model.client, attachment: $0) }
        #else
        .sheet(item: $viewing) { ImageViewer(client: model.client, attachment: $0).frame(minWidth: 480, minHeight: 360) }
        #endif
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

    private var atBottom: Bool { viewport == 0 || bottomY <= viewport + 80 }

    private var title: String {
        if let subject = model.conversation?.subject, !subject.isEmpty { return subject }
        if model.conversation?.isFeedback == true { return L.t("feedback.kind") }
        return model.inbox?.name ?? L.t("thread.title")
    }

    private var messages: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(spacing: 0) {
                    if model.canLoadOlder {
                        ProgressView().padding().onAppear { Task { await model.loadOlder() } }
                    }
                    if model.removed {
                        Text(L.t("conversation.removed"))
                            .font(.footnote).foregroundStyle(.secondary).multilineTextAlignment(.center)
                            .frame(maxWidth: .infinity).padding(.horizontal, 24).padding(.top, 24)
                            .accessibilityIdentifier("yuva.thread.removedNote")
                    }
                    if model.items.isEmpty && !model.loading && !model.failed {
                        greeting
                    }
                    ForEach(ThreadLayout.rows(model.items)) { row in
                        if row.startsDay {
                            Text(ShortTime.daySeparator(row.item.createdAt))
                                .font(.caption.weight(.medium)).foregroundStyle(.secondary)
                                .frame(maxWidth: .infinity).padding(.top, 16).padding(.bottom, 8)
                        }
                        MessageRow(
                            item: row.item, client: model.client, seen: row.item.id == model.seenMessageId,
                            showStatus: row.item.mine
                                && (row.item.id == model.items.last(where: \.mine)?.id || row.item.state != .sent),
                            first: row.first, last: row.last,
                            retry: { model.retry(row.item.id) },
                            openImage: { viewing = $0 }
                        )
                        .padding(.top, row.first && !row.startsDay ? 10 : 2)
                        .id(row.item.id)
                    }
                    if let state = model.feedbackNoteState, state == .resolved || state.isProgress {
                        Label(
                            L.t(state.isProgress ? "feedback.progressNote." + state.rawValue : "feedback.resolvedNote"),
                            systemImage: state.isProgress ? "flag.fill" : "checkmark.circle.fill")
                            .font(.footnote).foregroundStyle(.secondary).multilineTextAlignment(.center)
                            .frame(maxWidth: .infinity).padding(.horizontal, 24).padding(.vertical, 16)
                            .id("resolved")
                            .accessibilityIdentifier("yuva.feedback.resolvedNote")
                    }
                    if model.showsRating {
                        RatingCard(model: model).id("rating")
                    }
                    if let name = model.typingName {
                        HStack(alignment: .bottom, spacing: 8) {
                            AvatarView(initials: String(name.prefix(1)).uppercased(), size: 28)
                            TypingDots()
                                .padding(.horizontal, 14).padding(.vertical, 13)
                                .background(Color.secondary.opacity(0.18), in: Capsule())
                            Spacer()
                        }
                        .padding(.horizontal, 12).padding(.top, 10)
                        .accessibilityElement(children: .ignore)
                        .accessibilityLabel(name.isEmpty ? L.t("thread.typingSomeone") : L.f("thread.typing", name))
                        .accessibilityIdentifier("yuva.thread.typing")
                        .transition(.scale(scale: 0.6, anchor: .bottomLeading).combined(with: .opacity))
                        .id("typing")
                    }
                    Color.clear.frame(height: 1)
                        .id("bottom")
                        .onGeometryChange(for: CGFloat.self, of: { $0.frame(in: .scrollView).minY }) { bottomY = $0 }
                }
                .padding(.vertical, 8)
                .animation(.snappy, value: model.typingName)
            }
            .scrollDismissesKeyboard(.interactively)
            .defaultScrollAnchor(.bottom)
            .onGeometryChange(for: CGFloat.self, of: { $0.size.height }) { viewport = $0 }
            .onChange(of: atBottom) { _, now in if now { unseen = false } }
            .overlay {
                if model.loading {
                    ProgressView()
                } else if model.failed {
                    ContentUnavailableView {
                        Label(model.failure, systemImage: "wifi.exclamationmark")
                    } actions: {
                        Button(L.t("error.retry")) { Task { await model.load() } }
                            .buttonStyle(.borderedProminent)
                            .accessibilityIdentifier("yuva.thread.retry")
                    }
                }
            }
            .overlay(alignment: .bottomTrailing) {
                if !atBottom && !model.items.isEmpty {
                    Button {
                        withAnimation(.snappy) { proxy.scrollTo("bottom", anchor: .bottom) }
                    } label: {
                        Image(systemName: "chevron.down").font(.body.weight(.semibold)).frame(width: 44, height: 44)
                    }
                    .glassCircle()
                    .overlay(alignment: .topTrailing) {
                        if unseen { Circle().fill(Color.accentColor).frame(width: 10, height: 10) }
                    }
                    .padding(16)
                    .transition(.scale.combined(with: .opacity))
                    .accessibilityLabel(L.t("thread.scrollToBottom"))
                    .accessibilityIdentifier("yuva.thread.scrollToBottom")
                }
            }
            .animation(.snappy, value: atBottom)
            .onChange(of: model.items.last?.id) { _, id in
                guard let id, let last = model.items.last else { return }
                if atBottom || (last.mine && last.message == nil) {
                    withAnimation { proxy.scrollTo(id, anchor: .bottom) }
                } else if !last.mine {
                    unseen = true
                }
            }
            .onChange(of: model.showsRating) { _, shows in
                if shows { withAnimation { proxy.scrollTo("rating", anchor: .bottom) } }
            }
            .onChange(of: model.typingName) { _, name in
                if name != nil, atBottom { withAnimation { proxy.scrollTo("typing", anchor: .bottom) } }
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

    @ViewBuilder
    private var composerBackground: some View {
        if #available(iOS 26.0, macOS 26.0, *) {
            Color.clear
        } else {
            Rectangle().fill(.bar).ignoresSafeArea()
        }
    }

    private var canSend: Bool {
        !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || !uploads.isEmpty
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
            if let attachNotice {
                Text(attachNotice)
                    .font(.footnote).foregroundStyle(.red).multilineTextAlignment(.center)
                    .padding(.horizontal, 16)
                    .accessibilityIdentifier("yuva.thread.attachNotice")
            }
            HStack(alignment: .bottom, spacing: 8) {
                if model.inbox?.attachments?.allowsImages != false {
                    PhotosPicker(selection: $picked, maxSelectionCount: 10, matching: .images) {
                        Image(systemName: "plus").font(.title3.weight(.medium)).frame(width: 44, height: 44)
                    }
                    .glassCircle()
                    .accessibilityLabel(L.t("thread.attach"))
                    .accessibilityIdentifier("yuva.thread.attach")
                }
                HStack(alignment: .bottom, spacing: 4) {
                    TextField(L.t("thread.placeholder"), text: $text, axis: .vertical)
                        .lineLimit(1...5)
                        .textFieldStyle(.plain)
                        .padding(.leading, 16).padding(.vertical, 11)
                        .onChange(of: text) { _, value in model.typing(value) }
                        .accessibilityIdentifier("yuva.thread.input")
                    Button {
                        model.send(text, uploads: uploads)
                        text = ""
                        uploads = []
                        sent += 1
                    } label: {
                        Image(systemName: "arrow.up.circle.fill").font(.system(size: 32))
                            .symbolRenderingMode(.palette)
                            .foregroundStyle(.white, canSend ? Color.accentColor : Color.secondary.opacity(0.35))
                    }
                    .buttonStyle(.plain)
                    .disabled(!canSend)
                    .padding(.trailing, 6).padding(.bottom, 6)
                    .animation(.snappy, value: canSend)
                    .accessibilityLabel(L.t("thread.send"))
                    .accessibilityIdentifier("yuva.thread.send")
                }
                .glassCapsule()
            }
            .padding(.horizontal, 12)
        }
        .padding(.vertical, 8)
        .background { composerBackground }
        .onChange(of: picked) { _, items in
            guard !items.isEmpty else { return }
            picked = []
            Task {
                attachNotice = nil
                for item in items {
                    guard let data = try? await item.loadTransferable(type: Data.self),
                          let upload = ImageUpload.make(from: data) else { continue }
                    if let refusal = model.inbox?.attachments?.refusal(for: upload) {
                        attachNotice = L.message(for: refusal)
                    } else {
                        uploads.append(upload)
                    }
                }
            }
        }
        .onChange(of: model.inbox?.attachments) { _, limits in
            guard let limits else { return }
            let refusal = uploads.lazy.compactMap(limits.refusal(for:)).first
            uploads.removeAll { limits.refusal(for: $0) != nil }
            if let refusal { attachNotice = L.message(for: refusal) }
        }
    }
}

struct FeedbackHeader: View {
    let feedback: YuvaFeedbackInfo?
    let state: YuvaFeedbackState

    var body: some View {
        HStack(spacing: 10) {
            CategoryIcon(key: feedback?.categoryKey, size: 28)
            if let feedback {
                Text(feedback.displayName).font(.subheadline.weight(.semibold))
            }
            Spacer()
            FeedbackTag(text: state.label, tint: state.tint)
                .accessibilityIdentifier("yuva.feedback.state." + state.rawValue)
        }
        .padding(.horizontal, 16).padding(.vertical, 8)
        .background(.bar)
        .overlay(alignment: .bottom) { Divider() }
    }
}

struct RatingCard: View {
    let model: ThreadModel
    @State private var comment = ""

    var body: some View {
        VStack(spacing: 10) {
            if let rating = model.conversation?.rating {
                Image(systemName: Self.symbol(rating, filled: true))
                    .font(.title2).foregroundStyle(.tint)
                    .accessibilityLabel(L.t("rating." + rating.rawValue))
                Text(L.t("rating.thanks")).font(.callout).multilineTextAlignment(.center)
                    .accessibilityIdentifier("yuva.rating.thanks")
            } else {
                Text(L.t("rating.prompt")).font(.callout.weight(.medium)).multilineTextAlignment(.center)
                HStack(spacing: 16) {
                    ForEach(YuvaRating.allCases, id: \.self) { rating in
                        Button {
                            model.ratingChoice = rating
                        } label: {
                            Image(systemName: Self.symbol(rating, filled: model.ratingChoice == rating))
                                .font(.title2).frame(width: 44, height: 36)
                        }
                        .buttonStyle(.bordered)
                        .clipShape(Capsule())
                        .accessibilityLabel(L.t("rating." + rating.rawValue))
                        .accessibilityAddTraits(model.ratingChoice == rating ? .isSelected : [])
                        .accessibilityIdentifier("yuva.rating." + rating.rawValue)
                    }
                }
                if let choice = model.ratingChoice {
                    TextField(L.t("rating.commentPlaceholder"), text: $comment, axis: .vertical)
                        .lineLimit(2...5)
                        .textFieldStyle(.plain)
                        .padding(.horizontal, 12).padding(.vertical, 8)
                        .background(RoundedRectangle(cornerRadius: 12).fill(.background))
                        .onChange(of: comment) { _, value in
                            if value.count > 2000 { comment = String(value.prefix(2000)) }
                        }
                        .accessibilityIdentifier("yuva.rating.comment")
                    if let error = model.ratingError {
                        Text(error).font(.caption).foregroundStyle(.red)
                    }
                    HStack {
                        Button(L.t("rating.skip")) {
                            Task { await model.rate(choice, comment: nil) }
                        }
                        .buttonStyle(.bordered)
                        .accessibilityIdentifier("yuva.rating.skip")
                        Button(L.t("rating.send")) {
                            Task { await model.rate(choice, comment: comment) }
                        }
                        .buttonStyle(.borderedProminent)
                        .accessibilityIdentifier("yuva.rating.send")
                    }
                }
            }
        }
        .disabled(model.ratingSending)
        .frame(maxWidth: .infinity)
        .padding(16)
        .background(RoundedRectangle(cornerRadius: 16).fill(.quaternary.opacity(0.5)))
        .padding(.horizontal)
        .padding(.top, 8)
    }

    private static func symbol(_ rating: YuvaRating, filled: Bool) -> String {
        (rating == .good ? "hand.thumbsup" : "hand.thumbsdown") + (filled ? ".fill" : "")
    }
}

struct ThreadRowInfo: Identifiable {
    let item: ThreadItem
    let startsDay: Bool
    let first: Bool
    let last: Bool

    var id: String { item.id }
}

enum ThreadLayout {
    static let groupGap: TimeInterval = 5 * 60

    static func rows(_ items: [ThreadItem], calendar: Calendar = .current) -> [ThreadRowInfo] {
        items.indices.map { index in
            let item = items[index]
            let previous = index > 0 ? items[index - 1] : nil
            let next = index + 1 < items.count ? items[index + 1] : nil
            let startsDay = previous.map { !calendar.isDate($0.createdAt, inSameDayAs: item.createdAt) } ?? true
            let joinsPrevious = previous.map { !startsDay && joined($0, item) } ?? false
            let joinsNext = next.map { calendar.isDate($0.createdAt, inSameDayAs: item.createdAt) && joined(item, $0) } ?? false
            return ThreadRowInfo(item: item, startsDay: startsDay, first: !joinsPrevious, last: !joinsNext)
        }
    }

    private static func joined(_ a: ThreadItem, _ b: ThreadItem) -> Bool {
        key(a) == key(b) && b.createdAt.timeIntervalSince(a.createdAt) < groupGap
    }

    private static func key(_ item: ThreadItem) -> String {
        if item.mine { return "me" }
        guard let author = item.message?.author else { return "" }
        return author.type.rawValue + ":" + (author.name ?? "")
    }
}

struct MessageRow: View {
    let item: ThreadItem
    let client: YuvaClient
    let seen: Bool
    let showStatus: Bool
    let first: Bool
    let last: Bool
    let retry: () -> Void
    var openImage: (YuvaAttachment) -> Void = { _ in }

    private let avatarSize: CGFloat = 28
    private static let radius: CGFloat = 18
    private static let joined: CGFloat = 5

    var body: some View {
        HStack(alignment: .bottom, spacing: 8) {
            if item.mine {
                Spacer(minLength: 56)
            } else if let author = item.message?.author {
                if first {
                    AvatarView(initials: authorInitials(author), size: avatarSize)
                } else {
                    Color.clear.frame(width: avatarSize, height: avatarSize)
                }
            }
            VStack(alignment: item.mine ? .trailing : .leading, spacing: 4) {
                if first, !item.mine, let name = item.message?.author.name, !name.isEmpty {
                    Text(name).font(.caption).foregroundStyle(.secondary).padding(.horizontal, 4)
                }
                if item.message?.redacted == true {
                    Text(L.t("thread.removed"))
                        .italic()
                        .padding(.horizontal, 12).padding(.vertical, 8)
                        .foregroundStyle(.secondary)
                        .background(
                            UnevenRoundedRectangle(cornerRadii: corners, style: .continuous)
                                .strokeBorder(Color.secondary.opacity(0.4)))
                } else {
                    attachments
                }
                if item.message?.redacted != true, !item.body.isEmpty {
                    Text(Links.attributed(item.body))
                        .textSelection(.enabled)
                        .tint(item.mine ? Color.white : Color.accentColor)
                        .padding(.horizontal, 12).padding(.vertical, 8)
                        .foregroundStyle(item.mine ? Color.white : Color.primary)
                        .background(
                            UnevenRoundedRectangle(cornerRadii: corners, style: .continuous)
                                .fill(item.mine ? Color.accentColor : Color.secondary.opacity(0.18)))
                }
                if last || item.state != .sent { footer }
            }
            .opacity(item.state == .sending ? 0.6 : 1)
            if !item.mine { Spacer(minLength: 56) }
        }
        .padding(.horizontal, 12)
    }

    private var corners: RectangleCornerRadii {
        let big = Self.radius, small = Self.joined
        if item.mine {
            return .init(
                topLeading: big, bottomLeading: big, bottomTrailing: last ? big : small, topTrailing: first ? big : small)
        }
        return .init(
            topLeading: first ? big : small, bottomLeading: last ? big : small, bottomTrailing: big, topTrailing: big)
    }

    private func authorInitials(_ author: YuvaMessageAuthor) -> String {
        if let initials = author.initials, !initials.isEmpty { return initials }
        return String((author.name ?? "").prefix(1)).uppercased()
    }

    @ViewBuilder
    private var attachments: some View {
        if let message = item.message {
            ForEach(message.attachments) { attachment in
                if attachment.isImage {
                    AttachmentImageView(client: client, attachment: attachment) { openImage(attachment) }
                } else {
                    AttachmentFileView(client: client, attachment: attachment)
                }
            }
        } else {
            ForEach(Array(item.uploads.enumerated()), id: \.offset) { _, upload in
                if let image = PlatformImage(data: upload.data) {
                    Image(platformImage: image).resizable().scaledToFit()
                        .frame(maxWidth: 220, maxHeight: 260).clipShape(RoundedRectangle(cornerRadius: 14))
                }
            }
        }
    }

    private var footer: some View {
        HStack(spacing: 4) {
            Text(ShortTime.clock(item.createdAt))
            if showStatus { status }
        }
        .font(.caption2).foregroundStyle(.secondary)
        .padding(.horizontal, 4)
    }

    @ViewBuilder
    private var status: some View {
        switch item.state {
        case .sending:
            Text("· " + L.t("thread.sending"))
        case .failed:
            Button(action: retry) {
                Label(L.t("thread.failed"), systemImage: "exclamationmark.circle")
            }
            .foregroundStyle(.red).buttonStyle(.plain)
        case .sent:
            Text("· " + (seen ? L.t("thread.seen") : L.t("thread.sent")))
                .accessibilityIdentifier(seen ? "yuva.thread.seen" : "yuva.thread.sent")
        }
    }
}

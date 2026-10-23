import SwiftUI

enum ConversationsTab: Hashable {
    case messages, feedback
}

public enum YuvaConversationsSection: Sendable {
    case messages, feedback
}

@MainActor
@Observable
final class ConversationsModel {
    let client: YuvaClient
    var messages: [YuvaConversation] = []
    var feedback: [YuvaConversation] = []
    var session: YuvaSession?
    var loading = false
    var failed = false
    private var messagesCursor: String?
    private var feedbackCursor: String?

    init(client: YuvaClient) {
        self.client = client
    }

    var hasFeedback: Bool { !feedback.isEmpty }

    func items(_ tab: ConversationsTab) -> [YuvaConversation] {
        tab == .messages ? messages : feedback
    }

    func load() async {
        loading = messages.isEmpty && feedback.isEmpty
        defer { loading = false }
        do {
            session = try await client.start()
            async let sent = client.conversations(kind: [.feedback])
            let page = try await client.conversations(kind: [.conversation])
            let sentPage = try await sent
            messages = page.items
            messagesCursor = page.nextCursor
            feedback = sentPage.items
            feedbackCursor = sentPage.nextCursor
            failed = false
        } catch {
            failed = messages.isEmpty && feedback.isEmpty
        }
    }

    func loadMore(_ tab: ConversationsTab) async {
        guard let cursor = tab == .messages ? messagesCursor : feedbackCursor else { return }
        setCursor(nil, tab)
        let kind: YuvaConversationKind = tab == .messages ? .conversation : .feedback
        guard let page = try? await client.conversations(kind: [kind], cursor: cursor) else {
            setCursor(cursor, tab)
            return
        }
        let known = Set(items(tab).map(\.id))
        let added = page.items.filter { !known.contains($0.id) }
        if tab == .messages { messages += added } else { feedback += added }
        setCursor(page.nextCursor, tab)
    }

    private func setCursor(_ cursor: String?, _ tab: ConversationsTab) {
        if tab == .messages { messagesCursor = cursor } else { feedbackCursor = cursor }
    }

    func listen() async {
        for await event in client.events() {
            switch event {
            case .conversationCreated(let conversation):
                upsert(conversation)
            case .conversationUpdated(let id, _, _, _, let progress):
                await refresh(id, progress: .some(progress))
            case .read(let id, _):
                await refresh(id)
            case .messageCreated(let message):
                await refresh(message.conversationId)
            case .presence:
                session = try? await client.refreshSession()
            case .inboxUpdated:
                session = await client.session
            case .resyncRequired:
                await load()
            default:
                break
            }
        }
    }

    private func refresh(_ id: String, progress: YuvaFeedbackProgress?? = nil) async {
        guard var conversation = try? await client.conversation(id: id) else { return }
        if let progress { conversation = conversation.settingFeedbackProgress(progress) }
        upsert(conversation)
    }

    func upsert(_ conversation: YuvaConversation) {
        messages.removeAll { $0.id == conversation.id }
        feedback.removeAll { $0.id == conversation.id }
        if conversation.isFeedback {
            feedback.append(conversation)
            feedback.sort { ($0.lastMessageAt ?? $0.createdAt) > ($1.lastMessageAt ?? $1.createdAt) }
        } else {
            messages.append(conversation)
            messages.sort { ($0.lastMessageAt ?? $0.createdAt) > ($1.lastMessageAt ?? $1.createdAt) }
        }
    }
}

enum ThreadRoute: Hashable {
    case new
    case conversation(String)

    var id: String? {
        if case .conversation(let id) = self { return id }
        return nil
    }
}

public struct YuvaConversationsView: View {
    private let client: YuvaClient
    @State private var model: ConversationsModel
    @State private var path: [ThreadRoute] = []
    @State private var tab: ConversationsTab
    @State private var sendingFeedback = false
    @Binding private var openConversationId: String?
    private let section: YuvaConversationsSection?

    public init(
        client: YuvaClient, openConversationId: Binding<String?> = .constant(nil),
        section: YuvaConversationsSection? = nil
    ) {
        self.client = client
        self.section = section
        _model = State(initialValue: ConversationsModel(client: client))
        _tab = State(initialValue: section == .feedback ? .feedback : .messages)
        _openConversationId = openConversationId
    }

    public var body: some View {
        NavigationStack(path: $path) {
            content
                .safeAreaInset(edge: .top, spacing: 0) { YuvaStatusLine(status: model.session?.inbox.status) }
                .navigationTitle(navigationTitle)
                #if os(iOS)
                .navigationBarTitleDisplayMode(.inline)
                #endif
                .toolbar {
                    ToolbarItem(placement: .principal) { titleView }
                    ToolbarItem(placement: .primaryAction) {
                        if section == .feedback {
                            Button {
                                sendingFeedback = true
                            } label: {
                                Label(L.t("feedback.title"), systemImage: "plus")
                            }
                            .accessibilityIdentifier("yuva.feedback.new")
                        } else {
                            Button {
                                path.append(.new)
                            } label: {
                                Label(L.t("conversations.new"), systemImage: "square.and.pencil")
                            }
                            .accessibilityIdentifier("yuva.conversations.new")
                        }
                    }
                }
                .navigationDestination(for: ThreadRoute.self) { route in
                    YuvaThreadView(client: client, conversationId: route.id)
                }
        }
        .sheet(isPresented: $sendingFeedback) {
            YuvaFeedbackView(
                client: client,
                onDone: {
                    sendingFeedback = false
                    Task { await model.load() }
                },
                onViewFeedback: { conversation in
                    sendingFeedback = false
                    model.upsert(conversation)
                    path = [.conversation(conversation.id)]
                })
        }
        .countsAsOpen()
        .task { await model.load() }
        .task { await model.listen() }
        .onChange(of: openConversationId, initial: true) { _, id in
            guard let id else { return }
            path = [.conversation(id)]
            openConversationId = nil
        }
    }

    private var showsFeedback: Bool { section == .feedback || section == nil && tab == .feedback }

    private var navigationTitle: String {
        if showsFeedback { return L.t("feedback.kind") }
        return model.session?.inbox.name ?? L.t("conversations.title")
    }

    @ViewBuilder
    private var titleView: some View {
        if !showsFeedback, let inbox = model.session?.inbox {
            VStack(spacing: 0) {
                Text(inbox.name).font(.headline).lineLimit(1)
                Text(PresenceHeader.status(inbox)).font(.caption).foregroundStyle(.secondary).lineLimit(1)
            }
            .accessibilityElement(children: .combine)
            .accessibilityAddTraits(.isHeader)
        } else {
            Text(navigationTitle).font(.headline).lineLimit(1).accessibilityAddTraits(.isHeader)
        }
    }

    @ViewBuilder
    private var content: some View {
        if model.loading {
            ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if model.failed {
            ContentUnavailableView {
                Label(L.t("error.generic"), systemImage: "wifi.exclamationmark")
            } actions: {
                Button(L.t("error.retry")) { Task { await model.load() } }
            }
        } else if section == .feedback && model.feedback.isEmpty {
            ContentUnavailableView {
                Label(L.t("feedback.kind"), systemImage: "lightbulb.fill")
            } actions: {
                Button(L.t("feedback.title")) { sendingFeedback = true }
                    .buttonStyle(.borderedProminent)
            }
        } else if section == .messages && model.messages.isEmpty
            || section == nil && model.messages.isEmpty && !model.hasFeedback {
            ContentUnavailableView {
                Label(L.t("conversations.empty"), systemImage: "bubble.left.and.bubble.right.fill")
            } description: {
                Text(L.t("conversations.emptyHint"))
            } actions: {
                Button(L.t("conversations.new")) { path.append(.new) }
                    .buttonStyle(.borderedProminent)
            }
        } else {
            let items = model.items(tab)
            VStack(spacing: 0) {
                if section == nil && model.hasFeedback {
                    Picker("", selection: $tab) {
                        Text(L.t("conversations.messages")).tag(ConversationsTab.messages)
                        Text(L.t("feedback.kind")).tag(ConversationsTab.feedback)
                    }
                    .pickerStyle(.segmented)
                    .labelsHidden()
                    .padding(.horizontal, 16).padding(.bottom, 8)
                    .accessibilityIdentifier("yuva.conversations.tabs")
                }
                List(items) { conversation in
                    Button {
                        path.append(.conversation(conversation.id))
                    } label: {
                        if conversation.isFeedback {
                            FeedbackRow(conversation: conversation)
                        } else {
                            ConversationRow(conversation: conversation, inbox: model.session?.inbox)
                        }
                    }
                    .buttonStyle(.plain)
                    .listRowInsets(EdgeInsets(top: 10, leading: 16, bottom: 10, trailing: 16))
                    .alignmentGuide(.listRowSeparatorLeading) { $0[.leading] + ConversationRowMetrics.separatorInset }
                    .onAppear {
                        if conversation.id == items.last?.id { Task { await model.loadMore(tab) } }
                    }
                }
                .listStyle(.plain)
                .refreshable { await model.load() }
            }
        }
    }
}

enum ConversationRowMetrics {
    static let avatar: CGFloat = 48
    static let spacing: CGFloat = 12
    static let separatorInset: CGFloat = avatar + spacing
}

struct PresenceHeader: View {
    let inbox: YuvaInbox

    var body: some View {
        HStack(spacing: 10) {
            if let members = inbox.presence?.members, !members.isEmpty {
                HStack(spacing: -8) {
                    ForEach(Array(members.prefix(3).enumerated()), id: \.offset) { _, member in
                        AvatarView(initials: member.initials, size: 26)
                            .overlay(Circle().stroke(.background, lineWidth: 2))
                    }
                }
            }
            VStack(alignment: .leading, spacing: 2) {
                Text(inbox.name).font(.subheadline.weight(.semibold))
                Text(PresenceHeader.status(inbox)).font(.caption).foregroundStyle(.secondary)
            }
        }
    }

    static func status(_ inbox: YuvaInbox) -> String {
        if inbox.mode == .live {
            return inbox.presence?.available == true ? L.t("presence.online") : L.t("presence.away")
        }
        if let minutes = inbox.expectedReplyMinutes { return L.f("presence.replyIn", replyDuration(minutes)) }
        return L.t("presence.away")
    }
}

struct UnreadDot: View {
    var body: some View {
        Circle().fill(Color.accentColor).frame(width: 10, height: 10)
            .accessibilityLabel(L.t("conversations.unread"))
    }
}

struct RowLayout<Leading: View, Extra: View>: View {
    let title: String
    let preview: String
    let date: Date
    let unread: Bool
    var previewLines = 2
    @ViewBuilder let leading: Leading
    @ViewBuilder let extra: Extra

    var body: some View {
        HStack(alignment: .top, spacing: ConversationRowMetrics.spacing) {
            leading
            VStack(alignment: .leading, spacing: 2) {
                HStack(alignment: .firstTextBaseline, spacing: 8) {
                    Text(title).font(.headline.weight(unread ? .bold : .semibold)).lineLimit(1)
                    Spacer(minLength: 0)
                    Text(ShortTime.string(date)).font(.subheadline)
                        .foregroundStyle(unread ? Color.accentColor : .secondary)
                }
                HStack(alignment: .top, spacing: 8) {
                    Text(preview).font(.subheadline).foregroundStyle(.secondary)
                        .lineLimit(previewLines, reservesSpace: false)
                        .multilineTextAlignment(.leading)
                    Spacer(minLength: 0)
                    if unread { UnreadDot().padding(.top, 4) }
                }
                extra
            }
        }
        .contentShape(Rectangle())
    }
}

struct ConversationRow: View {
    let conversation: YuvaConversation
    var inbox: YuvaInbox?

    var body: some View {
        RowLayout(
            title: title, preview: preview, date: conversation.lastMessageAt ?? conversation.createdAt,
            unread: conversation.unread
        ) {
            AvatarView(
                initials: String((inbox?.name ?? title).prefix(1)).uppercased(),
                size: ConversationRowMetrics.avatar, imageURL: inbox?.branding.logoUrl)
        } extra: {
            if conversation.status == .closed {
                Text(L.t("status.closed")).font(.caption2).foregroundStyle(.secondary)
            }
        }
    }

    private var title: String {
        if !conversation.subject.isEmpty { return conversation.subject }
        return inbox?.name ?? L.t("thread.title")
    }

    private var preview: String {
        guard let last = conversation.lastMessage else { return "" }
        return last.authorType == .contact ? L.t("thread.you") + ": " + last.text : last.text
    }
}

struct FeedbackRow: View {
    let conversation: YuvaConversation

    var body: some View {
        RowLayout(
            title: conversation.feedback?.displayName ?? L.t("feedback.kind"),
            preview: firstLine, date: conversation.lastMessageAt ?? conversation.createdAt,
            unread: conversation.unread, previewLines: 1
        ) {
            CategoryIcon(key: conversation.feedback?.categoryKey, size: ConversationRowMetrics.avatar)
        } extra: {
            HStack(spacing: 8) {
                if let state = conversation.feedbackState {
                    FeedbackTag(text: state.label, tint: state.tint)
                        .accessibilityIdentifier("yuva.feedback.state." + state.rawValue)
                }
                if (conversation.feedback?.attachmentCount ?? 0) > 0 {
                    Image(systemName: "paperclip").font(.caption).foregroundStyle(.secondary)
                        .accessibilityLabel(L.t("feedback.attachments"))
                }
            }
            .padding(.top, 2)
        }
    }

    private var firstLine: String {
        let own = conversation.feedback?.text ?? ""
        let text = !own.isEmpty ? own : (conversation.subject.isEmpty ? (conversation.lastMessage?.text ?? "") : conversation.subject)
        return text.split(whereSeparator: \.isNewline).first.map(String.init) ?? text
    }
}

private func replyDuration(_ minutes: Int) -> String {
    let (value, unit): (Int, NSCalendar.Unit) =
        minutes < 60 ? (minutes, .minute) : minutes < 1440 ? ((minutes + 30) / 60, .hour) : ((minutes + 720) / 1440, .day)
    let formatter = DateComponentsFormatter()
    formatter.unitsStyle = .full
    formatter.allowedUnits = unit
    var components = DateComponents()
    switch unit {
    case .hour: components.hour = value
    case .day: components.day = value
    default: components.minute = value
    }
    return formatter.string(from: components) ?? "\(value)"
}

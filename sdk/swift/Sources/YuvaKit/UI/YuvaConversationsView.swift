import SwiftUI

@MainActor
@Observable
final class ConversationsModel {
    let client: YuvaClient
    var conversations: [YuvaConversation] = []
    var session: YuvaSession?
    var loading = false
    var failed = false
    private var nextCursor: String?

    init(client: YuvaClient) {
        self.client = client
    }

    func load() async {
        loading = conversations.isEmpty
        defer { loading = false }
        do {
            session = try await client.start()
            let page = try await client.conversations()
            conversations = page.items
            nextCursor = page.nextCursor
            failed = false
        } catch {
            failed = conversations.isEmpty
        }
    }

    func loadMore() async {
        guard let cursor = nextCursor else { return }
        nextCursor = nil
        guard let page = try? await client.conversations(cursor: cursor) else {
            nextCursor = cursor
            return
        }
        let known = Set(conversations.map(\.id))
        conversations += page.items.filter { !known.contains($0.id) }
        nextCursor = page.nextCursor
    }

    func listen() async {
        for await event in client.events() {
            switch event {
            case .conversationCreated(let conversation):
                upsert(conversation)
            case .conversationUpdated(let id, _, _, _), .read(let id, _):
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

    private func refresh(_ id: String) async {
        if let conversation = try? await client.conversation(id: id) { upsert(conversation) }
    }

    private func upsert(_ conversation: YuvaConversation) {
        conversations.removeAll { $0.id == conversation.id }
        conversations.append(conversation)
        conversations.sort { ($0.lastMessageAt ?? $0.createdAt) > ($1.lastMessageAt ?? $1.createdAt) }
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
    @Binding private var openConversationId: String?

    public init(client: YuvaClient, openConversationId: Binding<String?> = .constant(nil)) {
        self.client = client
        _model = State(initialValue: ConversationsModel(client: client))
        _openConversationId = openConversationId
    }

    public var body: some View {
        NavigationStack(path: $path) {
            content
                .navigationTitle(L.t("conversations.title"))
                .toolbar {
                    ToolbarItem(placement: .primaryAction) {
                        Button {
                            path.append(.new)
                        } label: {
                            Label(L.t("conversations.new"), systemImage: "square.and.pencil")
                        }
                        .accessibilityIdentifier("yuva.conversations.new")
                    }
                }
                .navigationDestination(for: ThreadRoute.self) { route in
                    YuvaThreadView(client: client, conversationId: route.id)
                }
        }
        .task { await model.load() }
        .task { await model.listen() }
        .onChange(of: openConversationId, initial: true) { _, id in
            guard let id else { return }
            path = [.conversation(id)]
            openConversationId = nil
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
        } else if model.conversations.isEmpty {
            ContentUnavailableView {
                Label(L.t("conversations.empty"), systemImage: "bubble.left.and.bubble.right")
            } description: {
                Text(L.t("conversations.emptyHint"))
            } actions: {
                Button(L.t("conversations.new")) { path.append(.new) }
                    .buttonStyle(.borderedProminent)
            }
        } else {
            List {
                if let inbox = model.session?.inbox {
                    PresenceHeader(inbox: inbox)
                }
                ForEach(model.conversations) { conversation in
                    NavigationLink(value: ThreadRoute.conversation(conversation.id)) {
                        ConversationRow(conversation: conversation)
                    }
                    .onAppear {
                        if conversation.id == model.conversations.last?.id { Task { await model.loadMore() } }
                    }
                }
            }
            .refreshable { await model.load() }
        }
    }
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
        if let minutes = inbox.expectedReplyMinutes { return L.f("presence.replyIn", Int32(minutes)) }
        return L.t("presence.away")
    }
}

struct ConversationRow: View {
    let conversation: YuvaConversation

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text(title).font(.headline).lineLimit(1)
                Spacer()
                if let date = conversation.lastMessageAt ?? Optional(conversation.createdAt) {
                    Text(date, format: .relative(presentation: .named)).font(.caption).foregroundStyle(.secondary)
                }
            }
            HStack {
                Text(preview).font(.subheadline).foregroundStyle(.secondary).lineLimit(2)
                Spacer()
                if conversation.unread {
                    Circle().fill(Color.accentColor).frame(width: 10, height: 10)
                        .accessibilityLabel(L.t("conversations.unread"))
                }
            }
            if conversation.status == .closed {
                Text(L.t("status.closed")).font(.caption2).foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 2)
    }

    private var title: String {
        if !conversation.subject.isEmpty { return conversation.subject }
        if let feedback = conversation.feedback {
            return L.t("feedback.kind") + " · " + L.t("feedback.category." + feedback.category.rawValue)
        }
        return L.t("thread.title")
    }

    private var preview: String {
        guard let last = conversation.lastMessage else { return "" }
        return last.authorType == .contact ? L.t("thread.you") + ": " + last.text : last.text
    }
}

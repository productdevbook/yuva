import SwiftUI
import UIKit
import YuvaKit

enum Screen: Identifiable {
    case conversations(YuvaConversationsSection?)
    case feedback(YuvaUpload?)

    var id: String {
        switch self {
        case .conversations(.none): "all"
        case .conversations(.messages): "messages"
        case .conversations(.feedback): "feedbackList"
        case .feedback: "feedback"
        }
    }

    init?(launchArgument: String?) {
        switch launchArgument {
        case "all", "messages": self = .conversations(nil)
        case "onlyMessages": self = .conversations(.messages)
        case "onlyFeedback": self = .conversations(.feedback)
        case "feedback": self = .feedback(nil)
        default: return nil
        }
    }
}

struct ContentView: View {
    let config: ExampleConfig
    @AppStorage("userId") private var userId = ""
    @AppStorage("identified") private var identified = false
    @State private var client: YuvaClient?
    @State private var swap: Task<Void, Never>?
    @State private var screen: Screen?
    @State private var openConversationId: String?

    init(config: ExampleConfig) {
        self.config = config
        let defaults = UserDefaults.standard
        _client = State(
            initialValue: Self.makeClient(
                config: config, identified: defaults.bool(forKey: "identified"),
                userId: defaults.string(forKey: "userId") ?? ""))
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Picker("Mode", selection: $identified) {
                        Text("Anonymous").tag(false)
                        Text("Identified").tag(true)
                    }
                    .pickerStyle(.segmented)
                    if identified {
                        TextField("Host user id", text: $userId)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                    }
                } header: {
                    Text("Mode")
                } footer: {
                    Text(identified ? "Signed in through the identity token URL." : "A device session without sign-in.")
                }
                Section("Support") {
                    ScreenRow("Messages and feedback", systemImage: "bubble.left.and.bubble.right.fill", tint: .blue) {
                        screen = .conversations(nil)
                    }
                    .accessibilityIdentifier("example.messages")
                    ScreenRow("Messages only", systemImage: "bubble.left.fill", tint: .teal) {
                        screen = .conversations(.messages)
                    }
                    .accessibilityIdentifier("example.messagesOnly")
                    ScreenRow("Feedback only", systemImage: "lightbulb.fill", tint: .orange) {
                        screen = .conversations(.feedback)
                    }
                    .accessibilityIdentifier("example.feedbackList")
                    ScreenRow("Send feedback", systemImage: "paperplane.fill", tint: .pink) {
                        screen = .feedback(captureScreen())
                    }
                    .accessibilityIdentifier("example.feedback")
                }
                .disabled(client == nil)
                if identified, !userId.isEmpty, let client {
                    NewsletterSection(client: client)
                        .id(ObjectIdentifier(client))
                }
                Section("Server") {
                    LabeledContent("URL", value: config.serverURL.absoluteString)
                    LabeledContent("YuvaKit", value: Yuva.version)
                }
            }
            .navigationTitle("Yuva Example")
        }
        .sheet(item: $screen) { screen in
            if let client {
                switch screen {
                case .conversations(let section):
                    YuvaConversationsView(client: client, openConversationId: $openConversationId, section: section)
                case .feedback(let screenshot):
                    YuvaFeedbackView(client: client, screenshot: screenshot, screen: "example.home") { self.screen = nil }
                }
            }
        }
        .onChange(of: identified) { old, new in
            replaceClient(signOut: old && !new)
        }
        .onChange(of: userId) { _, _ in
            replaceClient(signOut: false)
        }
        .onReceive(NotificationCenter.default.publisher(for: .exampleOpenConversation)) { note in
            openConversationId = note.object as? String
            screen = .conversations(nil)
        }
        .onAppear {
            enableShake()
            let defaults = UserDefaults.standard
            openConversationId = defaults.string(forKey: "YuvaThread")
            screen = Screen(launchArgument: defaults.string(forKey: "YuvaOpen"))
                ?? (openConversationId == nil ? nil : .conversations(nil))
        }
    }

    private func replaceClient(signOut: Bool) {
        let previous = client
        let pending = swap
        client = nil
        screen = nil
        Yuva.disableShakeToFeedback()
        swap = Task {
            await pending?.value
            if signOut, let old = client ?? previous { await old.signOut() }
            client = Self.makeClient(config: config, identified: identified, userId: userId)
            enableShake()
        }
    }

    private func enableShake() {
        guard let client else { return }
        Yuva.enableShakeToFeedback(client: client, screen: { ExampleRoute.name })
    }

    private static func makeClient(config: ExampleConfig, identified: Bool, userId user: String) -> YuvaClient {
        let tokenURL = config.identityTokenURL
        var provider: (@Sendable () async throws -> String?)?
        if identified, !user.isEmpty, let tokenURL {
            provider = { @Sendable () async throws -> String? in
                var url = tokenURL
                url.append(queryItems: [URLQueryItem(name: "sub", value: user)])
                let (data, _) = try await URLSession.shared.data(from: url)
                return String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
            }
        }
        return YuvaClient(
            configuration: YuvaConfiguration(serverURL: config.serverURL, channelKey: config.channelKey, identityToken: provider))
    }
}

struct ScreenRow: View {
    let title: String
    let systemImage: String
    let tint: Color
    let opensScreen: Bool
    let action: () -> Void

    init(
        _ title: String, systemImage: String, tint: Color, opensScreen: Bool = true, action: @escaping () -> Void
    ) {
        self.title = title
        self.systemImage = systemImage
        self.tint = tint
        self.opensScreen = opensScreen
        self.action = action
    }

    var body: some View {
        Button(action: action) {
            HStack {
                Label {
                    Text(title).foregroundStyle(.primary)
                } icon: {
                    SettingsIcon(systemImage: systemImage, tint: tint)
                }
                Spacer()
                if opensScreen {
                    Image(systemName: "chevron.forward").font(.footnote.weight(.semibold)).foregroundStyle(.tertiary)
                }
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }
}

struct SettingsIcon: View {
    let systemImage: String
    let tint: Color

    var body: some View {
        Image(systemName: systemImage)
            .font(.system(size: 14, weight: .semibold))
            .foregroundStyle(.white)
            .frame(width: 30, height: 30)
            .background(tint.gradient, in: RoundedRectangle(cornerRadius: 8, style: .continuous))
    }
}

struct NewsletterSection: View {
    let client: YuvaClient
    @State private var subscriptions: [YuvaSubscription] = []
    @State private var busy: Set<String> = []
    @State private var notice: String?

    var body: some View {
        Section {
            ForEach(subscriptions) { subscription in
                Toggle(isOn: binding(subscription)) {
                    VStack(alignment: .leading) {
                        Text(subscription.list.name)
                        if !subscription.list.description.isEmpty {
                            Text(subscription.list.description).font(.caption).foregroundStyle(.secondary)
                        }
                        if subscription.state == .pending {
                            Text("Waiting for the confirmation e-mail").font(.caption).foregroundStyle(.orange)
                        }
                    }
                }
                .disabled(busy.contains(subscription.id))
                .accessibilityIdentifier("example.newsletter.\(subscription.list.name)")
            }
            if subscriptions.isEmpty, notice == nil {
                Text("No public lists").foregroundStyle(.secondary)
            }
        } header: {
            Text("Newsletter")
        } footer: {
            if let notice { Text(notice) }
        }
        .task { await load() }
    }

    private func binding(_ subscription: YuvaSubscription) -> Binding<Bool> {
        Binding(
            get: { subscription.state == .subscribed || subscription.state == .pending },
            set: { on in Task { await toggle(subscription, on: on) } })
    }

    private func load() async {
        do {
            subscriptions = try await client.subscriptions()
        } catch {
            notice = error.localizedDescription
        }
    }

    private func toggle(_ subscription: YuvaSubscription, on: Bool) async {
        let list = subscription.list
        busy.insert(list.id)
        defer { busy.remove(list.id) }
        do {
            if on {
                let consent = list.description.isEmpty ? list.name : "\(list.name): \(list.description)"
                let state = try await client.subscribe(listId: list.id, consentText: consent)
                notice = state == .pending
                    ? "We sent a confirmation e-mail for \(list.name)."
                    : "Subscribed to \(list.name)."
            } else {
                try await client.unsubscribe(listId: list.id)
                notice = "Unsubscribed from \(list.name)."
            }
        } catch {
            notice = error.localizedDescription
        }
        if let fresh = try? await client.subscriptions() { subscriptions = fresh }
    }
}

@MainActor
func captureScreen() -> YuvaUpload? {
    guard let window = UIApplication.shared.connectedScenes.compactMap({ $0 as? UIWindowScene })
        .flatMap(\.windows).first(where: \.isKeyWindow)
    else { return nil }
    let image = UIGraphicsImageRenderer(bounds: window.bounds).image { _ in
        window.drawHierarchy(in: window.bounds, afterScreenUpdates: false)
    }
    return image.jpegData(compressionQuality: 0.8).map {
        YuvaUpload(filename: "screenshot.jpg", contentType: "image/jpeg", data: $0)
    }
}

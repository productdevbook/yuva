import PhotosUI
import SwiftUI

public struct YuvaFeedbackView: View {
    private let client: YuvaClient
    private let screen: String?
    private let onDone: () -> Void
    private let onViewFeedback: ((YuvaConversation) -> Void)?
    @State private var categories: [YuvaFeedbackCategoryItem] = YuvaFeedbackCategory.allCases.map(\.item)
    @State private var category: String = YuvaFeedbackCategory.bug.rawValue
    @State private var message = ""
    @State private var screenshots: [YuvaUpload]
    @State private var picked: [PhotosPickerItem] = []
    @State private var allowEmail = false
    @State private var knownEmail: String?
    @State private var email = ""
    @State private var sending = false
    @State private var sent = false
    @State private var failure: String?
    @State private var attachNotice: String?
    @State private var limits: YuvaAttachmentLimits?
    @State private var clientId = UUID().uuidString
    @State private var started: YuvaConversation?
    @State private var path: [String] = []

    public init(
        client: YuvaClient, screenshot: YuvaUpload? = nil, screen: String? = nil,
        onDone: @escaping () -> Void = {}, onViewFeedback: ((YuvaConversation) -> Void)? = nil
    ) {
        self.client = client
        self.screen = screen
        self.onDone = onDone
        self.onViewFeedback = onViewFeedback
        _screenshots = State(initialValue: screenshot.map { [$0] } ?? [])
    }

    public var body: some View {
        NavigationStack(path: $path) {
            Group {
                if sent {
                    ContentUnavailableView {
                        Label(L.t("feedback.sent"), systemImage: "checkmark.circle")
                    } description: {
                        Text(L.t("feedback.sentHint"))
                    } actions: {
                        Button(L.t("feedback.done"), action: onDone).buttonStyle(.borderedProminent)
                        if let started {
                            Button(L.t("feedback.view")) {
                                if let onViewFeedback { onViewFeedback(started) } else { path.append(started.id) }
                            }
                            .accessibilityIdentifier("yuva.feedback.view")
                        }
                    }
                } else {
                    form
                }
            }
            .navigationDestination(for: String.self) { id in
                YuvaThreadView(client: client, conversationId: id)
            }
            .navigationTitle(L.t("feedback.title"))
            #if os(iOS)
            .navigationBarTitleDisplayMode(.inline)
            #endif
            .toolbar {
                if !sent {
                    ToolbarItem(placement: .cancellationAction) {
                        Button(L.t("feedback.cancel"), action: onDone)
                    }
                    ToolbarItem(placement: .confirmationAction) {
                        Button(L.t("feedback.send")) { Task { await send() } }
                            .disabled(!canSend)
                            .accessibilityIdentifier("yuva.feedback.send")
                    }
                }
            }
        }
        .countsAsOpen()
        .task {
            guard let session = try? await client.start() else { return }
            offer(session.inbox)
            knownEmail = session.contact.email
        }
        .task {
            for await event in client.events() {
                if case .inboxUpdated(let inbox) = event { offer(inbox) }
            }
        }
    }

    private func offer(_ inbox: YuvaInbox) {
        limits = inbox.attachments
        if let limits = inbox.attachments {
            let refusal = screenshots.lazy.compactMap(limits.refusal(for:)).first
            screenshots.removeAll { limits.refusal(for: $0) != nil }
            if let refusal { attachNotice = L.message(for: refusal) }
        }
        let items = inbox.feedbackCategoryItems
        let offered = items.isEmpty ? inbox.feedbackCategories.map(\.item) : items
        guard !offered.isEmpty else { return }
        categories = offered
        if !offered.contains(where: { $0.key == category }), let first = offered.first { category = first.key }
    }

    private var showsScreenshots: Bool {
        limits?.allowsImages != false || !screenshots.isEmpty || attachNotice != nil
    }

    private var needsEmail: Bool { allowEmail && knownEmail == nil }

    private var canSend: Bool {
        guard !sending else { return false }
        let hasContent = !message.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || !screenshots.isEmpty
        return hasContent && (!needsEmail || email.contains("@"))
    }

    private var form: some View {
        Form {
            Section(L.t("feedback.category")) {
                LazyVGrid(columns: [GridItem(.adaptive(minimum: 130), spacing: 8)], spacing: 8) {
                    ForEach(categories) { option in
                        categoryChip(option)
                    }
                }
                .listRowInsets(EdgeInsets(top: 8, leading: 12, bottom: 8, trailing: 12))
                .listRowBackground(Color.clear)
            }
            Section(L.t("feedback.message")) {
                TextField(L.t("feedback.placeholder"), text: $message, axis: .vertical)
                    .lineLimit(4...12)
                    .accessibilityIdentifier("yuva.feedback.message")
            }
            if showsScreenshots {
                Section(L.t("feedback.screenshot")) {
                    ForEach(Array(screenshots.enumerated()), id: \.offset) { index, upload in
                        if let image = PlatformImage(data: upload.data) {
                            HStack {
                                Image(platformImage: image).resizable().scaledToFit().frame(height: 120)
                                    .clipShape(RoundedRectangle(cornerRadius: 8))
                                Spacer()
                                Button(role: .destructive) {
                                    screenshots.remove(at: index)
                                } label: {
                                    Image(systemName: "trash")
                                }
                                .accessibilityLabel(L.t("feedback.removeScreenshot"))
                            }
                        }
                    }
                    if limits?.allowsImages != false {
                        PhotosPicker(selection: $picked, maxSelectionCount: 10, matching: .images) {
                            Label(L.t("feedback.chooseScreenshot"), systemImage: "photo")
                        }
                    }
                    if let attachNotice {
                        Text(attachNotice).font(.footnote).foregroundStyle(.red)
                            .accessibilityIdentifier("yuva.feedback.attachNotice")
                    }
                }
            }
            Section {
                Toggle(L.t("feedback.allowEmail"), isOn: $allowEmail)
                    .accessibilityIdentifier("yuva.feedback.allowEmail")
                if needsEmail {
                    TextField(L.t("feedback.email"), text: $email)
                        .textContentType(.emailAddress)
                        #if os(iOS)
                        .keyboardType(.emailAddress)
                        .textInputAutocapitalization(.never)
                        #endif
                        .autocorrectionDisabled()
                        .accessibilityIdentifier("yuva.feedback.email")
                }
            } footer: {
                Text(L.t("feedback.metadataHint"))
            }
            if let failure {
                Section {
                    Text(failure).foregroundStyle(.red).accessibilityIdentifier("yuva.feedback.error")
                }
            }
        }
        .disabled(sending)
        .onChange(of: picked) { _, items in
            guard !items.isEmpty else { return }
            picked = []
            Task {
                attachNotice = nil
                for item in items {
                    guard let data = try? await item.loadTransferable(type: Data.self),
                          let upload = ImageUpload.make(from: data, name: "screenshot") else { continue }
                    if let refusal = limits?.refusal(for: upload) {
                        attachNotice = L.message(for: refusal)
                    } else {
                        screenshots.append(upload)
                    }
                }
            }
        }
    }

    private func categoryChip(_ option: YuvaFeedbackCategoryItem) -> some View {
        let selected = option.key == category
        let tint = CategoryStyle.tint(key: option.key)
        return Button {
            category = option.key
        } label: {
            Label(option.name, systemImage: CategoryStyle.symbol(key: option.key, icon: option.icon))
                .font(.subheadline.weight(.medium))
                .lineLimit(1)
                .minimumScaleFactor(0.8)
                .frame(maxWidth: .infinity)
                .padding(.vertical, 10)
                .foregroundStyle(selected ? Color.white : tint)
                .background(RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .fill(selected ? tint : tint.opacity(0.14)))
        }
        .buttonStyle(.plain)
        .accessibilityAddTraits(selected ? .isSelected : [])
    }

    private func send() async {
        sending = true
        failure = nil
        defer { sending = false }
        do {
            let result = try await client.sendFeedback(
                YuvaFeedback(
                    categoryKey: category, body: message.trimmingCharacters(in: .whitespacesAndNewlines),
                    screenshots: screenshots, allowEmail: allowEmail, email: needsEmail ? email : nil, screen: screen,
                    clientId: clientId))
            started = result.conversation
            sent = true
        } catch {
            failure = L.message(for: error)
        }
    }
}

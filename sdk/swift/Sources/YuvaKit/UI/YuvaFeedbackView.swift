import PhotosUI
import SwiftUI

public struct YuvaFeedbackView: View {
    private let client: YuvaClient
    private let screen: String?
    private let onDone: () -> Void
    @State private var categories: [YuvaFeedbackCategory] = YuvaFeedbackCategory.allCases
    @State private var category: YuvaFeedbackCategory = .bug
    @State private var message = ""
    @State private var screenshots: [YuvaUpload]
    @State private var picked: [PhotosPickerItem] = []
    @State private var allowEmail = false
    @State private var knownEmail: String?
    @State private var email = ""
    @State private var sending = false
    @State private var sent = false
    @State private var failed = false
    @State private var clientId = UUID().uuidString

    public init(client: YuvaClient, screenshot: YuvaUpload? = nil, screen: String? = nil, onDone: @escaping () -> Void = {}) {
        self.client = client
        self.screen = screen
        self.onDone = onDone
        _screenshots = State(initialValue: screenshot.map { [$0] } ?? [])
    }

    public var body: some View {
        NavigationStack {
            Group {
                if sent {
                    ContentUnavailableView {
                        Label(L.t("feedback.sent"), systemImage: "checkmark.circle")
                    } description: {
                        Text(L.t("feedback.sentHint"))
                    } actions: {
                        Button(L.t("feedback.done"), action: onDone).buttonStyle(.borderedProminent)
                    }
                } else {
                    form
                }
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
        .task {
            guard let session = try? await client.start() else { return }
            let offered = session.inbox.feedbackCategories
            if !offered.isEmpty {
                categories = offered
                if !offered.contains(category), let first = offered.first { category = first }
            }
            knownEmail = session.contact.email
        }
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
                Picker(L.t("feedback.category"), selection: $category) {
                    ForEach(categories, id: \.self) { category in
                        Text(L.t("feedback.category." + category.rawValue)).tag(category)
                    }
                }
                .pickerStyle(.segmented)
                .labelsHidden()
            }
            Section(L.t("feedback.message")) {
                TextField(L.t("feedback.placeholder"), text: $message, axis: .vertical)
                    .lineLimit(4...12)
                    .accessibilityIdentifier("yuva.feedback.message")
            }
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
                PhotosPicker(selection: $picked, maxSelectionCount: 10, matching: .images) {
                    Label(L.t("feedback.chooseScreenshot"), systemImage: "photo")
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
            if failed {
                Section {
                    Text(L.t("error.generic")).foregroundStyle(.red)
                }
            }
        }
        .disabled(sending)
        .onChange(of: picked) { _, items in
            guard !items.isEmpty else { return }
            picked = []
            Task {
                for item in items {
                    if let data = try? await item.loadTransferable(type: Data.self),
                       let upload = ImageUpload.make(from: data, name: "screenshot") {
                        screenshots.append(upload)
                    }
                }
            }
        }
    }

    private func send() async {
        sending = true
        failed = false
        defer { sending = false }
        do {
            try await client.sendFeedback(
                YuvaFeedback(
                    category: category, body: message.trimmingCharacters(in: .whitespacesAndNewlines),
                    screenshots: screenshots, allowEmail: allowEmail, email: needsEmail ? email : nil, screen: screen,
                    clientId: clientId))
            sent = true
        } catch {
            failed = true
        }
    }
}

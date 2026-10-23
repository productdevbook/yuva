import SwiftUI
import UniformTypeIdentifiers
#if canImport(UIKit)
import UIKit
typealias PlatformImage = UIImage
#else
import AppKit
typealias PlatformImage = NSImage
#endif

enum L {
    static func t(_ key: String) -> String {
        String(localized: String.LocalizationValue(key), bundle: .module)
    }

    static func f(_ key: String, _ args: CVarArg...) -> String {
        String(format: t(key), arguments: args)
    }

    static func message(for error: Error) -> String {
        if error is URLError { return t("error.network") }
        guard let error = error as? YuvaError else { return t("error.generic") }
        switch error.code {
        case "attachment_too_large": return t("error.attachmentTooLarge")
        case "attachment_type_not_allowed": return t("error.attachmentType")
        case "attachment_type_mismatch": return t("error.attachmentMismatch")
        default: break
        }
        switch error.status {
        case 413: return t("error.attachmentTooLarge")
        case 415: return t("error.attachmentType")
        default: return t("error.generic")
        }
    }
}

extension Image {
    init(platformImage: PlatformImage) {
        #if canImport(UIKit)
        self.init(uiImage: platformImage)
        #else
        self.init(nsImage: platformImage)
        #endif
    }
}

extension Color {
    init?(yuvaHex: String?) {
        guard let hex = yuvaHex, hex.count == 7, hex.hasPrefix("#"), let value = UInt32(hex.dropFirst(), radix: 16) else {
            return nil
        }
        self.init(
            red: Double((value >> 16) & 0xFF) / 255, green: Double((value >> 8) & 0xFF) / 255,
            blue: Double(value & 0xFF) / 255)
    }
}

enum ImageUpload {
    static func make(from data: Data, name: String = "image") -> YuvaUpload? {
        guard let image = PlatformImage(data: data) else { return nil }
        #if canImport(UIKit)
        guard let jpeg = image.jpegData(compressionQuality: 0.8) else { return nil }
        #else
        guard let tiff = image.tiffRepresentation, let rep = NSBitmapImageRep(data: tiff),
              let jpeg = rep.representation(using: .jpeg, properties: [.compressionFactor: 0.8])
        else { return nil }
        #endif
        return YuvaUpload(filename: name + ".jpg", contentType: "image/jpeg", data: jpeg)
    }
}

struct AvatarView: View {
    let initials: String
    var size: CGFloat = 28
    var imageURL: String?

    var body: some View {
        Text(initials.isEmpty ? "?" : initials)
            .font(.system(size: size * 0.4, weight: .semibold))
            .foregroundStyle(.white)
            .frame(width: size, height: size)
            .background(Circle().fill(Color.accentColor.gradient))
            .overlay {
                if let url = imageURL.flatMap(URL.init(string:)), url.scheme?.hasPrefix("http") == true {
                    AsyncImage(url: url) { phase in
                        if let image = phase.image { image.resizable().scaledToFill() }
                    }
                    .clipShape(Circle())
                }
            }
            .accessibilityHidden(true)
    }
}

struct CategoryIcon: View {
    let key: String?
    var icon: String?
    var size: CGFloat = 44

    var body: some View {
        Image(systemName: key.map { CategoryStyle.symbol(key: $0, icon: icon) } ?? "bubble.left.fill")
            .font(.system(size: size * 0.42, weight: .semibold))
            .foregroundStyle(.white)
            .frame(width: size, height: size)
            .background(Circle().fill((key.map(CategoryStyle.tint) ?? .gray).gradient))
            .accessibilityHidden(true)
    }
}

enum CategoryStyle {
    private static let lucide: [String: String] = [
        "bug": "ladybug.fill", "lightbulb": "lightbulb.fill", "heart": "heart.fill",
        "message-circle": "ellipsis.bubble.fill", "star": "star.fill", "palette": "paintpalette.fill",
        "zap": "bolt.fill", "wrench": "wrench.fill", "settings": "gearshape.fill", "mail": "envelope.fill",
        "user": "person.fill", "lock": "lock.fill", "shield": "shield.fill", "bell": "bell.fill",
        "credit-card": "creditcard.fill", "thumbs-up": "hand.thumbsup.fill", "thumbs-down": "hand.thumbsdown.fill",
        "alert-triangle": "exclamationmark.triangle.fill", "triangle-alert": "exclamationmark.triangle.fill",
        "info": "info.circle.fill", "help-circle": "questionmark.circle.fill", "circle-help": "questionmark.circle.fill",
        "search": "magnifyingglass", "home": "house.fill", "house": "house.fill", "globe": "globe",
        "smartphone": "iphone", "camera": "camera.fill", "image": "photo.fill", "file-text": "doc.text.fill",
        "gauge": "gauge.with.dots.needle.67percent", "rocket": "paperplane.fill", "sparkles": "sparkles",
    ]
    private static let palette: [Color] = [.teal, .indigo, .mint, .purple, .cyan, .brown]

    static func symbol(key: String, icon: String?) -> String {
        if let builtin = YuvaFeedbackCategory(rawValue: key) { return builtin.symbol }
        return icon.flatMap { lucide[$0] } ?? "tag.fill"
    }

    static func tint(key: String) -> Color {
        if let builtin = YuvaFeedbackCategory(rawValue: key) { return builtin.tint }
        return palette[key.utf8.reduce(0) { ($0 &* 31 &+ Int($1)) & 0xFFFF } % palette.count]
    }
}

extension YuvaFeedbackInfo {
    var displayName: String {
        categoryName ?? L.t("feedback.category." + category.rawValue)
    }
}

extension YuvaFeedbackCategory {
    var item: YuvaFeedbackCategoryItem {
        YuvaFeedbackCategoryItem(key: rawValue, name: L.t("feedback.category." + rawValue), icon: nil)
    }

    var symbol: String {
        switch self {
        case .bug: "ladybug.fill"
        case .idea: "lightbulb.fill"
        case .praise: "heart.fill"
        case .other: "ellipsis.bubble.fill"
        }
    }

    var tint: Color {
        switch self {
        case .bug: .red
        case .idea: .orange
        case .praise: .pink
        case .other: .gray
        }
    }
}

struct FeedbackTag: View {
    let text: String
    let tint: Color

    var body: some View {
        Text(text)
            .font(.caption2.weight(.semibold))
            .foregroundStyle(tint)
            .padding(.horizontal, 8).padding(.vertical, 3)
            .background(Capsule().fill(tint.opacity(0.15)))
    }
}

extension YuvaFeedbackState {
    var label: String { L.t("feedback.state." + rawValue) }

    var tint: Color {
        switch self {
        case .sent: .secondary
        case .answered: .accentColor
        case .resolved, .done: .green
        case .planned: Color(red: 0.35, green: 0.40, blue: 0.85)
        case .inProgress: .orange
        case .wontDo: .gray
        }
    }
}

enum ShortTime {
    static func string(_ date: Date, now: Date = Date(), calendar: Calendar = .current) -> String {
        if calendar.isDate(date, inSameDayAs: now) {
            return date.formatted(date: .omitted, time: .shortened)
        }
        if calendar.isDate(date, inSameDayAs: calendar.date(byAdding: .day, value: -1, to: now) ?? now) {
            return relativeDay(date, style: .short)
        }
        let start = calendar.startOfDay(for: now)
        if let days = calendar.dateComponents([.day], from: calendar.startOfDay(for: date), to: start).day,
           days > 0, days < 7 {
            return date.formatted(.dateTime.weekday(.wide))
        }
        return date.formatted(date: .numeric, time: .omitted)
    }

    static func daySeparator(_ date: Date) -> String {
        relativeDay(date, style: .medium)
    }

    static func clock(_ date: Date) -> String {
        date.formatted(date: .omitted, time: .shortened)
    }

    private static func relativeDay(_ date: Date, style: DateFormatter.Style) -> String {
        let formatter = DateFormatter()
        formatter.dateStyle = style
        formatter.timeStyle = .none
        formatter.doesRelativeDateFormatting = true
        return formatter.string(from: date)
    }
}

@MainActor
final class AttachmentCache {
    static let shared = AttachmentCache()
    private var images: [String: PlatformImage] = [:]

    func image(_ id: String, client: YuvaClient) async -> PlatformImage? {
        if let image = images[id] { return image }
        guard let data = try? await client.attachment(id: id), let image = PlatformImage(data: data) else { return nil }
        images[id] = image
        return image
    }
}

struct AttachmentImageView: View {
    let client: YuvaClient
    let attachment: YuvaAttachment
    var open: (() -> Void)?
    @State private var image: PlatformImage?

    var body: some View {
        content
            .contentShape(RoundedRectangle(cornerRadius: 12))
            .onTapGesture { if image != nil { open?() } }
            .accessibilityElement()
            .accessibilityLabel(attachment.filename)
            .accessibilityAddTraits(open == nil ? .isImage : [.isImage, .isButton])
            .accessibilityAction { open?() }
            .accessibilityHint(open == nil ? "" : L.t("thread.openImage"))
            .accessibilityIdentifier("yuva.thread.image")
    }

    private var content: some View {
        Group {
            if let image {
                Image(platformImage: image).resizable().scaledToFit()
            } else {
                RoundedRectangle(cornerRadius: 12).fill(.quaternary).frame(width: 180, height: 140)
                    .overlay(ProgressView())
            }
        }
        .frame(maxWidth: 220, maxHeight: 260)
        .clipShape(RoundedRectangle(cornerRadius: 12))
        .task(id: attachment.id) {
            for attempt in 1...3 {
                image = await AttachmentCache.shared.image(attachment.id, client: client)
                if image != nil || Task.isCancelled { return }
                try? await Task.sleep(for: .seconds(2 * attempt))
            }
        }
    }
}

struct AttachmentFileView: View {
    let client: YuvaClient
    let attachment: YuvaAttachment
    @State private var url: URL?
    @State private var loading = false

    var body: some View {
        Group {
            if let url {
                ShareLink(item: url) { label(systemImage: "square.and.arrow.up") }
            } else {
                Button {
                    Task { await download() }
                } label: {
                    label(systemImage: loading ? "hourglass" : "paperclip")
                }
                .buttonStyle(.plain)
            }
        }
    }

    private func label(systemImage: String) -> some View {
        HStack(spacing: 6) {
            Image(systemName: systemImage)
            Text(attachment.filename).lineLimit(1)
            Text(ByteCountFormatter.string(fromByteCount: attachment.size, countStyle: .file))
                .foregroundStyle(.secondary)
        }
        .font(.footnote)
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .background(Capsule().fill(.quaternary))
    }

    private func download() async {
        loading = true
        defer { loading = false }
        guard let data = try? await client.attachment(id: attachment.id) else { return }
        let directory = FileManager.default.temporaryDirectory.appending(path: attachment.id)
        try? FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let file = directory.appending(path: attachment.filename)
        if (try? data.write(to: file)) != nil { url = file }
    }
}

enum Links {
    private static let detector = try? NSDataDetector(types: NSTextCheckingResult.CheckingType.link.rawValue)

    static func attributed(_ text: String) -> AttributedString {
        var result = AttributedString(text)
        guard let detector, text.contains("://") || text.localizedCaseInsensitiveContains("www.") else { return result }
        let range = NSRange(text.startIndex..., in: text)
        for match in detector.matches(in: text, range: range) {
            guard let url = match.url, url.scheme?.hasPrefix("http") == true,
                  let swiftRange = Range(match.range, in: text),
                  let lower = AttributedString.Index(swiftRange.lowerBound, within: result),
                  let upper = AttributedString.Index(swiftRange.upperBound, within: result)
            else { continue }
            result[lower..<upper].link = url
            result[lower..<upper].underlineStyle = .single
        }
        return result
    }
}

struct ImageViewer: View {
    let client: YuvaClient
    let attachment: YuvaAttachment
    @Environment(\.dismiss) private var dismiss
    @State private var image: PlatformImage?
    @State private var scale: CGFloat = 1
    @State private var settled: CGFloat = 1
    @State private var offset: CGSize = .zero
    @State private var dragged: CGSize = .zero

    var body: some View {
        ZStack {
            Color.black.ignoresSafeArea()
            if let image {
                Image(platformImage: image).resizable().scaledToFit()
                    .scaleEffect(scale)
                    .offset(offset)
                    .gesture(
                        MagnifyGesture()
                            .onChanged { scale = min(max(settled * $0.magnification, 1), 5) }
                            .onEnded { _ in
                                settled = scale
                                if scale == 1 { withAnimation(.snappy) { offset = .zero; dragged = .zero } }
                            }
                            .simultaneously(with: DragGesture()
                                .onChanged { value in
                                    guard scale > 1 else { return }
                                    offset = CGSize(
                                        width: dragged.width + value.translation.width,
                                        height: dragged.height + value.translation.height)
                                }
                                .onEnded { _ in dragged = offset })
                    )
                    .onTapGesture(count: 2) {
                        withAnimation(.snappy) {
                            scale = scale > 1 ? 1 : 2.5
                            settled = scale
                            offset = .zero
                            dragged = .zero
                        }
                    }
                    .accessibilityLabel(attachment.filename)
                    .accessibilityAddTraits(.isImage)
            } else {
                ProgressView().tint(.white)
            }
        }
        .overlay(alignment: .top) {
            HStack {
                Button { dismiss() } label: {
                    Image(systemName: "xmark").font(.body.weight(.semibold)).frame(width: 44, height: 44)
                }
                .glassCircle()
                .accessibilityLabel(L.t("thread.close"))
                .accessibilityIdentifier("yuva.thread.imageViewer.close")
                Spacer()
                if let image {
                    let picture = Image(platformImage: image)
                    ShareLink(item: picture, preview: SharePreview(attachment.filename, image: picture)) {
                        Image(systemName: "square.and.arrow.up").font(.body.weight(.semibold)).frame(width: 44, height: 44)
                    }
                    .glassCircle()
                }
            }
            .foregroundStyle(.white)
            .padding(.horizontal, 16)
        }
        .accessibilityIdentifier("yuva.thread.imageViewer")
        .task { image = await AttachmentCache.shared.image(attachment.id, client: client) }
    }
}

struct TypingDots: View {
    var body: some View {
        TimelineView(.animation) { context in
            let time = context.date.timeIntervalSinceReferenceDate
            HStack(spacing: 5) {
                ForEach(0..<3, id: \.self) { index in
                    let phase = (time * 2.2 - Double(index) * 0.25).truncatingRemainder(dividingBy: 1.4)
                    let lift = phase < 0.6 ? sin(phase / 0.6 * .pi) : 0
                    Circle().frame(width: 8, height: 8)
                        .opacity(0.4 + 0.6 * lift)
                        .offset(y: -4 * lift)
                }
            }
        }
        .foregroundStyle(.secondary)
    }
}

extension View {
    @ViewBuilder
    func bottomBar<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        if #available(iOS 26.0, macOS 26.0, *) {
            safeAreaBar(edge: .bottom, spacing: 0, content: content)
        } else {
            safeAreaInset(edge: .bottom, spacing: 0, content: content)
        }
    }

    @ViewBuilder
    func glassCircle() -> some View {
        if #available(iOS 26.0, macOS 26.0, *) {
            buttonStyle(.plain).glassEffect(.regular.interactive(), in: .circle)
        } else {
            buttonStyle(.plain).background(.ultraThinMaterial, in: Circle())
        }
    }

    @ViewBuilder
    func glassCapsule() -> some View {
        if #available(iOS 26.0, macOS 26.0, *) {
            glassEffect(.regular.interactive(), in: .rect(cornerRadius: 22))
        } else {
            background(.background, in: RoundedRectangle(cornerRadius: 22))
                .overlay(RoundedRectangle(cornerRadius: 22).stroke(.separator, lineWidth: 1))
        }
    }
}

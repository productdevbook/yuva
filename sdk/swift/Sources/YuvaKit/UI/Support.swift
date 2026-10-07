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

    var body: some View {
        Text(initials.isEmpty ? "?" : initials)
            .font(.system(size: size * 0.4, weight: .semibold))
            .foregroundStyle(.white)
            .frame(width: size, height: size)
            .background(Circle().fill(Color.accentColor.gradient))
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
    @State private var image: PlatformImage?

    var body: some View {
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

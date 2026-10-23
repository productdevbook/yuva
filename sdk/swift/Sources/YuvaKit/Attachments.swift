import Foundation

public enum YuvaAttachmentRefusal: Sendable, Equatable {
    case tooLarge(maxBytes: Int64)
    case typeNotAllowed
}

public struct YuvaAttachmentLimits: Codable, Sendable, Hashable {
    public let maxBytes: Int64
    public let types: [String]

    public init(maxBytes: Int64, types: [String]) {
        self.maxBytes = maxBytes
        self.types = types
    }

    public init(from decoder: any Decoder) throws {
        let container = try? decoder.container(keyedBy: CodingKeys.self)
        maxBytes = (try? container?.decode(Int64.self, forKey: .maxBytes)) ?? Int64.max
        types = (try? container?.decode([String].self, forKey: .types)) ?? ["*/*"]
    }

    enum CodingKeys: String, CodingKey { case maxBytes, types }

    public var acceptsFiles: Bool { !types.isEmpty }

    public func allows(contentType: String) -> Bool {
        let type = contentType.split(separator: ";").first.map { $0.trimmingCharacters(in: .whitespaces).lowercased() } ?? ""
        return types.contains { entry in
            let entry = entry.lowercased()
            if entry == "*/*" || entry == type { return true }
            return entry.hasSuffix("/*") && type.hasPrefix(entry.dropLast())
        }
    }

    public var allowsImages: Bool { allows(contentType: "image/jpeg") }

    public func refusal(for upload: YuvaUpload) -> YuvaAttachmentRefusal? {
        if !allows(contentType: upload.contentType) { return .typeNotAllowed }
        if Int64(upload.data.count) > maxBytes { return .tooLarge(maxBytes: maxBytes) }
        return nil
    }
}

extension YuvaAttachmentRefusal {
    var error: YuvaError {
        switch self {
        case .tooLarge:
            YuvaError(status: 413, code: "attachment_too_large", detail: "an attachment is larger than the channel allows")
        case .typeNotAllowed:
            YuvaError(status: 415, code: "attachment_type_not_allowed", detail: "this type of file is not allowed")
        }
    }
}

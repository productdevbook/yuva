import Foundation

public enum YuvaConversationStatus: String, Codable, Sendable {
    case open, pending, snoozed, closed
}

public enum YuvaAuthorType: String, Codable, Sendable {
    case contact, member, system
}

public enum YuvaDirection: String, Codable, Sendable {
    case `in`, out
}

public enum YuvaInboxMode: String, Codable, Sendable {
    case live, async
}

public enum YuvaRating: String, Codable, Sendable, CaseIterable {
    case good, bad
}

public struct YuvaMember: Codable, Sendable, Hashable {
    public let name: String
    public let initials: String
}

public struct YuvaPresence: Codable, Sendable, Hashable {
    public let available: Bool
    public let members: [YuvaMember]
}

public struct YuvaBranding: Codable, Sendable, Hashable {
    public let color: String?
    public let logoUrl: String?
    public let greeting: String?
}

public struct YuvaInbox: Codable, Sendable {
    public let id: String
    public let name: String
    public let branding: YuvaBranding
    public let defaultLocale: String
    public let timezone: String
    public let mode: YuvaInboxMode
    public let openNow: Bool
    public let expectedReplyMinutes: Int?
    public let presence: YuvaPresence?
    private let feedbackCategoryNames: [String]?
    private let askForRatingValue: Bool?

    public var askForRating: Bool { askForRatingValue ?? false }

    public var feedbackCategories: [YuvaFeedbackCategory] {
        (feedbackCategoryNames ?? []).compactMap(YuvaFeedbackCategory.init(rawValue:))
    }

    enum CodingKeys: String, CodingKey {
        case id, name, branding, defaultLocale, timezone, mode, openNow, expectedReplyMinutes, presence
        case feedbackCategoryNames = "feedbackCategories"
        case askForRatingValue = "askForRating"
    }
}

public struct YuvaContact: Codable, Sendable {
    public let id: String
    public let name: String
    public let email: String?
    public let typedEmail: String?
    public let identified: Bool
}

public struct YuvaSession: Codable, Sendable {
    public let expiresAt: Date
    public let visitorId: String?
    public let contact: YuvaContact
    public let inbox: YuvaInbox
}

public struct YuvaMessagePreview: Codable, Sendable, Hashable {
    public let id: String
    public let authorType: YuvaAuthorType
    public let text: String
    public let createdAt: Date
}

public struct YuvaConversation: Codable, Sendable, Identifiable, Hashable {
    public let id: String
    public let kind: YuvaConversationKind?
    public let feedback: YuvaFeedbackInfo?
    public let subject: String
    public let status: YuvaConversationStatus
    public let lastMessage: YuvaMessagePreview?
    public let lastMessageAt: Date?
    public let unread: Bool
    public let lastReadByMemberAt: Date?
    public let canRate: Bool
    public let rating: YuvaRating?
    public let createdAt: Date

    public init(
        id: String, kind: YuvaConversationKind? = .conversation, feedback: YuvaFeedbackInfo? = nil, subject: String,
        status: YuvaConversationStatus, lastMessage: YuvaMessagePreview?, lastMessageAt: Date?, unread: Bool,
        lastReadByMemberAt: Date?, canRate: Bool = false, rating: YuvaRating? = nil, createdAt: Date
    ) {
        self.id = id
        self.kind = kind
        self.feedback = feedback
        self.subject = subject
        self.status = status
        self.lastMessage = lastMessage
        self.lastMessageAt = lastMessageAt
        self.unread = unread
        self.lastReadByMemberAt = lastReadByMemberAt
        self.canRate = canRate
        self.rating = rating
        self.createdAt = createdAt
    }

    public init(from decoder: any Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        kind = try container.decodeIfPresent(YuvaConversationKind.self, forKey: .kind)
        feedback = try container.decodeIfPresent(YuvaFeedbackInfo.self, forKey: .feedback)
        subject = try container.decode(String.self, forKey: .subject)
        status = try container.decode(YuvaConversationStatus.self, forKey: .status)
        lastMessage = try container.decodeIfPresent(YuvaMessagePreview.self, forKey: .lastMessage)
        lastMessageAt = try container.decodeIfPresent(Date.self, forKey: .lastMessageAt)
        unread = try container.decode(Bool.self, forKey: .unread)
        lastReadByMemberAt = try container.decodeIfPresent(Date.self, forKey: .lastReadByMemberAt)
        canRate = try container.decodeIfPresent(Bool.self, forKey: .canRate) ?? false
        rating = try? container.decodeIfPresent(YuvaRating.self, forKey: .rating)
        createdAt = try container.decode(Date.self, forKey: .createdAt)
    }
}

public struct YuvaMessageAuthor: Codable, Sendable, Hashable {
    public let type: YuvaAuthorType
    public let name: String?
    public let initials: String?
}

public struct YuvaAttachment: Codable, Sendable, Identifiable, Hashable {
    public let id: String
    public let filename: String
    public let contentType: String
    public let size: Int64
    public let contentId: String?
    public let inline: Bool

    public var isImage: Bool { contentType.hasPrefix("image/") }
}

public struct YuvaMessage: Codable, Sendable, Identifiable, Hashable {
    public let id: String
    public let conversationId: String
    public let direction: YuvaDirection
    public let author: YuvaMessageAuthor
    public let body: String
    public let html: String?
    public let clientId: String?
    public let attachments: [YuvaAttachment]
    public let createdAt: Date
}

public struct YuvaPage<Item: Sendable>: Sendable {
    public let items: [Item]
    public let nextCursor: String?
}

public struct YuvaConversationStarted: Codable, Sendable {
    public let conversation: YuvaConversation
    public let message: YuvaMessage
}

public struct YuvaReadState: Codable, Sendable {
    public let conversationId: String
    public let lastReadMessageId: String?
    public let unread: Bool
}

public struct YuvaUpload: Sendable {
    public let filename: String
    public let contentType: String
    public let data: Data

    public init(filename: String, contentType: String, data: Data) {
        self.filename = filename
        self.contentType = contentType
        self.data = data
    }
}

public enum YuvaEvent: Sendable {
    case conversationCreated(YuvaConversation)
    case conversationUpdated(conversationId: String, status: YuvaConversationStatus, canRate: Bool, rating: YuvaRating?)
    case messageCreated(YuvaMessage)
    case messageUpdated(YuvaMessage)
    case read(conversationId: String, readAt: Date)
    case typing(conversationId: String, typing: Bool, author: YuvaMessageAuthor)
    case presence(YuvaPresence)
    case inboxUpdated(YuvaInbox)
    case resyncRequired
    case connectionChanged(connected: Bool)
}

struct Page<Item: Codable & Sendable>: Codable, Sendable {
    let items: [Item]
    let nextCursor: String?
}

struct SessionResponse: Codable, Sendable {
    let token: String
    let expiresAt: Date
    let visitorId: String?
    let contact: YuvaContact
    let inbox: YuvaInbox

    var session: YuvaSession {
        YuvaSession(expiresAt: expiresAt, visitorId: visitorId, contact: contact, inbox: inbox)
    }
}

struct Problem: Codable, Sendable {
    let title: String?
    let status: Int?
    let code: String?
    let detail: String?
}

enum YuvaJSON {
    static let decoder: JSONDecoder = {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        decoder.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let text = try container.decode(String.self)
            guard let date = parseDate(text) else {
                throw DecodingError.dataCorruptedError(in: container, debugDescription: "Invalid date \(text)")
            }
            return date
        }
        return decoder
    }()

    static let encoder: JSONEncoder = {
        let encoder = JSONEncoder()
        encoder.keyEncodingStrategy = .convertToSnakeCase
        return encoder
    }()

    static func parseDate(_ text: String) -> Date? {
        var base = text
        var fraction = 0.0
        if let dot = text.firstIndex(of: ".") {
            var end = text.index(after: dot)
            while end < text.endIndex, text[end].isNumber { end = text.index(after: end) }
            fraction = Double("0" + text[dot..<end]) ?? 0
            base = String(text[..<dot]) + String(text[end...])
        }
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter.date(from: base)?.addingTimeInterval(fraction)
    }
}

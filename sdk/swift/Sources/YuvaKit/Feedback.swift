import Foundation

public enum YuvaFeedbackCategory: String, Codable, Sendable, CaseIterable {
    case bug, idea, praise, other
}

public struct YuvaFeedbackCategoryItem: Codable, Sendable, Hashable, Identifiable {
    public let key: String
    public let name: String
    public let icon: String?

    public var id: String { key }

    public init(key: String, name: String, icon: String? = nil) {
        self.key = key
        self.name = name
        self.icon = icon
    }
}

public struct YuvaFeedback: Sendable {
    public var categoryKey: String
    public var category: YuvaFeedbackCategory { YuvaFeedbackCategory(rawValue: categoryKey) ?? .other }
    public var body: String
    public var subject: String?
    public var screenshots: [YuvaUpload]
    public var allowEmail: Bool
    public var email: String?
    public var screen: String?
    public var clientId: String

    public init(
        category: YuvaFeedbackCategory, body: String, subject: String? = nil, screenshots: [YuvaUpload] = [],
        allowEmail: Bool = false, email: String? = nil, screen: String? = nil, clientId: String = UUID().uuidString
    ) {
        self.init(
            categoryKey: category.rawValue, body: body, subject: subject, screenshots: screenshots, allowEmail: allowEmail,
            email: email, screen: screen, clientId: clientId)
    }

    public init(
        categoryKey: String, body: String, subject: String? = nil, screenshots: [YuvaUpload] = [],
        allowEmail: Bool = false, email: String? = nil, screen: String? = nil, clientId: String = UUID().uuidString
    ) {
        self.categoryKey = categoryKey
        self.body = body
        self.subject = subject
        self.screenshots = screenshots
        self.allowEmail = allowEmail
        self.email = email
        self.screen = screen
        self.clientId = clientId
    }
}

public struct YuvaDeviceInfo: Codable, Sendable, Hashable {
    public let appVersion: String?
    public let build: String?
    public let os: String?
    public let osVersion: String?
    public let deviceModel: String?
    public let locale: String?
    public let screen: String?
    public let installationId: String?

    public static func current() -> YuvaDeviceInfo {
        let info = Bundle.main.infoDictionary ?? [:]
        let version = ProcessInfo.processInfo.operatingSystemVersion
        #if os(iOS)
        let os = "iOS"
        #elseif os(macOS)
        let os = "macOS"
        #else
        let os = "Apple"
        #endif
        return YuvaDeviceInfo(
            appVersion: info["CFBundleShortVersionString"] as? String,
            build: info["CFBundleVersion"] as? String,
            os: os,
            osVersion: "\(version.majorVersion).\(version.minorVersion).\(version.patchVersion)",
            deviceModel: modelIdentifier(),
            locale: Locale.preferredLanguages.first ?? Locale.current.identifier,
            screen: nil,
            installationId: installationId())
    }

    private static func installationId() -> String {
        let key = "yuva.installation_id"
        if let id = UserDefaults.standard.string(forKey: key) { return id }
        let id = UUID().uuidString.lowercased()
        UserDefaults.standard.set(id, forKey: key)
        return id
    }

    private static func modelIdentifier() -> String {
        if let simulated = ProcessInfo.processInfo.environment["SIMULATOR_MODEL_IDENTIFIER"] { return simulated }
        #if os(macOS)
        let name = "hw.model"
        #else
        let name = "hw.machine"
        #endif
        var size = 0
        sysctlbyname(name, nil, &size, nil, 0)
        var buffer = [UInt8](repeating: 0, count: max(size, 1))
        sysctlbyname(name, &buffer, &size, nil, 0)
        return String(decoding: buffer.prefix { $0 != 0 }, as: UTF8.self)
    }
}

public enum YuvaConversationKind: String, Codable, Sendable {
    case conversation, feedback
}

public enum YuvaFeedbackProgress: String, Codable, Sendable, CaseIterable {
    case planned
    case inProgress = "in_progress"
    case done
    case wontDo = "wont_do"
}

public enum YuvaFeedbackState: String, Sendable {
    case sent, answered, resolved
    case planned
    case inProgress = "in_progress"
    case done
    case wontDo = "wont_do"

    public static func of(
        status: YuvaConversationStatus, lastAuthor: YuvaAuthorType?, progress: YuvaFeedbackProgress? = nil
    ) -> YuvaFeedbackState {
        if let progress { return YuvaFeedbackState(rawValue: progress.rawValue) ?? .sent }
        if status == .closed { return .resolved }
        return lastAuthor?.isTeam == true ? .answered : .sent
    }

    public var isProgress: Bool { YuvaFeedbackProgress(rawValue: rawValue) != nil }
}

extension YuvaConversation {
    func settingFeedbackProgress(_ progress: YuvaFeedbackProgress?) -> YuvaConversation {
        guard let feedback else { return self }
        return YuvaConversation(
            id: id, kind: kind, feedback: feedback.settingProgress(progress), subject: subject, status: status,
            lastMessage: lastMessage, lastMessageAt: lastMessageAt, unread: unread,
            lastReadByMemberAt: lastReadByMemberAt, canRate: canRate, rating: rating, createdAt: createdAt)
    }

    public var isFeedback: Bool { kind == .feedback || feedback != nil }

    public var feedbackState: YuvaFeedbackState? {
        isFeedback ? .of(status: status, lastAuthor: lastMessage?.authorType, progress: feedback?.progress) : nil
    }
}

public struct YuvaFeedbackInfo: Codable, Sendable, Hashable {
    private let categoryValue: String?
    private let categoryKeyValue: String?
    public let categoryName: String?
    public let allowEmail: Bool
    public let text: String?
    public let attachmentCount: Int?
    public let appVersion: String?
    public let build: String?
    public let os: String?
    public let osVersion: String?
    public let deviceModel: String?
    public let locale: String?
    public let screen: String?
    public let installationId: String?
    private let progressName: String?

    public var categoryKey: String { categoryKeyValue ?? categoryValue ?? YuvaFeedbackCategory.other.rawValue }

    public var category: YuvaFeedbackCategory { YuvaFeedbackCategory(rawValue: categoryKey) ?? .other }

    public var progress: YuvaFeedbackProgress? { progressName.flatMap(YuvaFeedbackProgress.init(rawValue:)) }

    func settingProgress(_ progress: YuvaFeedbackProgress?) -> YuvaFeedbackInfo {
        YuvaFeedbackInfo(
            categoryValue: categoryValue, categoryKeyValue: categoryKeyValue, categoryName: categoryName,
            allowEmail: allowEmail, text: text, attachmentCount: attachmentCount,
            appVersion: appVersion, build: build, os: os, osVersion: osVersion, deviceModel: deviceModel,
            locale: locale, screen: screen, installationId: installationId, progressName: progress?.rawValue)
    }

    enum CodingKeys: String, CodingKey {
        case categoryName, allowEmail, text, attachmentCount, appVersion, build, os, osVersion, deviceModel, locale
        case screen, installationId
        case progressName = "progress"
        case categoryValue = "category"
        case categoryKeyValue = "categoryKey"
    }
}

extension YuvaClient {
    @discardableResult
    public func sendFeedback(_ feedback: YuvaFeedback, device: YuvaDeviceInfo = .current()) async throws
        -> YuvaConversationStarted
    {
        var fields: [String: String] = [
            "category": feedback.categoryKey,
            "body": feedback.body,
            "client_id": feedback.clientId,
            "allow_email": feedback.allowEmail ? "true" : "false",
        ]
        fields["subject"] = feedback.subject
        fields["email"] = feedback.email
        fields["screen"] = feedback.screen ?? device.screen
        fields["app_version"] = device.appVersion
        fields["build"] = device.build
        fields["os"] = device.os
        fields["os_version"] = device.osVersion
        fields["device_model"] = device.deviceModel
        fields["locale"] = device.locale
        fields["installation_id"] = device.installationId
        return try await postFeedback(fields: fields, files: feedback.screenshots)
    }
}

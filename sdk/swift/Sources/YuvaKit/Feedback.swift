import Foundation

public enum YuvaFeedbackCategory: String, Codable, Sendable, CaseIterable {
    case bug, idea, praise, other
}

public struct YuvaFeedback: Sendable {
    public var category: YuvaFeedbackCategory
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
        self.category = category
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

public struct YuvaFeedbackInfo: Codable, Sendable, Hashable {
    public let category: YuvaFeedbackCategory
    public let allowEmail: Bool
    public let appVersion: String?
    public let build: String?
    public let os: String?
    public let osVersion: String?
    public let deviceModel: String?
    public let locale: String?
    public let screen: String?
    public let installationId: String?
}

extension YuvaClient {
    @discardableResult
    public func sendFeedback(_ feedback: YuvaFeedback, device: YuvaDeviceInfo = .current()) async throws
        -> YuvaConversationStarted
    {
        var fields: [String: String] = [
            "category": feedback.category.rawValue,
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

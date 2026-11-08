import Foundation

#if canImport(UIKit)
import UIKit
#endif

struct YuvaClientInfo: Encodable, Sendable, Equatable {
    var appVersion: String?
    var build: String?
    var sdkName: String?
    var sdkVersion: String?
    var platform: String?
    var os: String?
    var osVersion: String?
    var deviceManufacturer: String?
    var deviceModel: String?
    var appLocale: String?
    var deviceLocale: String?
    var timezone: String?
    var screen: String?

    @MainActor
    static func current() -> YuvaClientInfo {
        let device = YuvaDeviceInfo.current()
        #if os(iOS)
        let platform: String? = "ios"
        let pixels = UIScreen.main.nativeBounds
        let screen: String? = "\(Int(pixels.width))x\(Int(pixels.height))"
        #else
        let platform: String? = nil
        let screen: String? = nil
        #endif
        return YuvaClientInfo(
            appVersion: device.appVersion,
            build: device.build,
            sdkName: "yuva-swift",
            sdkVersion: Yuva.version,
            platform: platform,
            os: device.os,
            osVersion: device.osVersion,
            deviceManufacturer: "Apple",
            deviceModel: device.deviceModel,
            appLocale: Bundle.main.preferredLocalizations.first,
            deviceLocale: device.locale,
            timezone: TimeZone.current.identifier,
            screen: screen)
    }
}

struct SessionRequest: Encodable, Sendable {
    var channelKey: String
    var identityToken: String?
    var visitorId: String?
    var clientInfo: YuvaClientInfo?
}

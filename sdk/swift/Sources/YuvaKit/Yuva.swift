import Foundation

public enum Yuva {
    public static let version = "0.0.6"

    public static func handleNotification(_ userInfo: [AnyHashable: Any]) -> String? {
        if let id = userInfo["yuva_conversation_id"] as? String { return id }
        if let yuva = userInfo["yuva"] as? [String: Any], let id = yuva["conversation_id"] as? String { return id }
        return nil
    }
}

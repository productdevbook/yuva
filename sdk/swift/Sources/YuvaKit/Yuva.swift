import Foundation

public enum Yuva {
    public static let version = "0.4.0"

    public static func handleNotification(_ userInfo: [AnyHashable: Any]) -> String? {
        if let id = userInfo["yuva_conversation_id"] as? String { return id }
        if let yuva = userInfo["yuva"] as? [String: Any], let id = yuva["conversation_id"] as? String { return id }
        return nil
    }

    public static func setPushToken(_ deviceToken: Data) {
        setPushToken(deviceToken.map { String(format: "%02x", $0) }.joined())
    }

    public static func setPushToken(_ token: String) {
        for client in PushRegistry.shared.set(token) {
            Task { await client.setPushToken(token) }
        }
    }
}

final class PushRegistry: @unchecked Sendable {
    static let shared = PushRegistry()

    private struct Weak {
        weak var client: YuvaClient?
    }

    private let lock = NSLock()
    private var token: String?
    private var clients: [Weak] = []

    func set(_ token: String) -> [YuvaClient] {
        lock.withLock {
            self.token = token
            clients.removeAll { $0.client == nil }
            return clients.compactMap(\.client)
        }
    }

    func add(_ client: YuvaClient) -> String? {
        lock.withLock {
            clients.removeAll { $0.client == nil }
            clients.append(Weak(client: client))
            return token
        }
    }
}

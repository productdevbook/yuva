import Foundation
import Security

struct StoredSession: Codable, Sendable {
    var token: String?
    var expiresAt: Date?
    var subject: String?
    var visitorId: String?
}

struct SessionStore: Sendable {
    let account: String

    init(channelKey: String) {
        account = "yuva.session." + channelKey
    }

    private var query: [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: "YuvaKit",
            kSecAttrAccount as String: account,
        ]
    }

    func load() -> StoredSession {
        var request = query
        request[kSecReturnData as String] = true
        request[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        guard SecItemCopyMatching(request as CFDictionary, &result) == errSecSuccess,
              let data = result as? Data,
              let stored = try? JSONDecoder().decode(StoredSession.self, from: data)
        else { return StoredSession() }
        return stored
    }

    func save(_ stored: StoredSession) {
        guard let data = try? JSONEncoder().encode(stored) else { return }
        let update = [kSecValueData as String: data]
        if SecItemUpdate(query as CFDictionary, update as CFDictionary) == errSecItemNotFound {
            var item = query
            item[kSecValueData as String] = data
            item[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
            SecItemAdd(item as CFDictionary, nil)
        }
    }

    func clear() {
        SecItemDelete(query as CFDictionary)
    }
}

enum IdentityToken {
    static func subject(of token: String) -> String? {
        let parts = token.split(separator: ".")
        guard parts.count == 3 else { return nil }
        var payload = parts[1].replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        payload += String(repeating: "=", count: (4 - payload.count % 4) % 4)
        guard let data = Data(base64Encoded: payload),
              let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any]
        else { return nil }
        return object["sub"] as? String
    }
}

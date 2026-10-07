import SwiftUI
import YuvaKit

struct ExampleConfig {
    let serverURL: URL
    let channelKey: String
    let identityTokenURL: URL?

    static func load() -> ExampleConfig? {
        guard let url = Bundle.main.url(forResource: "Config", withExtension: "plist"),
              let data = try? Data(contentsOf: url),
              let values = try? PropertyListSerialization.propertyList(from: data, format: nil) as? [String: String],
              let server = values["ServerURL"].flatMap(URL.init(string:)),
              let channel = values["ChannelKey"]
        else { return nil }
        return ExampleConfig(
            serverURL: server, channelKey: channel, identityTokenURL: values["IdentityTokenURL"].flatMap(URL.init(string:)))
    }
}

@main
struct YuvaExampleApp: App {
    var body: some Scene {
        WindowGroup {
            if let config = ExampleConfig.load() {
                ContentView(config: config)
            } else {
                ContentUnavailableView(
                    "Missing Config.plist", systemImage: "gearshape",
                    description: Text("Copy Config.example.plist to Config.plist and fill it in."))
            }
        }
    }
}

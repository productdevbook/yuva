import SwiftUI
import YuvaAnalytics
import UserNotifications
import YuvaKit

struct ExampleConfig {
    let serverURL: URL
    let channelKey: String
    let identityTokenURL: URL?
    let analyticsKey: String?
    let analyticsRequireConsent: Bool

    static func load() -> ExampleConfig? {
        guard let url = Bundle.main.url(forResource: "Config", withExtension: "plist"),
              let data = try? Data(contentsOf: url),
              let values = try? PropertyListSerialization.propertyList(from: data, format: nil) as? [String: String],
              let server = values["ServerURL"].flatMap(URL.init(string:)),
              let channel = values["ChannelKey"]
        else { return nil }
        return ExampleConfig(
            serverURL: server, channelKey: channel, identityTokenURL: values["IdentityTokenURL"].flatMap(URL.init(string:)),
            analyticsKey: values["AnalyticsKey"].flatMap { $0.isEmpty ? nil : $0 },
            analyticsRequireConsent: values["AnalyticsConsent"] == "required")
    }
}

@MainActor
enum ExampleRoute {
    static var name = "example.support"
}

extension Notification.Name {
    static let exampleOpenConversation = Notification.Name("example.openConversation")
}

final class ExampleAppDelegate: NSObject, UIApplicationDelegate, UNUserNotificationCenterDelegate {
    func application(
        _ application: UIApplication, didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        UNUserNotificationCenter.current().delegate = self
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .badge, .sound]) { granted, _ in
            if granted { DispatchQueue.main.async { application.registerForRemoteNotifications() } }
        }
        return true
    }

    func application(_ application: UIApplication, didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data) {
        Yuva.setPushToken(deviceToken)
    }

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse
    ) async {
        guard let id = Yuva.handleNotification(response.notification.request.content.userInfo) else { return }
        await MainActor.run { NotificationCenter.default.post(name: .exampleOpenConversation, object: id) }
    }
}

@main
struct YuvaExampleApp: App {
    @UIApplicationDelegateAdaptor(ExampleAppDelegate.self) private var delegate
    private let config = ExampleConfig.load()
    @State private var tab = UserDefaults.standard.string(forKey: "YuvaTab") ?? "support"

    init() {
        if let config, let key = config.analyticsKey {
            YuvaAnalytics.configure(
                YuvaAnalyticsConfiguration(
                    serverURL: config.serverURL, key: key, requireConsent: config.analyticsRequireConsent, debug: true))
        }
    }

    var body: some Scene {
        WindowGroup {
            if let config {
                TabView(selection: $tab) {
                    ContentView(config: config)
                        .yuvaScreen("Support")
                        .tabItem { Label("Support", systemImage: "bubble.left.and.bubble.right") }
                        .tag("support")
                    AnalyticsView(config: config)
                        .tabItem { Label("Analytics", systemImage: "chart.bar") }
                        .tag("analytics")
                }
                .onChange(of: tab, initial: true) { _, tab in
                    ExampleRoute.name = "example." + tab
                }
            } else {
                ContentUnavailableView(
                    "Missing Config.plist", systemImage: "gearshape",
                    description: Text("Copy Config.example.plist to Config.plist and fill it in."))
            }
        }
    }
}

// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "YuvaKit",
    defaultLocalization: "en",
    platforms: [.iOS(.v17), .macOS(.v14)],
    products: [
        .library(name: "YuvaKit", targets: ["YuvaKit"]),
    ],
    targets: [
        .target(
            name: "YuvaKit",
            path: "sdk/swift/Sources/YuvaKit",
            resources: [.process("Resources")]
        ),
    ]
)

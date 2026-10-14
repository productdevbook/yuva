pluginManagement {
    repositories {
        google()
        // JitPack's builders get HTTP 429 from Maven Central; Google's mirror of it serves them.
        if (System.getenv("JITPACK") != null) maven("https://maven-central.storage-download.googleapis.com/maven2/") else mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        if (System.getenv("JITPACK") != null) maven("https://maven-central.storage-download.googleapis.com/maven2/") else mavenCentral()
    }
}

rootProject.name = "yuva-kotlin"

include(":yuva")
include(":analytics")
include(":sample")

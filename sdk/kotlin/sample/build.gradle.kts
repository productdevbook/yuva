import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
}

val sampleConfig = Properties().apply {
    val file = rootProject.file("sample/yuva.properties")
    if (file.isFile) file.inputStream().use(::load)
}

fun configValue(key: String) = "\"" + sampleConfig.getProperty(key, "").replace("\"", "\\\"") + "\""

android {
    namespace = "dev.yuva.sample"
    compileSdk = 37
    defaultConfig {
        applicationId = "dev.yuva.sample"
        minSdk = 26
        targetSdk = 37
        versionCode = 1
        versionName = providers.gradleProperty("yuva.version").get()
        buildConfigField("String", "SERVER_URL", configValue("serverUrl"))
        buildConfigField("String", "CHANNEL_KEY", configValue("channelKey"))
        buildConfigField("String", "IDENTITY_TOKEN_URL", configValue("identityTokenUrl"))
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    androidResources {
        localeFilters += listOf("en", "tr")
    }
}

dependencies {
    implementation(project(":yuva"))
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.core.ktx)
}

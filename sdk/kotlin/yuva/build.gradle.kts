plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    `maven-publish`
}

android {
    namespace = "dev.yuva"
    compileSdk = 37
    defaultConfig {
        minSdk = 26
        consumerProguardFiles("consumer-rules.pro")
        buildConfigField("String", "YUVA_VERSION", "\"${providers.gradleProperty("yuva.version").get()}\"")
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    lint {
        abortOnError = true
        disable += setOf("GradleDependency", "NewerVersionAvailable", "AndroidGradlePluginVersion")
    }
    publishing {
        singleVariant("release") {
            withSourcesJar()
        }
    }
}

dependencies {
    api(platform(libs.androidx.compose.bom))
    api(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.foundation)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.compose.material.icons.core)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.core.ktx)
    api(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)
    api(libs.kotlinx.coroutines.android)
    testImplementation(libs.junit)
}

afterEvaluate {
    publishing {
        publications {
            register<MavenPublication>("release") {
                from(components["release"])
                groupId = "com.github.productdevbook.yuva"
                artifactId = "yuva-android"
                version = providers.environmentVariable("VERSION")
                    .orElse(providers.gradleProperty("yuva.version")).get()
                pom {
                    name.set("Yuva for Android")
                    description.set("In-app messaging and feedback for Yuva: a headless client and Jetpack Compose screens.")
                    url.set("https://github.com/productdevbook/yuva")
                    licenses {
                        license {
                            name.set("MIT")
                            url.set("https://github.com/productdevbook/yuva/blob/main/sdk/LICENSE")
                        }
                    }
                    scm {
                        url.set("https://github.com/productdevbook/yuva")
                        connection.set("scm:git:https://github.com/productdevbook/yuva.git")
                    }
                }
            }
        }
    }
}

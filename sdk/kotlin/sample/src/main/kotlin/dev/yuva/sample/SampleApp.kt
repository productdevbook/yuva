package dev.yuva.sample

import android.app.Application
import dev.yuva.analytics.YuvaAnalytics
import dev.yuva.analytics.YuvaAnalyticsConfiguration

// Push sent by Yuva: with Firebase set up in the app, hand the SDK the token from
// FirebaseMessagingService.onNewToken and FirebaseMessaging.getInstance().token with
// Yuva.setPushToken(token). The sample has no google-services.json, so it does not do this.
class SampleApp : Application() {
    override fun onCreate() {
        super.onCreate()
        if (BuildConfig.SERVER_URL.isNotEmpty() && BuildConfig.ANALYTICS_KEY.isNotEmpty()) {
            YuvaAnalytics.configure(
                this,
                YuvaAnalyticsConfiguration(
                    serverUrl = BuildConfig.SERVER_URL,
                    key = BuildConfig.ANALYTICS_KEY,
                    requireConsent = BuildConfig.ANALYTICS_CONSENT == "required",
                    debug = true,
                ),
            )
        }
    }
}

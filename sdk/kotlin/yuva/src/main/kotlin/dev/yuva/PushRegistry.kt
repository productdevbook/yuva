package dev.yuva

import java.util.Collections
import java.util.WeakHashMap

internal object PushRegistry {
    private val clients: MutableSet<YuvaClient> = Collections.newSetFromMap(WeakHashMap())
    private var token: String? = null

    @Synchronized fun set(token: String) {
        this.token = token
        clients.toList().forEach { it.setPushToken(token) }
    }

    @Synchronized fun add(client: YuvaClient): String? {
        clients += client
        return token
    }
}

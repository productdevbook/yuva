package dev.yuva

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyStore
import java.time.Instant
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

internal data class StoredSession(
    val token: String? = null,
    val expiresAt: Instant? = null,
    val subject: String? = null,
    val visitorId: String? = null,
    val pendingRevoke: List<String> = emptyList(),
    val pushDeviceId: String? = null,
)

internal interface SessionStorage {
    fun load(): StoredSession
    fun save(stored: StoredSession)
    fun clear()
}

internal class SessionStore(context: Context, channelKey: String) : SessionStorage {
    private val prefs = context.getSharedPreferences("yuva.session.$channelKey", Context.MODE_PRIVATE)

    override fun load(): StoredSession {
        val plain = prefs.getString("token", null)
        val token = plain ?: prefs.getString("token_sealed", null)?.let(TokenSeal::open)
        val stored = StoredSession(
            token = token,
            expiresAt = prefs.getString("expires_at", null)?.let(Instant::parse),
            subject = prefs.getString("subject", null),
            visitorId = prefs.getString("visitor_id", null),
            pendingRevoke = prefs.getStringSet("pending_revoke", null).orEmpty().sorted(),
            pushDeviceId = prefs.getString("push_device_id", null),
        )
        if (plain != null) save(stored)
        return stored
    }

    override fun save(stored: StoredSession) {
        prefs.edit()
            .remove("token")
            .putString("token_sealed", stored.token?.let(TokenSeal::seal))
            .putString("expires_at", stored.expiresAt?.toString())
            .putString("subject", stored.subject)
            .putString("visitor_id", stored.visitorId)
            .putStringSet("pending_revoke", stored.pendingRevoke.toSet().ifEmpty { null })
            .putString("push_device_id", stored.pushDeviceId)
            .apply()
    }

    override fun clear() {
        prefs.edit().clear().apply()
    }
}

// The key lives in the Android Keystore and never leaves the device, so a token restored from a
// backup onto another device cannot be opened and the contact simply signs in again.
internal object TokenSeal {
    private const val ALIAS = "dev.yuva.session"
    private const val IV_BYTES = 12

    private fun key(): SecretKey {
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (ks.getKey(ALIAS, null) as? SecretKey)?.let { return it }
        val gen = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        gen.init(
            KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .build(),
        )
        return gen.generateKey()
    }

    fun seal(token: String): String? = runCatching {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding").apply { init(Cipher.ENCRYPT_MODE, key()) }
        val sealed = cipher.iv + cipher.doFinal(token.toByteArray(Charsets.UTF_8))
        Base64.encodeToString(sealed, Base64.NO_WRAP)
    }.getOrNull()

    fun open(sealed: String): String? = runCatching {
        val raw = Base64.decode(sealed, Base64.NO_WRAP)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, raw, 0, IV_BYTES))
        String(cipher.doFinal(raw, IV_BYTES, raw.size - IV_BYTES), Charsets.UTF_8)
    }.getOrNull()
}

internal object IdentityToken {
    fun subject(token: String): String? {
        val parts = token.split(".")
        if (parts.size != 3) return null
        return runCatching {
            val payload = String(Base64.decode(parts[1], Base64.URL_SAFE or Base64.NO_PADDING or Base64.NO_WRAP))
            YuvaClient.json.parseToJsonElement(payload).jsonObject["sub"]?.jsonPrimitive?.contentOrNull
        }.getOrNull()
    }
}

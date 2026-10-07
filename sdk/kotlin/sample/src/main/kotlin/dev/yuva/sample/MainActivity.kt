package dev.yuva.sample

import android.content.Context
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalView
import androidx.core.view.drawToBitmap
import android.graphics.Bitmap
import android.view.View
import java.io.ByteArrayOutputStream
import androidx.compose.ui.unit.dp
import dev.yuva.Yuva
import dev.yuva.YuvaClient
import dev.yuva.YuvaConfiguration
import dev.yuva.YuvaUpload
import dev.yuva.ui.YuvaConversationsView
import dev.yuva.ui.YuvaFeedbackView
import java.net.URL
import java.net.URLEncoder
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val extras = intent.extras?.keySet()?.associateWith { intent.extras?.getString(it).orEmpty() }.orEmpty()
        val openConversation = Yuva.handleNotification(extras)
        val startScreen = intent.getStringExtra("yuva_open")
        setContent {
            MaterialTheme {
                Sample(this, openConversation, startScreen)
            }
        }
    }
}

private fun client(context: Context, identified: Boolean, userId: String): YuvaClient {
    val tokenUrl = BuildConfig.IDENTITY_TOKEN_URL
    val provider: (suspend () -> String?)? = if (identified && userId.isNotBlank() && tokenUrl.isNotEmpty()) {
        {
            withContext(Dispatchers.IO) {
                URL(tokenUrl + "?sub=" + URLEncoder.encode(userId, "UTF-8")).readText().trim()
            }
        }
    } else {
        null
    }
    return YuvaClient(context, YuvaConfiguration(BuildConfig.SERVER_URL, BuildConfig.CHANNEL_KEY, provider))
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun Sample(context: Context, openConversation: String?, startScreen: String?) {
    val prefs = remember { context.getSharedPreferences("sample", Context.MODE_PRIVATE) }
    var identified by remember { mutableStateOf(prefs.getBoolean("identified", false)) }
    var userId by remember { mutableStateOf(prefs.getString("user_id", "").orEmpty()) }
    var screen by remember { mutableStateOf(if (openConversation != null) "messages" else startScreen) }
    var yuva by remember { mutableStateOf<YuvaClient?>(client(context, identified, userId)) }
    var screenshot by remember { mutableStateOf<YuvaUpload?>(null) }
    val view = LocalView.current
    val scope = rememberCoroutineScope()
    val swap = remember { Mutex() }

    fun replaceClient(signOut: Boolean) {
        val previous = yuva
        yuva = null
        scope.launch {
            swap.withLock {
                if (signOut) (yuva ?: previous)?.signOut()
                yuva = client(context, identified, userId)
            }
        }
    }

    LaunchedEffect(identified, userId) {
        prefs.edit().putBoolean("identified", identified).putString("user_id", userId).apply()
    }

    if (BuildConfig.SERVER_URL.isEmpty()) {
        Text("Copy sample/yuva.properties.example to sample/yuva.properties and rebuild.", Modifier.padding(32.dp))
        return
    }
    val current = yuva
    when (screen.takeIf { current != null }) {
        "messages" -> {
            BackHandler { screen = null }
            YuvaConversationsView(current!!, openConversationId = openConversation, onClose = { screen = null })
            return
        }
        "feedback" -> {
            BackHandler { screen = null }
            YuvaFeedbackView(current!!, screenshot = screenshot, screen = "sample.home", onDone = { screen = null })
            return
        }
    }
    Scaffold(topBar = { TopAppBar(title = { Text("Yuva Sample") }) }) { padding ->
        Column(
            Modifier.fillMaxSize().padding(padding).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                listOf(false to "Anonymous", true to "Identified").forEachIndexed { index, (value, label) ->
                    SegmentedButton(
                        selected = identified == value,
                        onClick = {
                            val signOut = identified && !value
                            identified = value
                            replaceClient(signOut)
                        },
                        shape = SegmentedButtonDefaults.itemShape(index, 2),
                    ) { Text(label) }
                }
            }
            if (identified) {
                OutlinedTextField(
                    value = userId,
                    onValueChange = {
                        userId = it
                        replaceClient(false)
                    },
                    label = { Text("Host user id") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
            }
            Button(onClick = { screen = "messages" }, enabled = current != null, modifier = Modifier.fillMaxWidth()) {
                Text("Open messages")
            }
            Button(enabled = current != null, onClick = {
                screenshot = capture(view)
                screen = "feedback"
            }, modifier = Modifier.fillMaxWidth()) { Text("Send feedback") }
            Text("Server: ${BuildConfig.SERVER_URL}\nYuva SDK ${Yuva.VERSION}", style = MaterialTheme.typography.bodySmall)
        }
    }
}

private fun capture(view: View): YuvaUpload? = runCatching {
    val out = ByteArrayOutputStream()
    view.rootView.drawToBitmap().compress(Bitmap.CompressFormat.JPEG, 80, out)
    YuvaUpload("screenshot.jpg", "image/jpeg", out.toByteArray())
}.getOrNull()

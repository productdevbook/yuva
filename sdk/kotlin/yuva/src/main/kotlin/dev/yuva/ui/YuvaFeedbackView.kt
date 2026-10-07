package dev.yuva.ui

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.selection.toggleable
import androidx.compose.ui.semantics.Role
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import dev.yuva.R
import dev.yuva.YuvaClient
import dev.yuva.YuvaFeedback
import dev.yuva.YuvaFeedbackCategory
import dev.yuva.YuvaUpload
import java.util.UUID
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun YuvaFeedbackView(
    client: YuvaClient,
    modifier: Modifier = Modifier,
    screenshot: YuvaUpload? = null,
    screen: String? = null,
    onDone: () -> Unit = {},
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var categories by remember { mutableStateOf(YuvaFeedbackCategory.entries.toList()) }
    var category by remember { mutableStateOf(YuvaFeedbackCategory.BUG) }
    var message by remember { mutableStateOf("") }
    val screenshots = remember { mutableStateListOf<YuvaUpload>().apply { screenshot?.let(::add) } }
    var allowEmail by remember { mutableStateOf(false) }
    var knownEmail by remember { mutableStateOf<String?>(null) }
    var email by remember { mutableStateOf("") }
    var sending by remember { mutableStateOf(false) }
    var sent by remember { mutableStateOf(false) }
    var failed by remember { mutableStateOf(false) }
    val clientId = remember { UUID.randomUUID().toString() }
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.PickMultipleVisualMedia(10)) { uris ->
        scope.launch {
            screenshots.addAll(withContext(Dispatchers.IO) { uris.mapNotNull { imageUpload(context, it, "screenshot") } })
        }
    }

    LaunchedEffect(client) {
        val session = runCatching { client.start() }.getOrNull() ?: return@LaunchedEffect
        val offered = session.inbox.feedbackCategories
        if (offered.isNotEmpty()) {
            categories = offered
            if (category !in offered) category = offered.first()
        }
        knownEmail = session.contact.email
    }

    val needsEmail = allowEmail && knownEmail == null
    val canSend = !sending && (message.isNotBlank() || screenshots.isNotEmpty()) && (!needsEmail || "@" in email)

    Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.yuva_feedback_title)) },
                navigationIcon = {
                    if (!sent) TextButton(onClick = onDone) { Text(stringResource(R.string.yuva_feedback_cancel)) }
                },
                actions = {
                    if (!sent) {
                        TextButton(
                            enabled = canSend,
                            modifier = Modifier.testTag("yuva.feedback.send"),
                            onClick = {
                                scope.launch {
                                    sending = true
                                    failed = false
                                    try {
                                        client.sendFeedback(
                                            YuvaFeedback(
                                                category = category,
                                                body = message.trim(),
                                                screenshots = screenshots.toList(),
                                                allowEmail = allowEmail,
                                                email = if (needsEmail) email.trim() else null,
                                                screen = screen,
                                                clientId = clientId,
                                            ),
                                        )
                                        sent = true
                                    } catch (_: Exception) {
                                        failed = true
                                    } finally {
                                        sending = false
                                    }
                                }
                            },
                        ) { Text(stringResource(R.string.yuva_feedback_send)) }
                    }
                },
            )
        },
    ) { padding ->
        if (sent) {
            Box(Modifier.fillMaxSize().padding(padding)) {
                Placeholder(
                    title = stringResource(R.string.yuva_feedback_sent),
                    hint = stringResource(R.string.yuva_feedback_sent_hint),
                    action = stringResource(R.string.yuva_feedback_done),
                    onAction = onDone,
                )
            }
            return@Scaffold
        }
        Column(
            Modifier.fillMaxSize().padding(padding).verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Text(stringResource(R.string.yuva_feedback_category), style = MaterialTheme.typography.labelLarge)
            SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                categories.forEachIndexed { index, option ->
                    SegmentedButton(
                        selected = option == category,
                        onClick = { category = option },
                        shape = SegmentedButtonDefaults.itemShape(index, categories.size),
                        enabled = !sending,
                    ) { Text(stringResource(categoryLabel(option)), maxLines = 1) }
                }
            }
            OutlinedTextField(
                value = message,
                onValueChange = { message = it },
                label = { Text(stringResource(R.string.yuva_feedback_message)) },
                placeholder = { Text(stringResource(R.string.yuva_feedback_placeholder)) },
                minLines = 4,
                maxLines = 12,
                enabled = !sending,
                modifier = Modifier.fillMaxWidth().testTag("yuva.feedback.message"),
            )
            Text(stringResource(R.string.yuva_feedback_screenshot), style = MaterialTheme.typography.labelLarge)
            screenshots.forEachIndexed { index, upload ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    UploadImage(upload, Modifier.height(160.dp).clip(RoundedCornerShape(8.dp)))
                    Spacer(Modifier.weight(1f))
                    IconButton(onClick = { screenshots.removeAt(index) }) {
                        Icon(Icons.Default.Delete, contentDescription = stringResource(R.string.yuva_feedback_remove_screenshot))
                    }
                }
            }
            OutlinedButton(
                onClick = { picker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)) },
                enabled = !sending,
            ) { Text(stringResource(R.string.yuva_feedback_choose_screenshot)) }
            Row(
                Modifier.fillMaxWidth().toggleable(
                    value = allowEmail,
                    enabled = !sending,
                    role = Role.Switch,
                    onValueChange = { allowEmail = it },
                ).testTag("yuva.feedback.allowEmail"),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(stringResource(R.string.yuva_feedback_allow_email), modifier = Modifier.weight(1f))
                Switch(checked = allowEmail, onCheckedChange = null, enabled = !sending)
            }
            if (needsEmail) {
                OutlinedTextField(
                    value = email,
                    onValueChange = { email = it },
                    label = { Text(stringResource(R.string.yuva_feedback_email)) },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email),
                    enabled = !sending,
                    modifier = Modifier.fillMaxWidth().testTag("yuva.feedback.email"),
                )
            }
            Text(
                stringResource(R.string.yuva_feedback_metadata_hint),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (failed) Text(stringResource(R.string.yuva_error_generic), color = MaterialTheme.colorScheme.error)
        }
    }
}

package dev.yuva.ui

import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.FilledTonalButton
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.selection.selectable
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.LargeTopAppBar
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.Surface
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.material3.SwitchDefaults
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
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
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
import dev.yuva.TrackOpen
import dev.yuva.YuvaAttachmentLimits
import dev.yuva.YuvaClient
import dev.yuva.YuvaConversation
import dev.yuva.YuvaEvent
import dev.yuva.YuvaFeedback
import dev.yuva.YuvaFeedbackCategory
import dev.yuva.YuvaFeedbackCategoryItem
import dev.yuva.YuvaInbox
import dev.yuva.YuvaUpload
import java.util.Locale
import java.util.UUID
import kotlinx.coroutines.CancellationException
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
    onViewFeedback: ((YuvaConversation) -> Unit)? = null,
    onDone: () -> Unit = {},
) {
    TrackOpen()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var categories by remember { mutableStateOf<List<YuvaFeedbackCategoryItem>>(emptyList()) }
    var category by remember { mutableStateOf(YuvaFeedbackCategory.BUG.value) }
    var message by remember { mutableStateOf("") }
    val screenshots = remember { mutableStateListOf<YuvaUpload>().apply { screenshot?.let(::add) } }
    var allowEmail by remember { mutableStateOf(false) }
    var knownEmail by remember { mutableStateOf<String?>(null) }
    var email by remember { mutableStateOf("") }
    var sending by remember { mutableStateOf(false) }
    var sent by remember { mutableStateOf(false) }
    var started by remember { mutableStateOf<YuvaConversation?>(null) }
    var viewing by remember { mutableStateOf<String?>(null) }
    var failure by remember { mutableStateOf<Int?>(null) }
    val clientId = remember { UUID.randomUUID().toString() }
    var limits by remember { mutableStateOf<YuvaAttachmentLimits?>(null) }
    var attachNotice by remember { mutableStateOf<String?>(null) }
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.PickMultipleVisualMedia(10)) { uris ->
        scope.launch {
            attachNotice = null
            val picked = withContext(Dispatchers.IO) { uris.mapNotNull { imageUpload(context, it, "screenshot") } }
            picked.forEach { upload ->
                val refusal = limits?.refusal(upload)
                if (refusal != null) attachNotice = refusalMessage(context, refusal) else screenshots.add(upload)
            }
        }
    }

    val builtIns = YuvaFeedbackCategory.entries.map {
        YuvaFeedbackCategoryItem(it.value, stringResource(categoryLabel(it)))
    }
    val shown = categories.ifEmpty { builtIns }.map { item -> builtIns.firstOrNull { it.key == item.key }?.copy(icon = item.icon) ?: item }

    fun offer(inbox: YuvaInbox) {
        limits = inbox.attachments
        inbox.attachments?.let { allowed ->
            val refused = screenshots.mapNotNull { allowed.refusal(it) }.firstOrNull()
            if (refused != null) {
                screenshots.removeAll { allowed.refusal(it) != null }
                attachNotice = refusalMessage(context, refused)
            }
        }
        val offered = inbox.feedbackCategoryItems.ifEmpty {
            inbox.feedbackCategories.map { c -> builtIns.first { it.key == c.value } }
        }
        if (offered.isEmpty()) return
        categories = offered
        if (offered.none { it.key == category }) category = offered.first().key
    }

    LaunchedEffect(client) {
        val cached = runCatching { client.start() }.getOrNull() ?: return@LaunchedEffect
        val session = if (client.sessionLanguage == Locale.getDefault().toLanguageTag()) cached
        else runCatching { client.refreshSession() }.getOrDefault(cached)
        offer(session.inbox)
        knownEmail = session.contact.email
    }
    LaunchedEffect(client) {
        client.events().collect { event -> if (event is YuvaEvent.InboxUpdated) offer(event.inbox) }
    }

    val needsEmail = allowEmail && knownEmail == null
    val canSend = !sending && (message.isNotBlank() || screenshots.isNotEmpty()) && (!needsEmail || "@" in email)

    viewing?.let { id ->
        BackHandler { viewing = null }
        YuvaThreadView(client, id, modifier, onBack = { viewing = null })
        return
    }

    fun submit() {
        scope.launch {
            sending = true
            failure = null
            try {
                val result = client.sendFeedback(
                    YuvaFeedback(
                        categoryKey = category,
                        body = message.trim(),
                        screenshots = screenshots.toList(),
                        allowEmail = allowEmail,
                        email = if (needsEmail) email.trim() else null,
                        screen = screen,
                        clientId = clientId,
                    ),
                )
                started = result.conversation
                sent = true
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                failure = errorMessage(error)
            } finally {
                sending = false
            }
        }
    }

    val scrollBehavior = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    Scaffold(
        modifier = modifier.nestedScroll(scrollBehavior.nestedScrollConnection),
        topBar = {
            LargeTopAppBar(
                scrollBehavior = scrollBehavior,
                title = { Text(stringResource(R.string.yuva_feedback_title), fontWeight = FontWeight.SemiBold) },
                navigationIcon = {
                    if (!sent) {
                        IconButton(onClick = onDone) {
                            Icon(Icons.Default.Close, contentDescription = stringResource(R.string.yuva_feedback_cancel))
                        }
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
                    secondaryAction = stringResource(R.string.yuva_feedback_view).takeIf { started != null },
                    onSecondaryAction = {
                        started?.let { if (onViewFeedback != null) onViewFeedback(it) else viewing = it.id }
                    },
                )
            }
            return@Scaffold
        }
        Column(
            Modifier.fillMaxSize().padding(padding).verticalScroll(rememberScrollState())
                .padding(start = 16.dp, end = 16.dp, top = 8.dp, bottom = 24.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            SectionLabel(stringResource(R.string.yuva_feedback_category))
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                shown.chunked(2).forEach { pair ->
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        pair.forEach { option ->
                            CategoryTile(
                                option,
                                selected = option.key == category,
                                enabled = !sending,
                                onClick = { category = option.key },
                                modifier = Modifier.weight(1f),
                            )
                        }
                        if (pair.size == 1) Spacer(Modifier.weight(1f))
                    }
                }
            }
            Spacer(Modifier.height(4.dp))
            TextField(
                value = message,
                onValueChange = { message = it },
                label = { Text(stringResource(R.string.yuva_feedback_message)) },
                placeholder = { Text(stringResource(R.string.yuva_feedback_placeholder)) },
                minLines = 4,
                maxLines = 12,
                enabled = !sending,
                shape = RoundedCornerShape(24.dp),
                colors = roundFieldColors(),
                modifier = Modifier.fillMaxWidth().testTag("yuva.feedback.message"),
            )
            Spacer(Modifier.height(4.dp))
            val showsScreenshots = limits?.allowsImages != false || screenshots.isNotEmpty() || attachNotice != null
            if (showsScreenshots) SectionLabel(stringResource(R.string.yuva_feedback_screenshot))
            screenshots.forEachIndexed { index, upload ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    UploadImage(upload, Modifier.height(160.dp).clip(RoundedCornerShape(20.dp)))
                    Spacer(Modifier.weight(1f))
                    FilledTonalIconButton(onClick = { screenshots.removeAt(index) }) {
                        Icon(Icons.Default.Delete, contentDescription = stringResource(R.string.yuva_feedback_remove_screenshot))
                    }
                }
            }
            if (limits?.allowsImages != false) {
                FilledTonalButton(
                    onClick = { picker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)) },
                    enabled = !sending,
                    contentPadding = PaddingValues(horizontal = 20.dp, vertical = 14.dp),
                ) {
                    Icon(YuvaIcons.Attach, contentDescription = null, modifier = Modifier.size(ButtonDefaults.IconSize))
                    Spacer(Modifier.size(ButtonDefaults.IconSpacing))
                    Text(stringResource(R.string.yuva_feedback_choose_screenshot))
                }
            }
            attachNotice?.let {
                Text(
                    it,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.padding(horizontal = 8.dp).testTag("yuva.feedback.attachNotice"),
                )
            }
            Spacer(Modifier.height(4.dp))
            Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
                Row(
                    Modifier.fillMaxWidth()
                        .clip(groupShape(0, if (needsEmail) 2 else 1))
                        .background(MaterialTheme.colorScheme.surfaceContainerHigh)
                        .toggleable(
                            value = allowEmail,
                            enabled = !sending,
                            role = Role.Switch,
                            onValueChange = { allowEmail = it },
                        )
                        .padding(horizontal = 20.dp, vertical = 16.dp)
                        .testTag("yuva.feedback.allowEmail"),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(
                        stringResource(R.string.yuva_feedback_allow_email),
                        style = MaterialTheme.typography.bodyLarge,
                        modifier = Modifier.weight(1f),
                    )
                    Spacer(Modifier.size(12.dp))
                    Switch(
                        checked = allowEmail,
                        onCheckedChange = null,
                        enabled = !sending,
                        thumbContent = if (allowEmail) {
                            { Icon(Icons.Default.Check, contentDescription = null, modifier = Modifier.size(SwitchDefaults.IconSize)) }
                        } else null,
                    )
                }
                AnimatedVisibility(
                    needsEmail,
                    enter = expandVertically(spring(dampingRatio = 0.8f, stiffness = Spring.StiffnessMediumLow)) + fadeIn(),
                    exit = shrinkVertically() + fadeOut(),
                ) {
                    TextField(
                        value = email,
                        onValueChange = { email = it },
                        label = { Text(stringResource(R.string.yuva_feedback_email)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email),
                        enabled = !sending,
                        shape = groupShape(1, 2),
                        colors = roundFieldColors(),
                        modifier = Modifier.fillMaxWidth().testTag("yuva.feedback.email"),
                    )
                }
            }
            Text(
                stringResource(R.string.yuva_feedback_metadata_hint),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 8.dp),
            )
            failure?.let {
                Text(
                    stringResource(it),
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.padding(horizontal = 8.dp).testTag("yuva.feedback.error"),
                )
            }
            Button(
                onClick = ::submit,
                enabled = canSend,
                shape = CircleShape,
                contentPadding = PaddingValues(vertical = 18.dp),
                modifier = Modifier.fillMaxWidth().heightIn(min = 60.dp).testTag("yuva.feedback.send"),
            ) {
                if (sending) {
                    CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.5.dp, color = LocalContentColor.current)
                } else {
                    Icon(Icons.AutoMirrored.Filled.Send, contentDescription = null, modifier = Modifier.size(20.dp))
                    Spacer(Modifier.size(10.dp))
                    Text(stringResource(R.string.yuva_feedback_send), style = MaterialTheme.typography.titleMedium)
                }
            }
        }
    }
}

@Composable
private fun SectionLabel(text: String) {
    Text(
        text,
        style = MaterialTheme.typography.titleSmall,
        fontWeight = FontWeight.SemiBold,
        color = MaterialTheme.colorScheme.primary,
        modifier = Modifier.padding(start = 8.dp, top = 8.dp),
    )
}

@Composable
private fun roundFieldColors() = TextFieldDefaults.colors(
    focusedContainerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
    unfocusedContainerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
    disabledContainerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
    focusedIndicatorColor = Color.Transparent,
    unfocusedIndicatorColor = Color.Transparent,
    disabledIndicatorColor = Color.Transparent,
    errorIndicatorColor = Color.Transparent,
)

@Composable
private fun CategoryTile(
    item: YuvaFeedbackCategoryItem,
    selected: Boolean,
    enabled: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val colors = categoryColors(item.key)
    val bounce = spring<Dp>(dampingRatio = 0.6f, stiffness = Spring.StiffnessMedium)
    val corner by animateDpAsState(if (selected) 32.dp else 16.dp, bounce, label = "corner")
    val morph by animateFloatAsState(
        if (selected) 1f else 0f,
        spring(dampingRatio = 0.55f, stiffness = Spring.StiffnessMediumLow),
        label = "morph",
    )
    val container by animateColorAsState(
        if (selected) colors.container else MaterialTheme.colorScheme.surfaceContainerHigh,
        spring(stiffness = Spring.StiffnessMediumLow),
        label = "container",
    )
    Surface(
        shape = RoundedCornerShape(corner),
        color = container,
        contentColor = if (selected) colors.content else MaterialTheme.colorScheme.onSurface,
        modifier = modifier.heightIn(min = 72.dp).selectable(
            selected = selected,
            enabled = enabled,
            role = Role.RadioButton,
            onClick = onClick,
        ),
    ) {
        Row(Modifier.padding(horizontal = 14.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
            val iconContainer by animateColorAsState(
                if (selected) colors.content else colors.container,
                spring(stiffness = Spring.StiffnessMediumLow),
                label = "iconContainer",
            )
            val iconContent by animateColorAsState(
                if (selected) colors.container else colors.content,
                spring(stiffness = Spring.StiffnessMediumLow),
                label = "iconContent",
            )
            CategoryIcon(item.key, 44.dp, morph = 0.15f + 0.85f * morph, icon = item.icon, colors = TonalColors(iconContainer, iconContent))
            Spacer(Modifier.size(12.dp))
            Text(
                item.name,
                style = MaterialTheme.typography.titleMedium,
                fontWeight = if (selected) FontWeight.Bold else FontWeight.Medium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

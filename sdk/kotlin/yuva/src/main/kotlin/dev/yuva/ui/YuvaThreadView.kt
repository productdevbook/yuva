package dev.yuva.ui

import android.content.Context
import android.content.Intent
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.scaleOut
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material3.Badge
import androidx.compose.material3.BadgedBox
import androidx.compose.material3.SmallFloatingActionButton
import androidx.compose.runtime.derivedStateOf
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Info
import androidx.compose.animation.animateContentSize
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.ui.draw.alpha
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
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
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.core.content.FileProvider
import dev.yuva.R
import dev.yuva.YuvaAttachment
import dev.yuva.YuvaAuthorType
import dev.yuva.YuvaClient
import dev.yuva.YuvaConversation
import dev.yuva.YuvaConversationStatus
import dev.yuva.YuvaEvent
import dev.yuva.YuvaFeedbackState
import dev.yuva.withFeedbackProgress
import dev.yuva.isFeedback
import dev.yuva.YuvaException
import dev.yuva.YuvaInbox
import dev.yuva.YuvaMessage
import dev.yuva.YuvaOrder
import dev.yuva.YuvaRating
import dev.yuva.YuvaUpload
import java.io.File
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

internal enum class DeliveryState { SENDING, SENT, FAILED }

internal data class ThreadItem(
    val id: String,
    val message: YuvaMessage?,
    val body: String,
    val uploads: List<YuvaUpload>,
    val state: DeliveryState,
    val createdAt: Instant,
) {
    val mine: Boolean get() = message?.author?.type?.let { it == YuvaAuthorType.CONTACT } ?: true

    companion object {
        fun of(message: YuvaMessage) =
            ThreadItem(message.id, message, message.body, emptyList(), DeliveryState.SENT, message.createdAt)
    }
}

internal class ThreadState(val client: YuvaClient, conversationId: String?, private val scope: CoroutineScope) {
    var conversationId by mutableStateOf(conversationId)
    val items = mutableStateListOf<ThreadItem>()
    var conversation by mutableStateOf<YuvaConversation?>(null)
    var inbox by mutableStateOf<YuvaInbox?>(null)
    var memberReadAt by mutableStateOf<Instant?>(null)
    var typingName by mutableStateOf<String?>(null)
    var connected by mutableStateOf(true)
    var loading by mutableStateOf(conversationId != null)
    var failed by mutableStateOf(false)
    var failure by mutableStateOf(R.string.yuva_error_generic)
    var olderCursor by mutableStateOf<String?>(null)
    var ratingChoice by mutableStateOf<YuvaRating?>(null)
    var ratingSending by mutableStateOf(false)
    var ratingError by mutableStateOf<Int?>(null)
    var visible = false
    private var loadingOlder = false
    private var typingClear: Job? = null
    private var typingIdle: Job? = null
    private var typingSentAt = 0L

    suspend fun load() {
        loading = items.isEmpty() && conversationId != null
        try {
            inbox = client.start().inbox
            val id = conversationId ?: return
            conversation = client.conversation(id)
            memberReadAt = conversation?.lastReadByMemberAt
            val page = client.messages(id, YuvaOrder.DESC, limit = 30)
            val pending = items.filter { it.message == null }
            items.clear()
            items.addAll(page.items.reversed().map(ThreadItem::of) + pending)
            olderCursor = page.nextCursor
            failed = false
            markRead()
        } catch (error: Exception) {
            failure = errorMessage(error)
            failed = items.isEmpty()
        } finally {
            loading = false
        }
    }

    suspend fun loadOlder() {
        val id = conversationId ?: return
        val cursor = olderCursor ?: return
        if (loadingOlder) return
        loadingOlder = true
        try {
            val page = client.messages(id, YuvaOrder.DESC, cursor = cursor, limit = 30)
            val known = items.map { it.id }.toSet()
            items.addAll(0, page.items.reversed().filter { it.id !in known }.map(ThreadItem::of))
            olderCursor = page.nextCursor
        } catch (_: Exception) {
        } finally {
            loadingOlder = false
        }
    }

    suspend fun listen() {
        client.events().collect { event ->
            when (event) {
                is YuvaEvent.MessageCreated -> onMessage(event.message)
                is YuvaEvent.MessageUpdated -> onMessage(event.message)
                is YuvaEvent.ConversationUpdated -> if (event.conversationId == conversationId) {
                    conversation = runCatching { client.conversation(event.conversationId) }.getOrNull()
                        ?.withFeedbackProgress(event.feedbackProgress) ?: conversation
                }
                is YuvaEvent.Read -> if (event.conversationId == conversationId) {
                    memberReadAt = maxOf(memberReadAt ?: event.readAt, event.readAt)
                }
                is YuvaEvent.Typing -> if (event.conversationId == conversationId) {
                    typingClear?.cancel()
                    typingName = if (event.typing) event.author.name.orEmpty() else null
                    if (event.typing) typingClear = scope.launch {
                        delay(8000)
                        typingName = null
                    }
                }
                is YuvaEvent.Presence -> runCatching { inbox = client.refreshSession().inbox }
                is YuvaEvent.InboxUpdated -> {
                    val asked = inbox?.askForRating
                    inbox = event.inbox
                    val id = conversationId
                    if (asked != null && asked != event.inbox.askForRating && id != null) {
                        conversation = runCatching { client.conversation(id) }.getOrNull() ?: conversation
                    }
                }
                is YuvaEvent.ConnectionChanged -> connected = event.connected
                is YuvaEvent.ResyncRequired -> load()
                else -> Unit
            }
        }
    }

    private suspend fun onMessage(message: YuvaMessage) {
        if (message.conversationId != conversationId) return
        upsert(message)
        if (message.author.type.isTeam) {
            typingName = null
            markRead()
        }
    }

    val showsRating: Boolean
        get() {
            val current = conversation ?: return false
            if (current.isFeedback) return false
            return current.status == YuvaConversationStatus.CLOSED && (current.canRate || current.rating != null)
        }

    val feedbackState: YuvaFeedbackState?
        get() {
            val current = conversation?.takeIf { it.isFeedback } ?: return null
            val last = items.lastOrNull { it.message != null }?.message?.author?.type ?: current.lastMessage?.authorType
            return YuvaFeedbackState.of(current.status, last, current.feedback?.progress)
        }

    val feedbackResolved: Boolean
        get() = conversation?.let { it.isFeedback && it.status == YuvaConversationStatus.CLOSED } == true

    fun rate(rating: YuvaRating, comment: String?) {
        val id = conversationId ?: return
        if (ratingSending) return
        ratingSending = true
        ratingError = null
        scope.launch {
            try {
                conversation = client.rate(id, rating, comment)
                ratingChoice = null
            } catch (error: YuvaException) {
                if (error.status == 409) {
                    conversation = runCatching { client.conversation(id) }.getOrNull() ?: conversation
                    ratingChoice = null
                } else {
                    ratingError = errorMessage(error)
                }
            } catch (error: Exception) {
                ratingError = errorMessage(error)
            } finally {
                ratingSending = false
            }
        }
    }

    fun send(text: String, uploads: List<YuvaUpload>) {
        val body = text.trim()
        if (body.isEmpty() && uploads.isEmpty()) return
        val clientId = UUID.randomUUID().toString()
        items.add(ThreadItem(clientId, null, body, uploads, DeliveryState.SENDING, Instant.now()))
        stopTyping()
        scope.launch { deliver(clientId) }
    }

    fun retry(clientId: String) {
        val index = items.indexOfFirst { it.id == clientId }
        if (index < 0) return
        items[index] = items[index].copy(state = DeliveryState.SENDING)
        scope.launch { deliver(clientId) }
    }

    private suspend fun deliver(clientId: String) {
        val item = items.firstOrNull { it.id == clientId } ?: return
        try {
            val id = conversationId
            val message = if (id != null) {
                client.sendMessage(id, item.body, item.uploads, clientId)
            } else {
                val started = client.startConversation(item.body, attachments = item.uploads, clientId = clientId)
                conversationId = started.conversation.id
                conversation = started.conversation
                started.message
            }
            upsert(message)
        } catch (_: Exception) {
            val index = items.indexOfFirst { it.id == clientId }
            if (index >= 0) items[index] = items[index].copy(state = DeliveryState.FAILED)
        }
    }

    private fun upsert(message: YuvaMessage) {
        val index = items.indexOfFirst { it.id == message.id || (message.clientId != null && it.id == message.clientId) }
        if (index >= 0) items[index] = ThreadItem.of(message) else items.add(ThreadItem.of(message))
    }

    suspend fun markRead() {
        val id = conversationId ?: return
        if (!visible) return
        val last = items.lastOrNull { it.message?.author?.type?.isTeam == true }?.message ?: return
        runCatching { client.markRead(id, last.id) }
    }

    fun typing(text: String) {
        val id = conversationId ?: return
        if (text.isEmpty()) return
        typingIdle?.cancel()
        typingIdle = scope.launch {
            delay(4000)
            stopTyping()
        }
        val now = System.currentTimeMillis()
        if (now - typingSentAt < 3000) return
        typingSentAt = now
        scope.launch { runCatching { client.setTyping(id, true) } }
    }

    fun stopTyping() {
        typingIdle?.cancel()
        val id = conversationId ?: return
        if (typingSentAt == 0L) return
        typingSentAt = 0L
        scope.launch { runCatching { client.setTyping(id, false) } }
    }

    val seenMessageId: String?
        get() {
            val readAt = memberReadAt ?: return null
            return items.lastOrNull { it.mine && it.message != null && !it.createdAt.isAfter(readAt) }?.id
        }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun YuvaThreadView(
    client: YuvaClient,
    conversationId: String?,
    modifier: Modifier = Modifier,
    onBack: (() -> Unit)? = null,
) {
    val scope = rememberCoroutineScope()
    val state = remember(client, conversationId) { ThreadState(client, conversationId, scope) }
    val context = LocalContext.current
    var text by remember { mutableStateOf("") }
    val uploads = remember { mutableStateListOf<YuvaUpload>() }
    val listState = rememberLazyListState()
    val haptics = LocalHapticFeedback.current
    var viewing by remember { mutableStateOf<String?>(null) }
    var unseen by remember { mutableStateOf(false) }
    val awayFromBottom by remember {
        derivedStateOf {
            val info = listState.layoutInfo
            val last = info.visibleItemsInfo.lastOrNull()?.index ?: return@derivedStateOf false
            last < info.totalItemsCount - 2 && listState.canScrollForward
        }
    }
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.PickMultipleVisualMedia(10)) { uris ->
        scope.launch {
            val picked = withContext(Dispatchers.IO) { uris.mapNotNull { imageUpload(context, it) } }
            uploads.addAll(picked)
        }
    }

    LaunchedEffect(state) { state.load() }
    LaunchedEffect(state) { state.listen() }
    DisposableEffect(state) {
        state.visible = true
        scope.launch { state.markRead() }
        onDispose {
            state.visible = false
            state.stopTyping()
        }
    }
    val lastItem = state.items.lastOrNull()
    var settled by remember(state) { mutableStateOf(false) }
    LaunchedEffect(lastItem?.id, state.typingName, state.showsRating, state.feedbackState) {
        val info = listState.layoutInfo
        val count = info.totalItemsCount
        if (count == 0) return@LaunchedEffect
        val lastVisible = info.visibleItemsInfo.lastOrNull()?.index ?: 0
        if (!settled || lastItem?.mine == true && lastItem.message == null || lastVisible >= count - 3) {
            listState.animateScrollToItem(count - 1)
            settled = state.items.isNotEmpty()
        } else if (lastItem != null && !lastItem.mine) {
            unseen = true
        }
    }
    LaunchedEffect(awayFromBottom) { if (!awayFromBottom) unseen = false }
    viewing?.let { id -> ImageViewer(client, id) { viewing = null } }

    val title = state.conversation?.subject?.takeIf { it.isNotEmpty() }
        ?: if (state.conversation?.isFeedback == true) stringResource(R.string.yuva_feedback_kind) else null
        ?: state.inbox?.name
        ?: stringResource(R.string.yuva_thread_title)
    Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                title = {
                    val inbox = state.inbox
                    val feedback = state.conversation?.isFeedback == true
                    val category = state.conversation?.feedback
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        if (feedback) {
                            CategoryIcon(category?.categoryKey, 40.dp)
                            Spacer(Modifier.size(12.dp))
                        } else if (inbox != null) {
                            Avatar(inbox.name.take(1).uppercase(), 40.dp, inbox.branding.logoUrl)
                            Spacer(Modifier.size(12.dp))
                        }
                        Column {
                            Text(
                                if (feedback && category != null) categoryTitle(category) else title,
                                style = MaterialTheme.typography.titleLarge,
                                fontWeight = FontWeight.SemiBold,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                            if (feedback && category != null) {
                                Text(
                                    stringResource(R.string.yuva_feedback_kind),
                                    style = MaterialTheme.typography.bodyMedium,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    maxLines = 1,
                                )
                            } else if (!feedback && inbox != null && title != inbox.name) {
                                Text(
                                    inbox.name,
                                    style = MaterialTheme.typography.bodyMedium,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                )
                            }
                        }
                    }
                },
                navigationIcon = { if (onBack != null) BackButton(onBack) },
                actions = {
                    state.feedbackState?.let { feedbackState ->
                        FeedbackTag(
                            stringResource(stateLabel(feedbackState)),
                            stateTint(feedbackState),
                            Modifier.padding(end = 12.dp).testTag("yuva.feedback.state." + feedbackState.name.lowercase()),
                        )
                    }
                },
            )
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding).imePadding()) {
            if (!state.connected) {
                Text(
                    stringResource(R.string.yuva_connection_connecting),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSecondaryContainer,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp).clip(CircleShape)
                        .background(MaterialTheme.colorScheme.secondaryContainer).padding(6.dp),
                )
            }
            Box(Modifier.weight(1f)) {
                LazyColumn(
                    state = listState,
                    modifier = Modifier.fillMaxSize(),
                    verticalArrangement = Arrangement.Bottom,
                ) {
                    if (state.olderCursor != null) {
                        item(key = "older") {
                            LaunchedEffect(Unit) { state.loadOlder() }
                            Box(Modifier.fillMaxWidth().padding(8.dp), contentAlignment = Alignment.Center) {
                                CircularProgressIndicator(Modifier.size(20.dp))
                            }
                        }
                    }
                    if (state.items.isEmpty() && !state.loading && !state.failed) {
                        item(key = "greeting") { Greeting(state.inbox) }
                    }
                    val lastMine = state.items.lastOrNull { it.mine }?.id
                    val seen = state.seenMessageId
                    val rows = threadRows(state.items)
                    items(rows, key = { it.item.id }) { row ->
                        val item = row.item
                        if (row.startsDay) {
                            Text(
                                ShortTime.separator(context, item.createdAt),
                                style = MaterialTheme.typography.labelMedium,
                                fontWeight = FontWeight.Medium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                textAlign = TextAlign.Center,
                                modifier = Modifier.fillMaxWidth().padding(top = 20.dp, bottom = 8.dp),
                            )
                        }
                        MessageRow(
                            item = item,
                            client = client,
                            first = row.first,
                            last = row.last,
                            seen = item.id == seen,
                            showStatus = item.mine && (item.id == lastMine || item.state != DeliveryState.SENT),
                            onRetry = { state.retry(item.id) },
                            onOpenImage = { viewing = it },
                            modifier = Modifier.padding(top = if (row.first && !row.startsDay) 12.dp else 2.dp).animateItem(
                                fadeInSpec = spring(stiffness = Spring.StiffnessMediumLow),
                                placementSpec = spring(dampingRatio = 0.8f, stiffness = Spring.StiffnessMediumLow),
                            ),
                        )
                    }
                    val noteState = if (state.feedbackResolved) YuvaFeedbackState.RESOLVED
                    else state.feedbackState ?: YuvaFeedbackState.SENT
                    if (noteState == YuvaFeedbackState.RESOLVED || noteState.isProgress) {
                        item(key = "resolved") {
                            val tint = stateTint(noteState)
                            Box(Modifier.fillMaxWidth().padding(horizontal = 24.dp, vertical = 16.dp), contentAlignment = Alignment.Center) {
                                Row(
                                    Modifier.clip(CircleShape).background(tint.copy(alpha = if (isDarkSurface()) 0.18f else 0.1f))
                                        .padding(start = 12.dp, end = 16.dp, top = 8.dp, bottom = 8.dp)
                                        .testTag("yuva.feedback.resolvedNote"),
                                    verticalAlignment = Alignment.CenterVertically,
                                ) {
                                    Icon(
                                        if (noteState.isProgress) Icons.Default.Info else Icons.Default.CheckCircle,
                                        contentDescription = null,
                                        tint = tint,
                                        modifier = Modifier.size(18.dp),
                                    )
                                    Spacer(Modifier.size(8.dp))
                                    Text(
                                        stringResource(noteLabel(noteState)),
                                        style = MaterialTheme.typography.labelLarge,
                                        color = MaterialTheme.colorScheme.onSurface,
                                        textAlign = TextAlign.Center,
                                    )
                                }
                            }
                        }
                    }
                    if (state.showsRating) {
                        item(key = "rating") { RatingCard(state) }
                    }
                    state.typingName?.let { name ->
                        item(key = "typing") {
                            val label = if (name.isEmpty()) stringResource(R.string.yuva_thread_typing_someone)
                            else stringResource(R.string.yuva_thread_typing, name)
                            Row(
                                Modifier.fillMaxWidth().padding(start = 12.dp, end = 12.dp, top = 12.dp)
                                    .semantics(mergeDescendants = true) { contentDescription = label }
                                    .testTag("yuva.thread.typing")
                                    .animateItem(fadeInSpec = spring(stiffness = Spring.StiffnessMediumLow)),
                                verticalAlignment = Alignment.Bottom,
                            ) {
                                Avatar(name.take(1).uppercase(), ThreadAvatar)
                                Spacer(Modifier.size(8.dp))
                                Box(
                                    Modifier.clip(RoundedCornerShape(BubbleRadius))
                                        .background(MaterialTheme.colorScheme.surfaceContainerHigh)
                                        .padding(horizontal = 18.dp, vertical = 16.dp),
                                ) { TypingDots(MaterialTheme.colorScheme.onSurfaceVariant) }
                            }
                        }
                    }
                }
                if (state.loading) CircularProgressIndicator(Modifier.align(Alignment.Center))
                if (state.failed && !state.loading) {
                    Placeholder(
                        title = stringResource(state.failure),
                        action = stringResource(R.string.yuva_error_retry),
                        onAction = { scope.launch { state.load() } },
                    )
                }
                androidx.compose.animation.AnimatedVisibility(
                    awayFromBottom,
                    enter = scaleIn(spring(dampingRatio = 0.6f, stiffness = Spring.StiffnessMedium)) + fadeIn(),
                    exit = scaleOut() + fadeOut(),
                    modifier = Modifier.align(Alignment.BottomEnd).padding(16.dp),
                ) {
                    BadgedBox(badge = { if (unseen) Badge(containerColor = MaterialTheme.colorScheme.primary) }) {
                        SmallFloatingActionButton(
                            onClick = {
                                scope.launch {
                                    val count = listState.layoutInfo.totalItemsCount
                                    if (count > 0) listState.animateScrollToItem(count - 1)
                                }
                            },
                            containerColor = MaterialTheme.colorScheme.surfaceContainerHighest,
                            contentColor = MaterialTheme.colorScheme.onSurface,
                            modifier = Modifier.testTag("yuva.thread.scrollToBottom"),
                        ) {
                            Icon(Icons.Default.KeyboardArrowDown, contentDescription = stringResource(R.string.yuva_thread_scroll_bottom))
                        }
                    }
                }
            }
            YuvaStatusLine(state.inbox?.status)
            Composer(
                text = text,
                uploads = uploads,
                onText = {
                    text = it
                    state.typing(it)
                },
                onAttach = { picker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)) },
                onSend = {
                    haptics.performHapticFeedback(HapticFeedbackType.Confirm)
                    state.send(text, uploads.toList())
                    text = ""
                    uploads.clear()
                },
            )
        }
    }
}

@Composable
private fun Composer(
    text: String,
    uploads: MutableList<YuvaUpload>,
    onText: (String) -> Unit,
    onAttach: () -> Unit,
    onSend: () -> Unit,
) {
    val canSend = text.isNotBlank() || uploads.isNotEmpty()
    Row(
        Modifier.fillMaxWidth().padding(start = 12.dp, end = 12.dp, top = 6.dp, bottom = 10.dp),
        verticalAlignment = Alignment.Bottom,
    ) {
        FilledTonalIconButton(
            onClick = onAttach,
            modifier = Modifier.padding(bottom = 4.dp).size(48.dp),
        ) {
            Icon(Icons.Default.Add, contentDescription = stringResource(R.string.yuva_thread_attach))
        }
        Spacer(Modifier.size(8.dp))
        Surface(
            shape = RoundedCornerShape(28.dp),
            color = MaterialTheme.colorScheme.surfaceContainerHigh,
            modifier = Modifier.weight(1f),
        ) {
            Column(Modifier.animateContentSize(spring(dampingRatio = 0.8f, stiffness = Spring.StiffnessMediumLow))) {
                if (uploads.isNotEmpty()) {
                    LazyRow(
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        modifier = Modifier.padding(start = 12.dp, end = 12.dp, top = 12.dp),
                    ) {
                        itemsIndexed(uploads) { index, upload ->
                            Box {
                                UploadImage(upload, Modifier.size(64.dp).clip(RoundedCornerShape(16.dp)), crop = true)
                                Icon(
                                    Icons.Default.Close,
                                    contentDescription = null,
                                    modifier = Modifier.align(Alignment.TopEnd).padding(4.dp).size(20.dp)
                                        .clip(CircleShape)
                                        .background(MaterialTheme.colorScheme.surface)
                                        .clickable { uploads.removeAt(index) }
                                        .padding(2.dp),
                                )
                            }
                        }
                    }
                }
                Row(verticalAlignment = Alignment.Bottom) {
                    TextField(
                        value = text,
                        onValueChange = onText,
                        placeholder = { Text(stringResource(R.string.yuva_thread_placeholder)) },
                        maxLines = 5,
                        colors = TextFieldDefaults.colors(
                            focusedContainerColor = Color.Transparent,
                            unfocusedContainerColor = Color.Transparent,
                            disabledContainerColor = Color.Transparent,
                            focusedIndicatorColor = Color.Transparent,
                            unfocusedIndicatorColor = Color.Transparent,
                            disabledIndicatorColor = Color.Transparent,
                            errorIndicatorColor = Color.Transparent,
                        ),
                        modifier = Modifier.weight(1f).testTag("yuva.thread.input"),
                    )
                    val scale by animateFloatAsState(
                        if (canSend) 1f else 0.6f,
                        spring(dampingRatio = 0.5f, stiffness = Spring.StiffnessMedium),
                        label = "send",
                    )
                    FilledIconButton(
                        onClick = onSend,
                        enabled = canSend,
                        modifier = Modifier.padding(end = 4.dp, bottom = 4.dp).size(48.dp)
                            .graphicsLayer {
                                scaleX = scale
                                scaleY = scale
                                alpha = ((scale - 0.6f) / 0.4f).coerceIn(0f, 1f)
                            }
                            .testTag("yuva.thread.send"),
                    ) {
                        Icon(
                            Icons.AutoMirrored.Filled.Send,
                            contentDescription = stringResource(R.string.yuva_thread_send),
                            modifier = Modifier.size(22.dp),
                        )
                    }
                }
            }
        }
    }
}

private fun noteLabel(state: YuvaFeedbackState): Int = when (state) {
    YuvaFeedbackState.PLANNED -> R.string.yuva_feedback_note_planned
    YuvaFeedbackState.IN_PROGRESS -> R.string.yuva_feedback_note_in_progress
    YuvaFeedbackState.DONE -> R.string.yuva_feedback_note_done
    YuvaFeedbackState.WONT_DO -> R.string.yuva_feedback_note_wont_do
    else -> R.string.yuva_feedback_resolved_note
}

@Composable
private fun Greeting(inbox: YuvaInbox?) {
    if (inbox == null) return
    Column(Modifier.fillMaxWidth().padding(24.dp), horizontalAlignment = Alignment.CenterHorizontally) {
        PresenceHeader(inbox)
        inbox.branding.greeting?.takeIf { it.isNotEmpty() }?.let {
            Spacer(Modifier.size(8.dp))
            Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

@Composable
private fun RatingCard(state: ThreadState) {
    var comment by remember { mutableStateOf("") }
    val rated = state.conversation?.rating
    Column(
        Modifier
            .fillMaxWidth()
            .padding(start = 16.dp, end = 16.dp, top = 8.dp)
            .clip(RoundedCornerShape(28.dp))
            .background(MaterialTheme.colorScheme.surfaceContainerHigh)
            .padding(20.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        if (rated != null) {
            val label = stringResource(ratingLabel(rated))
            Text(
                ratingEmoji(rated),
                style = MaterialTheme.typography.headlineSmall,
                modifier = Modifier.semantics { contentDescription = label },
            )
            Text(
                stringResource(R.string.yuva_rating_thanks),
                style = MaterialTheme.typography.bodyMedium,
                textAlign = TextAlign.Center,
                modifier = Modifier.testTag("yuva.rating.thanks"),
            )
            return@Column
        }
        Text(stringResource(R.string.yuva_rating_prompt), style = MaterialTheme.typography.titleSmall, textAlign = TextAlign.Center)
        Row(horizontalArrangement = Arrangement.spacedBy(16.dp)) {
            YuvaRating.entries.forEach { rating ->
                val selected = state.ratingChoice == rating
                val label = stringResource(ratingLabel(rating))
                val modifier = Modifier
                    .testTag("yuva.rating.${rating.value}")
                    .semantics {
                        contentDescription = label
                        this.selected = selected
                    }
                val content: @Composable () -> Unit = {
                    Text(ratingEmoji(rating), style = MaterialTheme.typography.titleLarge)
                }
                if (selected) {
                    FilledTonalButton(onClick = {}, enabled = !state.ratingSending, modifier = modifier) { content() }
                } else {
                    OutlinedButton(onClick = { state.ratingChoice = rating }, enabled = !state.ratingSending, modifier = modifier) {
                        content()
                    }
                }
            }
        }
        val choice = state.ratingChoice ?: return@Column
        OutlinedTextField(
            value = comment,
            onValueChange = { comment = it.take(2000) },
            placeholder = { Text(stringResource(R.string.yuva_rating_comment_placeholder)) },
            minLines = 2,
            maxLines = 5,
            enabled = !state.ratingSending,
            shape = RoundedCornerShape(20.dp),
            modifier = Modifier.fillMaxWidth().testTag("yuva.rating.comment"),
        )
        state.ratingError?.let {
            Text(stringResource(it), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.error)
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            TextButton(
                onClick = { state.rate(choice, null) },
                enabled = !state.ratingSending,
                modifier = Modifier.testTag("yuva.rating.skip"),
            ) { Text(stringResource(R.string.yuva_rating_skip)) }
            Button(
                onClick = { state.rate(choice, comment) },
                enabled = !state.ratingSending,
                modifier = Modifier.testTag("yuva.rating.send"),
            ) { Text(stringResource(R.string.yuva_rating_send)) }
        }
    }
}

private fun ratingEmoji(rating: YuvaRating) = if (rating == YuvaRating.GOOD) "👍" else "👎"

private fun ratingLabel(rating: YuvaRating) =
    if (rating == YuvaRating.GOOD) R.string.yuva_rating_good else R.string.yuva_rating_bad

internal data class ThreadRowInfo(val item: ThreadItem, val startsDay: Boolean, val first: Boolean, val last: Boolean)

private const val GROUP_GAP_MS = 5 * 60 * 1000L

private fun senderKey(item: ThreadItem): String {
    if (item.mine) return "me"
    val author = item.message?.author ?: return ""
    return author.type.wire + ":" + author.name.orEmpty()
}

private fun joined(a: ThreadItem, b: ThreadItem): Boolean =
    senderKey(a) == senderKey(b) && b.createdAt.toEpochMilli() - a.createdAt.toEpochMilli() < GROUP_GAP_MS

internal fun threadRows(items: List<ThreadItem>): List<ThreadRowInfo> = items.mapIndexed { index, item ->
    val previous = items.getOrNull(index - 1)
    val next = items.getOrNull(index + 1)
    val startsDay = previous == null || !ShortTime.sameDay(previous.createdAt, item.createdAt)
    val joinsPrevious = previous != null && !startsDay && joined(previous, item)
    val joinsNext = next != null && ShortTime.sameDay(item.createdAt, next.createdAt) && joined(item, next)
    ThreadRowInfo(item, startsDay, first = !joinsPrevious, last = !joinsNext)
}

private val BubbleRadius = 22.dp
private val BubbleJoined = 6.dp
private val ThreadAvatar = 32.dp

@Composable
private fun MessageRow(
    item: ThreadItem,
    client: YuvaClient,
    first: Boolean,
    last: Boolean,
    seen: Boolean,
    showStatus: Boolean,
    onRetry: () -> Unit,
    onOpenImage: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    val mine = item.mine
    val author = item.message?.author
    Row(
        modifier.fillMaxWidth().padding(horizontal = 12.dp),
        horizontalArrangement = if (mine) Arrangement.End else Arrangement.Start,
        verticalAlignment = Alignment.Bottom,
    ) {
        if (!mine && author != null) {
            if (first) {
                Avatar(author.initials?.takeIf { it.isNotEmpty() } ?: author.name.orEmpty().take(1).uppercase(), ThreadAvatar)
            } else {
                Spacer(Modifier.size(ThreadAvatar))
            }
            Spacer(Modifier.size(8.dp))
        }
        Column(
            horizontalAlignment = if (mine) Alignment.End else Alignment.Start,
            modifier = Modifier.widthIn(max = 300.dp),
        ) {
            val authorName = author?.name
            if (first && !mine && !authorName.isNullOrEmpty()) {
                Text(
                    authorName,
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(start = 12.dp, bottom = 4.dp),
                )
            }
            val message = item.message
            if (message != null) {
                message.attachments.forEach { attachment ->
                    if (attachment.isImage) {
                        AttachmentImage(client, attachment.id) { onOpenImage(attachment.id) }
                    } else {
                        FileChip(client, attachment)
                    }
                    Spacer(Modifier.size(4.dp))
                }
            } else {
                item.uploads.forEach { upload ->
                    UploadImage(upload, Modifier.widthIn(max = 220.dp).heightIn(max = 260.dp).clip(RoundedCornerShape(14.dp)))
                    Spacer(Modifier.size(4.dp))
                }
            }
            if (item.body.isNotEmpty()) {
                val big = BubbleRadius
                val small = BubbleJoined
                val shape = if (mine) {
                    RoundedCornerShape(topStart = big, topEnd = if (first) big else small, bottomEnd = if (last) big else small, bottomStart = big)
                } else {
                    RoundedCornerShape(topStart = if (first) big else small, topEnd = big, bottomEnd = big, bottomStart = if (last) big else small)
                }
                val content = if (mine) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurface
                val linkColor = if (mine) content else MaterialTheme.colorScheme.primary
                val body = remember(item.body, linkColor) { linkified(item.body, linkColor) }
                SelectionContainer {
                    Text(
                        body,
                        color = content,
                        style = MaterialTheme.typography.bodyLarge,
                        modifier = Modifier
                            .alpha(if (item.state == DeliveryState.SENDING) 0.6f else 1f)
                            .clip(shape)
                            .background(if (mine) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.surfaceContainerHigh)
                            .padding(horizontal = 16.dp, vertical = 10.dp),
                    )
                }
            }
            if (last || item.state != DeliveryState.SENT) {
                val style = MaterialTheme.typography.labelSmall
                val subtle = MaterialTheme.colorScheme.onSurfaceVariant
                Row(Modifier.padding(horizontal = 8.dp, vertical = 3.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(ShortTime.clock(context, item.createdAt), style = style, color = subtle)
                    if (showStatus) {
                        when (item.state) {
                            DeliveryState.SENDING ->
                                Text(" · " + stringResource(R.string.yuva_thread_sending), style = style, color = subtle)
                            DeliveryState.FAILED -> Text(
                                " · " + stringResource(R.string.yuva_thread_failed),
                                style = style,
                                color = MaterialTheme.colorScheme.error,
                                modifier = Modifier.clickable(onClick = onRetry),
                            )
                            DeliveryState.SENT -> Text(
                                " · " + stringResource(if (seen) R.string.yuva_thread_seen else R.string.yuva_thread_sent),
                                style = style,
                                color = subtle,
                                modifier = Modifier.testTag(if (seen) "yuva.thread.seen" else "yuva.thread.sent"),
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun FileChip(client: YuvaClient, attachment: YuvaAttachment) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var loading by remember { mutableStateOf(false) }
    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier
            .clip(RoundedCornerShape(16.dp))
            .background(MaterialTheme.colorScheme.secondaryContainer)
            .clickable(enabled = !loading) {
                scope.launch {
                    loading = true
                    open(context, client, attachment)
                    loading = false
                }
            }
            .padding(horizontal = 14.dp, vertical = 10.dp),
    ) {
        Icon(YuvaIcons.Attach, contentDescription = null, modifier = Modifier.size(16.dp), tint = MaterialTheme.colorScheme.onSecondaryContainer)
        Spacer(Modifier.size(6.dp))
        Text(
            "${attachment.filename} · ${fileSize(context, attachment.size)}",
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSecondaryContainer,
        )
    }
}

private suspend fun open(context: Context, client: YuvaClient, attachment: YuvaAttachment) {
    val bytes = runCatching { client.attachment(attachment.id) }.getOrNull() ?: return
    val file = withContext(Dispatchers.IO) {
        File(context.cacheDir, "yuva/${attachment.id}").apply { mkdirs() }.resolve(attachment.filename.replace("/", "_"))
            .apply { writeBytes(bytes) }
    }
    val uri = FileProvider.getUriForFile(context, context.packageName + ".yuva.files", file)
    val intent = Intent(Intent.ACTION_VIEW).setDataAndType(uri, attachment.contentType)
        .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
    runCatching { context.startActivity(Intent.createChooser(intent, attachment.filename).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) }
}

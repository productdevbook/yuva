package dev.yuva.ui

import android.content.Context
import android.content.Intent
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
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
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
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
import androidx.compose.ui.unit.dp
import androidx.core.content.FileProvider
import dev.yuva.R
import dev.yuva.YuvaAttachment
import dev.yuva.YuvaAuthorType
import dev.yuva.YuvaClient
import dev.yuva.YuvaConversation
import dev.yuva.YuvaConversationStatus
import dev.yuva.YuvaEvent
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
        } catch (_: Exception) {
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
                    conversation = runCatching { client.conversation(event.conversationId) }.getOrNull() ?: conversation
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
        if (message.author.type == YuvaAuthorType.MEMBER) {
            typingName = null
            markRead()
        }
    }

    val showsRating: Boolean
        get() {
            val current = conversation ?: return false
            return current.status == YuvaConversationStatus.CLOSED && (current.canRate || current.rating != null)
        }

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
        val last = items.lastOrNull { it.message?.author?.type == YuvaAuthorType.MEMBER }?.message ?: return
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
    val lastId = state.items.lastOrNull()?.id
    LaunchedEffect(lastId, state.typingName, state.showsRating) {
        val count = listState.layoutInfo.totalItemsCount
        if (count > 0) listState.animateScrollToItem(count - 1)
    }

    val title = state.conversation?.subject?.takeIf { it.isNotEmpty() }
        ?: state.inbox?.name
        ?: stringResource(R.string.yuva_thread_title)
    Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                title = { Text(title) },
                navigationIcon = { if (onBack != null) BackButton(onBack) },
            )
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding).imePadding()) {
            if (!state.connected) {
                Text(
                    stringResource(R.string.yuva_connection_connecting),
                    style = MaterialTheme.typography.labelSmall,
                    modifier = Modifier.fillMaxWidth().background(MaterialTheme.colorScheme.surfaceVariant).padding(4.dp),
                )
            }
            Box(Modifier.weight(1f)) {
                LazyColumn(
                    state = listState,
                    modifier = Modifier.fillMaxSize(),
                    verticalArrangement = Arrangement.spacedBy(6.dp, Alignment.Bottom),
                ) {
                    if (state.olderCursor != null) {
                        item(key = "older") {
                            LaunchedEffect(Unit) { state.loadOlder() }
                            Box(Modifier.fillMaxWidth().padding(8.dp), contentAlignment = Alignment.Center) {
                                CircularProgressIndicator(Modifier.size(20.dp))
                            }
                        }
                    }
                    if (state.items.isEmpty() && !state.loading) {
                        item(key = "greeting") { Greeting(state.inbox) }
                    }
                    val lastMine = state.items.lastOrNull { it.mine }?.id
                    val seen = state.seenMessageId
                    items(state.items, key = { it.id }) { item ->
                        MessageRow(
                            item = item,
                            client = client,
                            seen = item.id == seen,
                            showStatus = item.mine && (item.id == lastMine || item.state != DeliveryState.SENT),
                            onRetry = { state.retry(item.id) },
                        )
                    }
                    if (state.showsRating) {
                        item(key = "rating") { RatingCard(state) }
                    }
                    state.typingName?.let { name ->
                        item(key = "typing") {
                            Text(
                                if (name.isEmpty()) stringResource(R.string.yuva_thread_typing_someone)
                                else stringResource(R.string.yuva_thread_typing, name),
                                style = MaterialTheme.typography.labelMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                modifier = Modifier.padding(horizontal = 16.dp),
                            )
                        }
                    }
                }
                if (state.loading) CircularProgressIndicator(Modifier.align(Alignment.Center))
            }
            Surface(tonalElevation = 2.dp) {
                Column(Modifier.padding(vertical = 8.dp)) {
                    if (uploads.isNotEmpty()) {
                        LazyRow(
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
                        ) {
                            itemsIndexed(uploads) { index, upload ->
                                Box {
                                    UploadImage(upload, Modifier.size(56.dp).clip(RoundedCornerShape(8.dp)), crop = true)
                                    Icon(
                                        Icons.Default.Close,
                                        contentDescription = null,
                                        modifier = Modifier.align(Alignment.TopEnd).size(18.dp)
                                            .background(MaterialTheme.colorScheme.surface, RoundedCornerShape(9.dp))
                                            .clickable { uploads.removeAt(index) },
                                    )
                                }
                            }
                        }
                    }
                    Row(Modifier.padding(horizontal = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                        IconButton(onClick = {
                            picker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly))
                        }) {
                            Icon(Icons.Default.Add, contentDescription = stringResource(R.string.yuva_thread_attach))
                        }
                        OutlinedTextField(
                            value = text,
                            onValueChange = {
                                text = it
                                state.typing(it)
                            },
                            placeholder = { Text(stringResource(R.string.yuva_thread_placeholder)) },
                            maxLines = 5,
                            shape = RoundedCornerShape(20.dp),
                            modifier = Modifier.weight(1f).testTag("yuva.thread.input"),
                        )
                        IconButton(
                            onClick = {
                                state.send(text, uploads.toList())
                                text = ""
                                uploads.clear()
                            },
                            enabled = text.isNotBlank() || uploads.isNotEmpty(),
                            modifier = Modifier.testTag("yuva.thread.send"),
                        ) {
                            Icon(Icons.AutoMirrored.Filled.Send, contentDescription = stringResource(R.string.yuva_thread_send))
                        }
                    }
                }
            }
        }
    }
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
            .clip(RoundedCornerShape(16.dp))
            .background(MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.5f))
            .padding(16.dp),
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
            shape = RoundedCornerShape(12.dp),
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

@Composable
private fun MessageRow(item: ThreadItem, client: YuvaClient, seen: Boolean, showStatus: Boolean, onRetry: () -> Unit) {
    val mine = item.mine
    Column(
        Modifier.fillMaxWidth().padding(horizontal = 16.dp),
        horizontalAlignment = if (mine) Alignment.End else Alignment.Start,
    ) {
        Row(verticalAlignment = Alignment.Bottom) {
            val author = item.message?.author
            if (!mine && author?.type == YuvaAuthorType.MEMBER) {
                Avatar(author.initials.orEmpty())
                Spacer(Modifier.size(6.dp))
            }
            Column(
                horizontalAlignment = if (mine) Alignment.End else Alignment.Start,
                modifier = Modifier.widthIn(max = 300.dp),
            ) {
                val authorName = author?.name
                if (!mine && !authorName.isNullOrEmpty()) {
                    Text(authorName, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                val message = item.message
                if (message != null) {
                    message.attachments.forEach { attachment ->
                        if (attachment.isImage) AttachmentImage(client, attachment.id) else FileChip(client, attachment)
                        Spacer(Modifier.size(4.dp))
                    }
                } else {
                    item.uploads.forEach { upload ->
                        UploadImage(upload, Modifier.widthIn(max = 220.dp).heightIn(max = 260.dp).clip(RoundedCornerShape(12.dp)))
                        Spacer(Modifier.size(4.dp))
                    }
                }
                if (item.body.isNotEmpty()) {
                    SelectionContainer {
                        Text(
                            item.body,
                            color = if (mine) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurface,
                            modifier = Modifier
                                .clip(RoundedCornerShape(18.dp))
                                .background(if (mine) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.surfaceVariant)
                                .padding(horizontal = 12.dp, vertical = 8.dp),
                        )
                    }
                }
            }
        }
        if (showStatus) {
            val style = MaterialTheme.typography.labelSmall
            when (item.state) {
                DeliveryState.SENDING -> Text(stringResource(R.string.yuva_thread_sending), style = style)
                DeliveryState.FAILED -> Text(
                    stringResource(R.string.yuva_thread_failed),
                    style = style,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.clickable(onClick = onRetry),
                )
                DeliveryState.SENT -> Text(
                    stringResource(if (seen) R.string.yuva_thread_seen else R.string.yuva_thread_sent),
                    style = style,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.testTag(if (seen) "yuva.thread.seen" else "yuva.thread.sent"),
                )
            }
        }
    }
}

@Composable
private fun FileChip(client: YuvaClient, attachment: YuvaAttachment) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var loading by remember { mutableStateOf(false) }
    Text(
        "📎 ${attachment.filename} · ${fileSize(context, attachment.size)}",
        style = MaterialTheme.typography.labelMedium,
        modifier = Modifier
            .clip(RoundedCornerShape(16.dp))
            .background(MaterialTheme.colorScheme.surfaceVariant)
            .clickable(enabled = !loading) {
                scope.launch {
                    loading = true
                    open(context, client, attachment)
                    loading = false
                }
            }
            .padding(horizontal = 10.dp, vertical = 6.dp),
    )
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

package dev.yuva.ui

import android.icu.text.MeasureFormat
import android.icu.util.Measure
import android.icu.util.MeasureUnit
import android.icu.util.ULocale
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Create
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.animation.core.spring
import androidx.compose.animation.expandHorizontally
import androidx.compose.animation.scaleIn
import androidx.compose.animation.scaleOut
import androidx.compose.animation.shrinkHorizontally
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.LargeTopAppBar
import androidx.compose.material3.Surface
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.Dp
import androidx.compose.material3.Badge
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBarDefaults
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
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.unit.dp
import androidx.compose.ui.platform.LocalContext
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.runtime.derivedStateOf
import androidx.compose.ui.unit.IntOffset
import androidx.compose.foundation.background
import dev.yuva.R
import dev.yuva.TrackOpen
import dev.yuva.YuvaAuthorType
import dev.yuva.YuvaClient
import dev.yuva.YuvaConversation
import dev.yuva.YuvaConversationKind
import dev.yuva.YuvaConversationStatus
import dev.yuva.YuvaFeedbackState
import dev.yuva.feedbackState
import dev.yuva.withFeedbackProgress
import dev.yuva.isFeedback
import dev.yuva.YuvaEvent
import dev.yuva.YuvaInbox
import dev.yuva.YuvaInboxMode
import dev.yuva.YuvaSession
import kotlinx.coroutines.launch

internal enum class ConversationsTab { MESSAGES, FEEDBACK }

enum class YuvaConversationsSection { MESSAGES, FEEDBACK }

internal class ConversationsState(private val client: YuvaClient) {
    val messages = mutableStateListOf<YuvaConversation>()
    val feedback = mutableStateListOf<YuvaConversation>()
    var session by mutableStateOf<YuvaSession?>(null)
    var loading by mutableStateOf(true)
    var failed by mutableStateOf(false)
    var removal by mutableStateOf<Removal?>(null)
    var notice by mutableStateOf(false)
    private var messagesCursor: String? = null
    private var feedbackCursor: String? = null

    class Removal(val id: String)

    val hasFeedback: Boolean get() = feedback.isNotEmpty()

    fun items(tab: ConversationsTab): List<YuvaConversation> =
        if (tab == ConversationsTab.MESSAGES) messages else feedback

    suspend fun load() {
        loading = messages.isEmpty() && feedback.isEmpty()
        try {
            session = client.start()
            val sent = client.conversations(kind = listOf(YuvaConversationKind.FEEDBACK))
            val page = client.conversations(kind = listOf(YuvaConversationKind.CONVERSATION))
            messages.clear()
            messages.addAll(page.items)
            messagesCursor = page.nextCursor
            feedback.clear()
            feedback.addAll(sent.items)
            feedbackCursor = sent.nextCursor
            failed = false
        } catch (_: Exception) {
            failed = messages.isEmpty() && feedback.isEmpty()
        } finally {
            loading = false
        }
    }

    suspend fun loadMore(tab: ConversationsTab) {
        val messagesTab = tab == ConversationsTab.MESSAGES
        val cursor = (if (messagesTab) messagesCursor else feedbackCursor) ?: return
        if (messagesTab) messagesCursor = null else feedbackCursor = null
        val kind = if (messagesTab) YuvaConversationKind.CONVERSATION else YuvaConversationKind.FEEDBACK
        val page = runCatching { client.conversations(kind = listOf(kind), cursor = cursor) }.getOrNull() ?: run {
            if (messagesTab) messagesCursor = cursor else feedbackCursor = cursor
            return
        }
        val list = if (messagesTab) messages else feedback
        val known = list.map { it.id }.toSet()
        list.addAll(page.items.filter { it.id !in known })
        if (messagesTab) messagesCursor = page.nextCursor else feedbackCursor = page.nextCursor
    }

    suspend fun listen() {
        client.events().collect { event ->
            when (event) {
                is YuvaEvent.ConversationCreated -> upsert(event.conversation)
                is YuvaEvent.ConversationRemoved -> {
                    messages.removeAll { it.id == event.id }
                    feedback.removeAll { it.id == event.id }
                    removal = Removal(event.id)
                }
                is YuvaEvent.ConversationUpdated -> refresh(event.conversationId) { it.withFeedbackProgress(event.feedbackProgress) }
                is YuvaEvent.Read -> refresh(event.conversationId)
                is YuvaEvent.MessageCreated -> refresh(event.message.conversationId)
                is YuvaEvent.MessageRedacted -> refresh(event.message.conversationId)
                is YuvaEvent.Presence -> runCatching { session = client.refreshSession() }
                is YuvaEvent.InboxUpdated -> session = session?.copy(inbox = event.inbox)
                is YuvaEvent.ResyncRequired -> load()
                else -> Unit
            }
        }
    }

    private suspend fun refresh(id: String, adjust: (YuvaConversation) -> YuvaConversation = { it }) {
        runCatching { client.conversation(id) }.getOrNull()?.let { upsert(adjust(it)) }
    }

    fun upsert(conversation: YuvaConversation) {
        messages.removeAll { it.id == conversation.id }
        feedback.removeAll { it.id == conversation.id }
        val list = if (conversation.isFeedback) feedback else messages
        list.add(conversation)
        list.sortByDescending { it.lastMessageAt ?: it.createdAt }
    }
}

private sealed interface Route {
    data class Thread(val id: String?) : Route
    data object FeedbackForm : Route
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun YuvaConversationsView(
    client: YuvaClient,
    modifier: Modifier = Modifier,
    openConversationId: String? = null,
    onClose: (() -> Unit)? = null,
    section: YuvaConversationsSection? = null,
) {
    TrackOpen()
    val state = remember(client) { ConversationsState(client) }
    var route by remember { mutableStateOf<Route?>(null) }
    var tab by remember {
        mutableStateOf(if (section == YuvaConversationsSection.FEEDBACK) ConversationsTab.FEEDBACK else ConversationsTab.MESSAGES)
    }
    val scope = rememberCoroutineScope()
    LaunchedEffect(client) { state.load() }
    LaunchedEffect(client) { state.listen() }
    LaunchedEffect(state.removal) {
        val removed = state.removal ?: return@LaunchedEffect
        if ((route as? Route.Thread)?.id == removed.id) {
            route = null
            state.notice = true
        }
    }
    LaunchedEffect(openConversationId) { if (openConversationId != null) route = Route.Thread(openConversationId) }

    AnimatedContent(
        targetState = route,
        transitionSpec = {
            val forward = initialState == null
            val motion = spring<IntOffset>(dampingRatio = 0.9f, stiffness = Spring.StiffnessMediumLow)
            (slideInHorizontally(motion) { if (forward) it / 4 else -it / 4 } + fadeIn(spring(stiffness = Spring.StiffnessMediumLow)))
                .togetherWith(
                    slideOutHorizontally(motion) { if (forward) -it / 4 else it / 4 } + fadeOut(spring(stiffness = Spring.StiffnessMedium)),
                )
        },
        label = "route",
    ) { current ->
        when (current) {
            is Route.Thread -> {
                BackHandler { route = null }
                YuvaThreadView(client, current.id, modifier, onBack = { route = null })
            }
            Route.FeedbackForm -> {
                BackHandler { route = null }
                YuvaFeedbackView(
                    client,
                    modifier,
                    onViewFeedback = { conversation ->
                        state.upsert(conversation)
                        route = Route.Thread(conversation.id)
                    },
                    onDone = {
                        route = null
                        scope.launch { state.load() }
                    },
                )
            }
            null -> ConversationsList(
                state = state,
                section = section,
                tab = tab,
                onTab = { tab = it },
                onRoute = { route = it },
                onClose = onClose,
                modifier = modifier,
            )
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ConversationsList(
    state: ConversationsState,
    section: YuvaConversationsSection?,
    tab: ConversationsTab,
    onTab: (ConversationsTab) -> Unit,
    onRoute: (Route) -> Unit,
    onClose: (() -> Unit)?,
    modifier: Modifier,
) {
    val scope = rememberCoroutineScope()
    val snackbars = remember { SnackbarHostState() }
    val removedNotice = stringResource(R.string.yuva_conversation_removed)
    LaunchedEffect(state.notice) {
        if (state.notice) {
            state.notice = false
            snackbars.showSnackbar(removedNotice)
        }
    }
    val listState = rememberLazyListState()
    val expanded by remember { derivedStateOf { listState.firstVisibleItemIndex == 0 } }
    val dark = isDarkSurface()
    val showsFeedback = section == YuvaConversationsSection.FEEDBACK ||
        section == null && tab == ConversationsTab.FEEDBACK
    val scrollBehavior = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    val background = if (dark) MaterialTheme.colorScheme.surface else MaterialTheme.colorScheme.surfaceContainerHigh
    val tileColor = if (dark) MaterialTheme.colorScheme.surfaceContainerHigh else MaterialTheme.colorScheme.surfaceContainerLowest
    val showsList = !state.loading && !state.failed &&
        !(section == YuvaConversationsSection.FEEDBACK && state.feedback.isEmpty()) &&
        !(section == YuvaConversationsSection.MESSAGES && state.messages.isEmpty() ||
            section == null && state.messages.isEmpty() && !state.hasFeedback)
    Scaffold(
        modifier = modifier.nestedScroll(scrollBehavior.nestedScrollConnection),
        containerColor = background,
        snackbarHost = { SnackbarHost(snackbars) },
        topBar = {
            LargeTopAppBar(
                scrollBehavior = scrollBehavior,
                colors = TopAppBarDefaults.topAppBarColors(containerColor = background, scrolledContainerColor = background),
                title = {
                    val inbox = state.session?.inbox
                    Text(
                        if (showsFeedback || inbox == null) {
                            stringResource(if (showsFeedback) R.string.yuva_feedback_kind else R.string.yuva_conversations_title)
                        } else {
                            inbox.name
                        },
                        fontWeight = FontWeight.SemiBold,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                },
                navigationIcon = {
                    if (onClose != null) {
                        IconButton(onClick = onClose) { Icon(Icons.Default.Close, contentDescription = null) }
                    }
                },
            )
        },
        floatingActionButton = {
            AnimatedVisibility(
                showsList,
                enter = scaleIn(spring(dampingRatio = 0.6f, stiffness = Spring.StiffnessMediumLow)) + fadeIn(),
                exit = scaleOut() + fadeOut(),
            ) {
                if (section == YuvaConversationsSection.FEEDBACK) {
                    ExtendedFloatingActionButton(
                        text = { Text(stringResource(R.string.yuva_feedback_title)) },
                        icon = { Icon(Icons.Default.Add, contentDescription = null) },
                        expanded = expanded,
                        onClick = { onRoute(Route.FeedbackForm) },
                        modifier = Modifier.testTag("yuva.feedback.new"),
                    )
                } else {
                    ExtendedFloatingActionButton(
                        text = { Text(stringResource(R.string.yuva_conversations_new)) },
                        icon = { Icon(Icons.Default.Create, contentDescription = null) },
                        expanded = expanded,
                        onClick = { onRoute(Route.Thread(null)) },
                        modifier = Modifier.testTag("yuva.conversations.new"),
                    )
                }
            }
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            YuvaStatusLine(state.session?.inbox?.status)
            Box(Modifier.weight(1f).fillMaxWidth()) {
                when {
                    state.loading -> CircularProgressIndicator(Modifier.align(Alignment.Center))
                    state.failed -> Placeholder(
                        title = stringResource(R.string.yuva_error_generic),
                        action = stringResource(R.string.yuva_error_retry),
                        onAction = { scope.launch { state.load() } },
                    )
                    section == YuvaConversationsSection.FEEDBACK && state.feedback.isEmpty() -> Placeholder(
                        title = stringResource(R.string.yuva_feedback_kind),
                        icon = YuvaIcons.Lightbulb,
                        action = stringResource(R.string.yuva_feedback_title),
                        onAction = { onRoute(Route.FeedbackForm) },
                    )
                    section == YuvaConversationsSection.MESSAGES && state.messages.isEmpty() ||
                        section == null && state.messages.isEmpty() && !state.hasFeedback -> Placeholder(
                        title = stringResource(R.string.yuva_conversations_empty),
                        icon = YuvaIcons.Chat,
                        hint = stringResource(R.string.yuva_conversations_empty_hint),
                        action = stringResource(R.string.yuva_conversations_new),
                        onAction = { onRoute(Route.Thread(null)) },
                    )
                    else -> {
                        val items = state.items(tab)
                        LazyColumn(
                            Modifier.fillMaxSize(),
                            state = listState,
                            contentPadding = PaddingValues(start = 16.dp, end = 16.dp, bottom = 96.dp),
                            verticalArrangement = Arrangement.spacedBy(4.dp),
                        ) {
                            val inbox = state.session?.inbox
                            if (!showsFeedback && inbox != null) {
                                item(key = "presence") { PresenceLine(inbox, Modifier.padding(start = 4.dp, bottom = 12.dp)) }
                            }
                            if (section == null && state.hasFeedback) {
                                item(key = "tabs") {
                                    ConnectedToggleGroup(
                                        options = ConversationsTab.entries,
                                        selected = tab,
                                        onSelect = onTab,
                                        label = {
                                            stringResource(
                                                if (it == ConversationsTab.MESSAGES) R.string.yuva_conversations_messages
                                                else R.string.yuva_feedback_kind,
                                            )
                                        },
                                        tag = { "yuva.conversations.tab." + it.name.lowercase() },
                                        modifier = Modifier.padding(bottom = 16.dp),
                                    )
                                }
                            }
                            itemsIndexed(items, key = { _, it -> it.id }) { index, conversation ->
                                val shape = groupShape(index, items.size)
                                val tile = Modifier.animateItem(
                                    fadeInSpec = spring(stiffness = Spring.StiffnessMediumLow),
                                    placementSpec = spring(dampingRatio = 0.75f, stiffness = Spring.StiffnessMediumLow),
                                )
                                if (conversation.isFeedback) {
                                    FeedbackRow(conversation, shape, tileColor, tile) { onRoute(Route.Thread(conversation.id)) }
                                } else {
                                    ConversationRow(conversation, state.session?.inbox, shape, tileColor, tile) {
                                        onRoute(Route.Thread(conversation.id))
                                    }
                                }
                                if (conversation.id == items.lastOrNull()?.id) {
                                    LaunchedEffect(conversation.id, tab) { state.loadMore(tab) }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

internal fun groupShape(index: Int, count: Int, large: Dp = 24.dp, small: Dp = 6.dp): Shape {
    val top = if (index == 0) large else small
    val bottom = if (index == count - 1) large else small
    return RoundedCornerShape(topStart = top, topEnd = top, bottomStart = bottom, bottomEnd = bottom)
}

@Composable
internal fun <T> ConnectedToggleGroup(
    options: List<T>,
    selected: T,
    onSelect: (T) -> Unit,
    label: @Composable (T) -> String,
    tag: (T) -> String,
    modifier: Modifier = Modifier,
) {
    Row(modifier.fillMaxWidth().selectableGroup(), horizontalArrangement = Arrangement.spacedBy(3.dp)) {
        options.forEachIndexed { index, option ->
            val isSelected = option == selected
            val inner by animateDpAsState(
                if (isSelected) 26.dp else 8.dp,
                spring(dampingRatio = 0.6f, stiffness = Spring.StiffnessMedium),
                label = "corner",
            )
            val start = if (index == 0) 26.dp else inner
            val end = if (index == options.lastIndex) 26.dp else inner
            val container by animateColorAsState(
                if (isSelected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.surfaceBright,
                spring(stiffness = Spring.StiffnessMediumLow),
                label = "container",
            )
            val content by animateColorAsState(
                if (isSelected) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurface,
                spring(stiffness = Spring.StiffnessMediumLow),
                label = "content",
            )
            Surface(
                shape = RoundedCornerShape(topStart = start, bottomStart = start, topEnd = end, bottomEnd = end),
                color = container,
                contentColor = content,
                modifier = Modifier.weight(1f).heightIn(min = 52.dp)
                    .selectable(selected = isSelected, role = Role.Tab, onClick = { onSelect(option) })
                    .testTag(tag(option)),
            ) {
                Row(
                    Modifier.padding(horizontal = 12.dp, vertical = 14.dp),
                    horizontalArrangement = Arrangement.Center,
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    AnimatedVisibility(
                        isSelected,
                        enter = scaleIn(spring(dampingRatio = 0.5f, stiffness = Spring.StiffnessMedium)) + expandHorizontally(),
                        exit = scaleOut() + shrinkHorizontally(),
                    ) {
                        Icon(Icons.Default.Check, contentDescription = null, modifier = Modifier.padding(end = 8.dp).size(18.dp))
                    }
                    Text(label(option), style = MaterialTheme.typography.labelLarge, fontWeight = FontWeight.SemiBold, maxLines = 1)
                }
            }
        }
    }
}

@Composable
private fun PresenceLine(inbox: YuvaInbox, modifier: Modifier = Modifier) {
    val online = inbox.mode == YuvaInboxMode.LIVE && inbox.presence?.available == true
    Row(modifier, verticalAlignment = Alignment.CenterVertically) {
        Box(
            Modifier.size(8.dp).clip(CircleShape)
                .background(if (online) Color(0xFF34A853) else MaterialTheme.colorScheme.outline),
        )
        Spacer(Modifier.width(8.dp))
        Text(
            presenceText(inbox),
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

@Composable
internal fun Placeholder(
    title: String,
    icon: ImageVector? = null,
    hint: String? = null,
    action: String? = null,
    onAction: () -> Unit = {},
    secondaryAction: String? = null,
    onSecondaryAction: () -> Unit = {},
) {
    Column(
        Modifier.fillMaxSize().padding(32.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        if (icon != null) {
            Box(
                Modifier.size(112.dp).clip(PlaceholderShape).background(MaterialTheme.colorScheme.primaryContainer),
                contentAlignment = Alignment.Center,
            ) {
                Icon(
                    icon,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.onPrimaryContainer,
                    modifier = Modifier.size(48.dp),
                )
            }
            Spacer(Modifier.height(24.dp))
        }
        Text(title, style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.SemiBold, textAlign = TextAlign.Center)
        if (hint != null) {
            Spacer(Modifier.height(8.dp))
            Text(
                hint,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
            )
        }
        if (action != null) {
            Spacer(Modifier.height(24.dp))
            Button(
                onClick = onAction,
                contentPadding = PaddingValues(horizontal = 28.dp, vertical = 16.dp),
            ) { Text(action, style = MaterialTheme.typography.titleSmall) }
        }
        if (secondaryAction != null) {
            TextButton(onClick = onSecondaryAction, modifier = Modifier.testTag("yuva.feedback.view")) { Text(secondaryAction) }
        }
    }
}

@Composable
internal fun PresenceHeader(inbox: YuvaInbox, modifier: Modifier = Modifier) {
    Row(modifier, verticalAlignment = Alignment.CenterVertically) {
        val members = inbox.presence?.members.orEmpty().take(3)
        members.forEach { member ->
            Avatar(member.initials, 26.dp)
            Spacer(Modifier.width(4.dp))
        }
        if (members.isNotEmpty()) Spacer(Modifier.width(6.dp))
        Column {
            Text(inbox.name, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.SemiBold)
            Text(
                presenceText(inbox),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
internal fun presenceText(inbox: YuvaInbox): String {
    if (inbox.mode == YuvaInboxMode.LIVE) {
        return stringResource(if (inbox.presence?.available == true) R.string.yuva_presence_online else R.string.yuva_presence_away)
    }
    val minutes = inbox.expectedReplyMinutes ?: return stringResource(R.string.yuva_presence_away)
    return stringResource(R.string.yuva_presence_reply_in, replyDuration(minutes))
}

private fun replyDuration(minutes: Int): String {
    val (value, unit) = when {
        minutes < 60 -> minutes to MeasureUnit.MINUTE
        minutes < 1440 -> Math.round(minutes / 60.0).toInt() to MeasureUnit.HOUR
        else -> Math.round(minutes / 1440.0).toInt() to MeasureUnit.DAY
    }
    return MeasureFormat.getInstance(ULocale.getDefault(), MeasureFormat.FormatWidth.WIDE).format(Measure(value, unit))
}

internal val RowAvatar = 52.dp

private val PlaceholderShape = LobedShape(9, 0.08f)

@Composable
private fun ConversationListRow(
    title: String,
    preview: String,
    previewLines: Int,
    time: String,
    unread: Boolean,
    shape: Shape,
    color: Color,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    leading: @Composable () -> Unit,
    extra: @Composable () -> Unit = {},
) {
    Surface(
        onClick = onClick,
        shape = shape,
        color = color,
        modifier = modifier.fillMaxWidth(),
    ) {
        Row(Modifier.padding(horizontal = 16.dp, vertical = 16.dp), verticalAlignment = Alignment.Top) {
            leading()
            Spacer(Modifier.width(16.dp))
            Column(Modifier.weight(1f)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        title,
                        style = MaterialTheme.typography.titleMedium,
                        fontWeight = if (unread) FontWeight.Bold else FontWeight.SemiBold,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f),
                    )
                    Spacer(Modifier.width(8.dp))
                    Text(
                        time,
                        style = MaterialTheme.typography.labelMedium,
                        fontWeight = if (unread) FontWeight.Bold else FontWeight.Normal,
                        color = if (unread) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Spacer(Modifier.height(2.dp))
                Row(verticalAlignment = Alignment.Top) {
                    Text(
                        preview,
                        style = MaterialTheme.typography.bodyMedium,
                        fontWeight = if (unread) FontWeight.Medium else FontWeight.Normal,
                        color = if (unread) MaterialTheme.colorScheme.onSurface else MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = previewLines,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f),
                    )
                    if (unread) {
                        val label = stringResource(R.string.yuva_conversations_unread)
                        Badge(
                            Modifier.padding(start = 8.dp, top = 5.dp).size(10.dp).semantics { contentDescription = label },
                            containerColor = MaterialTheme.colorScheme.primary,
                        )
                    }
                }
                extra()
            }
        }
    }
}

@Composable
private fun ConversationRow(
    conversation: YuvaConversation,
    inbox: YuvaInbox?,
    shape: Shape,
    color: Color,
    modifier: Modifier,
    onClick: () -> Unit,
) {
    val context = LocalContext.current
    val title = conversation.subject.ifEmpty { inbox?.name ?: stringResource(R.string.yuva_thread_title) }
    val last = conversation.lastMessage
    val preview = when {
        last == null -> ""
        last.redacted -> stringResource(R.string.yuva_thread_removed)
        last.authorType == YuvaAuthorType.CONTACT -> stringResource(R.string.yuva_thread_you) + ": " + last.text
        else -> last.text
    }
    ConversationListRow(
        title = title,
        preview = preview,
        previewLines = 2,
        time = ShortTime.list(context, conversation.lastMessageAt ?: conversation.createdAt),
        unread = conversation.unread,
        shape = shape,
        color = color,
        onClick = onClick,
        modifier = modifier,
        leading = { Avatar((inbox?.name ?: title).take(1).uppercase(), RowAvatar, inbox?.branding?.logoUrl) },
    ) {
        if (conversation.status == YuvaConversationStatus.CLOSED) {
            FeedbackTag(
                stringResource(R.string.yuva_status_closed),
                MaterialTheme.colorScheme.onSurfaceVariant,
                Modifier.padding(top = 8.dp),
            )
        }
    }
}

@Composable
internal fun BackButton(onBack: () -> Unit) {
    IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = null) }
}

internal fun categoryLabel(category: dev.yuva.YuvaFeedbackCategory): Int = when (category) {
    dev.yuva.YuvaFeedbackCategory.BUG -> R.string.yuva_feedback_category_bug
    dev.yuva.YuvaFeedbackCategory.IDEA -> R.string.yuva_feedback_category_idea
    dev.yuva.YuvaFeedbackCategory.PRAISE -> R.string.yuva_feedback_category_praise
    dev.yuva.YuvaFeedbackCategory.OTHER -> R.string.yuva_feedback_category_other
}


@Composable
internal fun categoryTitle(feedback: dev.yuva.YuvaFeedbackInfo): String =
    feedback.categoryName ?: stringResource(categoryLabel(feedback.category))

internal fun stateLabel(state: YuvaFeedbackState): Int = when (state) {
    YuvaFeedbackState.SENT -> R.string.yuva_feedback_state_sent
    YuvaFeedbackState.ANSWERED -> R.string.yuva_feedback_state_answered
    YuvaFeedbackState.RESOLVED -> R.string.yuva_feedback_state_resolved
    YuvaFeedbackState.PLANNED -> R.string.yuva_feedback_state_planned
    YuvaFeedbackState.IN_PROGRESS -> R.string.yuva_feedback_state_in_progress
    YuvaFeedbackState.DONE -> R.string.yuva_feedback_state_done
    YuvaFeedbackState.WONT_DO -> R.string.yuva_feedback_state_wont_do
}

@Composable
private fun FeedbackRow(conversation: YuvaConversation, shape: Shape, color: Color, modifier: Modifier, onClick: () -> Unit) {
    val context = LocalContext.current
    val title = conversation.feedback?.let { categoryTitle(it) } ?: stringResource(R.string.yuva_feedback_kind)
    val lastText = conversation.lastMessage?.let { if (it.redacted) stringResource(R.string.yuva_thread_removed) else it.text }.orEmpty()
    val text = conversation.feedback?.text?.takeIf { it.isNotEmpty() }
        ?: conversation.subject.ifEmpty { lastText }
    ConversationListRow(
        title = title,
        preview = text.lineSequence().firstOrNull().orEmpty(),
        previewLines = 1,
        time = ShortTime.list(context, conversation.lastMessageAt ?: conversation.createdAt),
        unread = conversation.unread,
        shape = shape,
        color = color,
        onClick = onClick,
        modifier = modifier,
        leading = { CategoryIcon(conversation.feedback?.categoryKey, RowAvatar) },
    ) {
        Row(Modifier.padding(top = 8.dp), verticalAlignment = Alignment.CenterVertically) {
            conversation.feedbackState?.let { state ->
                FeedbackTag(
                    stringResource(stateLabel(state)),
                    stateTint(state),
                    Modifier.testTag("yuva.feedback.state." + state.name.lowercase()),
                )
            }
            if ((conversation.feedback?.attachmentCount ?: 0) > 0) {
                val label = stringResource(R.string.yuva_feedback_attachments)
                Icon(
                    YuvaIcons.Attach,
                    contentDescription = label,
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(start = 8.dp).size(16.dp),
                )
            }
        }
    }
}

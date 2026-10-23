package dev.yuva.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Build
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import dev.yuva.R
import dev.yuva.StatusDismissals
import dev.yuva.StatusNotice
import dev.yuva.YuvaInboxStatus
import dev.yuva.YuvaStatusImpact
import dev.yuva.YuvaStatusIncident
import dev.yuva.YuvaStatusKind
import dev.yuva.YuvaStatusState

@Composable
internal fun YuvaStatusLine(status: YuvaInboxStatus?, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val dismissals = remember { StatusDismissals.of(context) }
    LaunchedEffect(status) { dismissals.prune(status) }
    val notice = dismissals.notice(status)
    val last = remember { arrayOfNulls<StatusNotice>(1) }
    if (notice != null) last[0] = notice
    AnimatedVisibility(
        visible = notice != null,
        modifier = modifier,
        enter = expandVertically(spring(dampingRatio = 0.9f, stiffness = Spring.StiffnessMediumLow)) + fadeIn(),
        exit = shrinkVertically(spring(stiffness = Spring.StiffnessMedium)) + fadeOut(),
    ) {
        last[0]?.let { shown -> StatusTile(shown, status?.url.orEmpty(), onDismiss = { dismissals.dismiss(shown) }) }
    }
}

@Composable
private fun StatusTile(notice: StatusNotice, pageUrl: String, onDismiss: () -> Unit) {
    val lead = notice.lead
    val colors = statusColors(lead)
    val uri = LocalUriHandler.current
    val subtitle = statusSubtitle(notice)
    val open = stringResource(R.string.yuva_status_open_hint)
    val url = lead.url.ifEmpty { pageUrl }
    Surface(
        shape = RoundedCornerShape(24.dp),
        color = colors.container,
        contentColor = colors.content,
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp).testTag("yuva.status"),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Row(
                Modifier.weight(1f)
                    .clickable(onClickLabel = open, role = Role.Button) {
                        if (url.startsWith("http://") || url.startsWith("https://")) runCatching { uri.openUri(url) }
                    }
                    .semantics(mergeDescendants = true) { contentDescription = "${lead.title}, $subtitle" }
                    .padding(start = 14.dp, top = 12.dp, bottom = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Box(
                    Modifier.size(40.dp).clip(LobedShape(8, 0.1f)).background(colors.content.copy(alpha = 0.16f)),
                    contentAlignment = Alignment.Center,
                ) { Icon(statusIcon(lead), contentDescription = null, tint = colors.content, modifier = Modifier.size(22.dp)) }
                Column(Modifier.weight(1f).padding(start = 12.dp, end = 4.dp)) {
                    Text(lead.title, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.SemiBold)
                    Text(subtitle, style = MaterialTheme.typography.bodySmall, color = colors.content.copy(alpha = 0.85f))
                }
            }
            IconButton(onClick = onDismiss, modifier = Modifier.testTag("yuva.status.dismiss")) {
                Icon(Icons.Default.Close, contentDescription = stringResource(R.string.yuva_status_dismiss), tint = colors.content)
            }
        }
    }
}

@Composable
private fun statusSubtitle(notice: StatusNotice): String {
    val lead = notice.lead
    val parts = mutableListOf(kindLabel(lead))
    stateLabel(lead.state)?.let(parts::add)
    if (notice.more > 0) parts.add(stringResource(R.string.yuva_status_more, notice.more))
    return parts.joinToString(" · ")
}

@Composable
private fun kindLabel(incident: YuvaStatusIncident): String = stringResource(
    when {
        incident.kind == YuvaStatusKind.MAINTENANCE -> R.string.yuva_status_kind_maintenance
        incident.impact == YuvaStatusImpact.CRITICAL -> R.string.yuva_status_kind_critical
        incident.impact == YuvaStatusImpact.MAJOR -> R.string.yuva_status_kind_major
        incident.impact == YuvaStatusImpact.MINOR -> R.string.yuva_status_kind_minor
        else -> R.string.yuva_status_kind_incident
    },
)

@Composable
private fun stateLabel(state: YuvaStatusState): String? {
    val id = when (state) {
        YuvaStatusState.INVESTIGATING -> R.string.yuva_status_state_investigating
        YuvaStatusState.IDENTIFIED -> R.string.yuva_status_state_identified
        YuvaStatusState.MONITORING -> R.string.yuva_status_state_monitoring
        YuvaStatusState.RESOLVED -> R.string.yuva_status_state_resolved
        YuvaStatusState.SCHEDULED -> R.string.yuva_status_state_scheduled
        YuvaStatusState.IN_PROGRESS -> R.string.yuva_status_state_in_progress
        YuvaStatusState.COMPLETED -> R.string.yuva_status_state_completed
        YuvaStatusState.UNKNOWN -> return null
    }
    return stringResource(id)
}

private fun statusIcon(incident: YuvaStatusIncident): ImageVector = when {
    incident.kind == YuvaStatusKind.MAINTENANCE -> Icons.Default.Build
    incident.impact == YuvaStatusImpact.NONE -> Icons.Default.Info
    else -> Icons.Default.Warning
}

@Composable
private fun statusColors(incident: YuvaStatusIncident): TonalColors {
    val dark = isDarkSurface()
    return when {
        incident.kind == YuvaStatusKind.MAINTENANCE ->
            if (dark) TonalColors(Color(0xFF0F3D6E), Color(0xFFA8C8FF)) else TonalColors(Color(0xFFD3E4FF), Color(0xFF0B3D7A))
        incident.impact == YuvaStatusImpact.CRITICAL -> categoryColors("bug")
        incident.impact == YuvaStatusImpact.MAJOR ->
            if (dark) TonalColors(Color(0xFF5A2D0C), Color(0xFFFFB68A)) else TonalColors(Color(0xFFFFDCC4), Color(0xFF7A3500))
        incident.impact == YuvaStatusImpact.MINOR -> categoryColors("idea")
        else -> TonalColors(MaterialTheme.colorScheme.secondaryContainer, MaterialTheme.colorScheme.onSecondaryContainer)
    }
}

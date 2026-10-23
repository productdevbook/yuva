package dev.yuva.ui

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.Uri
import android.text.format.DateUtils
import android.text.format.Formatter
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Build
import androidx.compose.material.icons.filled.Create
import androidx.compose.material.icons.filled.Email
import androidx.compose.material.icons.filled.Favorite
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.Person
import androidx.compose.material.icons.filled.Phone
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.ShoppingCart
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.filled.ThumbUp
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Outline
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.LayoutDirection
import kotlin.math.PI
import kotlin.math.cos
import kotlin.math.sin
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.luminance
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.widthIn
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.keyframes
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.gestures.detectTransformGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import dev.yuva.R
import dev.yuva.YuvaAttachmentRefusal
import dev.yuva.YuvaFeedbackCategory
import dev.yuva.YuvaFeedbackState
import dev.yuva.YuvaClient
import dev.yuva.YuvaException
import dev.yuva.YuvaUpload
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.net.URL
import java.time.Instant
import java.time.ZoneId
import java.time.temporal.ChronoUnit
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext

@Composable
internal fun Avatar(initials: String, size: Dp = 28.dp, imageUrl: String? = null) {
    Box(
        modifier = Modifier.size(size).clip(AvatarShape).background(MaterialTheme.colorScheme.primaryContainer),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            initials.ifEmpty { "?" },
            color = MaterialTheme.colorScheme.onPrimaryContainer,
            fontSize = (size.value * 0.4f).sp,
            fontWeight = FontWeight.SemiBold,
            style = MaterialTheme.typography.labelMedium,
        )
        val logo = imageUrl?.let { url -> produceState<Bitmap?>(null, url) { value = LogoCache.image(url) }.value }
        if (logo != null) {
            Image(logo.asImageBitmap(), contentDescription = null, contentScale = ContentScale.Crop, modifier = Modifier.size(size))
        }
    }
}

internal class LobedShape(private val lobes: Int, private val depth: Float, private val rotation: Float = 0f) : Shape {
    override fun createOutline(size: Size, layoutDirection: LayoutDirection, density: Density): Outline {
        val radius = minOf(size.width, size.height) / 2f
        val cx = size.width / 2f
        val cy = size.height / 2f
        val steps = 180
        val path = Path()
        for (i in 0..steps) {
            val theta = 2.0 * PI * i / steps
            val r = radius * (1f - depth * (1f - cos(lobes * theta).toFloat()) / 2f)
            val x = cx + r * cos(theta + rotation).toFloat()
            val y = cy + r * sin(theta + rotation).toFloat()
            if (i == 0) path.moveTo(x, y) else path.lineTo(x, y)
        }
        path.close()
        return Outline.Generic(path)
    }

    override fun equals(other: Any?) =
        other is LobedShape && other.lobes == lobes && other.depth == depth && other.rotation == rotation

    override fun hashCode() = (lobes * 31 + depth.hashCode()) * 31 + rotation.hashCode()
}

internal val AvatarShape: Shape = LobedShape(9, 0.09f)

internal data class CategoryShape(val lobes: Int, val depth: Float, val rotation: Float = 0f)

private fun builtInCategory(key: String?): YuvaFeedbackCategory? = YuvaFeedbackCategory.entries.firstOrNull { it.value == key }

private fun keyHash(key: String): Int = key.fold(7) { acc, c -> (acc * 31 + c.code) and 0xFFFF }

internal fun categoryShape(key: String?): CategoryShape {
    if (key == null) return categoryShape(YuvaFeedbackCategory.OTHER.value)
    return when (builtInCategory(key)) {
        YuvaFeedbackCategory.BUG -> CategoryShape(8, 0.13f)
        YuvaFeedbackCategory.IDEA -> CategoryShape(12, 0.08f)
        YuvaFeedbackCategory.PRAISE -> CategoryShape(4, 0.2f, (PI / 4).toFloat())
        YuvaFeedbackCategory.OTHER -> CategoryShape(4, 0.07f, (PI / 4).toFloat())
        null -> CategoryShape(5 + keyHash(key) % 6, 0.1f)
    }
}

internal object LogoCache {
    private val images = ConcurrentHashMap<String, Bitmap>()

    suspend fun image(url: String): Bitmap? {
        if (!url.startsWith("http")) return null
        images[url]?.let { return it }
        val bitmap = withContext(Dispatchers.IO) {
            runCatching { URL(url).openStream().use { it.readBytes() } }.getOrNull()?.let { decodeScaled(it, 256) }
        } ?: return null
        images[url] = bitmap
        return bitmap
    }
}

internal data class CategoryStyle(val icon: ImageVector, val tint: Color)

internal fun categoryStyle(key: String?, icon: String? = null): CategoryStyle {
    if (key == null) return categoryStyle(YuvaFeedbackCategory.OTHER.value)
    return when (builtInCategory(key)) {
        YuvaFeedbackCategory.BUG -> CategoryStyle(YuvaIcons.Bug, Color(0xFFE5484D))
        YuvaFeedbackCategory.IDEA -> CategoryStyle(YuvaIcons.Lightbulb, Color(0xFFF5A524))
        YuvaFeedbackCategory.PRAISE -> CategoryStyle(Icons.Filled.Favorite, Color(0xFFE5528A))
        YuvaFeedbackCategory.OTHER -> CategoryStyle(YuvaIcons.Chat, Color(0xFF8B8D98))
        null -> CategoryStyle(lucideIcon(icon), Color(0xFF8B8D98))
    }
}

internal fun lucideIcon(name: String?): ImageVector = when (name) {
    "bug" -> YuvaIcons.Bug
    "lightbulb" -> YuvaIcons.Lightbulb
    "heart" -> Icons.Filled.Favorite
    "star" -> Icons.Filled.Star
    "palette", "pen", "pencil" -> Icons.Filled.Create
    "zap", "alert-triangle", "triangle-alert" -> Icons.Filled.Warning
    "wrench", "hammer" -> Icons.Filled.Build
    "settings" -> Icons.Filled.Settings
    "mail" -> Icons.Filled.Email
    "user" -> Icons.Filled.Person
    "lock", "shield" -> Icons.Filled.Lock
    "bell" -> Icons.Filled.Notifications
    "credit-card", "shopping-cart" -> Icons.Filled.ShoppingCart
    "thumbs-up" -> Icons.Filled.ThumbUp
    "info", "help-circle", "circle-help" -> Icons.Filled.Info
    "search" -> Icons.Filled.Search
    "home", "house" -> Icons.Filled.Home
    "phone", "smartphone" -> Icons.Filled.Phone
    else -> YuvaIcons.Chat
}

internal data class TonalColors(val container: Color, val content: Color)

@Composable
internal fun isDarkSurface(): Boolean = MaterialTheme.colorScheme.surface.luminance() < 0.5f

@Composable
internal fun categoryColors(key: String?): TonalColors {
    val dark = isDarkSurface()
    val scheme = MaterialTheme.colorScheme
    return when (builtInCategory(key)) {
        YuvaFeedbackCategory.BUG ->
            if (dark) TonalColors(Color(0xFF5F1A1C), Color(0xFFFFB4AB)) else TonalColors(Color(0xFFFFDAD6), Color(0xFFA4161A))
        YuvaFeedbackCategory.IDEA ->
            if (dark) TonalColors(Color(0xFF4D3A00), Color(0xFFF8C54A)) else TonalColors(Color(0xFFFFE08A), Color(0xFF6B4F00))
        YuvaFeedbackCategory.PRAISE ->
            if (dark) TonalColors(Color(0xFF64193F), Color(0xFFFFB0CF)) else TonalColors(Color(0xFFFFD8E6), Color(0xFFA0175A))
        YuvaFeedbackCategory.OTHER, null ->
            if (key == null || key == YuvaFeedbackCategory.OTHER.value) {
                TonalColors(scheme.secondaryContainer, scheme.onSecondaryContainer)
            } else when (keyHash(key) % 3) {
                0 -> TonalColors(scheme.tertiaryContainer, scheme.onTertiaryContainer)
                1 -> TonalColors(scheme.primaryContainer, scheme.onPrimaryContainer)
                else -> TonalColors(scheme.secondaryContainer, scheme.onSecondaryContainer)
            }
    }
}

@Composable
internal fun CategoryIcon(
    key: String?,
    size: Dp = 48.dp,
    morph: Float = 1f,
    icon: String? = null,
    colors: TonalColors = categoryColors(key),
) {
    val style = categoryStyle(key, icon)
    val shape = categoryShape(key)
    Box(
        Modifier.size(size).clip(LobedShape(shape.lobes, shape.depth * morph, shape.rotation)).background(colors.container),
        contentAlignment = Alignment.Center,
    ) { Icon(style.icon, contentDescription = null, tint = colors.content, modifier = Modifier.size(size * 0.5f)) }
}

@Composable
internal fun FeedbackTag(text: String, tint: Color, modifier: Modifier = Modifier) {
    Text(
        text,
        style = MaterialTheme.typography.labelMedium,
        fontWeight = FontWeight.SemiBold,
        color = tint,
        maxLines = 1,
        modifier = modifier.clip(CircleShape).background(tint.copy(alpha = if (isDarkSurface()) 0.2f else 0.13f))
            .padding(horizontal = 10.dp, vertical = 4.dp),
    )
}

@Composable
internal fun stateTint(state: YuvaFeedbackState): Color = when (state) {
    YuvaFeedbackState.SENT -> MaterialTheme.colorScheme.onSurfaceVariant
    YuvaFeedbackState.ANSWERED -> MaterialTheme.colorScheme.primary
    YuvaFeedbackState.RESOLVED, YuvaFeedbackState.DONE ->
        if (MaterialTheme.colorScheme.surface.luminance() < 0.5f) Color(0xFF4ADE80) else Color(0xFF1E8E3E)
    YuvaFeedbackState.PLANNED -> if (MaterialTheme.colorScheme.surface.luminance() < 0.5f) Color(0xFF8C9EFF) else Color(0xFF3F51B5)
    YuvaFeedbackState.IN_PROGRESS -> if (MaterialTheme.colorScheme.surface.luminance() < 0.5f) Color(0xFFFFB74D) else Color(0xFFE65100)
    YuvaFeedbackState.WONT_DO -> MaterialTheme.colorScheme.outline
}

internal object ShortTime {
    private fun day(instant: Instant, zone: ZoneId) = instant.atZone(zone).toLocalDate()

    fun list(context: Context, instant: Instant, now: Instant = Instant.now(), zone: ZoneId = ZoneId.systemDefault()): String {
        val ms = instant.toEpochMilli()
        val days = ChronoUnit.DAYS.between(day(instant, zone), day(now, zone))
        return when {
            days <= 0 -> DateUtils.formatDateTime(context, ms, DateUtils.FORMAT_SHOW_TIME)
            days == 1L -> DateUtils.getRelativeTimeSpanString(ms, now.toEpochMilli(), DateUtils.DAY_IN_MILLIS).toString()
            days < 7 -> DateUtils.formatDateTime(context, ms, DateUtils.FORMAT_SHOW_WEEKDAY)
            else -> DateUtils.formatDateTime(context, ms, DateUtils.FORMAT_SHOW_DATE or DateUtils.FORMAT_NUMERIC_DATE)
        }
    }

    fun separator(context: Context, instant: Instant, now: Instant = Instant.now(), zone: ZoneId = ZoneId.systemDefault()): String {
        val ms = instant.toEpochMilli()
        val days = ChronoUnit.DAYS.between(day(instant, zone), day(now, zone))
        return when {
            days == 0L || days == 1L ->
                DateUtils.getRelativeTimeSpanString(ms, now.toEpochMilli(), DateUtils.DAY_IN_MILLIS).toString()
            days in 2..6 -> DateUtils.formatDateTime(context, ms, DateUtils.FORMAT_SHOW_WEEKDAY)
            else -> DateUtils.formatDateTime(context, ms, DateUtils.FORMAT_SHOW_DATE or DateUtils.FORMAT_SHOW_YEAR)
        }
    }

    fun clock(context: Context, instant: Instant): String =
        DateUtils.formatDateTime(context, instant.toEpochMilli(), DateUtils.FORMAT_SHOW_TIME)

    fun sameDay(a: Instant, b: Instant, zone: ZoneId = ZoneId.systemDefault()) = day(a, zone) == day(b, zone)
}

internal object AttachmentCache {
    private val images = ConcurrentHashMap<String, Bitmap>()

    suspend fun image(client: YuvaClient, id: String): Bitmap? {
        images[id]?.let { return it }
        val bytes = runCatching { client.attachment(id) }.getOrNull() ?: return null
        val bitmap = withContext(Dispatchers.Default) { decodeScaled(bytes, 1600) } ?: return null
        images[id] = bitmap
        return bitmap
    }
}

@Composable
internal fun AttachmentImage(client: YuvaClient, id: String, onOpen: (() -> Unit)? = null) {
    var bitmap by remember(id) { mutableStateOf<Bitmap?>(null) }
    LaunchedEffect(id) {
        repeat(3) { attempt ->
            bitmap = AttachmentCache.image(client, id)
            if (bitmap != null) return@LaunchedEffect
            delay(2000L * (attempt + 1))
        }
    }
    val shape = RoundedCornerShape(12.dp)
    val current = bitmap
    if (current == null) {
        Box(
            Modifier.size(180.dp, 140.dp).clip(shape).background(MaterialTheme.colorScheme.surfaceVariant),
            contentAlignment = Alignment.Center,
        ) { CircularProgressIndicator(Modifier.size(24.dp)) }
    } else {
        Image(
            current.asImageBitmap(),
            contentDescription = null,
            contentScale = ContentScale.Fit,
            modifier = Modifier.widthIn(max = 220.dp).heightIn(max = 260.dp).clip(shape).then(
                if (onOpen == null) Modifier
                else Modifier.clickable(onClickLabel = stringResource(R.string.yuva_thread_open_image), onClick = onOpen)
                    .testTag("yuva.thread.image"),
            ),
        )
    }
}

@Composable
internal fun UploadImage(upload: YuvaUpload, modifier: Modifier = Modifier, crop: Boolean = false) {
    val bitmap = remember(upload) { decodeScaled(upload.data, 800) } ?: return
    Image(
        bitmap.asImageBitmap(),
        contentDescription = null,
        contentScale = if (crop) ContentScale.Crop else ContentScale.Fit,
        modifier = if (crop) modifier else modifier.aspectRatio(bitmap.width.toFloat() / bitmap.height),
    )
}

internal fun decodeScaled(bytes: ByteArray, maxSide: Int): Bitmap? {
    val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
    BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
    var sample = 1
    while (maxOf(bounds.outWidth, bounds.outHeight) / (sample * 2) >= maxSide) sample *= 2
    return BitmapFactory.decodeByteArray(bytes, 0, bytes.size, BitmapFactory.Options().apply { inSampleSize = sample })
}

internal fun imageUpload(context: Context, uri: Uri, name: String = "image"): YuvaUpload? {
    val bytes = runCatching { context.contentResolver.openInputStream(uri)?.use { it.readBytes() } }.getOrNull() ?: return null
    val bitmap = decodeScaled(bytes, 2048) ?: return null
    val out = ByteArrayOutputStream()
    bitmap.compress(Bitmap.CompressFormat.JPEG, 80, out)
    return YuvaUpload("$name.jpg", "image/jpeg", out.toByteArray())
}

internal fun fileSize(context: Context, size: Long): String = Formatter.formatShortFileSize(context, size)

internal fun refusalMessage(context: Context, refusal: YuvaAttachmentRefusal): String = when (refusal) {
    is YuvaAttachmentRefusal.TooLarge ->
        context.getString(R.string.yuva_attachment_too_large, fileSize(context, refusal.maxBytes))
    YuvaAttachmentRefusal.TypeNotAllowed -> context.getString(R.string.yuva_attachment_type_not_allowed)
}

internal fun errorMessage(error: Throwable): Int {
    if (error is IOException) return R.string.yuva_error_network
    if (error !is YuvaException) return R.string.yuva_error_generic
    return when {
        error.code == "attachment_too_large" || error.status == 413 -> R.string.yuva_error_attachment_too_large
        error.code == "attachment_type_mismatch" -> R.string.yuva_error_attachment_mismatch
        error.code == "attachment_type_not_allowed" || error.status == 415 -> R.string.yuva_error_attachment_type
        else -> R.string.yuva_error_generic
    }
}

private val UrlPattern = Regex("""(?i)\b(?:https?://|www\.)[^\s<>"]+""")

internal fun linkRanges(text: String): List<Pair<IntRange, String>> = UrlPattern.findAll(text).map { match ->
    val trimmed = match.value.trimEnd('.', ',', ';', ':', '!', '?', ')', ']', '}', '\'', '"')
    val range = match.range.first until match.range.first + trimmed.length
    range to if (trimmed.startsWith("www.", ignoreCase = true)) "https://$trimmed" else trimmed
}.filter { !it.first.isEmpty() }.toList()

internal fun linkified(text: String, linkColor: Color): AnnotatedString = buildAnnotatedString {
    append(text)
    val style = TextLinkStyles(SpanStyle(color = linkColor, textDecoration = TextDecoration.Underline))
    linkRanges(text).forEach { (range, url) ->
        addLink(LinkAnnotation.Url(url, style), range.first, range.last + 1)
    }
}

@Composable
internal fun ImageViewer(client: YuvaClient, id: String, onDismiss: () -> Unit) {
    val bitmap by produceState<Bitmap?>(null, id) { value = AttachmentCache.image(client, id) }
    var scale by remember(id) { mutableFloatStateOf(1f) }
    var offset by remember(id) { mutableStateOf(Offset.Zero) }
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false)) {
        Box(
            Modifier.fillMaxSize().background(Color.Black).testTag("yuva.thread.imageViewer")
                .pointerInput(id) {
                    detectTransformGestures { _, pan, zoom, _ ->
                        scale = (scale * zoom).coerceIn(1f, 5f)
                        offset = if (scale == 1f) Offset.Zero else offset + pan
                    }
                }
                .pointerInput(id) {
                    detectTapGestures(onDoubleTap = {
                        scale = if (scale > 1f) 1f else 2.5f
                        offset = Offset.Zero
                    })
                },
            contentAlignment = Alignment.Center,
        ) {
            val current = bitmap
            if (current == null) {
                CircularProgressIndicator(color = Color.White)
            } else {
                Image(
                    current.asImageBitmap(),
                    contentDescription = null,
                    contentScale = ContentScale.Fit,
                    modifier = Modifier.fillMaxSize().graphicsLayer {
                        scaleX = scale
                        scaleY = scale
                        translationX = offset.x
                        translationY = offset.y
                    },
                )
            }
            FilledTonalIconButton(
                onClick = onDismiss,
                colors = IconButtonDefaults.filledTonalIconButtonColors(
                    containerColor = Color.White.copy(alpha = 0.16f),
                    contentColor = Color.White,
                ),
                modifier = Modifier.align(Alignment.TopStart).windowInsetsPadding(WindowInsets.safeDrawing).padding(12.dp)
                    .testTag("yuva.thread.imageViewer.close"),
            ) { Icon(Icons.Default.Close, contentDescription = stringResource(R.string.yuva_thread_close)) }
        }
    }
}

@Composable
internal fun TypingDots(color: Color, modifier: Modifier = Modifier) {
    val transition = rememberInfiniteTransition(label = "typing")
    Row(modifier, horizontalArrangement = Arrangement.spacedBy(5.dp), verticalAlignment = Alignment.CenterVertically) {
        repeat(3) { index ->
            val lift by transition.animateFloat(
                initialValue = 0f,
                targetValue = 0f,
                animationSpec = infiniteRepeatable(
                    keyframes {
                        durationMillis = 1200
                        0f at 0
                        1f at 200 + index * 150 using FastOutSlowInEasing
                        0f at 500 + index * 150
                        0f at 1200
                    },
                ),
                label = "dot$index",
            )
            Box(
                Modifier.size(8.dp).graphicsLayer {
                    translationY = -lift * 4.dp.toPx()
                    alpha = 0.45f + 0.55f * lift
                }.clip(CircleShape).background(color),
            )
        }
    }
}

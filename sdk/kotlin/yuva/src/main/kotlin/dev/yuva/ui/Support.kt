package dev.yuva.ui

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.Uri
import android.text.format.Formatter
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.widthIn
import dev.yuva.R
import dev.yuva.YuvaClient
import dev.yuva.YuvaException
import dev.yuva.YuvaUpload
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext

@Composable
internal fun Avatar(initials: String, size: Dp = 28.dp) {
    Box(
        modifier = Modifier.size(size).clip(CircleShape).background(MaterialTheme.colorScheme.primary),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            initials.ifEmpty { "?" },
            color = MaterialTheme.colorScheme.onPrimary,
            fontSize = (size.value * 0.4f).sp,
            style = MaterialTheme.typography.labelMedium,
        )
    }
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
internal fun AttachmentImage(client: YuvaClient, id: String) {
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
            modifier = Modifier.widthIn(max = 220.dp).heightIn(max = 260.dp).clip(shape),
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

package api

import (
	"bytes"
	"mime"
	"net/http"
	"strings"
)

const octetStream = "application/octet-stream"

var typeAliases = map[string]string{
	"image/jpg":                    "image/jpeg",
	"image/pjpeg":                  "image/jpeg",
	"image/x-png":                  "image/png",
	"image/vnd.microsoft.icon":     "image/x-icon",
	"image/x-ms-bmp":               "image/bmp",
	"application/gzip":             "application/x-gzip",
	"application/x-zip-compressed": "application/zip",
	"audio/mp3":                    "audio/mpeg",
	"audio/x-wav":                  "audio/wave",
	"audio/wav":                    "audio/wave",
	"application/x-pdf":            "application/pdf",
}

func canonicalType(ct string) string {
	if a, ok := typeAliases[ct]; ok {
		return a
	}
	return ct
}

var ftypBrands = map[string]string{
	"heic": "image/heic", "heix": "image/heic", "heim": "image/heic", "heis": "image/heic",
	"hevc": "image/heic-sequence", "hevx": "image/heic-sequence",
	"mif1": "image/heif", "msf1": "image/heif-sequence",
	"avif": "image/avif", "avis": "image/avif",
	"qt  ": "video/quicktime",
}

// sniffType is http.DetectContentType plus the image formats it does not know.
func sniffType(head []byte) string {
	if len(head) >= 12 && string(head[4:8]) == "ftyp" {
		if t, ok := ftypBrands[string(head[8:12])]; ok {
			return t
		}
	}
	if bytes.HasPrefix(head, []byte("II*\x00")) || bytes.HasPrefix(head, []byte("MM\x00*")) {
		return "image/tiff"
	}
	ct, _, _ := mime.ParseMediaType(http.DetectContentType(head))
	return canonicalType(ct)
}

func textual(ct string) bool {
	return strings.HasPrefix(ct, "text/") || strings.HasSuffix(ct, "+xml") || strings.HasSuffix(ct, "+json") ||
		ct == "application/json" || ct == "application/xml" || ct == "application/javascript"
}

func zipBased(ct string) bool {
	return strings.HasPrefix(ct, "application/vnd.openxmlformats-officedocument.") ||
		strings.HasPrefix(ct, "application/vnd.oasis.opendocument.") ||
		ct == "application/epub+zip" || ct == "application/java-archive" || ct == "application/vnd.android.package-archive"
}

var signedTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true, "image/bmp": true,
	"image/x-icon": true, "image/tiff": true, "image/heic": true, "image/heic-sequence": true,
	"image/heif": true, "image/heif-sequence": true, "image/avif": true,
	"application/pdf": true, "application/zip": true, "application/x-gzip": true,
	"application/x-rar-compressed": true, "application/wasm": true, "application/postscript": true,
}

func media(ct string) bool {
	return strings.HasPrefix(ct, "audio/") || strings.HasPrefix(ct, "video/") || ct == "application/ogg"
}

// Audio and video containers overlap, so any audio or video sniff fits any audio or video type.
func contentMatches(declared, sniffed string) bool {
	switch {
	case declared == sniffed:
		return true
	case strings.HasPrefix(sniffed, "text/"):
		return textual(declared)
	case sniffed == "application/zip":
		return zipBased(declared)
	case media(declared) && media(sniffed):
		return true
	case sniffed == octetStream:
		return !signedTypes[declared] && !textual(declared)
	}
	return false
}

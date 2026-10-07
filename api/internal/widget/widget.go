package widget

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"net/http"
	"time"
)

//go:embed dist/yuva.js dist/yuva-chat.js
var embedded embed.FS

var Files = []string{"yuva.js", "yuva-chat.js"}

func Handler(name string) http.Handler {
	body, err := embedded.ReadFile("dist/" + name)
	if err != nil {
		panic("widget: dist/" + name + " is missing: " + err.Error())
	}
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "text/javascript; charset=utf-8")
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Cache-Control", "public, max-age=300")
		h.Set("ETag", etag)
		h.Set("X-Content-Type-Options", "nosniff")
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(body))
	})
}

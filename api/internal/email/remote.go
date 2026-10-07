package email

import (
	"strings"

	"golang.org/x/net/html"
)

// HasRemoteImages reports sanitized HTML with an image that the browser would fetch. Relative
// sources count too: browsers resolve forms like `/\host/x` to another server.
func HasRemoteImages(v string) bool {
	z := html.NewTokenizer(strings.NewReader(v))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return false
		case html.StartTagToken, html.SelfClosingTagToken:
			name, more := z.TagName()
			if string(name) != "img" {
				continue
			}
			for more {
				var key, val []byte
				key, val, more = z.TagAttr()
				if string(key) != "src" {
					continue
				}
				src := strings.ToLower(strings.TrimSpace(string(val)))
				if src != "" && !strings.HasPrefix(src, "data:") && !strings.HasPrefix(src, "cid:") {
					return true
				}
			}
		}
	}
}

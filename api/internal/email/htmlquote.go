package email

import (
	"bytes"
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var quoteClasses = []string{
	"gmail_quote", "gmail_quote_container", "gmail_extra", "gmail_signature", "moz-cite-prefix",
	"moz-signature", "yahoo_quoted", "protonmail_quote", "outlookmessageheader", "x_gmail_quote",
}

var quoteIDs = []string{"divrplyfwdmsg", "appendonsend", "mail-editor-reference-message-container", "x_divrplyfwdmsg"}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func hasQuoteClass(n *html.Node) bool {
	for _, c := range strings.Fields(strings.ToLower(attr(n, "class"))) {
		if slices.Contains(quoteClasses, c) {
			return true
		}
	}
	return false
}

// cutsRest marks containers after which the client puts only the quoted thread.
func cutsRest(n *html.Node) bool {
	return slices.Contains(quoteIDs, strings.ToLower(attr(n, "id")))
}

// StripHTMLQuotes removes the quoted history and signature containers that common clients mark
// in their HTML, and reports whether anything was removed.
func StripHTMLQuotes(src string) (string, bool) {
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return src, false
	}
	removed := false
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			if c.Type == html.ElementNode {
				switch {
				case cutsRest(c):
					if p := c.PrevSibling; p != nil && p.Type == html.ElementNode && p.DataAtom == atom.Hr {
						n.RemoveChild(p)
					}
					for r := c; r != nil; {
						rn := r.NextSibling
						n.RemoveChild(r)
						r = rn
					}
					removed = true
					return
				case hasQuoteClass(c), c.DataAtom == atom.Blockquote && strings.EqualFold(attr(c, "type"), "cite"):
					n.RemoveChild(c)
					removed = true
				default:
					walk(c)
				}
			}
			c = next
		}
	}
	walk(doc)
	if !removed {
		return src, false
	}
	body := findBody(doc)
	if body == nil || strings.TrimSpace(textOf(body)) == "" {
		return src, false
	}
	var buf bytes.Buffer
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&buf, c); err != nil {
			return src, false
		}
	}
	return buf.String(), true
}

func findBody(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == atom.Body {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if b := findBody(c); b != nil {
			return b
		}
	}
	return nil
}

func textOf(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	if n.Type == html.ElementNode && (n.DataAtom == atom.Style || n.DataAtom == atom.Script) {
		return ""
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(textOf(c))
	}
	return b.String()
}

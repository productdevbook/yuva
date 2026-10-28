package design

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// richText turns stored rich text into plain text: paragraphs and list items on their own lines,
// links as "label (url)" unless the label is the URL.
func richText(src string, v Values) string {
	var b strings.Builder
	type list struct {
		ordered bool
		n       int
	}
	var lists []list
	var label strings.Builder
	href := ""
	inLink := false
	for _, n := range tokens(src) {
		switch n.kind {
		case html.TextToken:
			t := v.Text(n.text)
			if inLink {
				label.WriteString(t)
			}
			b.WriteString(t)
		case html.StartTagToken:
			switch n.tag {
			case "br":
				b.WriteString("\n")
			case "ul", "ol":
				lists = append(lists, list{ordered: n.tag == "ol"})
			case "li":
				if len(lists) > 0 {
					l := &lists[len(lists)-1]
					l.n++
					b.WriteString(strings.Repeat("  ", len(lists)-1))
					if l.ordered {
						b.WriteString(strconv.Itoa(l.n) + ". ")
					} else {
						b.WriteString("- ")
					}
				}
			case "a":
				inLink, href = true, n.href
				label.Reset()
			}
		case html.EndTagToken:
			switch n.tag {
			case "p":
				b.WriteString("\n\n")
			case "li":
				b.WriteString("\n")
			case "ul", "ol":
				if len(lists) > 0 {
					lists = lists[:len(lists)-1]
				}
				if len(lists) == 0 {
					b.WriteString("\n")
				}
			case "a":
				inLink = false
				if u := v.URL(href); u != "" && checkURL(href) == "" && strings.TrimSpace(label.String()) != u {
					b.WriteString(" (" + u + ")")
				}
			}
		}
	}
	return strings.TrimSpace(b.String())
}

const textWidth = 78

// signatureDelimiter is the RFC 3676 line before a signature; its trailing space is kept.
const signatureDelimiter = "-- "

// wrapText wraps lines at 78 columns between words; a word is never broken, so URLs stay whole.
func wrapText(s string) string {
	var out strings.Builder
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		if i > 0 {
			out.WriteString("\n")
		}
		if line == signatureDelimiter {
			out.WriteString(line)
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		col := 0
		for j, w := range strings.Fields(line) {
			n := len([]rune(w))
			switch {
			case j == 0:
				out.WriteString(line[:indent])
				col = indent
			case col+1+n > textWidth:
				out.WriteString("\n" + strings.Repeat(" ", indent))
				col = indent
			default:
				out.WriteString(" ")
				col++
			}
			out.WriteString(w)
			col += n
		}
	}
	return out.String() + "\n"
}

package helpdesk

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Rewrite tells HTMLToMarkdown what to put where an article names an image or a link. Image maps
// the address the HTML shows (resolved against Base) to the address the image now has; an image
// missing from it stays a link to its original. Link, when set, maps a link's resolved address.
type Rewrite struct {
	Base  *url.URL
	Image map[string]string
	Link  func(href string) string
}

// HTMLImages lists the addresses of the images an article shows, resolved against base, once each
// and in order. Only http and https addresses count.
func HTMLImages(src string, base *url.URL) []string {
	root := parseFragment(src)
	var out []string
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Img {
			if u := resolveURL(base, attr(n, "src")); u != "" && !seen[u] {
				seen[u] = true
				out = append(out, u)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

// HTMLLinks lists the addresses an article links to, resolved against base, once each. Only http
// and https addresses count.
func HTMLLinks(src string, base *url.URL) []string {
	var out []string
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.A {
			if u := resolveURL(base, attr(n, "href")); u != "" && !seen[u] {
				seen[u] = true
				out = append(out, u)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(parseFragment(src))
	return out
}

// HTMLToMarkdown converts the HTML subset help centers produce (paragraphs, headings, lists,
// tables, code, quotes, links, images, emphasis) to the Markdown of Yuva's help center. Scripts,
// styles and unknown widgets are dropped; their text stays.
func HTMLToMarkdown(src string, rw Rewrite) string {
	c := &mdConv{rw: rw}
	blocks := c.flow(parseFragment(src))
	out := strings.TrimSpace(strings.Join(blocks, "\n\n"))
	if out == "" {
		return ""
	}
	return out + "\n"
}

func parseFragment(src string) *html.Node {
	body := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
	nodes, err := html.ParseFragment(strings.NewReader(src), body)
	if err != nil {
		return &html.Node{Type: html.TextNode, Data: src}
	}
	root := &html.Node{Type: html.DocumentNode}
	for _, n := range nodes {
		root.AppendChild(n)
	}
	return root
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return strings.TrimSpace(a.Val)
		}
	}
	return ""
}

func resolveURL(base *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "data:") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		return ""
	}
	return u.String()
}

type mdConv struct {
	rw Rewrite
}

var blockAtoms = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Ul: true, atom.Ol: true, atom.Li: true, atom.Pre: true, atom.Blockquote: true, atom.Table: true, atom.Hr: true,
	atom.Figure: true, atom.Figcaption: true, atom.Section: true, atom.Article: true, atom.Header: true, atom.Footer: true,
	atom.Main: true, atom.Nav: true, atom.Aside: true, atom.Details: true, atom.Summary: true, atom.Dl: true, atom.Dt: true,
	atom.Dd: true, atom.Iframe: true, atom.Video: true, atom.Audio: true, atom.Object: true, atom.Embed: true, atom.Form: true,
	atom.Fieldset: true, atom.Address: true, atom.Center: true,
}

var droppedAtoms = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Noscript: true, atom.Template: true, atom.Head: true, atom.Button: true,
	atom.Select: true, atom.Input: true, atom.Textarea: true, atom.Svg: true, atom.Canvas: true,
}

func isBlock(n *html.Node) bool {
	return n.Type == html.ElementNode && blockAtoms[n.DataAtom]
}

// flow turns the children of n into blocks: runs of inline content become paragraphs.
func (c *mdConv) flow(n *html.Node) []string {
	var blocks []string
	var run strings.Builder
	flush := func() {
		if p := cleanInline(run.String()); p != "" {
			blocks = append(blocks, escapeBlockStart(p))
		}
		run.Reset()
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if isBlock(ch) {
			flush()
			blocks = append(blocks, c.block(ch)...)
			continue
		}
		run.WriteString(c.inline(ch))
	}
	flush()
	return blocks
}

func (c *mdConv) block(n *html.Node) []string {
	switch n.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		level := int(n.Data[1] - '0')
		text := oneLine(cleanInline(c.inlineChildren(n)))
		if text == "" {
			return nil
		}
		return []string{strings.Repeat("#", level) + " " + text}
	case atom.Ul, atom.Ol:
		if b := c.list(n); b != "" {
			return []string{b}
		}
		return nil
	case atom.Pre:
		return c.pre(n)
	case atom.Blockquote:
		inner := strings.Join(c.flow(n), "\n\n")
		if inner == "" {
			return nil
		}
		lines := strings.Split(inner, "\n")
		for i, l := range lines {
			lines[i] = strings.TrimRight("> "+l, " ")
		}
		return []string{strings.Join(lines, "\n")}
	case atom.Table:
		if b := c.table(n); b != "" {
			return []string{b}
		}
		return nil
	case atom.Hr:
		return []string{"---"}
	case atom.Iframe, atom.Video, atom.Audio, atom.Object, atom.Embed:
		return c.embed(n)
	case atom.Dl:
		return c.definitions(n)
	}
	return c.flow(n)
}

func (c *mdConv) embed(n *html.Node) []string {
	src := attr(n, "src")
	if src == "" {
		src = attr(n, "data")
	}
	if src == "" {
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			if ch.Type == html.ElementNode && ch.DataAtom == atom.Source {
				src = attr(ch, "src")
				break
			}
		}
	}
	dest := c.linkDest(src)
	if dest == "" {
		return nil
	}
	label := attr(n, "title")
	if label == "" {
		label = dest
	}
	return []string{"[" + escapeText(label) + "](" + escapeDest(dest) + ")"}
}

func (c *mdConv) definitions(n *html.Node) []string {
	var out []string
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type != html.ElementNode {
			continue
		}
		text := cleanInline(c.inlineChildren(ch))
		if text == "" {
			continue
		}
		if ch.DataAtom == atom.Dt {
			text = "**" + oneLine(text) + "**"
		}
		out = append(out, text)
	}
	return out
}

func (c *mdConv) pre(n *html.Node) []string {
	var sb strings.Builder
	lang := ""
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		switch {
		case x.Type == html.TextNode:
			sb.WriteString(x.Data)
		case x.Type == html.ElementNode && x.DataAtom == atom.Br:
			sb.WriteByte('\n')
		case x.Type == html.ElementNode:
			if x.DataAtom == atom.Code && lang == "" {
				for _, cl := range strings.Fields(attr(x, "class")) {
					if l, ok := strings.CutPrefix(cl, "language-"); ok {
						lang = l
					}
				}
			}
			for ch := x.FirstChild; ch != nil; ch = ch.NextSibling {
				walk(ch)
			}
		}
	}
	walk(n)
	code := strings.Trim(strings.ReplaceAll(strings.ReplaceAll(sb.String(), "\r\n", "\n"), " ", " "), "\n")
	if strings.TrimSpace(code) == "" {
		return nil
	}
	fence := "```"
	for strings.Contains(code, fence) {
		fence += "`"
	}
	if !safeInfo.MatchString(lang) {
		lang = ""
	}
	return []string{fence + lang + "\n" + code + "\n" + fence}
}

var safeInfo = regexp.MustCompile(`^[A-Za-z0-9_+#.-]{1,30}$`)

func (c *mdConv) list(n *html.Node) string {
	ordered := n.DataAtom == atom.Ol
	num := 1
	if ordered {
		if v, err := strconv.Atoi(attr(n, "start")); err == nil && v >= 0 {
			num = v
		}
	}
	var items []string
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type != html.ElementNode {
			continue
		}
		var blocks []string
		if ch.DataAtom == atom.Li {
			blocks = c.flow(ch)
		} else {
			blocks = c.block(ch)
		}
		if len(blocks) == 0 {
			continue
		}
		marker := "- "
		if ordered {
			marker = strconv.Itoa(num) + ". "
			num++
		}
		sep := "\n"
		plain := 0
		for _, b := range blocks {
			if !strings.HasPrefix(b, "- ") && !startsOrdered.MatchString(b) {
				plain++
			}
		}
		if plain > 1 {
			sep = "\n\n"
		}
		body := strings.Join(blocks, sep)
		pad := strings.Repeat(" ", len(marker))
		lines := strings.Split(body, "\n")
		for i := range lines {
			if i == 0 {
				lines[i] = marker + lines[i]
			} else if lines[i] != "" {
				lines[i] = pad + lines[i]
			}
		}
		items = append(items, strings.Join(lines, "\n"))
	}
	return strings.Join(items, "\n")
}

var startsOrdered = regexp.MustCompile(`^\d+\. `)

func (c *mdConv) table(n *html.Node) string {
	var rows [][]string
	var headerRows int
	var walk func(*html.Node, bool)
	walk = func(x *html.Node, head bool) {
		for ch := x.FirstChild; ch != nil; ch = ch.NextSibling {
			if ch.Type != html.ElementNode {
				continue
			}
			switch ch.DataAtom {
			case atom.Thead:
				walk(ch, true)
			case atom.Tbody, atom.Tfoot:
				walk(ch, false)
			case atom.Tr:
				var cells []string
				for cell := ch.FirstChild; cell != nil; cell = cell.NextSibling {
					if cell.Type == html.ElementNode && (cell.DataAtom == atom.Td || cell.DataAtom == atom.Th) {
						text := oneLine(cleanInline(c.cellText(cell)))
						cells = append(cells, strings.ReplaceAll(text, "|", `\|`))
					}
				}
				if len(cells) > 0 {
					rows = append(rows, cells)
					if head && headerRows == 0 {
						headerRows = 1
					}
				}
			case atom.Caption:
			}
		}
	}
	walk(n, false)
	if len(rows) == 0 {
		return ""
	}
	width := 0
	for _, r := range rows {
		width = max(width, len(r))
	}
	line := func(r []string) string {
		cells := make([]string, width)
		for i := range cells {
			if i < len(r) {
				cells[i] = r[i]
			}
		}
		return "| " + strings.Join(cells, " | ") + " |"
	}
	sep := make([]string, width)
	for i := range sep {
		sep[i] = "---"
	}
	out := []string{line(rows[0]), "| " + strings.Join(sep, " | ") + " |"}
	for _, r := range rows[1:] {
		out = append(out, line(r))
	}
	return strings.Join(out, "\n")
}

func (c *mdConv) cellText(cell *html.Node) string {
	var parts []string
	for _, b := range c.flow(cell) {
		parts = append(parts, oneLine(b))
	}
	return strings.Join(parts, " ")
}

func (c *mdConv) inlineChildren(n *html.Node) string {
	var sb strings.Builder
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		sb.WriteString(c.inline(ch))
	}
	return sb.String()
}

var spaceRun = regexp.MustCompile(`[ \t\r\n\f\x{00a0}]+`)

func (c *mdConv) inline(n *html.Node) string {
	switch n.Type {
	case html.TextNode:
		return escapeText(spaceRun.ReplaceAllString(n.Data, " "))
	case html.ElementNode:
	default:
		return ""
	}
	if droppedAtoms[n.DataAtom] {
		return ""
	}
	switch n.DataAtom {
	case atom.Br:
		return "\x00BR"
	case atom.Img:
		return c.image(n)
	case atom.A:
		return c.link(n)
	case atom.Strong, atom.B:
		return wrapMark(c.inlineChildren(n), "**")
	case atom.Em, atom.I, atom.Cite, atom.Dfn:
		return wrapMark(c.inlineChildren(n), "*")
	case atom.Del, atom.S, atom.Strike:
		return wrapMark(c.inlineChildren(n), "~~")
	case atom.Code, atom.Kbd, atom.Samp, atom.Tt:
		return codeSpan(textOf(n))
	}
	if isBlock(n) {
		return " " + strings.Join(c.flow(n), " ") + " "
	}
	return c.inlineChildren(n)
}

func (c *mdConv) image(n *html.Node) string {
	raw := resolveURL(c.rw.Base, attr(n, "src"))
	if raw == "" {
		return ""
	}
	alt := oneLine(strings.TrimSpace(attr(n, "alt")))
	if dest, ok := c.rw.Image[raw]; ok && dest != "" {
		return "![" + escapeText(alt) + "](" + escapeDest(dest) + ")"
	}
	label := alt
	if label == "" {
		label = "image"
	}
	return "[" + escapeText(label) + "](" + escapeDest(raw) + ")"
}

func (c *mdConv) linkDest(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "#") || strings.HasPrefix(strings.ToLower(raw), "mailto:") || strings.HasPrefix(strings.ToLower(raw), "tel:") {
		return raw
	}
	dest := resolveURL(c.rw.Base, raw)
	if dest != "" && c.rw.Link != nil {
		if r := c.rw.Link(dest); r != "" {
			dest = r
		}
	}
	return dest
}

func (c *mdConv) link(n *html.Node) string {
	inner := c.inlineChildren(n)
	dest := c.linkDest(attr(n, "href"))
	if dest == "" {
		return inner
	}
	if strings.TrimSpace(strings.ReplaceAll(inner, "\x00BR", "")) == "" {
		return ""
	}
	lead, core, trail := splitSpace(inner)
	core = strings.ReplaceAll(core, "\x00BR", " ")
	return lead + "[" + core + "](" + escapeDest(dest) + ")" + trail
}

func splitSpace(s string) (lead, core, trail string) {
	t := strings.TrimLeft(s, " ")
	lead = s[:len(s)-len(t)]
	core = strings.TrimRight(t, " ")
	trail = t[len(core):]
	return
}

func wrapMark(inner, mark string) string {
	lead, core, trail := splitSpace(inner)
	if strings.TrimSpace(strings.ReplaceAll(core, "\x00BR", "")) == "" {
		return inner
	}
	if strings.Contains(core, "\x00BR") {
		parts := strings.Split(core, "\x00BR")
		for i, p := range parts {
			if strings.TrimSpace(p) != "" {
				parts[i] = mark + strings.TrimSpace(p) + mark
			}
		}
		return lead + strings.Join(parts, "\x00BR") + trail
	}
	return lead + mark + core + mark + trail
}

func textOf(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.TextNode {
			sb.WriteString(x.Data)
		}
		for ch := x.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(n)
	return spaceRun.ReplaceAllString(sb.String(), " ")
}

func codeSpan(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	fence := "`"
	for strings.Contains(s, fence) {
		fence += "`"
	}
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		s = " " + s + " "
	}
	return fence + s + fence
}

var textEscaper = strings.NewReplacer(`\`, `\\`, "*", `\*`, "_", `\_`, "`", "\\`", "[", `\[`, "]", `\]`, "<", `\<`, "~~", `\~\~`)

func escapeText(s string) string { return textEscaper.Replace(s) }

var destEscaper = strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29", "<", "%3C", ">", "%3E", "\n", "")

func escapeDest(s string) string { return destEscaper.Replace(s) }

var blockStart = regexp.MustCompile(`^(#{1,6}\s|>|[-+]\s|\d+[.)]\s|---|===|\|)`)

func escapeBlockStart(p string) string {
	if blockStart.MatchString(p) {
		return `\` + p
	}
	return p
}

// cleanInline resolves the line-break markers and trims the spaces around each line.
func cleanInline(s string) string {
	parts := strings.Split(s, "\x00BR")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(spaceRun.ReplaceAllString(p, " "))
	}
	out := strings.Join(parts, "\\\n")
	out = strings.TrimSuffix(strings.TrimSpace(out), "\\")
	for strings.HasPrefix(out, "\\\n") {
		out = strings.TrimPrefix(out, "\\\n")
	}
	return strings.TrimSpace(out)
}

func oneLine(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\\\n", " "), "\n", " "))
}

// CutMarkdown shortens a Markdown body to at most limit characters at a block boundary and
// tells whether it cut anything.
func CutMarkdown(md string, limit int) (string, bool) {
	if utf8.RuneCountInString(md) <= limit {
		return md, false
	}
	cut := string([]rune(md)[:limit])
	if i := strings.LastIndex(cut, "\n\n"); i > 0 {
		cut = cut[:i]
	}
	if strings.Count(cut, "```")%2 == 1 {
		if i := strings.LastIndex(cut, "```"); i > 0 {
			cut = strings.TrimRight(cut[:i], "\n")
		}
	}
	return strings.TrimRight(cut, "\n") + "\n", true
}

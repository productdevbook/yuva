package design

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/productdevbook/yuva/api/internal/mail/style"
)

const (
	NearClipBytes = 90 << 10
	ClipBytes     = 102 << 10
	MaxHTMLBytes  = 500 << 10
)

var ErrHTMLTooLarge = errors.New("design: rendered HTML over 500 KB")

// Image is an image block's picture as stored: its public URL and size in pixels.
type Image struct {
	URL           string
	Width, Height int
}

// Sender is the inbox's identity the required footer names.
type Sender struct {
	LegalName, PostalAddress, Registration, Contact string
}

func (s Sender) complete() bool { return s.LegalName != "" && s.PostalAddress != "" }

type Footer int

const (
	FooterList Footer = iota
	FooterReply
	FooterOutreach
)

type MessageSlot struct {
	Text                         string
	CardsHTML, CardsText         string
	SignatureHTML, SignatureText string
}

type Input struct {
	Doc       Document
	Subject   string
	Preheader string
	// Lang is a FooterLang result; it sets the html lang and the required footer's language.
	Lang         string
	HTMLLang     string
	Sender       Sender
	SubscribedAt time.Time
	Values       Values
	// Image resolves an image block; false renders the empty frame.
	Image func(b Block) (Image, bool)
	// Icon is the URL of a social icon, variant dark, light or brand.
	Icon func(network, variant string) string
	// Dark draws the dark palette as the page's own colours.
	Dark    bool
	Message *MessageSlot
	Footer  Footer
}

type Output struct {
	HTML     string
	Text     string
	Size     int
	Warnings []Issue
}

type palette struct{ Page, Card, Text, Muted, Accent, Link, Border string }

// surface is where a block draws: its background in both schemes.
type surface struct{ light, dark string }

type renderer struct {
	in      Input
	t       footerText
	light   palette
	dark    palette
	font    string
	hfont   string
	rtl     bool
	dirAuto bool
	radius  int
	rules   map[string]string
	order   []string
	warn    []Issue
	warned  map[string]bool
	text    strings.Builder
	h       strings.Builder
}

// Render draws a validated document as HTML and text.
func Render(in Input) (Output, error) {
	th := in.Doc.Theme
	r := &renderer{
		in: in, t: textsFor(in.Lang), rules: map[string]string{}, warned: map[string]bool{},
		font: style.Fonts[th.Font], hfont: style.Fonts[th.HeadingFont], rtl: th.Direction == "rtl",
		dirAuto: th.Direction == "" || th.Direction == "auto", radius: th.Radius,
	}
	if r.font == "" {
		r.font = style.FontSystem
	}
	if r.hfont == "" {
		r.hfont = r.font
	}
	c := th.Colors
	r.light = palette{c.Page, c.Card, c.Text, c.Muted, c.Accent, c.Link, c.Border}
	d := Colors{}
	if th.Dark != nil {
		d = *th.Dark
	}
	pick := func(v, derived string) string {
		if v != "" {
			return v
		}
		return derived
	}
	r.dark.Page = pick(d.Page, style.DarkSurface(c.Page, 0))
	r.dark.Card = pick(d.Card, style.DarkSurface(c.Card, 1))
	r.dark.Text = pick(d.Text, style.DarkInk(c.Text, r.dark.Card))
	r.dark.Muted = pick(d.Muted, style.DarkInk(c.Muted, r.dark.Card))
	r.dark.Accent = pick(d.Accent, style.DarkAccent(c.Accent, r.dark.Card))
	r.dark.Link = pick(d.Link, style.DarkInk(c.Link, r.dark.Card))
	r.dark.Border = pick(d.Border, style.DarkLine(c.Border, r.dark.Card))
	if in.Dark {
		r.light = r.dark
	}
	r.document()
	out := Output{HTML: r.h.String(), Text: wrapText(r.text.String()), Warnings: r.warn}
	out.Size = len(out.HTML)
	switch {
	case out.Size > MaxHTMLBytes:
		return out, ErrHTMLTooLarge
	case out.Size > ClipBytes:
		out.Warnings = append(out.Warnings, Issue{Code: "html_clipped", Detail: strconv.Itoa(out.Size) + " bytes"})
	case out.Size > NearClipBytes:
		out.Warnings = append(out.Warnings, Issue{Code: "html_near_clip", Detail: strconv.Itoa(out.Size) + " bytes"})
	}
	return out, nil
}

func (r *renderer) warnOnce(id, field, code string) {
	k := id + "\x00" + field + "\x00" + code
	if r.warned[k] {
		return
	}
	r.warned[k] = true
	r.warn = append(r.warn, Issue{BlockID: id, Field: field, Code: code})
}

// cls is a class whose dark-mode rule sets prop (background-color, color, border-color) to dark.
func (r *renderer) cls(prop, light, dark string) string {
	if strings.EqualFold(light, dark) {
		return ""
	}
	key := prop + ":" + dark
	name, ok := r.rules[key]
	if !ok {
		name = "yd" + strconv.Itoa(len(r.order))
		r.rules[key] = name
		r.order = append(r.order, key)
	}
	return name
}

func classAttr(names ...string) string {
	var keep []string
	for _, n := range names {
		if n != "" {
			keep = append(keep, n)
		}
	}
	if len(keep) == 0 {
		return ""
	}
	return ` class="` + strings.Join(keep, " ") + `"`
}

func (r *renderer) dir() string {
	if r.rtl {
		return ` dir="rtl"`
	}
	return ""
}

func (r *renderer) cellDir() string {
	switch {
	case r.rtl:
		return ` dir="rtl"`
	case r.dirAuto:
		return ` dir="auto"`
	}
	return ""
}

// side maps a logical alignment to the physical one: start is right in right-to-left designs.
func (r *renderer) side(align string) string {
	if align == "" {
		align = "left"
	}
	if r.rtl && align == "left" {
		return "right"
	}
	if r.rtl && align == "right" {
		return "left"
	}
	return align
}

func (r *renderer) document() {
	in := r.in
	v := in.Values
	th := in.Doc.Theme
	h := &r.h
	lang := in.Lang
	if in.HTMLLang != "" {
		lang = in.HTMLLang
	}
	if lang == "" {
		lang = "en"
	}
	pageCls := r.cls("background-color", r.light.Page, r.dark.Page)
	cardCls := r.cls("background-color", r.light.Card, r.dark.Card)
	cardBg := surface{r.light.Card, r.dark.Card}
	for _, b := range in.Doc.Blocks {
		r.block(b, cardBg, th.Width, 0)
	}
	body := r.h.String()
	r.h.Reset()
	footer := r.footer()

	h.WriteString("<!doctype html>\n<html lang=\"" + style.Escape(lang) + "\"" + r.dir() + ">\n<head>\n")
	h.WriteString("<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	h.WriteString("<meta name=\"x-apple-disable-message-reformatting\">\n")
	h.WriteString("<meta name=\"color-scheme\" content=\"light dark\">\n<meta name=\"supported-color-schemes\" content=\"light dark\">\n")
	h.WriteString("<title>" + style.Escape(v.Text(in.Subject)) + "</title>\n<style>\n")
	h.WriteString(":root{color-scheme:light dark;supported-color-schemes:light dark;}\n")
	h.WriteString("body{margin:0;padding:0;}\n")
	h.WriteString("a[x-apple-data-detectors]{color:inherit!important;text-decoration:none!important;}\n")
	h.WriteString("@media (max-width:620px){.y-col{display:block!important;width:100%!important;max-width:100%!important;}" +
		".y-px{padding-left:20px!important;padding-right:20px!important;}" +
		".y-gap{padding-left:0!important;padding-right:0!important;}.y-col img.y-fluid{max-width:100%!important;}" +
		".y-wrap{padding:0!important;}.y-card{border-radius:0!important;}" +
		".y-full{width:100%!important;max-width:100%!important;}" +
		".y-h1{font-size:26px!important;line-height:34px!important;}" +
		".y-hm{display:none!important;max-height:0!important;overflow:hidden!important;}" +
		".y-sm{display:block!important;max-height:none!important;overflow:visible!important;}}\n")
	h.WriteString("@media (prefers-color-scheme:dark){.y-lo{display:none!important;}.y-do{display:inline-block!important;max-height:none!important;overflow:visible!important;}")
	r.darkRules(h, "")
	h.WriteString("}\n")
	r.darkRules(h, "[data-ogsc] ")
	h.WriteString("</style>\n</head>\n")
	h.WriteString("<body" + classAttr(pageCls) + " style=\"margin:0;padding:0;width:100%;background-color:" + r.light.Page + ";-webkit-text-size-adjust:100%;-ms-text-size-adjust:100%;\">\n")
	if pre := strings.TrimSpace(v.Text(in.Preheader)); pre != "" {
		h.WriteString("<div style=\"display:none;max-height:0;max-width:0;overflow:hidden;mso-hide:all;font-size:1px;line-height:1px;\">")
		h.WriteString(style.Escape(pre))
		if n := 150 - len([]rune(pre)); n > 0 {
			h.WriteString(strings.Repeat("&#847;&zwnj;&nbsp;", n))
		}
		h.WriteString("</div>\n")
	}
	h.WriteString("<table role=\"presentation\"" + classAttr(pageCls) + r.dir() + " width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" border=\"0\" bgcolor=\"" + r.light.Page +
		"\" style=\"width:100%;background-color:" + r.light.Page + ";\">\n<tr><td class=\"y-wrap\" align=\"center\" style=\"padding:24px 8px;\">\n")
	h.WriteString(style.MSOOpen(th.Width) + "\n")
	h.WriteString("<table role=\"presentation\"" + r.dir() + " width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" border=\"0\" style=\"width:100%;max-width:" + strconv.Itoa(th.Width) + "px;\">\n")
	h.WriteString("<tr><td" + classAttr("y-card", cardCls) + " bgcolor=\"" + r.light.Card + "\" style=\"background-color:" + r.light.Card +
		";border-radius:" + strconv.Itoa(r.radius) + "px;padding:16px 0;\">\n")
	h.WriteString("<table role=\"presentation\"" + r.dir() + " width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" border=\"0\" style=\"width:100%;\">\n")
	h.WriteString(body)
	h.WriteString("</table>\n</td></tr>\n")
	h.WriteString(footer)
	h.WriteString("</table>\n" + style.MSOClose() + "\n</td></tr>\n</table>\n</body>\n</html>\n")
}

func (r *renderer) darkRules(h *strings.Builder, prefix string) {
	for _, key := range r.order {
		prop, val, _ := strings.Cut(key, ":")
		sel := prefix
		if prefix != "" && prop != "color" {
			sel = "[data-ogsb] "
		}
		h.WriteString(sel + "." + r.rules[key] + "{" + prop + ":" + val + "!important;}")
	}
	if prefix != "" {
		h.WriteString("\n")
	}
}

func padding(b Block, def [4]int) [4]int {
	if len(b.Padding) == 4 {
		return [4]int{b.Padding[0], b.Padding[1], b.Padding[2], b.Padding[3]}
	}
	return def
}

func pad(p [4]int) string {
	return fmt.Sprintf("padding:%dpx %dpx %dpx %dpx;", p[0], p[1], p[2], p[3])
}

// defaults gives a block's padding: depth 0 is the card, 1 a section, 2 a column.
func defaults(typ string, depth int) [4]int {
	x := 32
	if depth == 2 {
		x = 0
	}
	switch typ {
	case "section", "spacer":
		return [4]int{0, 0, 0, 0}
	case "columns":
		return [4]int{8, 32, 8, 32}
	case "heading":
		return [4]int{16, x, 8, x}
	case "button", "divider", "social":
		return [4]int{16, x, 16, x}
	case "footer":
		return [4]int{8, x, 8, x}
	}
	return [4]int{8, x, 8, x}
}

// hideAttrs is the class and style that hide a block on phones or on wide screens.
func hideAttrs(b Block) (string, string) {
	switch b.Hide {
	case "mobile":
		return "y-hm", ""
	case "desktop":
		return "y-sm", "display:none;max-height:0;overflow:hidden;mso-hide:all;"
	}
	return "", ""
}

// block writes one <tr> for b in a container width wide on bg.
func (r *renderer) block(b Block, bg surface, width, depth int) {
	h := &r.h
	p := padding(b, defaults(b.Type, depth))
	inner := width - p[1] - p[3]
	if inner < 16 {
		inner = 16
	}
	here := bg
	var bgCls, bgStyle, bgAttr string
	if b.Background != "" {
		here = surface{b.Background, style.DarkSurface(b.Background, 2)}
		if r.in.Dark {
			here.light = here.dark
		}
		bgCls = r.cls("background-color", here.light, here.dark)
		bgStyle = "background-color:" + here.light + ";"
		bgAttr = ` bgcolor="` + here.light + `"`
	}
	hideCls, hideStyle := hideAttrs(b)
	pxCls := ""
	if depth < 2 && (p[1] > 20 || p[3] > 20) {
		pxCls = "y-px"
	}
	align := r.side(b.Align)
	if b.Align == "" && (b.Type == "button" || b.Type == "social" || b.Type == "divider") {
		align = "center"
	}
	radius := ""
	if b.Type == "section" {
		rad := r.radius
		if b.BorderRadius != nil {
			rad = *b.BorderRadius
		}
		radius = "border-radius:" + strconv.Itoa(rad) + "px;"
	}
	fmt.Fprintf(h, "<tr><td%s%s align=\"%s\"%s style=\"%s%s%s%s\">", classAttr(hideCls, pxCls, bgCls), bgAttr, align, r.cellDir(), pad(p), bgStyle, radius, hideStyle)
	switch b.Type {
	case "section":
		h.WriteString("<table role=\"presentation\"" + r.dir() + " width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" border=\"0\" style=\"width:100%;\">")
		for _, in := range b.Blocks {
			r.block(in, here, inner, 1)
		}
		h.WriteString("</table>")
	case "columns":
		r.columns(b, here, inner)
	case "heading":
		r.heading(b, here)
	case "text":
		r.textBlock(b, here)
	case "image":
		r.image(b, here, inner, align)
	case "button":
		r.button(b, here)
	case "divider":
		r.divider(b, here, align)
	case "spacer":
		fmt.Fprintf(h, "<div style=\"height:%dpx;line-height:%dpx;font-size:1px;mso-line-height-rule:exactly;\">&nbsp;</div>", b.Height, b.Height)
	case "social":
		r.social(b, here, align)
	case "footer":
		r.footerBlock(b, here)
	case "message":
		r.messageBlock(b, here)
	}
	h.WriteString("</td></tr>\n")
}

func (r *renderer) columns(b Block, bg surface, width int) {
	h := &r.h
	gap := 16
	if b.Gap != nil {
		gap = *b.Gap
	}
	stack := b.Stack == nil || *b.Stack
	n := len(b.Columns)
	outer := make([]int, n)
	used := 0
	for i, c := range b.Columns {
		outer[i] = width * c.Width / 100
		used += outer[i]
	}
	if n > 0 {
		outer[n-1] += width - used
	}
	h.WriteString("<div style=\"font-size:0;line-height:0;\">")
	h.WriteString("<!--[if mso]><table role=\"presentation\"" + r.dir() + " width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" border=\"0\"><tr><![endif]-->")
	for i, c := range b.Columns {
		left, right := gap/2, gap-gap/2
		if i == 0 {
			left = 0
		}
		if i == n-1 {
			right = 0
		}
		if r.rtl {
			left, right = right, left
		}
		inner := outer[i] - left - right
		fmt.Fprintf(h, "<!--[if mso]><td width=\"%d\" valign=\"top\"><![endif]-->", outer[i])
		if stack {
			fmt.Fprintf(h, "<div class=\"y-col\" style=\"display:inline-block;width:100%%;max-width:%dpx;vertical-align:top;\">", outer[i])
		} else {
			fmt.Fprintf(h, "<div style=\"display:inline-block;width:%d%%;vertical-align:top;\">", c.Width)
		}
		gapCls := ""
		if stack {
			gapCls = ` class="y-gap"`
		}
		fmt.Fprintf(h, "<table role=\"presentation\"%s width=\"100%%\" cellpadding=\"0\" cellspacing=\"0\" border=\"0\" style=\"width:100%%;\"><tr><td%s style=\"padding:0 %dpx 0 %dpx;\">", r.dir(), gapCls, right, left)
		h.WriteString("<table role=\"presentation\"" + r.dir() + " width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" border=\"0\" style=\"width:100%;\">")
		for _, in := range c.Blocks {
			r.block(in, bg, inner, 2)
		}
		h.WriteString("</table></td></tr></table></div><!--[if mso]></td><![endif]-->")
	}
	h.WriteString("<!--[if mso]></tr></table><![endif]--></div>")
}

// ink is a text colour on bg in both schemes, with a warning when either reads below 4.5:1.
func (r *renderer) ink(id, field, light string, bg surface, slot string) (string, string) {
	dark := ""
	if slot != "" && strings.EqualFold(bg.dark, r.dark.Card) && !r.in.Dark {
		switch slot {
		case "text":
			dark = r.dark.Text
		case "muted":
			dark = r.dark.Muted
		case "link":
			dark = r.dark.Link
		}
	}
	if dark == "" {
		dark = style.DarkInk(light, bg.dark)
	}
	if r.in.Dark {
		light = dark
	}
	if style.Contrast(light, bg.light) < 4.5 || style.Contrast(dark, bg.dark) < 4.5 {
		r.warnOnce(id, field, "low_contrast")
	}
	return light, r.cls("color", light, dark)
}

func (r *renderer) slotInk(b Block, field, own, slot string, bg surface) (string, string) {
	light := own
	if light == "" {
		switch slot {
		case "muted":
			light = r.in.Doc.Theme.Colors.Muted
		default:
			light = r.in.Doc.Theme.Colors.Text
		}
		return r.ink(b.ID, field, light, bg, slot)
	}
	return r.ink(b.ID, field, light, bg, "")
}

var headingSizes = map[int][2]int{1: {30, 38}, 2: {24, 32}, 3: {19, 26}}

func (r *renderer) heading(b Block, bg surface) {
	color, cls := r.slotInk(b, "color", b.Color, "text", bg)
	sz := headingSizes[b.Level]
	if sz[0] == 0 {
		sz = headingSizes[2]
	}
	lvl := strconv.Itoa(max(1, min(3, b.Level)))
	extra := ""
	if b.Level == 1 {
		extra = "y-h1"
	}
	fmt.Fprintf(&r.h, "<h%s%s style=\"margin:0;font-family:%s;font-size:%dpx;line-height:%dpx;font-weight:700;color:%s;text-align:%s;\">",
		lvl, classAttr(cls, extra), r.hfont, sz[0], sz[1], color, r.side(b.Align))
	r.rich(b, b.Text, bg, color, cls)
	r.h.WriteString("</h" + lvl + ">")
	r.text.WriteString(richText(b.Text, r.in.Values) + "\n\n")
	r.fieldWarnings(b, b.Text, true)
}

func (r *renderer) textBlock(b Block, bg surface) {
	color, cls := r.slotInk(b, "color", b.Color, "text", bg)
	size := b.Size
	if size == 0 {
		size = 16
	}
	r.flow(b, b.HTML, bg, color, cls, size, size*13/8)
}

func (r *renderer) footerBlock(b Block, bg surface) {
	color, cls := r.slotInk(b, "html", "", "muted", bg)
	r.flow(b, b.HTML, bg, color, cls, 13, 20)
}

var paragraphBreak = regexp.MustCompile(`\n[ \t]*\n\s*`)

func (r *renderer) messageBlock(b Block, bg surface) {
	color, cls := r.slotInk(b, "", "", "text", bg)
	h := &r.h
	m := r.in.Message
	if m == nil {
		mc, mcls := r.ink(b.ID, "", r.in.Doc.Theme.Colors.Muted, bg, "muted")
		bd := r.cls("border-color", r.light.Border, r.dark.Border)
		fmt.Fprintf(h, "<table role=\"presentation\" width=\"100%%\" cellpadding=\"0\" cellspacing=\"0\" border=\"0\" style=\"width:100%%;\"><tr><td%s height=\"96\" align=\"center\" style=\"height:96px;border:1px dashed %s;border-radius:%dpx;font-family:%s;font-size:14px;color:%s;\">%s</td></tr></table>",
			classAttr(bd, mcls), r.light.Border, r.radius, r.font, mc, style.Escape(r.t.message))
		r.text.WriteString("[" + r.t.message + "]\n\n")
		return
	}
	text := strings.TrimSpace(strings.ReplaceAll(m.Text, "\r\n", "\n"))
	if text != "" {
		fmt.Fprintf(h, "<div%s style=\"margin:0;font-family:%s;font-size:16px;line-height:26px;color:%s;text-align:%s;\">",
			classAttr(cls), r.font, color, r.side(""))
		paras := paragraphBreak.Split(text, -1)
		for i, p := range paras {
			margin := "margin:0 0 16px 0;"
			if i == len(paras)-1 {
				margin = "margin:0;"
			}
			lines := strings.Split(p, "\n")
			for j := range lines {
				lines[j] = style.Escape(lines[j])
			}
			h.WriteString("<p" + r.cellDir() + " style=\"" + margin + "\">" + strings.Join(lines, "<br>") + "</p>")
		}
		h.WriteString("</div>")
		r.text.WriteString(strings.Join(paras, "\n\n") + "\n\n")
	}
	if m.CardsHTML != "" {
		h.WriteString(m.CardsHTML)
	}
	if t := strings.TrimSpace(m.CardsText); t != "" {
		r.text.WriteString(t + "\n\n")
	}
	if m.SignatureHTML != "" {
		fmt.Fprintf(h, "<div class=\"%s\" style=\"margin:16px 0 0 0;font-family:%s;font-size:14px;line-height:22px;color:%s;text-align:%s;\">-- <br>%s</div>",
			strings.TrimSpace("yuva-signature "+cls), r.font, color, r.side(""), m.SignatureHTML)
	}
	if t := strings.TrimSpace(m.SignatureText); t != "" {
		r.text.WriteString(signatureDelimiter + "\n" + t + "\n\n")
	}
}

func (r *renderer) flow(b Block, src string, bg surface, color, cls string, size, line int) {
	fmt.Fprintf(&r.h, "<div%s style=\"margin:0;font-family:%s;font-size:%dpx;line-height:%dpx;color:%s;text-align:%s;\">",
		classAttr(cls), r.font, size, line, color, r.side(b.Align))
	r.rich(b, src, bg, color, cls)
	r.h.WriteString("</div>")
	if t := richText(src, r.in.Values); t != "" {
		r.text.WriteString(t + "\n\n")
	}
	r.fieldWarnings(b, src, true)
}

func (r *renderer) fieldWarnings(b Block, s string, rich bool) {
	var fields []Field
	if rich {
		fields, _ = richFields(s)
	} else {
		fields = Fields(s)
	}
	for _, f := range fields {
		if strings.HasPrefix(f.Name, "contact.") && !f.HasFallback {
			r.warnOnce(b.ID, "", "merge_field_without_fallback")
		}
	}
}

// rich writes stored rich text with inline styles; anything the policy would not keep is dropped.
func (r *renderer) rich(b Block, src string, bg surface, color, cls string) {
	if ok, _ := checkRich(src, b.Type == "heading"); !ok {
		r.warnOnce(b.ID, "", "invalid_rich_text")
		return
	}
	link, linkCls := r.ink(b.ID, "link", r.in.Doc.Theme.Colors.Link, bg, "link")
	toks := tokens(src)
	last := -1
	depth := 0
	for i, n := range toks {
		if n.kind == html.StartTagToken && depth == 0 && slices.Contains([]string{"p", "ul", "ol"}, n.tag) {
			last = i
		}
		if n.kind == html.StartTagToken && n.tag != "br" {
			depth++
		}
		if n.kind == html.EndTagToken {
			depth--
		}
	}
	listPad := "padding:0 0 0 24px;"
	if r.rtl {
		listPad = "padding:0 24px 0 0;"
	}
	h := &r.h
	v := r.in.Values
	for i, n := range toks {
		switch n.kind {
		case html.TextToken:
			h.WriteString(style.Escape(v.Text(n.text)))
		case html.StartTagToken:
			margin := "margin:0 0 16px 0;"
			if i == last {
				margin = "margin:0;"
			}
			switch n.tag {
			case "p":
				h.WriteString("<p style=\"" + margin + "\">")
			case "ul", "ol":
				h.WriteString("<" + n.tag + " style=\"" + margin + listPad + "\">")
			case "li":
				h.WriteString("<li style=\"margin:0 0 6px 0;\">")
			case "br":
				h.WriteString("<br>")
			case "strong":
				h.WriteString("<strong style=\"font-weight:700;\">")
			case "code":
				h.WriteString("<code style=\"font-family:" + style.FontMono + ";font-size:0.92em;\">")
			case "a":
				if checkURL(n.href) != "" {
					h.WriteString("<a>")
					continue
				}
				h.WriteString("<a href=\"" + style.URLAttr(v.URL(n.href)) + "\" target=\"_blank\"" + classAttr(linkCls) +
					" style=\"color:" + link + ";text-decoration:underline;\">")
			default:
				h.WriteString("<" + n.tag + ">")
			}
		case html.EndTagToken:
			h.WriteString("</" + n.tag + ">")
		}
	}
}

func (r *renderer) image(b Block, bg surface, avail int, align string) {
	h := &r.h
	img, ok := Image{}, false
	if r.in.Image != nil && (b.ImageID != "" || b.Asset != "") {
		img, ok = r.in.Image(b)
	}
	if !b.Decorative && strings.TrimSpace(b.Alt) == "" {
		r.warnOnce(b.ID, "alt", "alt_missing")
	}
	w := avail
	if b.Width > 0 && b.Width < w {
		w = b.Width
	}
	if !ok {
		r.warnOnce(b.ID, "image_id", "image_missing")
		label := b.Alt
		if label == "" {
			label = r.t.image
		}
		mc, mcls := r.ink(b.ID, "", r.in.Doc.Theme.Colors.Muted, bg, "muted")
		bd := r.cls("border-color", r.light.Border, r.dark.Border)
		fmt.Fprintf(h, "<table role=\"presentation\" width=\"%d\" align=\"%s\" cellpadding=\"0\" cellspacing=\"0\" border=\"0\" class=\"y-full\" style=\"width:%dpx;max-width:100%%;\"><tr><td%s height=\"120\" align=\"center\" style=\"height:120px;border:1px dashed %s;border-radius:%dpx;font-family:%s;font-size:13px;color:%s;\">%s</td></tr></table>",
			w, align, w, classAttr(bd, mcls), r.light.Border, r.radius, r.font, mc, style.Escape(label))
		return
	}
	if img.Width > 0 && img.Width < w && b.Width == 0 {
		w = img.Width
	}
	alt := b.Alt
	extra := ""
	if b.Decorative {
		alt, extra = "", ` role="presentation"`
	}
	margin := "margin:0 auto;"
	switch align {
	case "left":
		margin = "margin:0 auto 0 0;"
	case "right":
		margin = "margin:0 0 0 auto;"
	}
	if b.Width == 0 {
		extra += ` class="y-fluid"`
	}
	tag := fmt.Sprintf("<img src=\"%s\" width=\"%d\" alt=\"%s\"%s style=\"display:block;width:100%%;max-width:%dpx;height:auto;border:0;text-decoration:none;border-radius:%dpx;%s\">",
		style.URLAttr(img.URL), w, style.Escape(alt), extra, w, r.radius, margin)
	if b.Link != "" {
		tag = "<a href=\"" + style.URLAttr(r.in.Values.URL(b.Link)) + "\" target=\"_blank\" style=\"text-decoration:none;\">" + tag + "</a>"
	}
	if b.Plate {
		plate := r.cls("background-color", bg.light, "#f4f4f5")
		tag = fmt.Sprintf("<table role=\"presentation\" align=\"%s\" cellpadding=\"0\" cellspacing=\"0\" border=\"0\"><tr><td%s style=\"padding:8px;border-radius:%dpx;\">%s</td></tr></table>",
			align, classAttr(plate), r.radius, tag)
	}
	h.WriteString(tag)
	if !b.Decorative && strings.TrimSpace(b.Alt) != "" {
		r.text.WriteString("[" + r.in.Values.Text(b.Alt) + "]\n\n")
	}
}

func (r *renderer) button(b Block, bg surface) {
	color := b.Color
	if color == "" {
		color = r.in.Doc.Theme.Colors.Accent
	}
	dark := style.DarkAccent(color, bg.dark)
	if b.Color == "" && !r.in.Dark {
		dark = r.dark.Accent
	}
	if r.in.Dark {
		color = dark
	}
	label, darkLabel := style.TextOn(color), style.TextOn(dark)
	if b.TextColor != "" && style.Contrast(b.TextColor, color) >= 4.5 {
		label = b.TextColor
	}
	if b.TextColor != "" && style.Contrast(b.TextColor, dark) >= 4.5 {
		darkLabel = b.TextColor
	}
	if style.Contrast(label, color) < 4.5 || style.Contrast(darkLabel, dark) < 4.5 {
		r.warnOnce(b.ID, "color", "low_contrast")
	}
	text := r.in.Values.Text(b.Label)
	url := r.in.Values.URL(b.URL)
	btn := style.Button{
		URL: url, Label: text, Color: color, TextColor: label, Font: r.font, Radius: r.radius,
		FontSize: 16, LineHeight: 22, PadY: 14, PadX: 28, FullWidth: b.FullWidth,
		CellClass: r.cls("background-color", color, dark), Class: r.cls("color", label, darkLabel),
	}
	r.h.WriteString(btn.HTML())
	r.text.WriteString(text + ": " + url + "\n\n")
	r.fieldWarnings(b, b.Label, false)
}

func (r *renderer) divider(b Block, bg surface, align string) {
	color := b.Color
	if color == "" {
		color = r.in.Doc.Theme.Colors.Border
	}
	dark := style.DarkLine(color, bg.dark)
	if b.Color == "" && !r.in.Dark {
		dark = r.dark.Border
	}
	if r.in.Dark {
		color = dark
	}
	t := b.Thickness
	if t == 0 {
		t = 1
	}
	w := b.Width
	if w == 0 {
		w = 100
	}
	fmt.Fprintf(&r.h, "<table role=\"presentation\" width=\"%d%%\" align=\"%s\" cellpadding=\"0\" cellspacing=\"0\" border=\"0\" style=\"width:%d%%;\"><tr><td%s style=\"border-top:%dpx solid %s;font-size:1px;line-height:1px;mso-line-height-rule:exactly;\">&nbsp;</td></tr></table>",
		w, align, w, classAttr(r.cls("border-color", color, dark)), t, color)
	r.text.WriteString(strings.Repeat("—", 24) + "\n\n")
}

func (r *renderer) social(b Block, bg surface, align string) {
	h := &r.h
	v := r.in.Values
	name := func(n string) string {
		if n == "website" {
			return r.t.website
		}
		return networkNames[n]
	}
	if b.Style == "text" || r.in.Icon == nil {
		link, cls := r.ink(b.ID, "", r.in.Doc.Theme.Colors.Link, bg, "link")
		fmt.Fprintf(h, "<div style=\"font-family:%s;font-size:14px;line-height:22px;text-align:%s;\">", r.font, align)
		for i, it := range b.Items {
			if i > 0 {
				h.WriteString(" &nbsp;·&nbsp; ")
			}
			h.WriteString("<a href=\"" + style.URLAttr(v.URL(it.URL)) + "\" target=\"_blank\"" + classAttr(cls) + " style=\"color:" + link + ";text-decoration:underline;\">" + style.Escape(name(it.Network)) + "</a>")
		}
		h.WriteString("</div>")
	} else {
		fmt.Fprintf(h, "<table role=\"presentation\" align=\"%s\" cellpadding=\"0\" cellspacing=\"0\" border=\"0\"><tr>", align)
		variant := b.Color
		if variant == "" {
			variant = "dark"
		}
		if r.in.Dark && variant == "dark" {
			variant = "light"
		}
		for _, it := range b.Items {
			alt := style.Escape(name(it.Network))
			img := func(variant, extraCls, extraStyle string) string {
				return fmt.Sprintf("<img src=\"%s\" width=\"28\" height=\"28\" alt=\"%s\"%s style=\"display:inline-block;width:28px;height:28px;border:0;%s\">",
					style.URLAttr(r.in.Icon(it.Network, variant)), alt, classAttr(extraCls), extraStyle)
			}
			h.WriteString("<td style=\"padding:0 6px;\"><a href=\"" + style.URLAttr(v.URL(it.URL)) + "\" target=\"_blank\" style=\"text-decoration:none;\">")
			if variant == "dark" {
				h.WriteString(img("dark", "y-lo", ""))
				h.WriteString("<!--[if !mso]><!-->" + img("light", "y-do", "display:none;max-height:0;overflow:hidden;") + "<!--<![endif]-->")
			} else {
				h.WriteString(img(variant, "", ""))
			}
			h.WriteString("</a></td>")
		}
		h.WriteString("</tr></table>")
	}
	for _, it := range b.Items {
		r.text.WriteString(name(it.Network) + ": " + v.URL(it.URL) + "\n")
	}
	r.text.WriteString("\n")
}

// footer is the part every mail ends with: who sends it, why the person gets it, and the links
// to leave.
func (r *renderer) footer() string {
	if r.in.Footer != FooterList {
		return r.conversationFooter()
	}
	var h strings.Builder
	in := r.in
	page := surface{r.light.Page, r.dark.Page}
	muted, cls := r.ink("", "theme.colors.muted", in.Doc.Theme.Colors.Muted, page, "")
	var who []string
	for _, s := range []string{in.Sender.LegalName, in.Sender.PostalAddress, in.Sender.Registration, in.Sender.Contact} {
		if s = strings.TrimSpace(s); s != "" {
			who = append(who, s)
		}
	}
	if !in.Sender.complete() {
		r.warnOnce("", "", "sender_identity_missing")
		if len(who) == 0 {
			who = []string{r.t.identityMissing}
		}
	}
	list := in.Values.ListName
	if list == "" {
		list = in.Values.InboxName
	}
	why := ""
	if list != "" {
		why = fmt.Sprintf(r.t.because, list)
		if !in.SubscribedAt.IsZero() {
			why = fmt.Sprintf(r.t.since, list, r.t.date(in.SubscribedAt, r.t.months))
		}
	}
	link := func(url, label string) string {
		return "<a href=\"" + style.URLAttr(url) + "\" target=\"_blank\"" + classAttr(cls) + " style=\"color:" + muted + ";text-decoration:underline;\">" + style.Escape(label) + "</a>"
	}
	fmt.Fprintf(&h, "<tr><td%s align=\"center\" dir=\"auto\" style=\"padding:24px 24px 8px 24px;font-family:%s;font-size:12px;line-height:18px;color:%s;text-align:center;\">",
		classAttr(cls), r.font, muted)
	h.WriteString(style.Escape(strings.Join(who, " · ")))
	if why != "" {
		h.WriteString("<br>" + style.Escape(why))
	}
	h.WriteString("<br>" + link(in.Values.UnsubscribeURL, r.t.unsubscribe) + " &nbsp;·&nbsp; " + link(in.Values.PreferencesURL, r.t.prefs))
	h.WriteString("</td></tr>\n")

	r.text.WriteString(strings.Repeat("—", 24) + "\n")
	r.text.WriteString(strings.Join(who, " · ") + "\n")
	if why != "" {
		r.text.WriteString(why + "\n")
	}
	r.text.WriteString(r.t.unsubscribe + ": " + in.Values.UnsubscribeURL + "\n")
	r.text.WriteString(r.t.prefs + ": " + in.Values.PreferencesURL + "\n")
	return h.String()
}

func (r *renderer) conversationFooter() string {
	in := r.in
	var who []string
	for _, s := range []string{in.Sender.LegalName, in.Sender.PostalAddress, in.Sender.Registration, in.Sender.Contact} {
		if s = strings.TrimSpace(s); s != "" {
			who = append(who, s)
		}
	}
	outreach := in.Footer == FooterOutreach
	if outreach && !in.Sender.complete() {
		r.warnOnce("", "", "sender_identity_missing")
	}
	if strings.TrimSpace(in.Sender.LegalName) == "" && !outreach {
		return ""
	}
	if strings.TrimSpace(in.Sender.LegalName) == "" {
		who = nil
	}
	page := surface{r.light.Page, r.dark.Page}
	muted, cls := r.ink("", "theme.colors.muted", in.Doc.Theme.Colors.Muted, page, "")
	var h strings.Builder
	fmt.Fprintf(&h, "<tr><td%s align=\"center\" dir=\"auto\" style=\"padding:24px 24px 8px 24px;font-family:%s;font-size:12px;line-height:18px;color:%s;text-align:center;\">",
		classAttr(cls), r.font, muted)
	r.text.WriteString(strings.Repeat("—", 24) + "\n")
	if len(who) > 0 {
		h.WriteString(style.Escape(strings.Join(who, " · ")))
		r.text.WriteString(strings.Join(who, " · ") + "\n")
	}
	if outreach {
		if len(who) > 0 {
			h.WriteString("<br>")
		}
		h.WriteString("<a href=\"" + style.URLAttr(in.Values.UnsubscribeURL) + "\" target=\"_blank\"" + classAttr(cls) + " style=\"color:" + muted + ";text-decoration:underline;\">" + style.Escape(r.t.unsubscribe) + "</a>")
		r.text.WriteString(r.t.unsubscribe + ": " + in.Values.UnsubscribeURL + "\n")
	}
	h.WriteString("</td></tr>\n")
	return h.String()
}

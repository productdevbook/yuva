package api

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"strings"
	"time"
	"unicode"
	"uuid"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/productdevbook/yuva/api/internal/mail/design"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	changelogTemplate     = "product-update"
	changelogMailBlocks   = 150
	changelogMailTextMax  = 15000
	changelogMailAltMax   = 300
	changelogMergeBreaker = "{⁠{"
)

var (
	changelogTextPadding    = []int{4, 32, 12, 32}
	changelogHeadingPadding = []int{12, 32, 4, 32}

	changelogMailWords = map[string]struct {
		labels     map[string]string
		months     []string
		see, foot  string
		dateFormat string
	}{
		"en": {
			labels: map[string]string{"new": "New", "improved": "Improved", "fixed": "Fixed"},
			months: []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
			see:    "See the full entry on the web", foot: "You get this e-mail because you subscribed to updates from {{ inbox.name }}.",
			dateFormat: "%d %s %d",
		},
		"tr": {
			labels: map[string]string{"new": "Yeni", "improved": "İyileştirildi", "fixed": "Düzeltildi"},
			months: []string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"},
			see:    "Girişin tamamını web'de görün", foot: "Bu e-postayı {{ inbox.name }} güncellemelerine abone olduğunuz için alıyorsunuz.",
			dateFormat: "%d %s %d",
		},
		"de": {
			labels: map[string]string{"new": "Neu", "improved": "Verbessert", "fixed": "Behoben"},
			months: []string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"},
			see:    "Den ganzen Eintrag im Web ansehen", foot: "Sie erhalten diese E-Mail, weil Sie Neuigkeiten von {{ inbox.name }} abonniert haben.",
			dateFormat: "%d. %s %d",
		},
	}
)

// neutralMerge keeps text a member wrote from being read as a merge field when the mail is sent.
func neutralMerge(s string) string { return strings.ReplaceAll(s, "{{", changelogMergeBreaker) }

func changelogReleaseLine(lang string, labels []string, at time.Time) string {
	w := changelogMailWords[lang]
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = w.labels[l]
	}
	date := fmt.Sprintf(w.dateFormat, at.Day(), w.months[at.Month()-1], at.Year())
	line := strings.Join(names, " · ") + " · " + date
	if lang == "tr" {
		return strings.ToUpperSpecial(unicode.TurkishCase, line)
	}
	return strings.ToUpper(line)
}

// changelogMail turns the rendered body of an entry into design blocks.
type changelogMail struct {
	entryURL string
	see      string
	images   map[string]uuid.UUID
	blocks   []design.Block
	text     bytes.Buffer
	linked   bool
	full     bool
	reserve  bool
}

func (m *changelogMail) id() string { return fmt.Sprintf("b%d", len(m.blocks)+1) }

func (m *changelogMail) room() bool {
	if m.reserve {
		return true
	}
	if len(m.blocks) >= changelogMailBlocks {
		m.full = true
	}
	return !m.full
}

func plainRich(s string) string {
	frag, err := xhtml.ParseFragment(strings.NewReader(s), &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return ""
	}
	var b strings.Builder
	var walk func(n *xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range frag {
		walk(n)
	}
	return b.String()
}

func (m *changelogMail) flush() {
	if m.text.Len() == 0 {
		return
	}
	raw := m.text.String()
	m.text.Reset()
	if !m.room() {
		return
	}
	h := design.SanitizeRich(raw, false)
	if !design.ValidRich(h, false) {
		h = "<p>" + html.EscapeString(strings.Join(strings.Fields(neutralMerge(plainRich(raw))), " ")) + "</p>"
		if r := []rune(h); len(r) > 20000 {
			h = "<p>" + html.EscapeString(string(r[:2000])) + "</p>"
		}
	}
	if strings.TrimSpace(plainRich(h)) == "" {
		return
	}
	m.blocks = append(m.blocks, design.Block{ID: m.id(), Type: "text", HTML: h, Padding: changelogTextPadding})
}

func (m *changelogMail) add(h string) {
	if m.text.Len()+len(h) > changelogMailTextMax {
		m.flush()
	}
	m.text.WriteString(h)
	m.linked = false
}

func (m *changelogMail) other() {
	if m.linked {
		return
	}
	m.add("<p><a href=\"" + html.EscapeString(m.entryURL) + "\">" + html.EscapeString(m.see) + "</a></p>")
	m.linked = true
}

func neutralTree(n *xhtml.Node) {
	if n.Type == xhtml.TextNode {
		n.Data = neutralMerge(n.Data)
	}
	if n.Type == xhtml.ElementNode && n.Data == "a" {
		for i, a := range n.Attr {
			if a.Key == "href" {
				n.Attr[i].Val = strings.ReplaceAll(a.Val, "{{", "%7B%7B")
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		neutralTree(c)
	}
}

func renderNode(n *xhtml.Node) string {
	var b bytes.Buffer
	_ = xhtml.Render(&b, n)
	return b.String()
}

func renderChildren(n *xhtml.Node) string {
	var b bytes.Buffer
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		_ = xhtml.Render(&b, c)
	}
	return b.String()
}

func attr(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func (m *changelogMail) image(n *xhtml.Node) {
	token := helpImageTokens.FindStringSubmatch(attr(n, "src"))
	if token == nil || !m.room() {
		return
	}
	id, ok := m.images[token[1]]
	if !ok {
		return
	}
	alt := neutralMerge(strings.TrimSpace(attr(n, "alt")))
	if r := []rune(alt); len(r) > changelogMailAltMax {
		alt = string(r[:changelogMailAltMax])
	}
	m.flush()
	m.blocks = append(m.blocks, design.Block{ID: m.id(), Type: "image", ImageID: id.String(), Alt: alt, Decorative: alt == "", Padding: changelogTextPadding})
	m.linked = false
}

func soleImage(n *xhtml.Node) *xhtml.Node {
	if n.Type == xhtml.ElementNode && n.Data == "img" {
		return n
	}
	if n.Type == xhtml.ElementNode && n.Data == "a" && n.FirstChild != nil && n.FirstChild == n.LastChild {
		return soleImage(n.FirstChild)
	}
	return nil
}

func (m *changelogMail) paragraph(p *xhtml.Node, wrap func(string) string) {
	var inline bytes.Buffer
	emit := func() {
		if strings.TrimSpace(plainRich(inline.String())) != "" {
			m.add(wrap(inline.String()))
		}
		inline.Reset()
	}
	for c := p.FirstChild; c != nil; c = c.NextSibling {
		if img := soleImage(c); img != nil {
			emit()
			m.image(img)
			continue
		}
		_ = xhtml.Render(&inline, c)
	}
	emit()
}

func (m *changelogMail) node(n *xhtml.Node) {
	if n.Type != xhtml.ElementNode {
		if n.Type == xhtml.TextNode && strings.TrimSpace(n.Data) != "" {
			m.add("<p>" + html.EscapeString(n.Data) + "</p>")
		}
		return
	}
	switch n.Data {
	case "p":
		m.paragraph(n, func(s string) string { return "<p>" + s + "</p>" })
	case "ul", "ol":
		m.add(renderNode(n))
	case "blockquote":
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch {
			case c.Type == xhtml.ElementNode && c.Data == "p":
				m.paragraph(c, func(s string) string { return "<p><em>" + s + "</em></p>" })
			case c.Type == xhtml.ElementNode:
				m.add(renderNode(c))
			}
		}
	case "h1", "h2", "h3", "h4", "h5", "h6":
		text := design.SanitizeRich(renderChildren(n), true)
		if !design.ValidRich(text, true) {
			text = html.EscapeString(strings.Join(strings.Fields(neutralMerge(plainRich(renderChildren(n)))), " "))
		}
		if strings.TrimSpace(plainRich(text)) == "" || !m.room() {
			return
		}
		m.flush()
		level := 2
		if n.Data >= "h3" {
			level = 3
		}
		m.blocks = append(m.blocks, design.Block{ID: m.id(), Type: "heading", Level: level, Text: text, Padding: changelogHeadingPadding})
		m.linked = false
	case "hr":
		m.flush()
	default:
		m.other()
	}
}

func (s *Server) changelogMailBlocks(ctx context.Context, q *store.Queries, ws uuid.UUID, renderedHTML, entryURL, see string) ([]design.Block, error) {
	images := map[string]uuid.UUID{}
	var tokens []string
	for _, match := range helpImageTokens.FindAllStringSubmatch(renderedHTML, -1) {
		tokens = append(tokens, match[1])
	}
	if len(tokens) > 0 {
		rows, err := q.ListEmailImagesByTokens(ctx, store.ListEmailImagesByTokensParams{WorkspaceID: ws, Tokens: tokens})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			images[r.Token] = r.ID
		}
	}
	return mapChangelogHTML(renderedHTML, entryURL, see, images)
}

// mapChangelogHTML maps the published HTML of an entry to design blocks: headings and workspace
// images become blocks of their own, paragraphs, lists and quotes text blocks through the design's
// rich-text policy, and anything else, such as a table or code block, a line linking to the entry.
func mapChangelogHTML(renderedHTML, entryURL, see string, images map[string]uuid.UUID) ([]design.Block, error) {
	frag, err := xhtml.ParseFragment(strings.NewReader(renderedHTML), &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return nil, err
	}
	m := &changelogMail{entryURL: entryURL, see: see, images: images}
	for _, n := range frag {
		neutralTree(n)
		m.node(n)
	}
	m.flush()
	if m.full {
		m.reserve = true
		m.other()
		m.flush()
	}
	return m.blocks, nil
}

func templateLocaleFor(locale string) *oas.EmailTemplateLocale {
	return new(oas.EmailTemplateLocale(mailLang(locale)))
}

func (s *Server) changelogDocument(ctx context.Context, q *store.Queries, p principal, in store.Inbox, c store.Changelog, e store.ChangelogEntry, t store.ChangelogEntryTranslation) (design.Document, error) {
	lang := mailLang(t.Locale)
	words := changelogMailWords[lang]
	entryURL := s.changelogEntryURL(c, e.Number, t.Locale, deref(t.PublishedSlug))
	d, err := s.documentFromTemplate(ctx, q, p, changelogTemplate, templateLocaleFor(t.Locale))
	if err != nil {
		return d, err
	}
	body, err := s.changelogMailBlocks(ctx, q, e.WorkspaceID, deref(t.PublishedHtml), entryURL, words.see)
	if err != nil {
		return d, err
	}
	zone := time.UTC
	if loc, err := time.LoadLocation(in.Timezone); err == nil {
		zone = loc
	}
	title := deref(t.PublishedTitle)
	d.Preheader = neutralMerge(deref(t.PublishedExcerpt))
	var hero *design.Block
	for i := range d.Blocks {
		if d.Blocks[i].ID == "hero" {
			hero = &d.Blocks[i]
		}
	}
	out := make([]design.Block, 0, len(d.Blocks)+len(body))
	for _, b := range d.Blocks {
		switch b.ID {
		case "logo", "f1", "f2", "f3", "f4", "hero":
			continue
		case "release":
			b.HTML = "<p><strong>" + html.EscapeString(changelogReleaseLine(lang, e.Labels, e.PublishedAt.In(zone))) + "</strong></p>"
		case "title":
			b.Text = neutralMerge(html.EscapeString(title))
		case "intro":
			if hero != nil && e.ImageID != nil {
				hero.Asset, hero.ImageID, hero.Alt, hero.Decorative = "", e.ImageID.String(), neutralMerge(title), false
				out = append(out, *hero)
			}
			if len(body) > 0 && body[0].Type == "text" {
				b.HTML, body = body[0].HTML, body[1:]
				out = append(out, b)
			}
			out = append(out, body...)
			continue
		case "changelog":
			b.URL = entryURL
		case "footer":
			parts := strings.SplitN(words.foot, "{{ inbox.name }}", 2)
			b.HTML = "<p>" + html.EscapeString(parts[0]) + "{{ inbox.name }}" + html.EscapeString(parts[1]) + "</p>"
		}
		out = append(out, b)
	}
	d.Blocks = out
	if issues := design.Validate(d, design.Options{}); len(issues) > 0 {
		return d, issuesProblem(issues)
	}
	return d, nil
}

func (s *Server) BroadcastChangelogEntry(ctx context.Context, req oas.BroadcastChangelogEntryRequestObject) (oas.BroadcastChangelogEntryResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	b := req.Body
	if len(b.ListIds) < 1 || len(b.ListIds) > maxBroadcastLists {
		return nil, errValidation("list_ids names 1 to 10 lists")
	}
	if err := validHelpLocale(b.Locale, "locale"); err != nil {
		return nil, err
	}
	c, err := visibleChangelog(ctx, s.st.Queries, p, req.ChangelogId, false)
	if err != nil {
		return nil, err
	}
	if err := s.requireBroadcastWriter(p); err != nil {
		return nil, err
	}
	in, err := visibleInbox(ctx, s.st.Queries, p, c.InboxID)
	if err != nil {
		return nil, err
	}
	var title, excerpt string
	var entryID uuid.UUID
	create := oas.BroadcastCreate{InboxId: in.ID, ChannelId: b.ChannelId, Audience: &oas.BroadcastAudienceInput{ListIds: b.ListIds}}
	var out store.Broadcast
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		e, err := changelogEntry(ctx, q, c, req.EntryId, false)
		if err != nil {
			return err
		}
		if e.State != changelogStatePublished {
			return errChangelogNotPublished
		}
		t, err := q.GetChangelogTranslation(ctx, store.GetChangelogTranslationParams{WorkspaceID: c.WorkspaceID, EntryID: e.ID, Locale: b.Locale})
		if store.IsNotFound(err) || (err == nil && t.PublishedAt == nil) {
			return errChangelogTranslation
		}
		if err != nil {
			return err
		}
		neutral := []rune(neutralMerge(deref(t.PublishedTitle)))
		entryID, title, excerpt = e.ID, deref(t.PublishedTitle), deref(t.PublishedExcerpt)
		name, err := trimmed("What's new: "+string(neutral[:min(len(neutral), 180)]), 1, 200, "name")
		if err != nil {
			return err
		}
		subject, preheader := neutralMerge(title), neutralMerge(excerpt)
		create.Name, create.Subject, create.Preheader = name, &subject, &preheader
		draft, err := checkBroadcastDraft(create)
		if err != nil {
			return err
		}
		doc, err := s.changelogDocument(ctx, q, p, in, c, e, t)
		if err != nil {
			return err
		}
		out, err = s.insertBroadcast(ctx, q, p, in, create, draft, &entryID, func(q *store.Queries) (uuid.UUID, error) {
			row, err := q.CreateEmailDesign(ctx, store.CreateEmailDesignParams{
				ID: newID(), WorkspaceID: p.workspaceID, InboxID: &in.ID, Name: name, Document: design.Marshal(doc),
				TemplateSlug: new(changelogTemplate), MemberID: p.memberRef(), Now: s.now(),
			})
			if err != nil {
				return uuid.Nil(), err
			}
			return row.ID, s.snapshotDesign(ctx, q, p, row, "autosave", true)
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.BroadcastChangelogEntry201JSONResponse(s.broadcastBody(out)), nil
}

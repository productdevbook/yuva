package api

import (
	"bytes"
	"context"
	"strings"
	"unicode"
	"unicode/utf8"
	"uuid"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
	"golang.org/x/net/html"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const maxSignatureRunes = 4000

var signatureMarkdown = goldmark.New(
	goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.Linkify),
	goldmark.WithParserOptions(parser.WithAutoHeadingID(), parser.WithASTTransformers(util.Prioritized(helpImageFilter{}, 100))),
	goldmark.WithRendererOptions(gmhtml.WithHardWraps()),
)

func renderSignatureMarkdown(body string, images helpImageSet) string {
	pc := parser.NewContext(parser.WithIDs(&helpIDs{seen: map[string]bool{}}))
	pc.Set(helpImagesKey, images)
	var buf bytes.Buffer
	if err := signatureMarkdown.Convert([]byte(body), &buf, parser.WithContext(pc)); err != nil {
		return "<p>" + html.EscapeString(body) + "</p>"
	}
	return helpHTML.Sanitize(buf.String())
}

func signatureValue(v string) string {
	var b strings.Builder
	for _, r := range strings.Join(strings.FieldsFunc(v, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " ") {
		if r < utf8.RuneSelf && unicode.IsPunct(r) || r < utf8.RuneSelf && unicode.IsSymbol(r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

type signatureAuthor struct {
	name, title string
}

func (a signatureAuthor) fill(body string) string {
	return strings.NewReplacer(
		"{{member.name}}", signatureValue(a.name),
		"{{member.title}}", signatureValue(a.title),
	).Replace(body)
}

type renderedSignature struct {
	html, text string
	images     []uuid.UUID
}

func validSignature(body string) error {
	if utf8.RuneCountInString(body) > maxSignatureRunes {
		return errValidation("signature must be at most 4000 characters")
	}
	return nil
}

func (s *Server) renderSignature(ctx context.Context, q *store.Queries, ws uuid.UUID, a signatureAuthor, body string) (renderedSignature, error) {
	if strings.TrimSpace(body) == "" {
		return renderedSignature{}, nil
	}
	set, err := s.helpImages(ctx, q, ws, body)
	if err != nil {
		return renderedSignature{}, err
	}
	out := renderedSignature{html: renderSignatureMarkdown(a.fill(body), set), images: set.ids()}
	out.text = signatureText(out.html)
	if out.text == "" && !strings.Contains(out.html, "<img") {
		return renderedSignature{}, nil
	}
	return out, nil
}

func (s *Server) signatureAuthorOf(ctx context.Context, q *store.Queries, ws, memberID uuid.UUID) (signatureAuthor, error) {
	m, err := q.GetMember(ctx, store.GetMemberParams{WorkspaceID: ws, ID: memberID})
	if err != nil {
		return signatureAuthor{}, err
	}
	return signatureAuthor{name: m.Name, title: m.Title}, nil
}

// An own signature with an empty body signs with nothing; it does not fall back to the default.
func (s *Server) effectiveSignature(ctx context.Context, q *store.Queries, ws, memberID uuid.UUID, inbox store.Inbox) (renderedSignature, *store.MemberSignature, error) {
	a, err := s.signatureAuthorOf(ctx, q, ws, memberID)
	if err != nil {
		return renderedSignature{}, nil, err
	}
	own, err := q.GetMemberSignature(ctx, store.GetMemberSignatureParams{WorkspaceID: ws, MemberID: memberID, InboxID: inbox.ID})
	if err != nil && !store.IsNotFound(err) {
		return renderedSignature{}, nil, err
	}
	body := inbox.Signature
	var mine *store.MemberSignature
	if err == nil {
		body, mine = own.Body, &own
	}
	r, err := s.renderSignature(ctx, q, ws, a, body)
	return r, mine, err
}

func memberSignatureBody(inboxID uuid.UUID, own *store.MemberSignature, r renderedSignature) oas.MemberSignature {
	out := oas.MemberSignature{InboxId: inboxID, Html: r.html, Text: r.text}
	if own != nil {
		out.Body = new(own.Body)
	}
	return out
}

func (s *Server) setInboxSignature(ctx context.Context, q *store.Queries, in store.Inbox, body string) (store.Inbox, error) {
	if err := validSignature(body); err != nil {
		return in, err
	}
	set, err := s.helpImages(ctx, q, in.WorkspaceID, body)
	if err != nil {
		return in, err
	}
	return q.SetInboxSignature(ctx, store.SetInboxSignatureParams{
		WorkspaceID: in.WorkspaceID, ID: in.ID, Signature: body, ImageIds: set.ids(), Now: s.now(),
	})
}

func (s *Server) ListMySignatures(ctx context.Context, _ oas.ListMySignaturesRequestObject) (oas.ListMySignaturesResponseObject, error) {
	p := principalFrom(ctx)
	q := s.st.Queries
	inboxes, err := q.ListInboxes(ctx, store.ListInboxesParams{WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID()})
	if err != nil {
		return nil, err
	}
	rows, err := q.ListMemberSignatures(ctx, store.ListMemberSignaturesParams{WorkspaceID: p.workspaceID, MemberID: p.memberID})
	if err != nil {
		return nil, err
	}
	own := make(map[uuid.UUID]*store.MemberSignature, len(rows))
	for i := range rows {
		own[rows[i].InboxID] = &rows[i]
	}
	a, err := s.signatureAuthorOf(ctx, q, p.workspaceID, p.memberID)
	if err != nil {
		return nil, err
	}
	out := oas.ListMySignatures200JSONResponse{Items: make([]oas.MemberSignature, len(inboxes))}
	for i, in := range inboxes {
		body := in.Signature
		if o := own[in.ID]; o != nil {
			body = o.Body
		}
		r, err := s.renderSignature(ctx, q, p.workspaceID, a, body)
		if err != nil {
			return nil, err
		}
		out.Items[i] = memberSignatureBody(in.ID, own[in.ID], r)
	}
	return out, nil
}

func (s *Server) SetMySignature(ctx context.Context, req oas.SetMySignatureRequestObject) (oas.SetMySignatureResponseObject, error) {
	p := principalFrom(ctx)
	if err := validSignature(req.Body.Body); err != nil {
		return nil, err
	}
	var out oas.MemberSignature
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		in, err := visibleInbox(ctx, q, p, req.InboxId)
		if err != nil {
			return err
		}
		set, err := s.helpImages(ctx, q, p.workspaceID, req.Body.Body)
		if err != nil {
			return err
		}
		row, err := q.UpsertMemberSignature(ctx, store.UpsertMemberSignatureParams{
			WorkspaceID: p.workspaceID, MemberID: p.memberID, InboxID: in.ID, Body: req.Body.Body, ImageIds: set.ids(), Now: s.now(),
		})
		if err != nil {
			return err
		}
		r, own, err := s.effectiveSignature(ctx, q, p.workspaceID, p.memberID, in)
		if err != nil {
			return err
		}
		if own == nil {
			own = &row
		}
		out = memberSignatureBody(in.ID, own, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.SetMySignature200JSONResponse(out), nil
}

func (s *Server) DeleteMySignature(ctx context.Context, req oas.DeleteMySignatureRequestObject) (oas.DeleteMySignatureResponseObject, error) {
	p := principalFrom(ctx)
	if _, err := visibleInbox(ctx, s.st.Queries, p, req.InboxId); err != nil {
		return nil, err
	}
	n, err := s.st.DeleteMemberSignature(ctx, store.DeleteMemberSignatureParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, InboxID: req.InboxId})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errNotFound
	}
	return oas.DeleteMySignature204Response{}, nil
}

func (s *Server) messageSignature(ctx context.Context, q *store.Queries, p principal, inbox uuid.UUID) (renderedSignature, error) {
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: p.workspaceID, ID: inbox})
	if err != nil {
		return renderedSignature{}, err
	}
	r, _, err := s.effectiveSignature(ctx, q, p.workspaceID, p.memberID, in)
	if err != nil || r.html == "" {
		return renderedSignature{}, err
	}
	if len(r.images) > 0 {
		if err := q.MarkEmailImagesSent(ctx, store.MarkEmailImagesSentParams{WorkspaceID: p.workspaceID, Ids: r.images, Now: s.now()}); err != nil {
			return renderedSignature{}, err
		}
	}
	return r, nil
}

func isEmailConversation(ctx context.Context, q *store.Queries, c store.Conversation) (bool, error) {
	if c.ChannelID == nil {
		return false, nil
	}
	_, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: c.WorkspaceID, ChannelID: *c.ChannelID})
	if store.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

var signatureBlocks = map[string]bool{
	"p": true, "div": true, "li": true, "tr": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"blockquote": true, "pre": true, "table": true, "ul": true, "ol": true, "hr": true,
}

func signatureText(src string) string {
	z := html.NewTokenizer(strings.NewReader(src))
	var b strings.Builder
	var href, linkText string
	inLink := false
	newline := func() {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
	}
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			lines := strings.Split(b.String(), "\n")
			out := lines[:0]
			for _, l := range lines {
				if l = strings.Join(strings.Fields(l), " "); l != "" {
					out = append(out, l)
				}
			}
			return strings.Join(out, "\n")
		case html.TextToken:
			raw := string(z.Text())
			t := strings.Join(strings.Fields(raw), " ")
			if t == "" {
				b.WriteString(raw[:min(len(raw), 1)])
				continue
			}
			if inLink {
				linkText += t
			}
			if strings.TrimLeftFunc(raw, unicode.IsSpace) != raw {
				t = " " + t
			}
			if strings.TrimRightFunc(raw, unicode.IsSpace) != raw {
				t += " "
			}
			b.WriteString(t)
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := string(name)
			attrs := map[string]string{}
			for hasAttr {
				var k, v []byte
				k, v, hasAttr = z.TagAttr()
				attrs[string(k)] = string(v)
			}
			switch {
			case tag == "br":
				b.WriteByte('\n')
			case tag == "img" && tt != html.EndTagToken:
				b.WriteString(attrs["alt"])
			case tag == "a" && tt == html.StartTagToken:
				inLink, href, linkText = true, attrs["href"], ""
			case tag == "a" && tt == html.EndTagToken:
				if inLink && href != "" && strings.TrimPrefix(href, "mailto:") != linkText && strings.TrimRight(href, "/") != strings.TrimRight(linkText, "/") {
					b.WriteString(" <" + href + ">")
				}
				inLink = false
			case signatureBlocks[tag]:
				newline()
			}
		}
	}
}

func splitQuote(text string) (reply, quote string) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	start := -1
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if strings.HasPrefix(l, ">") {
			start = i
			continue
		}
		if l != "" {
			break
		}
	}
	if start < 0 {
		return text, ""
	}
	return strings.TrimRight(strings.Join(lines[:start], "\n"), " \t\n"), strings.TrimRight(strings.Join(lines[start:], "\n"), " \t\n")
}

func batchSignature(msgs []messageRow) (htmlPart, textPart string, ok bool) {
	if len(msgs) == 0 {
		return "", "", false
	}
	m := msgs[len(msgs)-1]
	if m.SignatureHtml == nil || *m.SignatureHtml == "" {
		return "", "", false
	}
	text := ""
	if m.SignatureText != nil {
		text = *m.SignatureText
	}
	return *m.SignatureHtml, text, true
}

func signatureDiv(h string) string {
	return `<div class="yuva-signature">` + h + `</div>`
}

func signedText(text, sig string) string {
	if sig == "" {
		return text
	}
	reply, quote := splitQuote(text)
	out := "-- \n" + sig
	if reply != "" {
		out = reply + "\n\n" + out
	}
	if quote != "" {
		out += "\n\n" + quote
	}
	return out
}

func signedHTML(text, cards, sig string) string {
	reply, quote := splitQuote(text)
	out := email.TextToHTML(reply) + cards + signatureDiv(sig)
	if quote != "" {
		out += email.TextToHTML(quote)
	}
	return out
}

// A message that brought its own HTML has no quote we can find, so both parts end with the signature.
func signedOwnHTML(text, htmlPart, cards, sigText, sigHTML string) (string, string) {
	if sigText != "" {
		text = strings.TrimRight(text, " \t\r\n")
		if text != "" {
			text += "\n\n"
		}
		text += "-- \n" + sigText
	}
	add := cards + signatureDiv(sigHTML)
	if i := strings.LastIndex(strings.ToLower(htmlPart), "</body"); i >= 0 {
		return text, htmlPart[:i] + add + htmlPart[i:]
	}
	return text, htmlPart + add
}

func (s *Server) signMessage(ctx context.Context, q *store.Queries, p principal, c store.Conversation, in *messageInput, emailed bool, arg *store.CreateMessageParams) error {
	if in.unsigned || in.kind != string(oas.MessageKindMessage) || in.direction != string(oas.Out) || arg.AuthorMemberID == nil {
		return nil
	}
	if in.draft {
		var err error
		if emailed, err = isEmailConversation(ctx, q, c); err != nil {
			return err
		}
	}
	if !emailed {
		return nil
	}
	r, err := s.messageSignature(ctx, q, p, c.InboxID)
	if err != nil || r.html == "" {
		return err
	}
	arg.SignatureHtml, arg.SignatureText = &r.html, &r.text
	return nil
}

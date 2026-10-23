package mail_test

import (
	"bufio"
	"bytes"
	"context"
	"html/template"
	"io"
	"io/fs"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/productdevbook/yuva/api/internal/config"
	"github.com/productdevbook/yuva/api/internal/mail"
)

func TestContinuitySubjectByLocale(t *testing.T) {
	for locale, want := range map[string]string{
		"en": "Acme: new reply", "tr": "Acme: yeni yanıt", "tr-TR": "Acme: yeni yanıt", "TR_tr": "Acme: yeni yanıt",
		"de": "Acme: new reply", "": "Acme: new reply",
	} {
		m, err := mail.Render("continuity", locale, map[string]any{"Inbox": "Acme"})
		if err != nil {
			t.Fatal(err)
		}
		if m.Subject != want {
			t.Errorf("%q: subject %q, want %q", locale, m.Subject, want)
		}
	}
}

func samples(inbox, text string) map[string]map[string]any {
	return map[string]map[string]any{
		"sign_in_code":    {"Code": "482913", "Minutes": 10},
		"sign_in_locked":  {"Failures": 10, "Hours": 24},
		"analytics_limit": {"Workspace": inbox, "Limit": "1,000,000", "Month": "2026-10", "URL": "https://yuva.example.com/analytics"},
		"analytics_report": {
			"Property": "Acme docs", "From": "2026-10-05", "To": "2026-10-11",
			"Visitors": map[string]string{"Value": "1,204", "Change": "+12%"}, "Visits": map[string]string{"Value": "1,530", "Change": "+9%"},
			"Pageviews": map[string]string{"Value": "4,210", "Change": "−3%"}, "BounceRate": map[string]string{"Value": "41%"},
			"Duration": map[string]string{"Value": "2:05"},
			"Pages":    []map[string]string{{"Name": "/docs/install", "Value": "912"}, {"Name": "/pricing", "Value": "640"}},
			"Sources":  []map[string]string{{"Name": "Google", "Value": "530"}, {"Name": "(direct)", "Value": "410"}},
			"Goals":    []map[string]string{{"Name": "Signup", "Value": "38", "Change": "+19%"}},
			"Support": map[string]string{"Conversations": "14", "Feedback": "6", "Up": "40", "Down": "9", "TopPage": "/docs/billing",
				"TopCount": "5", "TopRate": "12.4"},
			"URL": "https://yuva.example.com/analytics/1", "SettingsURL": "https://yuva.example.com/analytics/1/settings",
		},
		"analytics_alert": {"Kind": "drop", "Property": "Acme docs", "Day": "2026-10-12", "HourFrom": "10:00", "HourTo": "12:00", "Current": "4",
			"Baseline": "120", "URL": "https://yuva.example.com/analytics/1", "SettingsURL": "https://yuva.example.com/analytics/1/settings"},
		"invite": {"Workspace": inbox, "Inviter": "Ayşe Yılmaz", "Role": "admin",
			"URL": "https://yuva.example.com", "Days": 7},
		"confirm_email": {"Inbox": inbox, "URL": "https://yuva.example.com/email/confirm?token=abc", "Hours": 24},
		"notification": {
			"Inbox": inbox, "Contact": "Jane Doe", "Subject": "Invoice question",
			"Messages": []string{text, "Can you also send a copy to accounting?"}, "More": 2,
			"Mentions": []map[string]string{{"By": "Mehmet", "Text": "@you can you take this one?"}},
			"Answers":  []map[string]string{{"By": "Vendor Ltd", "Text": "The tax rate is outdated."}},
			"Assigned": true, "AssignedBy": "Mehmet", "URL": "https://yuva.example.com/c/1",
			"SettingsURL": "https://yuva.example.com/settings/notifications", "Minutes": 15,
		},
		"sla_breached": {
			"Inbox": inbox, "Contact": "Jane Doe", "Subject": "Invoice question", "Target": "first_reply", "Due": "2026-10-23 15:00 Europe/Istanbul",
			"URL": "https://yuva.example.com/c/1", "SettingsURL": "https://yuva.example.com/settings/notifications",
		},
		"discussion_digest": {
			"Inbox": inbox, "Title": "How do I export my data?",
			"Answer": map[string]string{"Author": "Mehmet Kaya", "Text": "Open Settings, then Export.\nThe file arrives by e-mail."},
			"Posts":  []map[string]string{{"Author": "Jane Doe", "Text": text}, {"Author": "", "Text": "Same question here."}},
			"More":   3, "URL": "https://docs.example.com/export#yuva-discussion-1",
			"UnsubscribeURL": "https://yuva.example.com/discussions/unsubscribe/tok",
			"SettingsURL":    "https://yuva.example.com/account",
		},
		"rating_request": {"Inbox": inbox, "GoodURL": "https://yuva.example.com/rate/tok?rating=good",
			"BadURL": "https://yuva.example.com/rate/tok?rating=bad"},
		"list_confirm":   {"Inbox": inbox, "List": "Product news", "URL": "https://yuva.example.com/s/tok"},
		"status_confirm": {"Inbox": inbox, "Page": text, "Components": "API, Dashboard", "URL": "https://yuva.example.com/s/tok"},
		"status_notice": {"Inbox": inbox, "Page": "Acme status", "Title": text, "State": "In progress", "Impact": "Major", "Maintenance": true,
			"Window":     "Oct 19, 2026, 14:00 – 16:00 CEST",
			"Components": []map[string]string{{"Name": "API", "State": "Major outage"}, {"Name": "Website", "State": "Degraded performance"}},
			"Updates": []map[string]any{
				{"State": "Identified", "At": "Oct 19, 2026, 14:05 CEST", "Text": text, "HTML": template.HTML(template.HTMLEscapeString(text)), "Edited": true},
				{"State": "Investigating", "At": "Oct 19, 2026, 13:50 CEST", "Text": "We are looking into it.", "HTML": template.HTML("We are looking into it.")},
			},
			"Zone": "Europe/Berlin", "URL": "https://status.example.com/acme/incidents/1", "ManageURL": "https://yuva.example.com/m/tok",
			"Sender": "Acme Ltd, 1 Main Street, Springfield"},
		"status_monitor": {"Inbox": inbox, "Page": "Acme status", "Monitor": "Nightly backup", "Component": "Backups", "Down": true,
			"State": "Major outage", "Message": text, "URL": "https://yuva.example.com/status-pages/1/monitors",
			"SettingsURL": "https://yuva.example.com/settings/notifications"},
		"list_welcome": {"Inbox": inbox, "List": "Product news", "Description": "One e-mail a month.",
			"UnsubscribeURL": "https://yuva.example.com/m/tok", "ManageURL": "https://yuva.example.com/m/tok",
			"Sender": "Acme Ltd, 1 Main Street, Springfield"},
	}
}

var htmlMails = []string{"sign_in_code", "sign_in_locked", "invite", "confirm_email", "notification", "discussion_digest", "rating_request", "list_confirm", "status_confirm", "status_notice", "list_welcome", "analytics_limit", "analytics_report", "analytics_alert", "status_monitor", "sla_breached"}

// englishMails go only to the server's operator.
var englishMails = []string{"operator_digest"}

func TestEveryNonPushTemplateHasHTML(t *testing.T) {
	names, err := fs.Glob(os.DirFS("templates/en"), "*.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		name := strings.TrimSuffix(n, ".txt")
		if strings.HasPrefix(name, "push_") || name == "continuity" {
			continue
		}
		if !slices.Contains(htmlMails, name) && !slices.Contains(englishMails, name) {
			t.Errorf("%s has no HTML part in this test", name)
		}
	}
}

func TestRenderEveryLocale(t *testing.T) {
	data := samples("Acme Support", "Hello, my invoice is wrong.")
	for _, name := range htmlMails {
		for _, locale := range []string{"en", "tr", "de"} {
			for _, b := range []mail.Brand{{}, {Name: "Acme Support", Color: "#0e7490"}} {
				m, err := mail.RenderAs(b, name, locale, data[name])
				if err != nil {
					t.Fatalf("%s/%s: %v", locale, name, err)
				}
				for part, s := range map[string]string{"subject": m.Subject, "text": m.Text, "html": m.HTML} {
					if strings.TrimSpace(s) == "" || strings.Contains(s, "<no value>") || strings.Contains(s, "ZgotmplZ") {
						t.Errorf("%s/%s %s: %q", locale, name, part, s)
					}
				}
				if !strings.Contains(m.HTML, `<html lang="`+locale+`">`) {
					t.Errorf("%s/%s: html not in its locale", locale, name)
				}
				if locale != "en" {
					en, _ := mail.RenderAs(b, name, "en", data[name])
					if en.Subject == m.Subject || en.Text == m.Text {
						t.Errorf("%s/%s: text falls back to English", locale, name)
					}
				}
				checkHTML(t, locale+"/"+name, m.HTML)
			}
		}
	}
}

func checkHTML(t *testing.T, label, doc string) {
	t.Helper()
	z := html.NewTokenizer(strings.NewReader(doc))
	var open []string
	void := map[string]bool{"meta": true, "br": true, "img": true, "hr": true, "link": true}
	for {
		switch z.Next() {
		case html.ErrorToken:
			if z.Err() != io.EOF {
				t.Errorf("%s: %v", label, z.Err())
			}
			if len(open) != 0 {
				t.Errorf("%s: unclosed %v", label, open)
			}
			return
		case html.StartTagToken:
			name, _ := z.TagName()
			if !void[string(name)] {
				open = append(open, string(name))
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if len(open) == 0 || open[len(open)-1] != string(name) {
				t.Errorf("%s: </%s> closes %v", label, name, open)
				return
			}
			open = open[:len(open)-1]
		}
	}
}

func TestHTMLEscapesInboxNameAndMessages(t *testing.T) {
	evil := `<script>alert(1)</script>"><img src=x onerror=alert(1)>`
	data := samples(evil, evil)
	b := mail.Brand{Name: evil, Color: `red;background:url(javascript:alert(1))`}
	for _, name := range htmlMails {
		for _, locale := range []string{"en", "tr", "de"} {
			m, err := mail.RenderAs(b, name, locale, data[name])
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(m.HTML, "<script") || strings.Contains(m.HTML, "<img") || strings.Contains(m.HTML, "javascript:") {
				t.Errorf("%s/%s: unescaped input in html", locale, name)
			}
			if name != "sign_in_code" && name != "sign_in_locked" && !strings.Contains(m.HTML, "&lt;script&gt;") {
				t.Errorf("%s/%s: escaped input missing", locale, name)
			}
			checkHTML(t, locale+"/"+name, m.HTML)
		}
	}
}

func TestInboxBrandReplacesYuva(t *testing.T) {
	m, err := mail.RenderAs(mail.Brand{Name: "Acme Support", Color: "#FFEE00"}, "rating_request", "en", samples("Acme Support", "")["rating_request"])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m.HTML, "Yuva") {
		t.Error("an inbox's mail names Yuva")
	}
	if !strings.Contains(m.HTML, "#ffee00") || !strings.Contains(m.HTML, "color:#111111") {
		t.Error("the inbox colour or its readable text colour is missing")
	}
	y, _ := mail.Render("sign_in_code", "en", samples("", "")["sign_in_code"])
	if !strings.Contains(y.HTML, ">Yuva</span>") {
		t.Error("a team mail does not name Yuva")
	}
}

func TestSMTPSendsMultipartAlternative(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan []byte, 1)
	go fakeSMTP(ln, got)
	m, err := mail.Render("invite", "tr", samples("Acme <Destek>", "")["invite"])
	if err != nil {
		t.Fatal(err)
	}
	m.To, m.Unsubscribe = "success@simulator.amazonses.com", "https://yuva.example.com/u/tok"
	s := mail.SMTP{Config: config.SMTP{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, TLS: "none", From: "Yuva <yuva@example.com>"}}
	if err := s.Send(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	raw := <-got
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if subj, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject")); subj != m.Subject {
		t.Errorf("subject %q, want %q", subj, m.Subject)
	}
	if msg.Header.Get("List-Unsubscribe") != "<"+m.Unsubscribe+">" {
		t.Errorf("List-Unsubscribe %q", msg.Header.Get("List-Unsubscribe"))
	}
	mt, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/alternative" {
		t.Fatalf("content type %q: %v", msg.Header.Get("Content-Type"), err)
	}
	r := multipart.NewReader(msg.Body, params["boundary"])
	var kinds []string
	for {
		p, err := r.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if p.Header.Get("Content-Transfer-Encoding") != "quoted-printable" {
			t.Errorf("encoding %q", p.Header.Get("Content-Transfer-Encoding"))
		}
		body, err := io.ReadAll(quotedprintable.NewReader(p))
		if err != nil {
			t.Fatal(err)
		}
		ct := p.Header.Get("Content-Type")
		kinds = append(kinds, ct)
		want := m.Text
		if strings.HasPrefix(ct, "text/html") {
			want = m.HTML
		}
		if strings.ReplaceAll(string(body), "\r\n", "\n") != strings.ReplaceAll(want, "\r\n", "\n") {
			t.Errorf("%s part differs from the rendered one", ct)
		}
	}
	if !slices.Equal(kinds, []string{"text/plain; charset=utf-8", "text/html; charset=utf-8"}) {
		t.Errorf("parts %v", kinds)
	}
}

func TestSMTPRefusesControlCharactersInHeaders(t *testing.T) {
	s := mail.SMTP{Config: config.SMTP{Host: "127.0.0.1", Port: 1, TLS: "none", From: "Yuva <yuva@example.com>"}}
	for _, m := range []mail.Message{
		{To: "a@example.com\r\nBcc: b@example.com", Subject: "s", Text: "t"},
		{To: "a@example.com", Subject: "s", Text: "t", Unsubscribe: "https://example.com/u\r\nX-Evil: 1"},
		{To: "a@example.com\x00", Subject: "s", Text: "t"},
	} {
		if err := s.Send(context.Background(), m); err == nil || !strings.Contains(err.Error(), "control character") {
			t.Errorf("%q %q: %v", m.To, m.Unsubscribe, err)
		}
	}
}

func fakeSMTP(ln net.Listener, got chan<- []byte) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	say := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
	say("220 test")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			say("250 test")
		case cmd == "DATA":
			say("354 go on")
			var data bytes.Buffer
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				data.WriteString(strings.TrimPrefix(l, "."))
			}
			got <- data.Bytes()
			say("250 queued")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

func TestPreview(t *testing.T) {
	dir := os.Getenv("YUVA_MAIL_PREVIEW_DIR")
	if dir == "" {
		t.Skip("YUVA_MAIL_PREVIEW_DIR is not set")
	}
	data := samples("Acme Support", "Hello, my invoice for October shows the wrong amount.\nCould you check it?")
	for _, name := range htmlMails {
		for _, locale := range []string{"en", "tr", "de"} {
			b := mail.Brand{}
			if name == "confirm_email" || name == "discussion_digest" || name == "rating_request" {
				b = mail.Brand{Name: "Acme Support", Color: "#0e7490"}
			}
			m, err := mail.RenderAs(b, name, locale, data[name])
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(dir, locale), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, locale, name+".html"), []byte(m.HTML), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestDigestNamesUnnamedReadersInTheirLanguage(t *testing.T) {
	data := map[string]any{
		"Inbox": "Acme", "Title": "Export", "URL": "https://docs.example.com/x", "UnsubscribeURL": "https://yuva.example.com/u",
		"Answer": map[string]string{"Author": "", "Text": "Use Export."},
		"Posts":  []map[string]string{{"Author": "", "Text": "Thanks!"}},
	}
	for locale, want := range map[string][]string{
		"en": {"written by a reader", "A reader wrote:"},
		"tr": {"(bir okuyucu)", "Bir okuyucu yazdı:"},
		"de": {"geschrieben von jemandem aus der Leserschaft", "Jemand aus der Leserschaft schrieb:"},
	} {
		m, err := mail.RenderAs(mail.Brand{Name: "Acme"}, "discussion_digest", locale, data)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range want {
			if !strings.Contains(m.Text, w) || !strings.Contains(m.HTML, w) {
				t.Errorf("%s: %q missing", locale, w)
			}
		}
		if locale != "en" && strings.Contains(m.Text+m.HTML, "reader") {
			t.Errorf("%s: English fallback name", locale)
		}
	}
}

func TestFallbackLinkShowsAShortLabel(t *testing.T) {
	token := "0190a3c4-7b2e-7d1f-9a3b-1c2d3e4f5a6b.Zq3xV9mK2pL8nR4tW7yB1cD6fH0jG5sA_e-uIoPkMnB"
	link := "https://yuva.example.com/email/confirm?token=" + token
	m, err := mail.RenderAs(mail.Brand{Name: "Acme"}, "confirm_email", "en", map[string]any{"Inbox": "Acme", "URL": link, "Hours": 24})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.HTML, `>yuva.example.com/email/confirm…</a>`) {
		t.Error("the link text is not the short label")
	}
	if strings.Count(m.HTML, `href="`+link+`"`) != 2 {
		t.Error("the button and the link do not both carry the full URL")
	}
	if !strings.Contains(m.Text, link) {
		t.Error("the text part lost the full URL")
	}
}

func TestOperatorDigest(t *testing.T) {
	one := map[string]any{"Server": "yuva.example.com", "URL": "https://yuva.example.com", "Signups": 0, "Workspaces": 1,
		"Items": []map[string]any{{"Time": "2026-10-11 09:00 UTC", "Email": "a@example.com", "Workspace": "<Acme>", "From": "TR"}}}
	many := map[string]any{"Server": "yuva.example.com", "URL": "https://yuva.example.com", "Signups": 2, "Workspaces": 1,
		"Items": []map[string]any{
			{"Time": "2026-10-11 09:00 UTC", "Email": "a@example.com", "From": "203.0.113.7"},
			{"Time": "2026-10-11 09:01 UTC", "Email": "r@example.com", "Reader": true},
			{"Time": "2026-10-11 09:02 UTC", "Email": "a@example.com", "Workspace": "Acme"},
		}}
	for _, c := range []struct {
		data    map[string]any
		subject string
		text    []string
	}{
		{one, "New workspace: <Acme> by a@example.com", []string{`Workspace "<Acme>" created by a@example.com, from TR`}},
		{many, "2 new sign-ups and 1 new workspace on yuva.example.com", []string{
			"Sign-up: a@example.com, from 203.0.113.7", "Reader: r@example.com\n", `Workspace "Acme" created by a@example.com`,
		}},
	} {
		m, err := mail.Render("operator_digest", "tr", c.data)
		if err != nil {
			t.Fatal(err)
		}
		if m.Subject != c.subject {
			t.Errorf("subject %q, want %q", m.Subject, c.subject)
		}
		for _, w := range c.text {
			if !strings.Contains(m.Text, w) {
				t.Errorf("text has no %q:\n%s", w, m.Text)
			}
		}
		if !strings.Contains(m.HTML, `<html lang="en">`) || strings.Contains(m.HTML, "<Acme>") || strings.Contains(m.HTML, "<no value>") {
			t.Errorf("html:\n%s", m.HTML)
		}
		checkHTML(t, "operator_digest", m.HTML)
	}
}

func TestSMTPQuitsAfterDataAndIgnoresAFailedQuit(t *testing.T) {
	for _, dropAfterData := range []bool{false, true} {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		cmds := make(chan string, 16)
		go func() {
			defer close(cmds)
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			r := bufio.NewReader(conn)
			say := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
			say("220 test")
			for {
				line, err := r.ReadString('\n')
				if err != nil {
					return
				}
				cmd := strings.ToUpper(strings.TrimSpace(line))
				cmds <- strings.Fields(cmd + " x")[0]
				switch {
				case strings.HasPrefix(cmd, "EHLO"):
					say("250 test")
				case cmd == "DATA":
					say("354 go on")
					for l, err := r.ReadString('\n'); l != ".\r\n"; l, err = r.ReadString('\n') {
						if err != nil {
							return
						}
					}
					say("250 Ok 0102019a00000000-11111111-2222-3333-4444-555555555555-000000")
					if dropAfterData {
						return
					}
				case cmd == "QUIT":
					say("221 bye")
					return
				default:
					say("250 ok")
				}
			}
		}()
		s := mail.SMTP{Config: config.SMTP{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, TLS: "none", From: "Yuva <yuva@example.com>"}}
		if err := s.Send(context.Background(), mail.Message{To: "success@simulator.amazonses.com", Subject: "s", Text: "t"}); err != nil {
			t.Fatalf("drop after DATA %v: %v", dropAfterData, err)
		}
		var seen []string
		for c := range cmds {
			seen = append(seen, c)
		}
		ln.Close()
		want := []string{"EHLO", "MAIL", "RCPT", "DATA", "QUIT"}
		if dropAfterData {
			want = want[:4]
		}
		if !slices.Equal(seen, want) {
			t.Errorf("drop after DATA %v: commands %v, want %v", dropAfterData, seen, want)
		}
	}
}

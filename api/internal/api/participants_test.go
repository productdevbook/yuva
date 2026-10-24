package api_test

import (
	"bytes"
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/email"
)

func dmarcPass(domain string) string {
	return testAuthservID + "; spf=pass; dkim=pass; dmarc=pass header.from=" + domain
}

func participantEmails(t *testing.T, c *client, conv string) []string {
	t.Helper()
	r := c.expect(http.StatusOK, "GET", "/v1/conversations/"+conv+"/participants", nil)
	var out []string
	for _, p := range r.body["items"].([]any) {
		out = append(out, p.(map[string]any)["email"].(string))
	}
	return out
}

func TestReplyCopies(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	other := newEmailTeam(t, h, false)
	ctx := context.Background()
	d := unique("cc") + ".example.org"
	cust, col, boss, hidden := "cust@"+d, "col@"+d, "boss@"+d, "hidden@"+d

	first := newMessageID()
	r := h.ingest(et.address, buildMail(mailOpts{
		from: cust, to: et.address + ", " + other.address, subject: "Quote", messageID: first, body: "Please send a quote.",
		headers: map[string]string{"Cc": "Colleague <" + strings.ToUpper(col) + ">, " + boss + ", " + cust + ", " + col, "Authentication-Results": dmarcPass(d)},
	}), nil)
	if r.status != http.StatusAccepted {
		t.Fatalf("ingest: %d %v", r.status, r.body)
	}
	conv := r.str("conversation_id")
	if got := participantEmails(t, et.owner, conv); len(got) != 3 || !slices.Contains(got, boss) || !slices.Contains(got, col) || !slices.Contains(got, other.address) {
		t.Fatalf("participants %v", got)
	}
	et.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+conv+"/participants/"+other.address, nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+conv+"/participants", nil)

	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{
		"kind": "message", "body": "Here is the quote.", "bcc": []string{hidden},
	})
	if strs(reply.body["email"].(map[string]any)["bcc"])[0] != hidden {
		t.Fatalf("bcc not returned to the member: %s", reply.raw)
	}
	h.sendQueued(t, et.ws, reply.str("id"))
	sent, parsed := h.smtp.last(t)
	slices.Sort(sent.to)
	want := []string{boss, col, cust, hidden}
	if !slices.Equal(sent.to, want) {
		t.Fatalf("envelope %v, want %v", sent.to, want)
	}
	if bytes.Contains(sent.raw, []byte("hidden@")) || parsed.Header.Get("Bcc") != "" {
		t.Fatal("the Bcc address is in the sent message")
	}
	cc := parsed.Header.Get("Cc")
	if !strings.Contains(cc, col) || !strings.Contains(cc, boss) || parsed.Header.Get("To") != "<"+cust+">" && !strings.Contains(parsed.Header.Get("To"), cust) {
		t.Fatalf("To %q Cc %q", parsed.Header.Get("To"), cc)
	}
	if parsed.Header.Get("Reply-To") != "Acme Support <"+et.address+">" {
		t.Fatalf("Reply-To %q", parsed.Header.Get("Reply-To"))
	}
	ourID := email.NormalizeID(parsed.Header.Get("Message-ID"))

	listed := lastOf(messages(et.owner, conv), "message")
	em := listed["email"].(map[string]any)
	if !slices.Equal(strs(em["bcc"]), []string{hidden}) || len(strs(em["cc"])) != 2 {
		t.Fatalf("thread view: %v", em)
	}
	detail := et.owner.expect(http.StatusOK, "GET", "/v1/messages/"+reply.str("id")+"/email", nil)
	if bytes.Contains(detail.raw, []byte("hidden@")) {
		t.Fatalf("stored headers carry the Bcc: %s", detail.raw)
	}
	var leaks int
	if err := h.st.Pool.QueryRow(ctx, "SELECT count(*) FROM events WHERE workspace_id = $1 AND payload::text LIKE '%hidden@%'", et.ws).Scan(&leaks); err != nil {
		t.Fatal(err)
	}
	if leaks != 0 {
		t.Fatalf("%d events carry the Bcc", leaks)
	}
	feed := et.owner.expect(http.StatusOK, "GET", "/v1/events?after=0&limit=500", nil)
	if bytes.Contains(feed.raw, []byte("hidden@")) || !bytes.Contains(feed.raw, []byte(col)) {
		t.Fatal("the event feed shows the Bcc or misses the Cc")
	}

	none := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Only you.", "cc": []string{}})
	h.sendQueued(t, et.ws, none.str("id"))
	if sent, parsed = h.smtp.last(t); !slices.Equal(sent.to, []string{cust}) || parsed.Header.Get("Cc") != "" {
		t.Fatalf("cc [] still copied: %v %q", sent.to, parsed.Header.Get("Cc"))
	}

	if _, err := h.st.Pool.Exec(ctx, "INSERT INTO email_suppressions (workspace_id, email, reason, scope, created_at) VALUES ($1, $2, 'bounce', 'all', now())", et.ws, boss); err != nil {
		t.Fatal(err)
	}
	marketing := "news@" + d
	if _, err := h.st.Pool.Exec(ctx, "INSERT INTO email_suppressions (workspace_id, email, reason, scope, created_at) VALUES ($1, $2, 'complaint', 'broadcasts', now())", et.ws, marketing); err != nil {
		t.Fatal(err)
	}
	post := "/v1/conversations/" + conv + "/messages"
	et.owner.expectProblem(http.StatusConflict, "email_undeliverable", "POST", post, map[string]any{"kind": "message", "body": "x", "cc": []string{boss}})
	et.owner.expectProblem(http.StatusConflict, "email_undeliverable", "POST", post, map[string]any{"kind": "message", "body": "x", "bcc": []string{boss}})
	def := et.owner.expect(http.StatusCreated, "POST", post, map[string]any{"kind": "message", "body": "Default copies.", "bcc": []string{marketing}})
	h.sendQueued(t, et.ws, def.str("id"))
	sent, _ = h.smtp.last(t)
	slices.Sort(sent.to)
	if want := []string{col, cust, marketing}; !slices.Equal(sent.to, want) {
		t.Fatalf("suppressed participant copied or broadcast unsubscribe refused: %v, want %v", sent.to, want)
	}

	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", post, map[string]any{"kind": "note", "body": "x", "cc": []string{col}})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", post, map[string]any{"kind": "message", "body": "x", "draft": true, "cc": []string{col}})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", post, map[string]any{"kind": "message", "body": "x", "cc": []string{strings.Replace(et.address, "@", "+x@", 1)}})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", post, map[string]any{"kind": "message", "body": "x", "bcc": []string{"not an address"}})
	many := make([]string, 51)
	for i := range many {
		many[i] = unique("m") + "@" + d
	}
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", post, map[string]any{"kind": "message", "body": "x", "cc": many})
	chat := et.conversation(et.owner)
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+chat+"/messages", map[string]any{"kind": "message", "body": "x", "cc": []string{col}})

	draft := et.owner.expect(http.StatusCreated, "POST", post, map[string]any{"kind": "message", "body": "Drafted.", "draft": true})
	newcomer := "newcomer@" + d
	sentDraft := et.owner.expect(http.StatusOK, "POST", "/v1/messages/"+draft.str("id")+"/send", map[string]any{"cc": []string{newcomer}})
	h.sendQueued(t, et.ws, sentDraft.str("id"))
	if sent, _ = h.smtp.last(t); !slices.Equal(sent.to, []string{cust, newcomer}) {
		t.Fatalf("draft envelope %v", sent.to)
	}
	if !slices.Contains(participantEmails(t, et.owner, conv), newcomer) {
		t.Fatal("an address copied by a member is not a participant")
	}

	joined := h.ingest(et.address, buildMail(mailOpts{
		from: col, to: et.address, subject: "Re: Quote", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + ourID + ">", "Cc": cust + ", fresh@" + d, "Authentication-Results": dmarcPass(d)}, body: "Looks good to me.",
	}), nil)
	if joined.status != http.StatusAccepted || joined.str("conversation_id") != conv {
		t.Fatalf("participant reply did not join: %d %v", joined.status, joined.body)
	}
	c := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	last := lastOf(messages(et.owner, conv), "message")
	author := last["author"].(map[string]any)
	if author["contact_id"] == nil || author["contact_id"] == c.str("contact_id") || last["body"] != "Looks good to me." {
		t.Fatalf("participant message: %v (conversation contact %s)", last, c.str("contact_id"))
	}
	colContact := author["contact_id"].(string)
	if !slices.Contains(participantEmails(t, et.owner, conv), "fresh@"+d) {
		t.Fatal("a participant's Cc was not kept")
	}

	stranger := h.ingest(et.address, buildMail(mailOpts{
		from: "stranger@" + d, to: et.address, subject: "Re: Quote", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + ourID + ">"}, body: "I hold your Message-ID.",
	}), nil)
	if stranger.str("conversation_id") == conv {
		t.Fatal("a stranger joined the conversation")
	}
	sc := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+stranger.str("conversation_id"), nil)
	if sc.str("related_conversation_id") != conv {
		t.Fatalf("stranger's conversation: %s", sc.raw)
	}
	blind := h.ingest(et.address, buildMail(mailOpts{
		from: hidden, to: et.address, subject: "Re: Quote", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + ourID + ">"}, body: "I was in Bcc.",
	}), nil)
	if blind.str("conversation_id") == conv {
		t.Fatal("a Bcc recipient joined the conversation")
	}

	parts := et.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+colContact+"/participations", nil)
	items := parts.body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["conversation_id"] != conv || items[0].(map[string]any)["participant"] != true {
		t.Fatalf("participations: %s", parts.raw)
	}
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/contacts/"+colContact+"/participations", nil)

	et.owner.expect(http.StatusNoContent, "DELETE", "/v1/contacts/"+colContact, nil)
	if slices.Contains(participantEmails(t, et.owner, conv), col) {
		t.Fatal("contact deletion kept the participant")
	}
	var copies int
	if err := h.st.Pool.QueryRow(ctx, "SELECT count(*) FROM message_emails WHERE workspace_id = $1 AND ($2 = ANY(to_addresses) OR $2 = ANY(cc_addresses) OR $2 = ANY(bcc_addresses))", et.ws, col).Scan(&copies); err != nil {
		t.Fatal(err)
	}
	if copies != 0 {
		t.Fatalf("%d stored e-mails still list the deleted contact", copies)
	}
}

func participantStatus(t *testing.T, c *client, conv, addr string) string {
	t.Helper()
	r := c.expect(http.StatusOK, "GET", "/v1/conversations/"+conv+"/participants", nil)
	for _, p := range r.body["items"].([]any) {
		if p.(map[string]any)["email"] == addr {
			return p.(map[string]any)["status"].(string)
		}
	}
	return ""
}

func TestUnverifiedMailOnlySuggestsParticipants(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	other := newEmailTeam(t, h, false)
	ctx := context.Background()
	d := unique("spoof") + ".example.org"
	cust, mallory, friend := "cust@"+d, "mallory@"+d, "friend@"+d

	r := h.ingest(et.address, buildMail(mailOpts{
		from: cust, to: et.address, subject: "Order", messageID: newMessageID(), body: "Where is my order?",
		headers: map[string]string{"Cc": mallory},
	}), nil)
	conv := r.str("conversation_id")
	if s := participantStatus(t, et.owner, conv, mallory); s != "suggested" {
		t.Fatalf("Cc of mail without DMARC is %q, want suggested", s)
	}
	untrusted := h.ingest(et.address, buildMail(mailOpts{
		from: cust, to: et.address, subject: "Other", messageID: newMessageID(), body: "Also this.",
		headers: map[string]string{"Cc": friend, "Authentication-Results": "mx.other.example; dmarc=pass header.from=" + d},
	}), nil)
	if s := participantStatus(t, et.owner, untrusted.str("conversation_id"), friend); s != "suggested" {
		t.Fatalf("a pass from an untrusted authserv-id made a participant %q", s)
	}
	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Shipped."})
	h.sendQueued(t, et.ws, reply.str("id"))
	sent, parsed := h.smtp.last(t)
	if !slices.Equal(sent.to, []string{cust}) || parsed.Header.Get("Cc") != "" {
		t.Fatalf("a suggested participant was copied: %v %q", sent.to, parsed.Header.Get("Cc"))
	}
	ourID := email.NormalizeID(parsed.Header.Get("Message-ID"))
	answer := func(from string, verified bool, body string) string {
		hdr := map[string]string{"In-Reply-To": "<" + ourID + ">"}
		if verified {
			hdr["Authentication-Results"] = dmarcPass(d)
		}
		return h.ingest(et.address, buildMail(mailOpts{from: from, to: et.address, subject: "Re: Order", messageID: newMessageID(), headers: hdr, body: body}), nil).str("conversation_id")
	}
	if answer(mallory, true, "Suggested and verified.") == conv {
		t.Fatal("a suggested participant joined the conversation")
	}

	path := "/v1/conversations/" + conv + "/participants"
	added := et.owner.expect(http.StatusOK, "POST", path, map[string]any{"email": strings.ToUpper(mallory)})
	if participantStatus(t, et.owner, conv, mallory) != "active" || len(added.body["items"].([]any)) != 1 {
		t.Fatalf("add: %s", added.raw)
	}
	var events int
	if err := h.st.Pool.QueryRow(ctx, "SELECT count(*) FROM events WHERE workspace_id = $1 AND conversation_id = $2 AND type = 'conversation.participants'", et.ws, conv).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events < 2 {
		t.Fatalf("%d conversation.participants events, want one for the inbound mail and one for the add", events)
	}
	if answer(mallory, false, "Active but unverified.") == conv {
		t.Fatal("an active participant's mail without DMARC joined the conversation")
	}
	if answer(mallory, true, "Active and verified.") != conv {
		t.Fatal("an active participant's verified mail did not join")
	}
	again := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Copying you."})
	h.sendQueued(t, et.ws, again.str("id"))
	if sent, _ = h.smtp.last(t); !slices.Equal(sent.to, []string{cust, mallory}) {
		t.Fatalf("an added participant was not copied: %v", sent.to)
	}

	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", path, map[string]any{"email": friend})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", path+"/"+mallory, nil)
	bad := et.owner.do("POST", path, map[string]any{"email": "x@nodot"})
	ours := et.owner.do("POST", path, map[string]any{"email": et.address})
	if bad.status != http.StatusBadRequest || ours.status != http.StatusBadRequest ||
		strings.Replace(ours.str("detail"), et.address, "x@nodot", 1) != bad.str("detail") {
		t.Fatalf("the workspace's own address answers differently: %d %s / %d %s", bad.status, bad.raw, ours.status, ours.raw)
	}
	et.owner.expect(http.StatusOK, "POST", path, map[string]any{"email": other.address})
	et.owner.expect(http.StatusOK, "DELETE", path+"/"+other.address, nil)

	removed := et.owner.expect(http.StatusOK, "DELETE", path+"/"+mallory, nil)
	if participantStatus(t, et.owner, conv, mallory) != "" || len(removed.body["items"].([]any)) != 0 {
		t.Fatalf("remove: %s", removed.raw)
	}
	et.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", path+"/"+mallory, nil)
	for i := 0; i < 100; i++ {
		et.owner.expect(http.StatusOK, "POST", path, map[string]any{"email": unique("p") + "@" + d})
	}
	et.owner.expectProblem(http.StatusConflict, "participants_full", "POST", path, map[string]any{"email": friend})
}

func TestWithoutAuthservIDNoParticipantIsActive(t *testing.T) {
	t.Parallel()
	h := newHarnessWith(t, func(d *api.Deps) { d.Ingress.AuthservID = "" })
	et := newEmailTeam(t, h, false)
	d := unique("noauth") + ".example.org"
	r := h.ingest(et.address, buildMail(mailOpts{
		from: "cust@" + d, to: et.address, subject: "Hi", messageID: newMessageID(), body: "Hello",
		headers: map[string]string{"Cc": "col@" + d, "Authentication-Results": dmarcPass(d)},
	}), nil)
	if s := participantStatus(t, et.owner, r.str("conversation_id"), "col@"+d); s != "suggested" {
		t.Fatalf("participant without a trusted authserv-id is %q", s)
	}
}

func TestDeletedCopyCannotForgeReports(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	d := unique("dsn") + ".example.org"
	cust, col := "cust@"+d, "col@"+d
	r := h.ingest(et.address, buildMail(mailOpts{from: cust, to: et.address, subject: "Q", messageID: newMessageID(), body: "Q?"}), nil)
	conv := r.str("conversation_id")
	colContact := et.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{"emails": []string{col}}).str("id")
	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "A.", "cc": []string{col}})
	h.sendQueued(t, et.ws, reply.str("id"))
	ourID := lastOf(messages(et.owner, conv), "message")["email"].(map[string]any)["message_id"].(string)

	et.owner.expect(http.StatusNoContent, "DELETE", "/v1/contacts/"+colContact, nil)
	if cc, ok := lastOf(messages(et.owner, conv), "message")["email"].(map[string]any)["cc"]; ok {
		t.Fatalf("the deleted contact is still listed: %v", cc)
	}
	forged := h.ingestFrom(et.address, "", dsnMail(et.address, ourID, cust, "5.1.1"), nil)
	if forged.str("status") == "bounce" {
		t.Fatalf("a report about a mail that went to two people counted: %v", forged.body)
	}
	custID := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).str("contact_id")
	if und := undeliverable(et.owner, custID); len(und) != 0 {
		t.Fatalf("the customer was suppressed by a forged report: %v", und)
	}
}

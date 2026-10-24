package api_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/email"
)

func setDeliverability(t *testing.T, h *harness, ws, channel, state, result string) {
	t.Helper()
	h.clock.Advance(time.Second)
	if _, err := h.st.Pool.Exec(t.Context(), `INSERT INTO deliverability_checks (workspace_id, channel_id, probe_token, state, result, created_at, finished_at)
		VALUES ($1, $2, 'abcdefghijklmnopqrstuvwx', $3, $4, $5, $5)
		ON CONFLICT (workspace_id, channel_id) DO UPDATE SET state = EXCLUDED.state, result = EXCLUDED.result, created_at = EXCLUDED.created_at`,
		ws, channel, state, result, h.clock.Now()); err != nil {
		t.Fatal(err)
	}
}

func checkResult(domain string, dkimAligned bool, dmarc string) string {
	aligned := "false"
	if dkimAligned {
		aligned = "true"
	}
	return `{"domain":"` + domain + `","auth":{"spf":{"status":"pass","aligned":true},"dkim":{"status":"pass","aligned":` + aligned + `,"covers_list_unsubscribe":false,"signatures":[]},"dmarc":"` + dmarc + `"}}`
}

func memberAddressCode(t *testing.T, h *harness, addr string, n int) string {
	t.Helper()
	m := h.mail.wait(t, addr, n)[n-1]
	code := codePattern.FindString(m.Text)
	if code == "" || !strings.Contains(m.Text, addr) {
		t.Fatalf("code mail %q", m.Text)
	}
	return code
}

func verifiedAddress(t *testing.T, h *harness, c *client, addr string) string {
	t.Helper()
	id := c.expect(http.StatusAccepted, "POST", "/v1/me/addresses", map[string]any{"address": addr}).str("id")
	c.expect(http.StatusOK, "POST", "/v1/me/addresses/"+id+"/verify", map[string]any{"code": memberAddressCode(t, h, addr, 1)})
	return id
}

func myAddresses(c *client) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, it := range c.expect(http.StatusOK, "GET", "/v1/me/addresses", nil).body["items"].([]any) {
		a := it.(map[string]any)
		out[a["address"].(string)] = a
	}
	return out
}

func TestMemberAddressCodes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	other := newEmailTeam(t, h, false)
	addr := unique("ann") + "@example.com"

	for _, bad := range []string{"*@example.com", "Ann <ann@example.com>", `"a b"@example.com`, "a b@example.com", "a,b@example.com", "a@b@example.com", "a..b@example.com", ".a@example.com"} {
		if r := et.agent.do("POST", "/v1/me/addresses", map[string]any{"address": bad}); r.status != http.StatusBadRequest {
			t.Fatalf("%q: %d %s", bad, r.status, r.raw)
		}
	}
	et.agent.expectProblem(http.StatusConflict, "address_taken", "POST", "/v1/me/addresses", map[string]any{"address": strings.ToUpper(et.address)})
	et.agent.expectProblem(http.StatusConflict, "address_taken", "POST", "/v1/me/addresses", map[string]any{"address": other.address})

	r := et.agent.expect(http.StatusAccepted, "POST", "/v1/me/addresses", map[string]any{"address": strings.ToUpper(addr)})
	id := r.str("id")
	if r.str("address") != addr || r.body["verified"] != false || r.body["code_expires_at"] == nil {
		t.Fatalf("added: %s", r.raw)
	}
	code := memberAddressCode(t, h, addr, 1)
	et.agent.expectProblem(http.StatusTooManyRequests, "rate_limited", "POST", "/v1/me/addresses", map[string]any{"address": addr})

	et.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/me/addresses/"+id+"/verify", map[string]any{"code": code})
	et.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/me/addresses/"+id, nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/me/addresses/"+id+"/verify", map[string]any{"code": code})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/me/addresses/"+id, nil)

	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	et.agent.expectProblem(http.StatusBadRequest, "verification_code_invalid", "POST", "/v1/me/addresses/"+id+"/verify", map[string]any{"code": wrong})
	v := et.agent.expect(http.StatusOK, "POST", "/v1/me/addresses/"+id+"/verify", map[string]any{"code": code})
	if v.body["verified"] != true || v.body["verified_at"] == nil || len(v.body["usable_channel_ids"].([]any)) != 0 {
		t.Fatalf("verified: %s", v.raw)
	}
	et.agent.expectProblem(http.StatusConflict, "address_verified", "POST", "/v1/me/addresses", map[string]any{"address": addr})
	et.owner.expectProblem(http.StatusConflict, "address_taken", "POST", "/v1/me/addresses", map[string]any{"address": addr})
	if _, ok := myAddresses(et.agent)[addr]; !ok {
		t.Fatal("the verified address is not listed")
	}
	if len(myAddresses(et.owner)) != 0 {
		t.Fatal("another member's address is listed")
	}

	expiring := unique("exp") + "@example.com"
	eid := et.agent.expect(http.StatusAccepted, "POST", "/v1/me/addresses", map[string]any{"address": expiring}).str("id")
	ecode := memberAddressCode(t, h, expiring, 1)
	h.clock.Advance(10*time.Minute + time.Second)
	et.agent.expectProblem(http.StatusBadRequest, "verification_code_invalid", "POST", "/v1/me/addresses/"+eid+"/verify", map[string]any{"code": ecode})
	et.agent.expect(http.StatusAccepted, "POST", "/v1/me/addresses", map[string]any{"address": expiring})
	et.agent.expect(http.StatusOK, "POST", "/v1/me/addresses/"+eid+"/verify", map[string]any{"code": memberAddressCode(t, h, expiring, 2)})

	guessed := unique("guess") + "@example.com"
	gid := et.agent.expect(http.StatusAccepted, "POST", "/v1/me/addresses", map[string]any{"address": guessed}).str("id")
	gcode := memberAddressCode(t, h, guessed, 1)
	gwrong := "000000"
	if gcode == gwrong {
		gwrong = "111111"
	}
	for range 5 {
		et.agent.expectProblem(http.StatusBadRequest, "verification_code_invalid", "POST", "/v1/me/addresses/"+gid+"/verify", map[string]any{"code": gwrong})
	}
	et.agent.expectProblem(http.StatusBadRequest, "verification_code_invalid", "POST", "/v1/me/addresses/"+gid+"/verify", map[string]any{"code": gcode})

	squatted := unique("squat") + "@example.com"
	et.owner.expect(http.StatusAccepted, "POST", "/v1/me/addresses", map[string]any{"address": squatted})
	memberAddressCode(t, h, squatted, 1)
	sid := et.agent.expect(http.StatusAccepted, "POST", "/v1/me/addresses", map[string]any{"address": squatted}).str("id")
	if _, ok := myAddresses(et.owner)[squatted]; ok {
		t.Fatal("another member's pending address was kept")
	}
	et.agent.expect(http.StatusOK, "POST", "/v1/me/addresses/"+sid+"/verify", map[string]any{"code": memberAddressCode(t, h, squatted, 2)})

	et.agent.expect(http.StatusNoContent, "DELETE", "/v1/me/addresses/"+id, nil)
	if _, ok := myAddresses(et.agent)[addr]; ok {
		t.Fatal("a removed address is listed")
	}
	et.agent.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/me/addresses/"+id, nil)

	key := et.apiKey(h)
	key.expectProblem(http.StatusForbidden, "member_session_required", "GET", "/v1/me/addresses", nil)

	var added, verified, removed int
	if err := h.st.Pool.QueryRow(t.Context(), `SELECT count(*) FILTER (WHERE action = 'member_address.added'), count(*) FILTER (WHERE action = 'member_address.verified'),
		count(*) FILTER (WHERE action = 'member_address.removed') FROM audit_log WHERE workspace_id = $1 AND target_id = $2`, et.ws, et.agentID).Scan(&added, &verified, &removed); err != nil {
		t.Fatal(err)
	}
	if added != 4 || verified != 3 || removed != 1 {
		t.Fatalf("audit: %d added, %d verified, %d removed", added, verified, removed)
	}
}

func TestSendingAsTheMember(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	support := newEmailTeam(t, h, false)
	et.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+et.inbox, map[string]any{"purpose": "mail"})
	et.owner.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"name": "Olga Owner"})
	support.owner.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"name": "Olga Owner"})
	d := unique("member") + ".example.org"
	cust := "cust@" + d

	compose := func(c *client, inbox string, extra map[string]any) (string, string) {
		t.Helper()
		body := map[string]any{"inbox_id": inbox, "to": []string{cust}, "subject": "Hello", "body": "Hi."}
		for k, v := range extra {
			body[k] = v
		}
		r := c.expect(http.StatusCreated, "POST", "/v1/conversations/mail", body)
		return r.body["conversation"].(map[string]any)["id"].(string), r.body["message"].(map[string]any)["id"].(string)
	}

	mailConv, msg := compose(et.owner, et.inbox, nil)
	h.sendQueued(t, et.ws, msg)
	sent, parsed := h.smtp.last(t)
	if from := parsed.Header.Get("From"); sent.from != et.address || !strings.Contains(from, `"Olga Owner (Acme Support)"`) || !strings.Contains(from, et.address) {
		t.Fatalf("mail inbox From %q envelope %s", from, sent.from)
	}
	fw := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+mailConv+"/forward", map[string]any{"to": []string{"boss@" + d}})
	h.sendQueued(t, et.ws, fw.str("id"))
	if _, parsed = h.smtp.last(t); !strings.Contains(parsed.Header.Get("From"), `"Olga Owner (Acme Support)"`) {
		t.Fatalf("mail inbox forward From %q", parsed.Header.Get("From"))
	}
	_, smsg := compose(support.owner, support.inbox, nil)
	h.sendQueued(t, support.ws, smsg)
	if _, parsed = h.smtp.last(t); strings.Contains(parsed.Header.Get("From"), "Olga") || !strings.Contains(parsed.Header.Get("From"), "Acme Support") {
		t.Fatalf("support inbox From %q", parsed.Header.Get("From"))
	}

	mine := unique("olga") + "@example.com"
	verifiedAddress(t, h, et.owner, mine)
	if u := myAddresses(et.owner)[mine]["usable_channel_ids"].([]any); len(u) != 0 {
		t.Fatalf("usable without a check: %v", u)
	}
	for _, res := range []string{checkResult("example.com", false, "pass"), checkResult("example.com", true, "fail"), checkResult("other.example", true, "pass")} {
		setDeliverability(t, h, et.ws, et.channel, "received", res)
		if u := myAddresses(et.owner)[mine]["usable_channel_ids"].([]any); len(u) != 0 {
			t.Fatalf("usable with %s: %v", res, u)
		}
		et.owner.expectProblem(http.StatusConflict, "from_not_allowed", "POST", "/v1/conversations/mail", map[string]any{"inbox_id": et.inbox, "to": []string{cust}, "subject": "S", "body": "B", "from_address": mine})
	}
	setDeliverability(t, h, et.ws, et.channel, "sent", checkResult("example.com", true, "pass"))
	if u := myAddresses(et.owner)[mine]["usable_channel_ids"].([]any); len(u) != 0 {
		t.Fatalf("usable with an unfinished check: %v", u)
	}
	setDeliverability(t, h, et.ws, et.channel, "received", checkResult("example.com", true, "pass"))
	if u := myAddresses(et.owner)[mine]["usable_channel_ids"].([]any); !slices.Equal(u, []any{et.channel}) {
		t.Fatalf("usable channels %v, want %s", u, et.channel)
	}

	if _, err := h.st.Pool.Exec(t.Context(), "UPDATE deliverability_checks SET created_at = created_at - interval '31 days' WHERE channel_id = $1", et.channel); err != nil {
		t.Fatal(err)
	}
	if u := myAddresses(et.owner)[mine]["usable_channel_ids"].([]any); len(u) != 0 {
		t.Fatalf("usable with a check older than 30 days: %v", u)
	}
	setDeliverability(t, h, et.ws, et.channel, "received", checkResult("example.com", true, "pass"))
	smtp := func(host string) {
		t.Helper()
		h.clock.Advance(time.Second)
		et.owner.expect(http.StatusOK, "PATCH", "/v1/channels/"+et.channel, map[string]any{"email": map[string]any{"address": et.address,
			"smtp": map[string]any{"host": host, "port": 2525, "username": "mailer", "password": "smtp-secret", "tls": "starttls"}}})
	}
	et.owner.expect(http.StatusOK, "PATCH", "/v1/channels/"+et.channel, map[string]any{"name": "Support mail"})
	if u := myAddresses(et.owner)[mine]["usable_channel_ids"].([]any); len(u) != 1 {
		t.Fatalf("a change of the name made the check stale: %v", u)
	}
	smtp("smtp2.example.com")
	if u := myAddresses(et.owner)[mine]["usable_channel_ids"].([]any); len(u) != 0 {
		t.Fatalf("usable with a check older than the SMTP account: %v", u)
	}
	setDeliverability(t, h, et.ws, et.channel, "received", checkResult("example.com", true, "pass"))

	et.owner.expectProblem(http.StatusConflict, "from_not_allowed", "POST", "/v1/conversations/mail", map[string]any{"inbox_id": et.inbox, "to": []string{cust}, "subject": "S", "body": "B", "from_address": "nobody@example.com"})
	verifiedAddress(t, h, et.agent, unique("agent")+"@example.com")
	et.owner.expectProblem(http.StatusConflict, "from_not_allowed", "POST", "/v1/conversations/mail", map[string]any{"inbox_id": et.inbox, "to": []string{cust}, "subject": "S", "body": "B", "from_address": strings.ToUpper(et.address)})

	convID, msg := compose(et.owner, et.inbox, map[string]any{"from_address": strings.ToUpper(mine)})
	h.sendQueued(t, et.ws, msg)
	sent, parsed = h.smtp.last(t)
	if from := parsed.Header.Get("From"); sent.from != et.address || from != `Olga Owner <`+mine+`>` {
		t.Fatalf("From %q envelope %s", from, sent.from)
	}
	if rt := parsed.Header.Get("Reply-To"); !strings.Contains(rt, et.address) {
		t.Fatalf("Reply-To %q", rt)
	}
	if mid := parsed.Header.Get("Message-ID"); !strings.HasSuffix(mid, "@example.com>") {
		t.Fatalf("Message-ID %q", mid)
	}
	ourID := email.NormalizeID(parsed.Header.Get("Message-ID"))

	key := et.apiKey(h)
	key.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+convID+"/messages", map[string]any{"kind": "message", "body": "B", "draft": true, "from_address": mine})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+convID+"/messages", map[string]any{"kind": "note", "body": "B", "from_address": mine})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+convID+"/messages", map[string]any{"kind": "message", "body": "B", "draft": true, "from_address": mine})

	blank := et.owner.uploadFields("/v1/conversations/"+convID+"/messages", []formField{{"kind", "message"}, {"body", "Blank."}, {"from_address", " "}}, nil)
	if blank.status != http.StatusCreated {
		t.Fatalf("a blank from_address field: %d %s", blank.status, blank.raw)
	}
	h.sendQueued(t, et.ws, blank.str("id"))
	if _, parsed = h.smtp.last(t); !strings.Contains(parsed.Header.Get("From"), et.address) {
		t.Fatalf("a blank from_address field sent From %q", parsed.Header.Get("From"))
	}

	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+convID+"/messages", map[string]any{"kind": "message", "body": "Again.", "from_address": mine})
	h.sendQueued(t, et.ws, reply.str("id"))
	if _, parsed = h.smtp.last(t); parsed.Header.Get("From") != `Olga Owner <`+mine+`>` {
		t.Fatalf("reply From %q", parsed.Header.Get("From"))
	}

	late := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+convID+"/messages", map[string]any{"kind": "message", "body": "Late.", "from_address": mine})
	setDeliverability(t, h, et.ws, et.channel, "received", checkResult("example.com", false, "pass"))
	h.sendQueued(t, et.ws, late.str("id"))
	if dl := lastOf(messages(et.owner, convID), "message")["delivery"].(map[string]any); dl["state"] != "failed" || !strings.Contains(dl["error"].(string), "may no longer send") {
		t.Fatalf("send-time re-check: %v", dl)
	}
	setDeliverability(t, h, et.ws, et.channel, "received", checkResult("example.com", true, "pass"))
	stale := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+convID+"/messages", map[string]any{"kind": "message", "body": "Stale.", "from_address": mine})
	smtp("smtp3.example.com")
	h.sendQueued(t, et.ws, stale.str("id"))
	if dl := lastOf(messages(et.owner, convID), "message")["delivery"].(map[string]any); dl["state"] != "failed" || !strings.Contains(dl["error"].(string), "sending settings") {
		t.Fatalf("send-time re-check after an SMTP change: %v", dl)
	}
	setDeliverability(t, h, et.ws, et.channel, "received", checkResult("example.com", true, "pass"))

	joined := h.ingest(mine, buildMail(mailOpts{
		from: cust, to: mine, subject: "Re: Hello", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + ourID + ">", "Authentication-Results": dmarcPass(d)}, body: "Thanks.",
	}), nil)
	if joined.status != http.StatusAccepted && joined.status != http.StatusOK && joined.status != http.StatusCreated || joined.str("conversation_id") != convID {
		t.Fatalf("reply to the member address: %d %v", joined.status, joined.body)
	}
	if last := lastOf(messages(et.owner, convID), "message"); !strings.Contains(last["body"].(string), "Thanks.") {
		t.Fatalf("the reply did not land: %v", last)
	}

	contact := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+convID, nil).str("contact_id")
	et.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+contact, map[string]any{"blocked": true})
	blocked := h.ingest(mine, buildMail(mailOpts{
		from: cust, to: mine, subject: "Re: Hello", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + ourID + ">", "Authentication-Results": dmarcPass(d)}, body: "Blocked.",
	}), nil)
	if blocked.status != http.StatusNotFound || blocked.str("code") != "unknown_recipient" {
		t.Fatalf("a blocked sender through a member address: %d %v", blocked.status, blocked.body)
	}
	et.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+contact, map[string]any{"blocked": false})

	stray := h.ingest(mine, buildMail(mailOpts{
		from: cust, to: mine, subject: "New", messageID: newMessageID(),
		headers: map[string]string{"Authentication-Results": dmarcPass(d)}, body: "Hi.",
	}), nil)
	unknown := "nobody-" + unique("x") + "@example.com"
	nobody := h.ingest(unknown, buildMail(mailOpts{
		from: cust, to: unknown, subject: "New", messageID: newMessageID(),
		headers: map[string]string{"Authentication-Results": dmarcPass(d)}, body: "Hi.",
	}), nil)
	if stray.status != http.StatusNotFound || stray.str("code") != "unknown_recipient" || nobody.status != stray.status || nobody.str("code") != stray.str("code") {
		t.Fatalf("mail to the member address without a token: %d %v, unknown address: %d %v", stray.status, stray.body, nobody.status, nobody.body)
	}

	cross := h.ingest(mine, buildMail(mailOpts{
		from: cust, to: mine, subject: "Re: Hello", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + email.NormalizeID(parsedSupportID(t, h, support, smsg)) + ">", "Authentication-Results": dmarcPass(d)}, body: "Wrong place.",
	}), nil)
	if cross.status != http.StatusNotFound {
		t.Fatalf("a token of another workspace through a member address: %d %v", cross.status, cross.body)
	}
}

func parsedSupportID(t *testing.T, h *harness, tm emailTeam, msg string) string {
	t.Helper()
	var id string
	if err := h.st.Pool.QueryRow(t.Context(), "SELECT header_message_id FROM message_emails WHERE workspace_id = $1 AND message_id = $2", tm.ws, msg).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestMemberAddressApproval(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	other := newEmailTeam(t, h, false)
	d := unique("shared") + ".example.org"
	et.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+et.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Everything", "email": map[string]any{"address": "*@" + d},
	})
	ownerID := et.owner.expect(http.StatusOK, "GET", "/v1/me", nil).body["memberships"].([]any)[0].(map[string]any)["member_id"].(string)
	addr := "ann@" + d

	r := et.agent.expect(http.StatusAccepted, "POST", "/v1/me/addresses", map[string]any{"address": addr})
	id := r.str("id")
	if r.str("verification") != "approval" || r.body["code_sent_at"] != nil || r.body["verified"] != false {
		t.Fatalf("an address under a catch-all: %s", r.raw)
	}
	et.agent.expect(http.StatusAccepted, "POST", "/v1/me/addresses", map[string]any{"address": addr})
	time.Sleep(100 * time.Millisecond)
	if n := len(h.mail.to(addr)); n != 0 {
		t.Fatalf("%d code mails to an address under a catch-all", n)
	}
	et.agent.expectProblem(http.StatusBadRequest, "verification_code_invalid", "POST", "/v1/me/addresses/"+id+"/verify", map[string]any{"code": "123456"})

	path := "/v1/members/" + et.agentID + "/addresses"
	et.agent.expectProblem(http.StatusForbidden, "forbidden", "GET", path, nil)
	et.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", path+"/"+id+"/approve", nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", path, nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", path+"/"+id+"/approve", nil)
	et.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/members/"+ownerID+"/addresses/"+id+"/approve", nil)
	list := et.owner.expect(http.StatusOK, "GET", path, nil).body["items"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["verification"] != "approval" {
		t.Fatalf("the member's addresses: %v", list)
	}
	ok := et.owner.expect(http.StatusOK, "POST", path+"/"+id+"/approve", nil)
	if ok.body["verified"] != true {
		t.Fatalf("approved: %s", ok.raw)
	}
	et.owner.expect(http.StatusOK, "POST", path+"/"+id+"/approve", nil)
	if _, ok := myAddresses(et.agent)[addr]; !ok {
		t.Fatal("the approved address is not listed")
	}

	coded := unique("coded") + "@example.com"
	cid := et.agent.expect(http.StatusAccepted, "POST", "/v1/me/addresses", map[string]any{"address": coded}).str("id")
	et.owner.expectProblem(http.StatusConflict, "approval_not_needed", "POST", path+"/"+cid+"/approve", nil)

	var by string
	if err := h.st.Pool.QueryRow(t.Context(), `SELECT details->>'by' FROM audit_log WHERE workspace_id = $1 AND action = 'member_address.verified' AND target_id = $2`, et.ws, et.agentID).Scan(&by); err != nil || by != "approval" {
		t.Fatalf("approval audit: %q %v", by, err)
	}
}

func TestFromAddressBeforeRecipientBudget(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	d := unique("budget") + ".example.org"
	mail := func(n int, extra map[string]any) map[string]any {
		hidden := make([]string, 50)
		for i := range hidden {
			hidden[i] = fmt.Sprintf("b%d-%d@%s", n, i, d)
		}
		b := map[string]any{"inbox_id": et.inbox, "to": []string{fmt.Sprintf("to%d@%s", n, d)}, "bcc": hidden, "subject": "Budget", "body": "x"}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	for i := range 7 {
		et.owner.expectProblem(http.StatusConflict, "from_not_allowed", "POST", "/v1/conversations/mail", mail(i, map[string]any{"from_address": "nobody@example.com"}))
	}
	draft := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", mail(100, map[string]any{"draft": true})).body["message"].(map[string]any)["id"].(string)
	for range 7 {
		et.owner.expectProblem(http.StatusConflict, "from_not_allowed", "POST", "/v1/messages/"+draft+"/send", map[string]any{"from_address": "nobody@example.com"})
	}
	for i := range 5 {
		et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", mail(200+i, nil))
	}
}

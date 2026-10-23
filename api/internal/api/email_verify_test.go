package api_test

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/store"
)

var addressCodePattern = regexp.MustCompile(`\b\d{8}\b`)

func verificationOf(t *testing.T, c *client, channel string) map[string]any {
	t.Helper()
	r := c.expect(http.StatusOK, "GET", "/v1/channels/"+channel, nil)
	return r.body["email"].(map[string]any)["verification"].(map[string]any)
}

func channelOfConversation(t *testing.T, c *client, conv string) string {
	t.Helper()
	return c.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).str("channel_id")
}

// legacyChannel stores address on a channel as proven, the way a row saved before today's checks
// looks, bypassing them.
func legacyChannel(t *testing.T, h *harness, tm team, address string) string {
	t.Helper()
	id := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Legacy", "email": map[string]any{"address": unique("legacy") + "@example.com"},
	}).str("id")
	ctx := context.Background()
	if _, err := h.st.Pool.Exec(ctx, "UPDATE email_channels SET verified_at = NULL, verified_by = NULL WHERE address = $1", address); err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.Pool.Exec(ctx,
		"UPDATE email_channels SET address = $2, verified_at = now(), verified_by = 'dns' WHERE channel_id = $1", id, address); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAddressClaimsActOnlyInTheirOwnWorkspace(t *testing.T) {
	t.Parallel()
	fwd := unique("fwd") + ".example"
	h := newHarnessWith(t, func(d *api.Deps) { d.Ingress.ForwardingDomain = fwd })
	victim := newEmailTeam(t, h, false)
	attacker := newTeam(t, h)
	create := func(email map[string]any) response {
		return attacker.owner.do("POST", "/v1/inboxes/"+attacker.inbox+"/channels", map[string]any{"kind": "email", "name": "x", "email": email})
	}

	t.Run("a catch-all on the forwarding domain never receives another team's forwarded mail", func(t *testing.T) {
		old := victim.owner.expect(http.StatusOK, "GET", "/v1/channels/"+victim.channel, nil).str("forwarding_address")
		victim.owner.expect(http.StatusOK, "POST", "/v1/channels/"+victim.channel+"/forwarding-address/rotate", nil)
		if r := create(map[string]any{"address": "*@" + fwd}); r.status != http.StatusBadRequest || r.str("code") != "validation_failed" {
			t.Fatalf("catch-all on the forwarding domain: %d %s", r.status, r.raw)
		}
		legacyChannel(t, h, attacker, "*@"+fwd)
		raw := buildMail(mailOpts{from: "lea@example.org", to: old, subject: "Order", messageID: newMessageID(), body: "Where is my order?"})
		if r := h.ingest(old, raw, nil); r.status != http.StatusNotFound {
			t.Fatalf("mail to a rotated forwarding address: %d %v", r.status, r.body)
		}
	})

	t.Run("another workspace's channel addresses never drop a sender", func(t *testing.T) {
		ceo := "ceo@" + unique("bigcustomer") + ".example"
		if r := create(map[string]any{"address": unique("help") + "@attacker.example", "from_address": ceo}); r.status != http.StatusCreated {
			t.Fatalf("from_address: %d %s", r.status, r.raw)
		}
		if r := create(map[string]any{"address": ceo}); r.status != http.StatusCreated {
			t.Fatalf("address: %d %s", r.status, r.raw)
		}
		raw := buildMail(mailOpts{from: ceo, to: victim.address, subject: "Renewal", messageID: newMessageID(), body: "Please renew."})
		r := h.ingest(victim.address, raw, nil)
		if r.status != http.StatusAccepted || r.str("status") != "stored" {
			t.Fatalf("mail from an address another workspace uses: %d %v", r.status, r.body)
		}
	})

	t.Run("another workspace's catch-all never makes a domain ours", func(t *testing.T) {
		if r := create(map[string]any{"address": "*@gmail.com"}); r.status != http.StatusBadRequest || r.str("code") != "validation_failed" {
			t.Fatalf("catch-all on a public provider: %d %s", r.status, r.raw)
		}
		legacyChannel(t, h, attacker, "*@googlemail.com")
		corp := unique("corp") + ".example"
		if r := create(map[string]any{"address": "*@" + corp}); r.status != http.StatusCreated {
			t.Fatalf("catch-all on the attacker's own domain: %d %s", r.status, r.raw)
		}
		d := unique("cust") + ".example.org"
		cust := "cust@" + d
		r := h.ingest(victim.address, buildMail(mailOpts{
			from: cust, to: victim.address, subject: "Team", messageID: newMessageID(), body: "Adding my colleagues.",
			headers: map[string]string{"Cc": "pal@googlemail.com, x@" + corp, "Authentication-Results": dmarcPass(d)},
		}), nil)
		if r.status != http.StatusAccepted {
			t.Fatalf("ingest: %d %v", r.status, r.body)
		}
		conv := r.str("conversation_id")
		got := participantEmails(t, victim.owner, conv)
		if !slices.Contains(got, "pal@googlemail.com") || !slices.Contains(got, "x@"+corp) {
			t.Fatalf("participants %v", got)
		}
		victim.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/participants", map[string]any{"email": "friend@gmail.com"})
		victim.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/participants", map[string]any{"email": "y@" + corp})
		victim.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+conv+"/participants",
			map[string]any{"email": strings.Replace(victim.address, "@", "+tag@", 1)})
	})
}

func TestAddressProofByCode(t *testing.T) {
	t.Parallel()
	fwd := unique("fwd") + ".example"
	h := newHarnessWith(t, func(d *api.Deps) { d.Ingress.ForwardingDomain = fwd })
	h.unproven = true
	a := newEmailTeam(t, h, false)
	v := verificationOf(t, a.owner, a.channel)
	if v["state"] != "unverified" || v["code_allowed"] != true || v["txt_name"] != "_yuva-mail.example.com" {
		t.Fatalf("new channel: %v", v)
	}
	mailTo := func(to string) ingressResult {
		return h.ingest(to, buildMail(mailOpts{from: unique("c") + "@example.org", to: a.address, subject: "Hi", messageID: newMessageID(), body: "Hello"}), nil)
	}
	if r := mailTo(a.address); r.status != http.StatusNotFound {
		t.Fatalf("mail to an unproven address: %d %v", r.status, r.body)
	}
	fwdAddr := a.owner.expect(http.StatusOK, "GET", "/v1/channels/"+a.channel, nil).str("forwarding_address")
	if r := mailTo(fwdAddr); r.status != http.StatusAccepted || r.str("status") != "stored" {
		t.Fatalf("mail to an unproven channel's forwarding address: %d %v", r.status, r.body)
	}

	b := newTeam(t, h)
	b.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/channels/"+a.channel+"/verification-code", nil)
	b.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/channels/"+a.channel+"/verify", map[string]any{"code": "12345678"})
	a.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/channels/"+a.channel+"/verification-code", nil)

	sent := a.owner.expect(http.StatusAccepted, "POST", "/v1/channels/"+a.channel+"/verification-code", nil)
	if sent.body["email"].(map[string]any)["verification"].(map[string]any)["code_sent_at"] == nil {
		t.Fatalf("code_sent_at missing: %s", sent.raw)
	}
	a.owner.expectProblem(http.StatusTooManyRequests, "rate_limited", "POST", "/v1/channels/"+a.channel+"/verification-code", nil)
	msg := h.mail.wait(t, a.address, 1)[0]
	code := addressCodePattern.FindString(msg.Text)
	if code == "" || !strings.Contains(msg.Subject, code) {
		t.Fatalf("code mail: %q %q", msg.Subject, msg.Text)
	}
	if strings.Contains(msg.Text, "Acme") || strings.Contains(msg.HTML, "Acme") {
		t.Fatal("the code mail names the workspace's inbox or channel")
	}
	wrong := "00000000"
	if code == wrong {
		wrong = "11111111"
	}
	a.owner.expectProblem(http.StatusBadRequest, "verification_code_invalid", "POST", "/v1/channels/"+a.channel+"/verify", map[string]any{"code": wrong})
	r := a.owner.expect(http.StatusOK, "POST", "/v1/channels/"+a.channel+"/verify", map[string]any{"code": code})
	if v := r.body["email"].(map[string]any)["verification"].(map[string]any); v["state"] != "verified" || v["method"] != "code" {
		t.Fatalf("after the code: %s", r.raw)
	}
	a.owner.expectProblem(http.StatusConflict, "email_verified", "POST", "/v1/channels/"+a.channel+"/verification-code", nil)
	in := mailTo(a.address)
	if in.status != http.StatusAccepted || channelOfConversation(t, a.owner, in.str("conversation_id")) != a.channel {
		t.Fatalf("mail to a proven address: %d %v", in.status, in.body)
	}

	t.Run("the latest prover takes the address", func(t *testing.T) {
		bc := b.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+b.inbox+"/channels", map[string]any{
			"kind": "email", "name": "Same", "email": map[string]any{"address": a.address},
		}).str("id")
		b.owner.expect(http.StatusAccepted, "POST", "/v1/channels/"+bc+"/verification-code", nil)
		code := addressCodePattern.FindString(h.mail.wait(t, a.address, 2)[1].Text)
		b.owner.expect(http.StatusOK, "POST", "/v1/channels/"+bc+"/verify", map[string]any{"code": code})
		if v := verificationOf(t, a.owner, a.channel); v["state"] != "unverified" || v["problem"] != "taken" {
			t.Fatalf("previous holder: %v", v)
		}
		in := mailTo(a.address)
		if in.status != http.StatusAccepted || channelOfConversation(t, b.owner, in.str("conversation_id")) != bc {
			t.Fatalf("mail after the takeover: %d %v", in.status, in.body)
		}
		var lost int
		if err := h.st.Pool.QueryRow(context.Background(),
			"SELECT count(*) FROM audit_log WHERE workspace_id = $1 AND action = 'channel.address_lost' AND target_id = $2", a.ws, a.channel).Scan(&lost); err != nil || lost != 1 {
			t.Fatalf("channel.address_lost entries: %d %v", lost, err)
		}
	})

	t.Run("a code mail back through the forwarding address proves the channel", func(t *testing.T) {
		c := newEmailTeam(t, h, false)
		c.owner.expect(http.StatusAccepted, "POST", "/v1/channels/"+c.channel+"/verification-code", nil)
		m := h.mail.wait(t, c.address, 1)[0]
		fwdAddr := c.owner.expect(http.StatusOK, "GET", "/v1/channels/"+c.channel, nil).str("forwarding_address")
		forged := h.ingest(fwdAddr, buildMail(mailOpts{from: "noreply@yuva.example", to: c.address, subject: m.Subject, messageID: newMessageID(), body: m.Text}), nil)
		if forged.status != http.StatusAccepted || forged.str("status") != "stored" {
			t.Fatalf("a copy with another Message-ID: %d %v", forged.status, forged.body)
		}
		if v := verificationOf(t, c.owner, c.channel); v["state"] != "unverified" {
			t.Fatalf("verified by a copy: %v", v)
		}
		back := h.ingest(fwdAddr, buildMail(mailOpts{from: "noreply@yuva.example", to: c.address, subject: m.Subject, messageID: m.MessageID, body: m.Text}), nil)
		if back.status != http.StatusAccepted || back.str("status") != "dropped" {
			t.Fatalf("code mail back: %d %v", back.status, back.body)
		}
		if v := verificationOf(t, c.owner, c.channel); v["state"] != "verified" || v["method"] != "code" {
			t.Fatalf("after the code mail came back: %v", v)
		}
		c.owner.expect(http.StatusOK, "PATCH", "/v1/channels/"+c.channel, map[string]any{"email": map[string]any{"address": unique("new") + "@example.com"}})
		if v := verificationOf(t, c.owner, c.channel); v["state"] != "unverified" {
			t.Fatalf("a new address kept the proof: %v", v)
		}
	})

	t.Run("an address on the forwarding domain needs the TXT record", func(t *testing.T) {
		d := b.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+b.inbox+"/channels", map[string]any{
			"kind": "email", "name": "Fwd", "email": map[string]any{"address": "hello@" + fwd},
		}).str("id")
		b.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/channels/"+d+"/verification-code", nil)
	})
}

func TestAddressProofByDNS(t *testing.T) {
	t.Parallel()
	dns := &ssoDNS{txt: map[string][]string{}}
	h := newHarnessWith(t, func(d *api.Deps) { d.Status.Resolver = dns })
	h.unproven = true
	tm := newTeam(t, h)
	other := newTeam(t, h)
	domain := unique("acme") + ".example"
	ch := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
		"kind": "email", "name": "All", "email": map[string]any{"address": "*@" + domain},
	}).str("id")
	v := verificationOf(t, tm.owner, ch)
	if v["code_allowed"] != false || v["txt_name"] != "_yuva-mail."+domain {
		t.Fatalf("catch-all: %v", v)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/channels/"+ch+"/verification-code", nil)
	r := tm.owner.expect(http.StatusOK, "POST", "/v1/channels/"+ch+"/verify", nil)
	if v := r.body["email"].(map[string]any)["verification"].(map[string]any); v["state"] != "unverified" || !strings.Contains(v["problem"].(string), "no TXT record") {
		t.Fatalf("without a record: %s", r.raw)
	}
	mail := func(to string) ingressResult {
		return h.ingest(to, buildMail(mailOpts{from: unique("c") + "@example.org", to: to, subject: "Hi", messageID: newMessageID(), body: "Hello"}), nil)
	}
	if r := mail("sales@" + domain); r.status != http.StatusNotFound {
		t.Fatalf("mail to an unproven catch-all: %d %v", r.status, r.body)
	}

	oc := other.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+other.inbox+"/channels", map[string]any{
		"kind": "email", "name": "All", "email": map[string]any{"address": "*@" + domain},
	}).str("id")
	otherValue := verificationOf(t, other.owner, oc)["txt_value"].(string)
	if otherValue == v["txt_value"] {
		t.Fatal("two workspaces share a TXT value")
	}
	dns.set(v["txt_name"].(string), otherValue)
	r = tm.owner.expect(http.StatusOK, "POST", "/v1/channels/"+ch+"/verify", nil)
	if v := r.body["email"].(map[string]any)["verification"].(map[string]any); v["state"] != "unverified" || !strings.Contains(v["problem"].(string), "does not hold") {
		t.Fatalf("with another workspace's value: %s", r.raw)
	}
	dns.set(v["txt_name"].(string), v["txt_value"].(string))
	r = tm.owner.expect(http.StatusOK, "POST", "/v1/channels/"+ch+"/verify", nil)
	if v := r.body["email"].(map[string]any)["verification"].(map[string]any); v["state"] != "verified" || v["method"] != "dns" {
		t.Fatalf("with the record: %s", r.raw)
	}
	in := mail("sales@" + domain)
	if in.status != http.StatusAccepted || channelOfConversation(t, tm.owner, in.str("conversation_id")) != ch {
		t.Fatalf("mail to a proven catch-all: %d %v", in.status, in.body)
	}

	exact := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Billing", "email": map[string]any{"address": "billing@" + domain},
	}).str("id")
	tm.owner.expect(http.StatusOK, "POST", "/v1/channels/"+exact+"/verify", nil)
	in = mail("billing@" + domain)
	if in.status != http.StatusAccepted || channelOfConversation(t, tm.owner, in.str("conversation_id")) != exact {
		t.Fatalf("one record proves every channel of the workspace on the domain: %d %v", in.status, in.body)
	}
}

// TestAddressProofUpgradeRule runs alone: the operator notices it writes and reads are server-wide.
func TestAddressProofUpgradeRule(t *testing.T) {
	fwd := unique("fwd") + ".example"
	h := newHarnessWith(t, func(d *api.Deps) { d.Ingress.ForwardingDomain = fwd })
	h.unproven = true
	ctx := context.Background()
	used := newEmailTeam(t, h, false)
	to := used.owner.expect(http.StatusOK, "GET", "/v1/channels/"+used.channel, nil).str("forwarding_address")
	if r := h.ingest(to, buildMail(mailOpts{from: unique("c") + "@example.org", to: used.address, subject: "Hi", messageID: newMessageID(), body: "Hello"}), nil); r.status != http.StatusAccepted {
		t.Fatalf("ingest: %d %v", r.status, r.body)
	}
	catchAll := used.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+used.inbox+"/channels", map[string]any{
		"kind": "email", "name": "All", "email": map[string]any{"address": "*@" + unique("shop") + ".example"},
	}).str("id")

	files, err := filepath.Glob("../store/migrations/*_email_address_proof.sql")
	if err != nil || len(files) != 1 {
		t.Fatalf("migration: %v %v", files, err)
	}
	src, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "UPDATE email_channels e SET verified_at") {
		t.Fatal("the upgrade proves channels by the mail they received")
	}
	start := strings.Index(string(src), "INSERT INTO operator_notices")
	end := strings.Index(string(src)[start:], "w.deleted_at IS NULL") + len("w.deleted_at IS NULL")
	notices := string(src)[start:start+end] + " AND e.workspace_id = $1"
	if _, err := h.st.Pool.Exec(ctx, notices, used.ws); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{used.channel, catchAll} {
		if v := verificationOf(t, used.owner, id); v["state"] != "unverified" {
			t.Fatalf("an upgraded channel is proven: %v", v)
		}
		var n int
		if err := h.st.Pool.QueryRow(ctx, "SELECT count(*) FROM operator_notices WHERE kind = 'email_unproven' AND sent_at IS NULL AND detail LIKE '%' || $1 || '%'", id).Scan(&n); err != nil || n != 1 {
			t.Fatalf("operator notices for %s: %d %v", id, n, err)
		}
	}
	if r := h.ingest(used.address, buildMail(mailOpts{from: unique("c") + "@example.org", to: used.address, subject: "Hi", messageID: newMessageID(), body: "Hello"}), nil); r.status != http.StatusNotFound {
		t.Fatalf("mail by address to an upgraded, unproven channel: %d %v", r.status, r.body)
	}
	if err := h.srv.SendOperatorDigest(ctx); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := h.st.Pool.QueryRow(ctx, "SELECT count(*) FROM operator_notices WHERE kind = 'email_unproven' AND sent_at IS NULL").Scan(&left); err != nil || left != 0 {
		t.Fatalf("notices left after the log: %d %v", left, err)
	}
}

func TestOwnUnprovenAddressesStayOurs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	h.unproven = true
	sibling := unique("sales") + "@example.com"
	et.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+et.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Sales", "email": map[string]any{"address": sibling, "from_address": "noreply-" + sibling},
	})
	h.unproven = false
	d := unique("cust") + ".example.org"
	cust := "cust@" + d
	r := h.ingest(et.address, buildMail(mailOpts{
		from: cust, to: et.address, subject: "Quote", messageID: newMessageID(), body: "Copying your sales team.",
		headers: map[string]string{"Cc": sibling + ", col@" + d, "Authentication-Results": dmarcPass(d)},
	}), nil)
	if r.status != http.StatusAccepted {
		t.Fatalf("ingest: %d %v", r.status, r.body)
	}
	conv := r.str("conversation_id")
	if got := participantEmails(t, et.owner, conv); !slices.Equal(got, []string{"col@" + d}) {
		t.Fatalf("participants %v", got)
	}
	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Here you go."})
	h.sendQueued(t, et.ws, reply.str("id"))
	if sent, _ := h.smtp.last(t); slices.Contains(sent.to, sibling) {
		t.Fatalf("a reply copied the team's own mailbox: %v", sent.to)
	}
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "x", "cc": []string{sibling}})
	for _, from := range []string{sibling, "noreply-" + sibling} {
		loop := h.ingest(et.address, buildMail(mailOpts{from: from, to: et.address, subject: "Auto", messageID: newMessageID(), body: "Looping."}), nil)
		if loop.status != http.StatusAccepted || loop.str("status") != "dropped" {
			t.Fatalf("mail from the workspace's unproven channel %s: %d %v", from, loop.status, loop.body)
		}
	}
}

func ownerEmail(t *testing.T, h *harness, ws string) string {
	t.Helper()
	var out string
	if err := h.st.Pool.QueryRow(context.Background(),
		"SELECT p.email FROM members m JOIN people p ON p.id = m.person_id WHERE m.workspace_id = $1 AND m.role = 'owner' LIMIT 1", ws).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAddressMovesAreReported(t *testing.T) {
	t.Parallel()
	dns := &ssoDNS{txt: map[string][]string{}}
	h := newHarnessWith(t, func(d *api.Deps) { d.Status.Resolver = dns })
	holder := newTeam(t, h)
	domain := unique("corp") + ".example"
	all := holder.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+holder.inbox+"/channels", map[string]any{
		"kind": "email", "name": "All", "email": map[string]any{"address": "*@" + domain},
	}).str("id")
	h.unproven = true
	mover := newTeam(t, h)
	exact := "jobs@" + domain
	ch := mover.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+mover.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Jobs", "email": map[string]any{"address": exact},
	}).str("id")
	h.unproven = false
	mover.owner.expect(http.StatusAccepted, "POST", "/v1/channels/"+ch+"/verification-code", nil)
	code := addressCodePattern.FindString(h.mail.wait(t, exact, 1)[0].Text)
	mover.owner.expect(http.StatusOK, "POST", "/v1/channels/"+ch+"/verify", map[string]any{"code": code})
	var via string
	if err := h.st.Pool.QueryRow(context.Background(),
		"SELECT details->>'via' FROM audit_log WHERE workspace_id = $1 AND action = 'channel.address_lost' AND target_id = $2 AND details->>'address' = $3",
		holder.ws, all, exact).Scan(&via); err != nil || via != "*@"+domain {
		t.Fatalf("address_lost for mail moving off the catch-all: %q %v", via, err)
	}
	byDNS := func(addr string) {
		t.Helper()
		h.unproven = true
		id := mover.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+mover.inbox+"/channels", map[string]any{
			"kind": "email", "name": addr, "email": map[string]any{"address": addr},
		}).str("id")
		h.unproven = false
		v := verificationOf(t, mover.owner, id)
		dns.set(v["txt_name"].(string), v["txt_value"].(string))
		if r := mover.owner.expect(http.StatusOK, "POST", "/v1/channels/"+id+"/verify", nil); !strings.Contains(string(r.raw), `"state":"verified"`) {
			t.Fatalf("dns proof: %s", r.raw)
		}
	}
	byDNS("sales@" + domain)
	ctx := context.Background()
	owner := ownerEmail(t, h, holder.ws)
	lossMails := func() []string {
		var out []string
		for _, m := range h.mail.to(owner) {
			if strings.Contains(m.Subject, "no longer reaches") {
				out = append(out, m.Text)
			}
		}
		return out
	}
	if next, err := h.srv.SendAddressLossMail(ctx, uuid.MustParse(holder.ws)); err != nil || next != nil {
		t.Fatalf("first loss mail: %v %v", next, err)
	}
	got := lossMails()
	if len(got) != 1 || !strings.Contains(got[0], exact) || !strings.Contains(got[0], "sales@"+domain) || !strings.Contains(got[0], "/channels/"+all) {
		t.Fatalf("one mail listing both addresses: %q", got)
	}
	byDNS("support@" + domain)
	if next, err := h.srv.SendAddressLossMail(ctx, uuid.MustParse(holder.ws)); err != nil || next == nil {
		t.Fatalf("a second loss within a day: %v %v", next, err)
	}
	if len(lossMails()) != 1 {
		t.Fatal("a second loss mail within a day")
	}
	h.clock.Advance(25 * time.Hour)
	if _, err := h.srv.SendAddressLossMail(ctx, uuid.MustParse(holder.ws)); err != nil {
		t.Fatal(err)
	}
	if got := lossMails(); len(got) != 2 || !strings.Contains(got[1], "support@"+domain) || strings.Contains(got[1], exact) {
		t.Fatalf("the next day's mail: %q", got)
	}
}

func TestDNSProofsAreRateLimited(t *testing.T) {
	t.Parallel()
	h := newHarnessWith(t, func(d *api.Deps) { d.Status.Resolver = &ssoDNS{txt: map[string][]string{}} })
	h.unproven = true
	et := newEmailTeam(t, h, false)
	for i := range 30 {
		if r := et.owner.do("POST", "/v1/channels/"+et.channel+"/verify", nil); r.status != http.StatusOK {
			t.Fatalf("check %d: %d %s", i+1, r.status, r.raw)
		}
	}
	et.owner.expectProblem(http.StatusTooManyRequests, "rate_limited", "POST", "/v1/channels/"+et.channel+"/verify", nil)
}

func TestAddressCodeCapsOutliveChannels(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.unproven = true
	tm := newTeam(t, h)
	target := unique("victim") + "@example.org"
	send := func(c *client, inbox, address string) response {
		id := c.expect(http.StatusCreated, "POST", "/v1/inboxes/"+inbox+"/channels", map[string]any{
			"kind": "email", "name": "x", "email": map[string]any{"address": address},
		}).str("id")
		r := c.do("POST", "/v1/channels/"+id+"/verification-code", nil)
		c.expect(http.StatusNoContent, "DELETE", "/v1/channels/"+id, nil)
		return r
	}
	local, domain, _ := strings.Cut(target, "@")
	for i := range 5 {
		addr := target
		if i > 0 {
			addr = strings.ToUpper(local) + "+" + strconv.Itoa(i) + "@" + domain
		}
		if r := send(tm.owner, tm.inbox, addr); r.status != http.StatusAccepted {
			t.Fatalf("code %d to a re-created channel: %d %s", i+1, r.status, r.raw)
		}
	}
	other := newTeam(t, h)
	if r := send(other.owner, other.inbox, local+"+other@"+domain); r.status != http.StatusTooManyRequests {
		t.Fatalf("a sixth code to one address from another workspace: %d %s", r.status, r.raw)
	}
	for i := 5; i < 20; i++ {
		if r := send(tm.owner, tm.inbox, unique("x")+"@example.org"); r.status != http.StatusAccepted {
			t.Fatalf("code %d of the workspace: %d %s", i+1, r.status, r.raw)
		}
	}
	if r := send(tm.owner, tm.inbox, unique("x")+"@example.org"); r.status != http.StatusTooManyRequests {
		t.Fatalf("a 21st code in a day from one workspace: %d %s", r.status, r.raw)
	}
}

func TestAddressLossMailIsQueuedAndSent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	schema := strings.ReplaceAll(unique("river"), "-", "_")
	dns := &ssoDNS{txt: map[string][]string{}}
	var queued *store.Store
	h := newHarnessWith(t, func(d *api.Deps) {
		d.Status.Resolver = dns
		if _, err := d.Store.Pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = d.Store.Pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
		m, err := rivermigrate.New(riverpgxv5.New(d.Store.Pool), &rivermigrate.Config{Schema: schema, Logger: slog.New(slog.DiscardHandler)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
			t.Fatal(err)
		}
		dsn := os.Getenv("YUVA_TEST_DATABASE_URL")
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		if queued, err = store.Open(ctx, dsn+sep+"search_path="+schema+",public"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(queued.Close)
		d.Store = queued
	})
	workers := river.NewWorkers()
	h.srv.AddWorkers(workers)
	client, err := river.NewClient(riverpgxv5.New(queued.Pool), &river.Config{
		Logger: slog.New(slog.DiscardHandler), Queues: jobs.Queues(2, 0, 0), Workers: workers,
		FetchCooldown: 10 * time.Millisecond, FetchPollInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = client.StopAndCancel(stopCtx)
	})

	holder := newTeam(t, h)
	domain := unique("corp") + ".example"
	lost := "help@" + domain
	holder.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+holder.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Help", "email": map[string]any{"address": lost},
	})
	mover := newTeam(t, h)
	h.unproven = true
	id := mover.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+mover.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Help", "email": map[string]any{"address": lost},
	}).str("id")
	h.unproven = false
	v := verificationOf(t, mover.owner, id)
	dns.set(v["txt_name"].(string), v["txt_value"].(string))
	if r := mover.owner.expect(http.StatusOK, "POST", "/v1/channels/"+id+"/verify", nil); !strings.Contains(string(r.raw), `"state":"verified"`) {
		t.Fatalf("dns proof: %s", r.raw)
	}

	owner := ownerEmail(t, h, holder.ws)
	deadline := time.Now().Add(15 * time.Second)
	for {
		var got []string
		for _, m := range h.mail.to(owner) {
			if strings.Contains(m.Subject, "no longer reaches") {
				got = append(got, m.Text)
			}
		}
		if len(got) == 1 && strings.Contains(got[0], lost) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the previous holder was not mailed: %q", got)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

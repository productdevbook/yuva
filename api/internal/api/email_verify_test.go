package api_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/productdevbook/yuva/api/internal/api"
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

// legacyChannel stores address on a channel as proven by the upgrade rule, the way a row saved
// before proofs existed looks, bypassing the checks a save does today.
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
		"UPDATE email_channels SET address = $2, verified_at = now(), verified_by = 'received' WHERE channel_id = $1", id, address); err != nil {
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

func TestAddressProofUpgradeRule(t *testing.T) {
	t.Parallel()
	fwd := unique("fwd") + ".example"
	h := newHarnessWith(t, func(d *api.Deps) { d.Ingress.ForwardingDomain = fwd })
	h.unproven = true
	ctx := context.Background()
	live := newEmailTeam(t, h, false)
	imported := newEmailTeam(t, h, false)
	silent := newEmailTeam(t, h, false)
	for _, et := range []emailTeam{live, imported} {
		to := et.owner.expect(http.StatusOK, "GET", "/v1/channels/"+et.channel, nil).str("forwarding_address")
		r := h.ingest(to, buildMail(mailOpts{from: unique("c") + "@example.org", to: et.address, subject: "Hi", messageID: newMessageID(), body: "Hello"}), nil)
		if r.status != http.StatusAccepted {
			t.Fatalf("ingest: %d %v", r.status, r.body)
		}
	}
	if _, err := h.st.Pool.Exec(ctx, "UPDATE message_emails SET imported_at = now() WHERE workspace_id = $1", imported.ws); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob("../store/migrations/*_email_address_proof.sql")
	if err != nil || len(files) != 1 {
		t.Fatalf("migration: %v %v", files, err)
	}
	src, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(src), "UPDATE email_channels e SET verified_at")
	end := strings.Index(string(src)[start:], ";")
	upgrade := string(src)[start:start+end] + " AND e.workspace_id = ANY($1::uuid[])"
	if _, err := h.st.Pool.Exec(ctx, upgrade, []string{live.ws, imported.ws, silent.ws}); err != nil {
		t.Fatal(err)
	}
	if v := verificationOf(t, live.owner, live.channel); v["state"] != "verified" || v["method"] != "received" {
		t.Fatalf("a channel that received mail: %v", v)
	}
	for _, et := range []emailTeam{imported, silent} {
		if v := verificationOf(t, et.owner, et.channel); v["state"] != "unverified" {
			t.Fatalf("a channel without live mail: %v", v)
		}
	}
	r := h.ingest(live.address, buildMail(mailOpts{from: unique("c") + "@example.org", to: live.address, subject: "Hi", messageID: newMessageID(), body: "Hello"}), nil)
	if r.status != http.StatusAccepted {
		t.Fatalf("mail to an upgraded address: %d %v", r.status, r.body)
	}
}

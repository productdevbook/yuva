package api_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/productdevbook/yuva/api/internal/api"
)

func pngOf(size int) []byte {
	head := []byte("\x89PNG\r\n\x1a\n")
	return append(head, bytes.Repeat([]byte{0}, size-len(head))...)
}

func limitsOf(t *testing.T, body map[string]any) (any, any, float64, []any) {
	t.Helper()
	a, ok := body["attachments"].(map[string]any)
	if !ok {
		t.Fatalf("no attachments in %v", body)
	}
	eff, _ := a["effective_types"].([]any)
	return a["max_bytes"], a["types"], a["effective_max_bytes"].(float64), eff
}

func clientLimits(t *testing.T, body map[string]any) (float64, []any) {
	t.Helper()
	a, ok := body["attachments"].(map[string]any)
	if !ok {
		t.Fatalf("no attachments in %v", body)
	}
	return a["max_bytes"].(float64), a["types"].([]any)
}

func expectUpload(t *testing.T, r response, status int, code string) {
	t.Helper()
	if r.status != status || (code != "" && r.str("code") != code) {
		t.Fatalf("want %d %s, got %d %s", status, code, r.status, r.raw)
	}
}

func TestChannelAttachmentLimits(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	other := newChatTeam(t, h, "live", true)
	chPath := "/v1/channels/" + ct.channel

	r := ct.owner.expect(http.StatusOK, "GET", chPath, nil)
	if mb, ty, eff, effTypes := limitsOf(t, r.body); mb != nil || ty != nil || eff != testAttachmentMaxBytes ||
		!reflect.DeepEqual(effTypes, []any{"text/plain", "image/*"}) {
		t.Fatalf("default limits: %s", r.raw)
	}

	for field, attachments := range map[string]map[string]any{
		"attachments.max_bytes": {"max_bytes": testAttachmentMaxBytes + 1},
		"attachments.types":     {"types": []string{"image/png", "application/pdf"}},
	} {
		w := ct.owner.do("PATCH", chPath, map[string]any{"attachments": attachments})
		if w.status != http.StatusBadRequest || w.str("code") != "validation_failed" || !strings.Contains(w.str("detail"), field) {
			t.Fatalf("widening %s: %d %s", field, w.status, w.raw)
		}
	}
	for _, bad := range [][]string{{"*/*"}, {"image"}, {"image/p*"}, {}} {
		ct.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", chPath, map[string]any{"attachments": map[string]any{"types": bad}})
	}
	ct.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", chPath, map[string]any{"attachments": map[string]any{"max_bytes": 0}})
	ct.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", chPath, map[string]any{"attachments": map[string]any{"max_bytes": 10}})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", chPath, map[string]any{"attachments": map[string]any{"max_bytes": 10}})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", chPath, nil)

	r = ct.owner.expect(http.StatusOK, "PATCH", chPath, map[string]any{"attachments": map[string]any{"max_bytes": 100, "types": []string{"image/png", " IMAGE/PNG"}}})
	if mb, ty, eff, effTypes := limitsOf(t, r.body); mb != float64(100) || !reflect.DeepEqual(ty, []any{"image/png"}) || eff != 100 ||
		!reflect.DeepEqual(effTypes, []any{"image/png"}) {
		t.Fatalf("narrowed: %s", r.raw)
	}
	r = ct.owner.expect(http.StatusOK, "PATCH", chPath, map[string]any{"attachments": map[string]any{"types": []string{"image/*"}}})
	if mb, ty, _, effTypes := limitsOf(t, r.body); mb != float64(100) || !reflect.DeepEqual(ty, []any{"image/*"}) || !reflect.DeepEqual(effTypes, []any{"image/*"}) {
		t.Fatalf("types alone: %s", r.raw)
	}
	ct.owner.expect(http.StatusOK, "PATCH", chPath, map[string]any{"attachments": map[string]any{"types": []string{"image/png"}}})
	r = ct.owner.expect(http.StatusOK, "PATCH", chPath, map[string]any{"name": "Renamed"})
	if mb, _, _, _ := limitsOf(t, r.body); mb != float64(100) {
		t.Fatalf("a patch without attachments changed them: %s", r.raw)
	}

	c := h.client()
	c.origin = ct.origin
	pub := c.expect(http.StatusOK, "GET", "/client/v1/channels/"+ct.key, nil)
	if mb, ty := clientLimits(t, pub.body); mb != 100 || !reflect.DeepEqual(ty, []any{"image/png"}) {
		t.Fatalf("client limits: %s", pub.raw)
	}
	oc := h.client()
	oc.origin = other.origin
	pub = oc.expect(http.StatusOK, "GET", "/client/v1/channels/"+other.key, nil)
	if mb, ty := clientLimits(t, pub.body); mb != testAttachmentMaxBytes || !reflect.DeepEqual(ty, []any{"text/plain", "image/*"}) {
		t.Fatalf("another workspace's channel took the limits: %s", pub.raw)
	}

	cs := ct.session(h, map[string]any{})
	if mb, _ := clientLimits(t, cs.body["inbox"].(map[string]any)); mb != 100 {
		t.Fatalf("session inbox limits: %v", cs.body["inbox"])
	}
	fields := map[string]string{"body": "files", "client_id": unique("c")}
	big := cs.upload("/client/v1/conversations", fields, []file{{"big.png", "image/png", pngOf(101)}})
	expectUpload(t, big, http.StatusRequestEntityTooLarge, "attachment_too_large")
	refused := cs.upload("/client/v1/conversations", fields, []file{{"a.txt", "text/plain", []byte("hello")}})
	expectUpload(t, refused, http.StatusUnsupportedMediaType, "attachment_type_not_allowed")
	if !strings.Contains(big.str("detail"), "this channel allows") || !strings.Contains(refused.str("detail"), "this channel does not allow") {
		t.Fatalf("refusals by the channel's limits name the server: %s %s", big.raw, refused.raw)
	}
	started := cs.upload("/client/v1/conversations", fields, []file{{"ok.png", "image/png", pngOf(100)}})
	expectUpload(t, started, http.StatusCreated, "")
	conv := started.body["conversation"].(map[string]any)["id"].(string)

	msgPath := "/client/v1/conversations/" + conv + "/messages"
	expectUpload(t, cs.upload(msgPath, map[string]string{"body": "more"}, []file{{"big.png", "image/png", pngOf(101)}}), http.StatusRequestEntityTooLarge, "attachment_too_large")
	expectUpload(t, cs.upload(msgPath, map[string]string{"body": "more"}, []file{{"a.txt", "text/plain", []byte("hello")}}), http.StatusUnsupportedMediaType, "attachment_type_not_allowed")
	expectUpload(t, cs.upload(msgPath, map[string]string{"body": "more"}, []file{{"ok.png", "image/png", pngOf(50)}}), http.StatusCreated, "")

	fb := map[string]string{"category": "bug", "body": "it broke"}
	expectUpload(t, cs.upload("/client/v1/feedback", fb, []file{{"shot.png", "image/png", pngOf(101)}}), http.StatusRequestEntityTooLarge, "attachment_too_large")
	expectUpload(t, cs.upload("/client/v1/feedback", fb, []file{{"a.txt", "text/plain", []byte("hello")}}), http.StatusUnsupportedMediaType, "attachment_type_not_allowed")
	expectUpload(t, cs.upload("/client/v1/feedback", fb, []file{{"shot.png", "image/png", pngOf(100)}}), http.StatusCreated, "")

	memberPath := "/v1/conversations/" + conv + "/messages"
	reply := map[string]string{"kind": "message", "direction": "out", "body": "files"}
	expectUpload(t, ct.owner.upload(memberPath, reply, []file{{"big.png", "image/png", pngOf(101)}}), http.StatusRequestEntityTooLarge, "attachment_too_large")
	expectUpload(t, ct.owner.upload(memberPath, reply, []file{{"a.txt", "text/plain", []byte("hello")}}), http.StatusUnsupportedMediaType, "attachment_type_not_allowed")
	expectUpload(t, ct.owner.upload(memberPath, reply, []file{{"ok.png", "image/png", pngOf(100)}}), http.StatusCreated, "")

	ocs := other.session(h, map[string]any{})
	expectUpload(t, ocs.upload("/client/v1/conversations", map[string]string{"body": "files"}, []file{{"a.txt", "text/plain", bytes.Repeat([]byte("x"), 500)}}), http.StatusCreated, "")

	r = ct.owner.expect(http.StatusOK, "PATCH", chPath, map[string]any{"attachments": map[string]any{"max_bytes": nil}})
	if mb, ty, eff, _ := limitsOf(t, r.body); mb != nil || !reflect.DeepEqual(ty, []any{"image/png"}) || eff != testAttachmentMaxBytes {
		t.Fatalf("cleared size: %s", r.raw)
	}
	r = ct.owner.expect(http.StatusOK, "PATCH", chPath, map[string]any{"attachments": map[string]any{"types": nil}})
	if _, ty, _, effTypes := limitsOf(t, r.body); ty != nil || !reflect.DeepEqual(effTypes, []any{"text/plain", "image/*"}) {
		t.Fatalf("cleared types: %s", r.raw)
	}

	created := ct.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+ct.chatInbox+"/channels", map[string]any{
		"kind": "api", "name": "Server", "attachments": map[string]any{"max_bytes": 10, "types": []string{"text/plain"}},
	})
	if mb, _, eff, _ := limitsOf(t, created.body); mb != float64(10) || eff != 10 {
		t.Fatalf("created with limits: %s", created.raw)
	}
	ct.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/inboxes/"+ct.chatInbox+"/channels", map[string]any{
		"kind": "api", "name": "Wide", "attachments": map[string]any{"types": []string{"application/zip"}},
	})
}

// A server limit lowered after a channel was narrowed wins: the channel's stored values are
// written straight into the table as an older, wider configuration would have left them.
func TestChannelAttachmentLimitsFollowLoweredServer(t *testing.T) {
	t.Parallel()
	h := newHarnessWith(t, func(d *api.Deps) {
		d.Attachments = api.AttachmentSettings{MaxBytes: 200, Types: []string{"text/plain", "image/png", "image/gif"}}
	})
	ct := newChatTeam(t, h, "live", true)
	if _, err := h.st.Pool.Exec(context.Background(),
		`UPDATE channels SET attachment_max_bytes = 5000, attachment_types = ARRAY['image/*', 'application/pdf'] WHERE workspace_id = $1 AND id = $2`,
		ct.ws, ct.channel); err != nil {
		t.Fatal(err)
	}
	r := ct.owner.expect(http.StatusOK, "GET", "/v1/channels/"+ct.channel, nil)
	if mb, _, eff, effTypes := limitsOf(t, r.body); mb != float64(5000) || eff != 200 || !reflect.DeepEqual(effTypes, []any{"image/png", "image/gif"}) {
		t.Fatalf("effective after lowering: %s", r.raw)
	}
	c := h.client()
	c.origin = ct.origin
	pub := c.expect(http.StatusOK, "GET", "/client/v1/channels/"+ct.key, nil)
	if mb, ty := clientLimits(t, pub.body); mb != 200 || !reflect.DeepEqual(ty, []any{"image/png", "image/gif"}) {
		t.Fatalf("client after lowering: %s", pub.raw)
	}
	cs := ct.session(h, map[string]any{})
	fields := map[string]string{"body": "files"}
	expectUpload(t, cs.upload("/client/v1/conversations", fields, []file{{"big.png", "image/png", pngOf(201)}}), http.StatusRequestEntityTooLarge, "attachment_too_large")
	expectUpload(t, cs.upload("/client/v1/conversations", fields, []file{{"a.txt", "text/plain", []byte("hello")}}), http.StatusUnsupportedMediaType, "attachment_type_not_allowed")
	expectUpload(t, cs.upload("/client/v1/conversations", fields, []file{{"ok.png", "image/png", pngOf(200)}}), http.StatusCreated, "")

	chPath := "/v1/channels/" + ct.channel
	r = ct.owner.expect(http.StatusOK, "PATCH", chPath, map[string]any{"attachments": map[string]any{"types": []string{"image/*"}, "max_bytes": nil}})
	if _, ty, eff, effTypes := limitsOf(t, r.body); !reflect.DeepEqual(ty, []any{"image/*"}) || eff != 200 || !reflect.DeepEqual(effTypes, []any{"image/png", "image/gif"}) {
		t.Fatalf("image/* over concrete server types: %s", r.raw)
	}
	for _, wide := range []string{"video/*", "image/jpeg"} {
		w := ct.owner.do("PATCH", chPath, map[string]any{"attachments": map[string]any{"types": []string{"image/png", wide}}})
		if w.status != http.StatusBadRequest || !strings.Contains(w.str("detail"), "attachments.types: "+wide) {
			t.Fatalf("%s allows nothing the server allows: %d %s", wide, w.status, w.raw)
		}
	}
}

func TestInboundMailOverChannelLimit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	other := newEmailTeam(t, h, false)
	et.owner.expect(http.StatusOK, "PATCH", "/v1/channels/"+et.channel, map[string]any{"attachments": map[string]any{"max_bytes": 5}})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/channels/"+et.channel, map[string]any{"attachments": map[string]any{"max_bytes": 1}})

	for _, tc := range []struct {
		name  string
		et    emailTeam
		files []string
	}{
		{"narrowed", et, nil},
		{"other workspace", other, []string{"log.txt"}},
	} {
		raw := []byte(fmt.Sprintf(attachmentMail, tc.et.address, newMessageID()))
		r := h.ingest(tc.et.address, raw, nil)
		if r.status != http.StatusAccepted || r.str("status") != "stored" {
			t.Fatalf("%s ingest: %d %v", tc.name, r.status, r.body)
		}
		m := lastOf(messages(tc.et.owner, r.str("conversation_id")), "message")
		if m["body"] != "The export is empty." {
			t.Fatalf("%s message: %v", tc.name, m)
		}
		var names []string
		for _, a := range m["attachments"].([]any) {
			names = append(names, a.(map[string]any)["filename"].(string))
		}
		if !reflect.DeepEqual(names, tc.files) {
			t.Fatalf("%s attachments %v, want %v", tc.name, names, tc.files)
		}
		dl := tc.et.owner.do("GET", "/v1/messages/"+r.str("message_id")+"/raw", nil)
		if dl.status != http.StatusOK || !bytes.Equal(dl.raw, raw) {
			t.Fatalf("%s raw kept: %d", tc.name, dl.status)
		}
	}

	et.owner.expect(http.StatusOK, "PATCH", "/v1/channels/"+et.channel, map[string]any{"attachments": map[string]any{"max_bytes": nil, "types": []string{"image/*"}}})
	r := h.ingest(et.address, []byte(fmt.Sprintf(attachmentMail, et.address, newMessageID())), nil)
	m := lastOf(messages(et.owner, r.str("conversation_id")), "message")
	if r.str("status") != "stored" || len(m["attachments"].([]any)) != 0 {
		t.Fatalf("type-narrowed ingest: %v %v", r.body, m)
	}
}

func TestChannelAttachmentPatchesDoNotRevertEachOther(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	chPath := "/v1/channels/" + ct.channel
	for i := range 10 {
		size := 10 + i
		types := []string{"text/plain"}
		if i%2 == 1 {
			types = []string{"image/*"}
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			ct.owner.do("PATCH", chPath, map[string]any{"attachments": map[string]any{"max_bytes": size}})
		}()
		go func() {
			defer wg.Done()
			ct.owner.do("PATCH", chPath, map[string]any{"attachments": map[string]any{"types": types}})
		}()
		wg.Wait()
		r := ct.owner.expect(http.StatusOK, "GET", chPath, nil)
		if mb, ty, _, _ := limitsOf(t, r.body); mb != float64(size) || !reflect.DeepEqual(ty, []any{types[0]}) {
			t.Fatalf("round %d: one patch reverted the other: %s", i, r.raw)
		}
	}
}

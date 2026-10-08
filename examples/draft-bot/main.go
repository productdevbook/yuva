package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/sdk/go/client"
	"github.com/productdevbook/yuva/sdk/go/webhook"
)

type event struct {
	Type string `json:"type"`
	Data struct {
		Message struct {
			ID             uuid.UUID `json:"id"`
			ConversationID uuid.UUID `json:"conversation_id"`
			Kind           string    `json:"kind"`
			Direction      string    `json:"direction"`
			Body           string    `json:"body"`
		} `json:"message"`
	} `json:"data"`
}

type bot struct {
	api    *client.ClientWithResponses
	secret string
	reply  string
	send   bool
}

func main() {
	server := env("YUVA_SERVER", "http://localhost:8080")
	key := os.Getenv("YUVA_API_KEY")
	secret := os.Getenv("YUVA_WEBHOOK_SECRET")
	if key == "" || secret == "" {
		log.Fatal("set YUVA_API_KEY and YUVA_WEBHOOK_SECRET")
	}
	api, err := client.NewClientWithResponses(server, client.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+key)
		return nil
	}))
	if err != nil {
		log.Fatal(err)
	}
	b := &bot{
		api:    api,
		secret: secret,
		reply:  env("BOT_REPLY", "Thanks for your message. We have received it and will get back to you soon."),
		send:   os.Getenv("BOT_SEND") == "true",
	}
	addr := env("LISTEN_ADDR", "127.0.0.1:3000")
	log.Printf("draft bot listening on %s/webhook", addr)
	log.Fatal(http.ListenAndServe(addr, http.HandlerFunc(b.handle)))
}

func (b *bot) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/webhook" {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	if err := webhook.Verify(b.secret, r.Header, body, 5*time.Minute); err != nil {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	var e event
	if err := json.Unmarshal(body, &e); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	m := e.Data.Message
	if e.Type != "message.created" || m.Direction != "in" || m.Kind != "message" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := b.answer(r.Context(), m.ConversationID, m.ID); err != nil {
		log.Print(err)
		http.Error(w, "failed", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// answer writes a draft reply. The Idempotency-Key is derived from the incoming message, so a
// webhook that Yuva retries returns the first draft instead of writing a second one.
func (b *bot) answer(ctx context.Context, conversation, incoming uuid.UUID) error {
	key := "draft-bot:" + incoming.String()
	draft, out := true, client.Out
	res, err := b.api.CreateMessageWithResponse(ctx, conversation, &client.CreateMessageParams{IdempotencyKey: &key},
		client.CreateMessageJSONRequestBody{Kind: client.MessageCreateKindMessage, Direction: &out, Draft: &draft, Body: &b.reply})
	if err != nil {
		return err
	}
	msg := res.JSON201
	if msg == nil {
		msg = res.JSON200
	}
	if msg == nil {
		return fmt.Errorf("create draft: %s: %s", res.Status(), res.Body)
	}
	log.Printf("draft %s on conversation %s (replayed: %s)", msg.Id, conversation, res.HTTPResponse.Header.Get("Idempotent-Replayed"))
	if !b.send {
		return nil
	}
	sendKey := "send:" + msg.Id.String()
	sent, err := b.api.SendMessageWithResponse(ctx, msg.Id, &client.SendMessageParams{IdempotencyKey: &sendKey})
	if err != nil {
		return err
	}
	if sent.JSON200 == nil {
		return fmt.Errorf("send draft: %s: %s", sent.Status(), sent.Body)
	}
	log.Printf("sent %s", msg.Id)
	return nil
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

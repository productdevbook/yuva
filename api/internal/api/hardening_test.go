package api_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/productdevbook/yuva/api/internal/api"
)

func TestShareUnlockLimitsAndHash(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	id, _ := createProperty(tm.owner, map[string]any{"name": "Site", "domains": []string{"example.com"}})
	share := tm.owner.expect(http.StatusCreated, "POST", "/v1/analytics/properties/"+id+"/shares",
		map[string]any{"name": "Locked", "views": []string{"overview"}, "password": "correct horse"})
	token, shareID := share.str("token"), share.body["share"].(map[string]any)["id"].(string)
	var stored string
	if err := h.st.Pool.QueryRow(t.Context(), "SELECT password_hash FROM analytics_shares WHERE id = $1", shareID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, "$argon2id$v=19$m=7168,t=5,p=1$") {
		t.Fatalf("stored hash %q", stored)
	}
	unlock := func(c *client, pw string) response {
		return c.do("POST", "/client/v1/analytics-shares/"+token+"/access", map[string]any{"password": pw})
	}
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	old := fmt.Sprintf("$argon2id$v=19$m=19456,t=2,p=1$%s$%s", base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(argon2.IDKey([]byte("old horse"), salt, 2, 19456, 1, 32)))
	if _, err := h.st.Pool.Exec(t.Context(), "UPDATE analytics_shares SET password_hash = $2 WHERE id = $1", shareID, old); err != nil {
		t.Fatal(err)
	}
	if r := unlock(h.client(), "old horse"); r.status != http.StatusOK {
		t.Fatalf("a password hashed with the earlier settings: %d %s", r.status, r.raw)
	}
	tries := 0
	for n := 0; tries < 69; n++ {
		c := h.client()
		c.ip = "10.77.1." + strconv.Itoa(n+1)
		for range 9 {
			if tries == 69 {
				break
			}
			pw, want := "guess", http.StatusUnauthorized
			if tries%7 == 3 && tries < 63 {
				pw, want = "old horse", http.StatusOK
			}
			if r := unlock(c, pw); r.status != want {
				t.Fatalf("try %d: %d %s", tries+1, r.status, r.raw)
			}
			tries++
		}
	}
	neighbour := h.client()
	neighbour.ip = "10.77.1.200"
	expectLimited(t, unlock(neighbour, "old horse"))
	if r := unlock(h.client(), "old horse"); r.status != http.StatusOK {
		t.Fatalf("another network after one network's 60 wrong passwords: %d %s", r.status, r.raw)
	}
}

func TestRateLimiterFallsBackLocally(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", false)
	h.srv.BreakRateLimits(errors.New("database unavailable"))
	if got := h.srv.TakeRateAs(true, "member-key", 5, 2, time.Minute); got != 5 {
		t.Fatalf("member traffic got %d of 5 while the database failed", got)
	}
	if got := h.srv.TakeRateAs(false, "anon-key", 5, 2, time.Minute); got != 2 {
		t.Fatalf("anonymous traffic got %d of 5 with a limit of 2 while the database failed", got)
	}
	c := h.client()
	c.origin = ct.origin
	for i := range 30 {
		if r := c.do("POST", "/client/v1/session", map[string]any{"channel_key": ct.key, "identity_token": ct.token(h, map[string]any{"sub": unique("user")})}); r.status != http.StatusCreated {
			t.Fatalf("session %d: %d %s", i+1, r.status, r.raw)
		}
	}
	expectLimited(t, c.do("POST", "/client/v1/session", map[string]any{"channel_key": ct.key, "identity_token": ct.token(h, map[string]any{"sub": unique("user")})}))
	for range 40 {
		ct.owner.expect(http.StatusOK, "GET", "/v1/conversations", nil)
	}
	h.clock.Advance(time.Minute)
	if r := c.do("POST", "/client/v1/session", map[string]any{"channel_key": ct.key, "identity_token": ct.token(h, map[string]any{"sub": unique("user")})}); r.status != http.StatusCreated {
		t.Fatalf("a new local window: %d %s", r.status, r.raw)
	}
}

func TestAnalyticsUnknownKeysKeepRealOnes(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	id, key := createProperty(tm.owner, map[string]any{"name": "Site", "domains": []string{"example.com"}})
	ctx := context.Background()
	if ok, err := h.srv.AnalyticsKeyKnown(ctx, key); err != nil || !ok {
		t.Fatalf("real key: %v %v", ok, err)
	}
	if _, err := h.st.Pool.Exec(ctx, "UPDATE analytics_properties SET public_key = $2 WHERE id = $1", id, api.NewAnalyticsKey()); err != nil {
		t.Fatal(err)
	}
	for i := range 10_001 {
		if ok, err := h.srv.AnalyticsKeyKnown(ctx, api.NewAnalyticsKey()); err != nil || ok {
			t.Fatalf("made-up key %d: %v %v", i, ok, err)
		}
	}
	if ok, _ := h.srv.AnalyticsKeyKnown(ctx, key); !ok {
		t.Fatal("10,001 made-up keys pushed the real property out of the cache")
	}
	for _, bad := range []string{"yuva_ak_short", "nope", key + "x", strings.Repeat("a", 32)} {
		if ok, err := h.srv.AnalyticsKeyKnown(ctx, bad); err != nil || ok {
			t.Fatalf("%q: %v %v", bad, ok, err)
		}
	}
}

func TestPublicCacheKeepsReadEntries(t *testing.T) {
	c := api.NewPublicCache(1000)
	now := time.Now()
	c.Put("page-a|home", now)
	for i := range 5000 {
		now = now.Add(time.Millisecond)
		if i%100 == 0 && !c.Has("page-a|home", now) {
			t.Fatalf("the read entry went after %d others", i)
		}
		c.Put("page-a|search|"+strconv.Itoa(i), now)
		c.Put("page-"+strconv.Itoa(i%20)+"|x|"+strconv.Itoa(i), now)
	}
	if !c.Has("page-a|home", now) {
		t.Fatal("a flood of one page's keys pushed out its read entry")
	}
	if n := c.Len(); n > 1000 {
		t.Fatalf("%d entries, cap 1000", n)
	}
	n := 0
	for i := range 5000 {
		if c.Has("page-a|search|"+strconv.Itoa(i), now) {
			n++
		}
	}
	if n > api.PublicCachePerPage {
		t.Fatalf("one page holds %d entries, cap %d", n, api.PublicCachePerPage)
	}
}

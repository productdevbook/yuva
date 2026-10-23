package helpdesk_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk"
	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk/helpdesktest"
)

func TestZendeskGuideAgainstFixtures(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	ctx := context.Background()
	verifier, _ := helpdesk.PKCE()
	app := helpdesk.App{ClientID: "zd-client", ClientSecret: "zd-secret"}
	tokens, err := helpdesk.ZendeskExchange(ctx, cfg, helpdesktest.ZendeskSubdomain, app, helpdesktest.ZendeskCode, "https://yuva.example.com/cb", verifier)
	if err != nil {
		t.Fatal(err)
	}
	var src helpdesk.ArticleSource = helpdesk.NewZendesk(cfg, helpdesktest.ZendeskSubdomain, func() string { return tokens.Access }, &helpdesk.Budget{})

	loc, err := src.HelpLocales(ctx)
	if err != nil || !slices.Equal(loc.All, []string{"en-us", "tr"}) || loc.Default != "en-us" {
		t.Fatalf("locales: %+v %v", loc, err)
	}
	cols, err := src.HelpCollections(ctx, "en-us")
	if err != nil || len(cols) != 5 {
		t.Fatalf("collections: %+v %v", cols, err)
	}
	flat := helpdesk.FlattenHelpCollections(cols)
	byID := map[string]helpdesk.HelpCollection{}
	for _, c := range flat {
		byID[c.ID] = c
	}
	if byID["c100"].Name != "Getting started" || byID["s200"].ParentID != "c100" || byID["s201"].ParentID != "c100" || byID["s201"].Name != "Accounts / Passwords" || byID["s202"].ParentID != "c101" {
		t.Errorf("flattened collections: %+v", flat)
	}
	tr, err := src.HelpCollections(ctx, "tr")
	if err != nil || len(tr) != 2 || tr[0].Name != "Başlarken" || tr[1].Name != "Hesaplar" {
		t.Errorf("turkish collections: %+v %v", tr, err)
	}

	var en []helpdesk.HelpArticle
	c := helpdesk.ArticleCursor{}
	for {
		page, err := src.HelpArticles(ctx, "en-us", c)
		if err != nil {
			t.Fatal(err)
		}
		en = append(en, page.Articles...)
		c = page.Next
		if page.End {
			break
		}
	}
	if len(en) != 4 || en[0].ID != "5001" || en[0].CollectionID != "s201" || !slices.Equal(en[0].Keywords, []string{"password", "login"}) || !en[2].Draft {
		t.Fatalf("english articles: %+v", en)
	}
	trPage, err := src.HelpArticles(ctx, "tr", helpdesk.ArticleCursor{})
	if err != nil || len(trPage.Articles) != 1 || trPage.Articles[0].Title != "Parolanızı sıfırlayın" {
		t.Errorf("an article the account only shows in English is not a Turkish translation: %+v %v", trPage.Articles, err)
	}

	img, err := src.HelpImage(ctx, "https://acme.zendesk.com/hc/article_attachments/360001/reset-form.png")
	if err != nil || len(img) == 0 {
		t.Errorf("image from the account: %v", err)
	}
	if _, err := src.HelpImage(ctx, "https://files.example.org/screens/profile.png"); err != nil {
		t.Errorf("image from another host: %v", err)
	}
	var status *helpdesk.StatusError
	if _, err := src.HelpImage(ctx, "https://acme.zendesk.com/hc/article_attachments/missing.png"); !errors.As(err, &status) || status.Status != http.StatusNotFound {
		t.Errorf("missing image: %v", err)
	}
	if _, err := src.HelpImage(ctx, "http://acme.zendesk.com/a.png"); err == nil {
		t.Error("a plain http image was fetched")
	}

	for href, want := range map[string][2]string{
		"/hc/en-us/articles/5002-Change-your-email":                  {"5002", "en-us"},
		"https://acme.zendesk.com/hc/articles/5001":                  {"5001", ""},
		"https://acme.zendesk.com/hc/tr/articles/5002?x=1#top":       {"5002", "tr"},
		"https://other.zendesk.com/hc/en-us/articles/5002-Elsewhere": {"", ""},
		"https://acme.zendesk.com/hc/en-us/sections/200":             {"", ""},
	} {
		key, locale, ok := src.HelpLink(href)
		if key != want[0] || locale != want[1] || ok != (want[0] != "") {
			t.Errorf("link %s: %q %q %v", href, key, locale, ok)
		}
	}

	hasToken := map[string]bool{}
	for _, r := range srv.Requests() {
		if r.Method != http.MethodGet && r.Path != "/oauth/tokens" {
			t.Errorf("%s %s%s was sent", r.Method, r.Host, r.Path)
		}
		if r.Host == helpdesktest.OutsideHost {
			hasToken[r.Host] = r.Auth != ""
		}
	}
	if hasToken[helpdesktest.OutsideHost] {
		t.Error("the token went to a host outside Zendesk")
	}
}

func TestHelpScoutDocsAgainstFixtures(t *testing.T) {
	srv, cfg, _ := fixtureConfig(t)
	ctx := context.Background()
	docs := helpdesk.NewHelpScoutDocs(cfg, helpdesktest.DocsKey, "", &helpdesk.Budget{})
	var src helpdesk.ArticleSource = docs
	if loc, err := src.HelpLocales(ctx); err != nil || len(loc.All) != 0 || loc.Default != "" {
		t.Fatalf("locales: %+v %v", loc, err)
	}
	cols, err := src.HelpCollections(ctx, "")
	if err != nil || len(cols) != 3 || cols[0].ID != "col-a" || cols[1].ID != "cat-a1" || cols[1].ParentID != "col-a" || cols[2].ID != "col-b" {
		t.Fatalf("collections: %+v %v", cols, err)
	}
	var all []helpdesk.HelpArticle
	c := helpdesk.ArticleCursor{}
	for pages := 0; ; pages++ {
		if pages > 10 {
			t.Fatal("the listing does not end")
		}
		page, err := src.HelpArticles(ctx, "", c)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, page.Articles...)
		c = page.Next
		if page.End {
			break
		}
	}
	if len(all) != 4 {
		t.Fatalf("articles: %+v", all)
	}
	if all[0].ID != "art-1" || all[0].LinkKey != "101" || all[0].CollectionID != "cat-a1" || all[0].Title != "Invite a teammate" || !slices.Equal(all[0].Keywords, []string{"team", "invite"}) {
		t.Errorf("first article: %+v", all[0])
	}
	if all[2].CollectionID != "col-a" || !all[2].Draft || all[3].CollectionID != "col-b" {
		t.Errorf("an article with no category sits in its collection: %+v %+v", all[2], all[3])
	}
	if _, err := src.HelpImage(ctx, "https://files.example.org/docs/invite.png"); err != nil {
		t.Errorf("image: %v", err)
	}
	if key, _, ok := src.HelpLink("https://acme.helpscoutdocs.com/article/102-remove-a-teammate"); !ok || key != "102" {
		t.Errorf("link: %q %v", key, ok)
	}
	for _, r := range srv.Requests() {
		if r.Method != http.MethodGet {
			t.Errorf("%s %s%s was sent", r.Method, r.Host, r.Path)
		}
		if r.Host == helpdesktest.OutsideHost && r.Auth != "" {
			t.Error("the Docs key went to an image host")
		}
	}

	bad := helpdesk.NewHelpScoutDocs(cfg, "wrong-key", "", &helpdesk.Budget{})
	var auth *helpdesk.AuthError
	if _, err := bad.HelpLocales(ctx); !errors.As(err, &auth) {
		t.Errorf("a wrong key: %v", err)
	}
	if !strings.Contains(auth.Error(), "401") {
		t.Errorf("auth error: %v", auth)
	}
}

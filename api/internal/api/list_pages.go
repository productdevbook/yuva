package api

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	maxListFormBytes = 32 << 10
	undoWindow       = 10 * time.Minute
)

type listWords struct {
	Lang, Gone, Error                                             string
	ConfirmTitle, ConfirmText, ConfirmButton, Confirmed, Already  string
	Expired                                                       string
	ManageTitle, SubscribedAs, NotSubscribed, UnsubscribeFrom     string
	Lists, Save, UnsubscribeAll, UnsubscribeAllText               string
	Unsubscribed, Saved, AllGone, Undo, Restored, OneClickDone    string
	ManageLink, TestMail                                          string
	StatusConfirmText, StatusOnly, StatusConfirmed                string
	StatusSubscribedAs, StatusNotSubscribed, StatusLegend         string
	StatusSave, StatusNoneChosen, StatusBack                      string
	SubscribeTitle, SubscribeConsent, EmailLabel, SubscribeButton string
	SentTitle, SentText, TooMany, FormExpired, PageFull           string
	InvalidEmail, ChooseOne                                       string
}

var listLanguages = map[string]listWords{
	"en": {
		Lang: "en", Gone: "This link is no longer valid.", Error: "Something went wrong. Try the link again later.",
		ConfirmTitle: "Confirm your subscription", ConfirmText: "Confirm that %s should receive “%l” from %i.",
		ConfirmButton: "Confirm subscription", Confirmed: "You are subscribed to “%l”. Thank you!",
		Already: "You are already subscribed to “%l”.", Expired: "This link has expired. Subscribe again to get a new one.",
		ManageTitle: "Your subscriptions", SubscribedAs: "You are subscribed to “%l” as %s.", NotSubscribed: "You are not subscribed to “%l”.",
		UnsubscribeFrom: "Unsubscribe from “%l”", Lists: "Lists from %i", Save: "Save choices",
		UnsubscribeAll: "Unsubscribe from all mail from %i", UnsubscribeAllText: "No more lists or newsletters from %i at this address.",
		Unsubscribed: "You are unsubscribed from “%l”.", Saved: "Your choices are saved.",
		AllGone: "You will get no more list mail from %i.", Undo: "Undo", Restored: "Your subscriptions are back.",
		ManageLink:        "Choose which e-mails you get",
		TestMail:          "This was a test of a broadcast. Its link changes nothing; in the real mail it unsubscribes the recipient.",
		StatusConfirmText: "Confirm that %s should get notices about incidents and maintenance of “%l”.", StatusOnly: "Only about: %c",
		StatusConfirmed:    "You will get notices from “%l”. You can unsubscribe in every mail.",
		StatusSubscribedAs: "You get notices from “%l” at %s.", StatusNotSubscribed: "You get no notices from “%l”.",
		StatusLegend: "Send me notices about", StatusSave: "Save choices",
		StatusNoneChosen: "Choose at least one component, or unsubscribe instead.", StatusBack: "Back to the status page",
		SubscribeTitle:   "Get notices from “%l”",
		SubscribeConsent: "Send me notices about incidents and maintenance of %l. I confirm by e-mail and can unsubscribe in every mail.",
		EmailLabel:       "E-mail address", SubscribeButton: "Subscribe",
		SentTitle: "Check your mail", SentText: "If the address can receive mail, a confirmation is on its way.",
		TooMany:     "Too many requests from here. Try again in a while.",
		FormExpired: "The form is out of date. Please send it again.", PageFull: "This page takes no more subscribers.",
		InvalidEmail: "Enter a valid e-mail address.", ChooseOne: "Choose at least one component.",
	},
	"tr": {
		Lang: "tr", Gone: "Bu bağlantı artık geçerli değil.", Error: "Bir sorun oluştu. Bağlantıyı daha sonra yeniden deneyin.",
		ConfirmTitle: "Aboneliğinizi onaylayın", ConfirmText: "%s adresinin %i tarafından gönderilen “%l” listesini almasını onaylayın.",
		ConfirmButton: "Aboneliği onayla", Confirmed: "“%l” listesine abone oldunuz. Teşekkürler!",
		Already: "“%l” listesine zaten abonesiniz.", Expired: "Bu bağlantının süresi doldu. Yeni bir bağlantı için yeniden abone olun.",
		ManageTitle: "Abonelikleriniz", SubscribedAs: "“%l” listesine %s adresiyle abonesiniz.", NotSubscribed: "“%l” listesine abone değilsiniz.",
		UnsubscribeFrom: "“%l” aboneliğinden çık", Lists: "%i listeleri", Save: "Seçimleri kaydet",
		UnsubscribeAll: "%i tarafından gelen tüm e-postalardan çık", UnsubscribeAllText: "Bu adrese %i tarafından artık liste ya da bülten gönderilmez.",
		Unsubscribed: "“%l” aboneliğinden çıktınız.", Saved: "Seçimleriniz kaydedildi.",
		AllGone: "%i tarafından artık liste e-postası almayacaksınız.", Undo: "Geri al", Restored: "Abonelikleriniz geri geldi.",
		ManageLink:        "Hangi e-postaları alacağınızı seçin",
		TestMail:          "Bu, bir bültenin deneme gönderimiydi. Bu bağlantı hiçbir şeyi değiştirmez; gerçek e-postada alıcının aboneliğini sonlandırır.",
		StatusConfirmText: "%s adresinin “%l” olay ve bakım bildirimlerini almasını onaylayın.", StatusOnly: "Yalnızca: %c",
		StatusConfirmed:    "“%l” bildirimlerini alacaksınız. Her e-postadan aboneliğinizi sonlandırabilirsiniz.",
		StatusSubscribedAs: "“%l” bildirimlerini %s adresine alıyorsunuz.", StatusNotSubscribed: "“%l” bildirimlerini almıyorsunuz.",
		StatusLegend: "Şunlarla ilgili bildirim gönder", StatusSave: "Seçimleri kaydet",
		StatusNoneChosen: "En az bir bileşen seçin ya da aboneliğinizi sonlandırın.", StatusBack: "Durum sayfasına dön",
		SubscribeTitle:   "“%l” bildirimlerini alın",
		SubscribeConsent: "%l olay ve bakım bildirimlerini bana gönderin. E-postayla onaylarım ve her e-postadan aboneliğimi sonlandırabilirim.",
		EmailLabel:       "E-posta adresi", SubscribeButton: "Abone ol",
		SentTitle: "E-postanıza bakın", SentText: "Adres e-posta alabiliyorsa bir onay e-postası yolda.",
		TooMany:     "Buradan çok fazla istek geldi. Bir süre sonra yeniden deneyin.",
		FormExpired: "Formun süresi doldu. Lütfen yeniden gönderin.", PageFull: "Bu sayfa yeni abone kabul etmiyor.",
		InvalidEmail: "Geçerli bir e-posta adresi girin.", ChooseOne: "En az bir bileşen seçin.",
	},
	"de": {
		Lang: "de", Gone: "Dieser Link ist nicht mehr gültig.", Error: "Etwas ist schiefgelaufen. Versuchen Sie den Link später erneut.",
		ConfirmTitle: "Bestätigen Sie Ihr Abonnement", ConfirmText: "Bestätigen Sie, dass %s „%l“ von %i erhalten soll.",
		ConfirmButton: "Abonnement bestätigen", Confirmed: "Sie haben „%l“ abonniert. Vielen Dank!",
		Already: "Sie haben „%l“ bereits abonniert.", Expired: "Dieser Link ist abgelaufen. Abonnieren Sie erneut, um einen neuen zu erhalten.",
		ManageTitle: "Ihre Abonnements", SubscribedAs: "Sie haben „%l“ mit %s abonniert.", NotSubscribed: "Sie haben „%l“ nicht abonniert.",
		UnsubscribeFrom: "Von „%l“ abmelden", Lists: "Listen von %i", Save: "Auswahl speichern",
		UnsubscribeAll: "Von allen E-Mails von %i abmelden", UnsubscribeAllText: "Keine Listen oder Newsletter von %i mehr an diese Adresse.",
		Unsubscribed: "Sie sind von „%l“ abgemeldet.", Saved: "Ihre Auswahl ist gespeichert.",
		AllGone: "Sie erhalten keine Listen-E-Mails von %i mehr.", Undo: "Rückgängig", Restored: "Ihre Abonnements sind wieder da.",
		ManageLink:        "Wählen, welche E-Mails Sie erhalten",
		TestMail:          "Dies war ein Test eines Newsletters. Dieser Link ändert nichts; in der echten E-Mail meldet er den Empfänger ab.",
		StatusConfirmText: "Bestätigen Sie, dass %s Meldungen zu Störungen und Wartungen von „%l“ erhalten soll.", StatusOnly: "Nur zu: %c",
		StatusConfirmed:    "Sie erhalten Meldungen von „%l“. Abmelden können Sie sich in jeder E-Mail.",
		StatusSubscribedAs: "Sie erhalten Meldungen von „%l“ an %s.", StatusNotSubscribed: "Sie erhalten keine Meldungen von „%l“.",
		StatusLegend: "Meldungen senden zu", StatusSave: "Auswahl speichern",
		StatusNoneChosen: "Wählen Sie mindestens eine Komponente oder melden Sie sich ab.", StatusBack: "Zurück zur Statusseite",
		SubscribeTitle:   "Meldungen von „%l“ erhalten",
		SubscribeConsent: "Senden Sie mir Meldungen zu Störungen und Wartungen von %l. Ich bestätige per E-Mail und kann mich in jeder E-Mail abmelden.",
		EmailLabel:       "E-Mail-Adresse", SubscribeButton: "Abonnieren",
		SentTitle: "Sehen Sie in Ihr Postfach", SentText: "Wenn die Adresse E-Mails empfangen kann, ist eine Bestätigung unterwegs.",
		TooMany:     "Zu viele Anfragen von hier. Versuchen Sie es später erneut.",
		FormExpired: "Das Formular ist veraltet. Bitte senden Sie es erneut.", PageFull: "Diese Seite nimmt keine weiteren Abonnenten an.",
		InvalidEmail: "Geben Sie eine gültige E-Mail-Adresse ein.", ChooseOne: "Wählen Sie mindestens eine Komponente.",
	},
}

func listWordsFor(locale string) listWords {
	base, _, _ := strings.Cut(strings.ToLower(strings.ReplaceAll(locale, "_", "-")), "-")
	if w, ok := listLanguages[base]; ok {
		return w
	}
	return listLanguages["en"]
}

func fill(s, list, inbox, addr string) string {
	return strings.NewReplacer("%l", list, "%i", inbox, "%s", addr).Replace(s)
}

// maskAddress shows enough of an address for its owner to recognise it: a•••@example.com.
func maskAddress(addr string) string {
	local, domain, ok := strings.Cut(addr, "@")
	if !ok || local == "" {
		return "•••"
	}
	r := []rune(local)
	return string(r[0]) + "•••@" + domain
}

type pageChoice struct {
	ID, Name, Description string
	Checked               bool
}

type pageForm struct {
	Action, Button string
	Hidden         map[string]string
	Lists          []string
	Choices        []pageChoice
	Legend, Note   string
	Field          string
	Inputs         []formInput
	Trap           string
	Secondary      bool
}

type formInput struct{ Name, Type, Label, Value, Auto string }

func (f pageForm) ChoiceName() string {
	if f.Field != "" {
		return f.Field
	}
	return "lists"
}

type pageLink struct{ URL, Label string }

type listPageView struct {
	W              listWords
	Inbox, Heading string
	Texts          []string
	Color          template.CSS
	Target         string
	Forms          []pageForm
	Links          []pageLink
}

var listPage = template.Must(template.New("list").Parse(`<!doctype html>
<html lang="{{.W.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>{{.Heading}}</title>
<style>
:root{--brand:{{.Color}};color-scheme:light dark}
body{font:16px/1.5 system-ui,sans-serif;max-width:32rem;margin:3rem auto;padding:0 1rem;color:#1f2328;background:#fff}
@media (prefers-color-scheme:dark){body{color:#e6e6e6;background:#16181d}}
.inbox{font-weight:600;color:var(--brand);margin:0 0 .5rem}
h1{font-size:1.5rem;margin:0 0 1rem}
form{margin:1.5rem 0}
fieldset{border:1px solid #8884;border-radius:.75rem;padding:.75rem 1rem;margin:0 0 1rem}
legend{font-weight:600;padding:0 .25rem}
label{display:flex;gap:.6rem;align-items:flex-start;padding:.4rem 0}
label small{display:block;opacity:.75}
input[type=checkbox]{width:1.1rem;height:1.1rem;margin-top:.2rem;accent-color:var(--brand)}
button{font:inherit;font-weight:600;padding:.7rem 1.4rem;border:0;border-radius:.6rem;background:var(--brand);color:#fff;cursor:pointer}
button.secondary{background:transparent;color:inherit;border:1px solid #8888}
.note{font-size:.9rem;opacity:.8;margin:.5rem 0 0}
label.field{display:block;font-weight:600;margin:0 0 1rem}
label.field input{display:block;box-sizing:border-box;width:100%;font:inherit;font-weight:400;margin-top:.35rem;padding:.6rem .75rem;border:1px solid #8888;border-radius:.6rem;background:transparent;color:inherit}
.trap{position:absolute;left:-10000px;width:1px;height:1px;overflow:hidden}
</style>
</head><body>
{{if .Inbox}}<p class="inbox">{{.Inbox}}</p>{{end}}
<h1>{{.Heading}}</h1>
{{range .Texts}}<p>{{.}}</p>{{end}}
{{$target := .Target}}{{range .Forms}}<form method="post" action="{{$target}}">
<input type="hidden" name="action" value="{{.Action}}">
{{range $k, $v := .Hidden}}<input type="hidden" name="{{$k}}" value="{{$v}}">{{end}}
{{range .Lists}}<input type="hidden" name="lists" value="{{.}}">{{end}}
{{range .Inputs}}<label class="field">{{.Label}}<input type="{{.Type}}" name="{{.Name}}" value="{{.Value}}" autocomplete="{{.Auto}}" required></label>
{{end}}{{if .Trap}}<div class="trap" aria-hidden="true"><label>Website<input type="text" name="{{.Trap}}" tabindex="-1" autocomplete="off"></label></div>
{{end}}{{if .Choices}}<fieldset><legend>{{.Legend}}</legend>
{{$name := .ChoiceName}}{{range .Choices}}<label><input type="checkbox" name="{{$name}}" value="{{.ID}}"{{if .Checked}} checked{{end}}><span>{{.Name}}{{if .Description}}<small>{{.Description}}</small>{{end}}</span></label>
{{end}}</fieldset>{{end}}
<button type="submit"{{if .Secondary}} class="secondary"{{end}}>{{.Button}}</button>
{{if .Note}}<p class="note">{{.Note}}</p>{{end}}
</form>{{end}}
{{range .Links}}<p><a href="{{.URL}}" style="color:var(--brand)">{{.Label}}</a></p>{{end}}
</body></html>`))

func writeListPage(w http.ResponseWriter, status int, v listPageView) {
	if v.Color == "" {
		v.Color = defaultBrandColor
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = listPage.Execute(w, v)
}

func (s *Server) listPageView(in store.Inbox, c *store.Contact) listPageView {
	locale := in.DefaultLocale
	if c != nil && c.Locale != nil && *c.Locale != "" {
		locale = *c.Locale
	}
	rv := ratingViewFor(in)
	return listPageView{W: listWordsFor(locale), Inbox: in.Name, Color: rv.Color}
}

func statusPageColor(v *listPageView, pg *store.StatusPage) {
	if pg != nil && pg.AccentColor != nil {
		v.Color = template.CSS(*pg.AccentColor)
	}
}

func fillComponents(s string, names []string) string {
	return strings.ReplaceAll(s, "%c", strings.Join(names, ", "))
}

func (s *Server) listPageError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.ErrorContext(r.Context(), "list page", slog.Any("error", err))
	writeListPage(w, http.StatusInternalServerError, listPageView{W: listWordsFor("en"), Heading: listWordsFor("en").Error})
}

func goneListPage(w http.ResponseWriter) {
	writeListPage(w, http.StatusNotFound, listPageView{W: listWordsFor("en"), Heading: listWordsFor("en").Gone})
}

type confirmTarget struct {
	sub     store.ListSubscription
	list    store.MailingList
	inbox   store.Inbox
	contact store.Contact
	issued  time.Time
	page    *store.StatusPage
	chosen  []string
}

func (s *Server) confirmTarget(ctx context.Context, q *store.Queries, token string, lock bool) (*confirmTarget, error) {
	ids, issued, ok := s.openToken(token, confirmTokenContext, 2, true)
	if !ok {
		return nil, nil
	}
	ws := ids[0]
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live {
		return nil, err
	}
	get := q.GetSubscription
	if lock {
		get = func(ctx context.Context, arg store.GetSubscriptionParams) (store.ListSubscription, error) {
			return q.LockSubscription(ctx, store.LockSubscriptionParams(arg))
		}
	}
	t := confirmTarget{issued: issued}
	var err error
	if t.sub, err = get(ctx, store.GetSubscriptionParams{WorkspaceID: ws, ID: ids[1]}); store.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if t.list, err = q.GetMailingList(ctx, store.GetMailingListParams{WorkspaceID: ws, ID: t.sub.ListID}); err != nil {
		return nil, err
	}
	if t.inbox, err = q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: t.list.InboxID}); err != nil {
		return nil, err
	}
	c, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: ws, ID: t.sub.ContactID})
	if err != nil {
		return nil, err
	}
	t.contact = store.Contact{ID: c.ID, Locale: c.Locale}
	if err := t.loadStatus(ctx, q); err != nil {
		return nil, err
	}
	return &t, nil
}

func (t *confirmTarget) loadStatus(ctx context.Context, q *store.Queries) error {
	pg, ok, err := statusListPage(ctx, q, t.list)
	if err != nil || !ok {
		return err
	}
	t.page = &pg
	chosen, err := subscriptionComponentIDs(ctx, q, t.sub)
	if err != nil || len(chosen) == 0 {
		return err
	}
	visible, err := visibleComponents(ctx, q, pg)
	if err != nil {
		return err
	}
	for _, c := range visible {
		if slices.Contains(chosen, c.ID) {
			t.chosen = append(t.chosen, c.Name)
		}
	}
	return nil
}

func (s *Server) confirmView(w http.ResponseWriter, t *confirmTarget, token string) {
	v := s.listPageView(t.inbox, &t.contact)
	ln, in := t.list.Name, t.inbox.Name
	if t.page != nil {
		ln = t.page.Name
		statusPageColor(&v, t.page)
	}
	switch {
	case t.sub.State == subSubscribed && t.page != nil:
		v.Heading = fill(v.W.StatusConfirmed, ln, in, "")
		writeListPage(w, http.StatusOK, v)
	case t.sub.State == subSubscribed:
		v.Heading = fill(v.W.Already, ln, in, "")
		writeListPage(w, http.StatusOK, v)
	case t.sub.State != subPending:
		v.Heading = v.W.Gone
		writeListPage(w, http.StatusNotFound, v)
	case s.now().Sub(t.issued) > confirmLinkTTL:
		v.Heading = v.W.Expired
		writeListPage(w, http.StatusGone, v)
	default:
		v.Heading = v.W.ConfirmTitle
		v.Texts = []string{fill(v.W.ConfirmText, ln, in, maskAddress(t.sub.Email))}
		if t.page != nil {
			v.Texts = []string{fill(v.W.StatusConfirmText, ln, in, maskAddress(t.sub.Email))}
			if len(t.chosen) > 0 {
				v.Texts = append(v.Texts, fillComponents(v.W.StatusOnly, t.chosen))
			}
		} else if t.list.Description != "" {
			v.Texts = append(v.Texts, t.list.Description)
		}
		v.Target = confirmPathPrefix + token
		v.Forms = []pageForm{{Action: "confirm", Button: v.W.ConfirmButton}}
		writeListPage(w, http.StatusOK, v)
	}
}

// serveConfirmPage shows the confirmation button. Opening the link stores nothing: mail scanners
// open every link, so only the button's POST confirms.
func (s *Server) serveConfirmPage(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	t, err := s.confirmTarget(r.Context(), s.st.Queries, token, false)
	if err != nil {
		s.listPageError(w, r, err)
		return
	}
	if t == nil {
		goneListPage(w)
		return
	}
	s.confirmView(w, t, token)
}

func (s *Server) serveConfirmPost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxListFormBytes)
	token := r.PathValue("token")
	ids, _, ok := s.openToken(token, confirmTokenContext, 2, true)
	if !ok {
		goneListPage(w)
		return
	}
	var (
		t    *confirmTarget
		done bool
	)
	err := s.inTx(r.Context(), ids[0], func(q *store.Queries, events *eventBatch) error {
		var err error
		if t, err = s.confirmTarget(r.Context(), q, token, true); err != nil || t == nil {
			return err
		}
		if t.sub.State != subPending || s.now().Sub(t.issued) > confirmLinkTTL {
			return nil
		}
		t.sub, err = s.confirmListSubscription(r.Context(), q, events, t.list, t.sub, s.requestProof(r, "link"))
		done = err == nil
		return err
	})
	switch {
	case err != nil:
		s.listPageError(w, r, err)
	case t == nil:
		goneListPage(w)
	case done && t.page != nil:
		v := s.listPageView(t.inbox, &t.contact)
		statusPageColor(&v, t.page)
		v.Heading = fill(v.W.StatusConfirmed, t.page.Name, t.inbox.Name, "")
		v.Links = []pageLink{{URL: s.pageURL(*t.page, ""), Label: v.W.StatusBack}}
		writeListPage(w, http.StatusOK, v)
	case done:
		v := s.listPageView(t.inbox, &t.contact)
		v.Heading = fill(v.W.Confirmed, t.list.Name, t.inbox.Name, "")
		writeListPage(w, http.StatusOK, v)
	default:
		s.confirmView(w, t, token)
	}
}

type manageTarget struct {
	ws, broadcast uuid.UUID
	list          store.MailingList
	inbox         store.Inbox
	contact       store.Contact
	lists         []store.MailingList
	subs          []store.ListSubscription
	page          *store.StatusPage
	components    []store.StatusComponent
}

func (t *manageTarget) subsOf(listID uuid.UUID) []store.ListSubscription {
	var out []store.ListSubscription
	for _, sub := range t.subs {
		if sub.ListID == listID {
			out = append(out, sub)
		}
	}
	return out
}

// address is where new subscriptions from this page go: the address the person got the link at.
func (t *manageTarget) address(ctx context.Context, q *store.Queries) (string, error) {
	if subs := t.subsOf(t.list.ID); len(subs) > 0 {
		return subs[0].Email, nil
	}
	if len(t.subs) > 0 {
		return t.subs[0].Email, nil
	}
	emails, err := q.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: t.ws, ContactIds: []uuid.UUID{t.contact.ID}})
	if err != nil || len(emails) == 0 {
		return "", err
	}
	return emails[0].Email, nil
}

func (s *Server) manageTarget(ctx context.Context, q *store.Queries, token string, lock bool) (*manageTarget, error) {
	ids, _, ok := s.openToken(token, manageTokenContext, 4, false)
	if !ok {
		return nil, nil
	}
	t := manageTarget{ws: ids[0], broadcast: ids[3]}
	if live, err := s.workspaceLive(ctx, t.ws); err != nil || !live {
		return nil, err
	}
	var err error
	if t.list, err = q.GetMailingList(ctx, store.GetMailingListParams{WorkspaceID: t.ws, ID: ids[1]}); store.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	c, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: t.ws, ID: ids[2]})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.contact = store.Contact{ID: c.ID, Locale: c.Locale}
	if lock {
		if _, err := q.LockContact(ctx, store.LockContactParams{WorkspaceID: t.ws, ID: c.ID}); err != nil {
			return nil, err
		}
	}
	if t.inbox, err = q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: t.ws, ID: t.list.InboxID}); err != nil {
		return nil, err
	}
	if t.list.Kind == listKindStatus {
		pg, ok, err := statusListPage(ctx, q, t.list)
		if err != nil || !ok {
			return nil, err
		}
		t.page = &pg
		if t.components, err = visibleComponents(ctx, q, pg); err != nil {
			return nil, err
		}
		t.lists = []store.MailingList{t.list}
	} else {
		if t.lists, err = q.ListInboxMailingLists(ctx, store.ListInboxMailingListsParams{WorkspaceID: t.ws, InboxID: t.inbox.ID}); err != nil {
			return nil, err
		}
		t.lists = slices.DeleteFunc(t.lists, func(l store.MailingList) bool { return l.Kind == listKindStatus })
	}
	all, err := q.ListContactSubscriptions(ctx, store.ListContactSubscriptionsParams{WorkspaceID: t.ws, ContactID: c.ID})
	if err != nil {
		return nil, err
	}
	for _, sub := range all {
		if slices.ContainsFunc(t.lists, func(l store.MailingList) bool { return l.ID == sub.ListID }) {
			t.subs = append(t.subs, sub)
		}
	}
	return &t, nil
}

func activeState(subs []store.ListSubscription) bool {
	return slices.ContainsFunc(subs, func(sub store.ListSubscription) bool { return sub.State != subUnsubscribed })
}

// offered are the lists the page shows as choices: the inbox's public lists and the link's own.
func (t *manageTarget) offered() []store.MailingList {
	var out []store.MailingList
	for _, l := range t.lists {
		if l.ID == t.list.ID || (l.Public && l.ArchivedAt == nil) {
			out = append(out, l)
		}
	}
	return out
}

func (s *Server) manageView(w http.ResponseWriter, ctx context.Context, t *manageTarget, token string, status int, heading string, undo []uuid.UUID) {
	v := s.listPageView(t.inbox, &t.contact)
	ln, in := t.list.Name, t.inbox.Name
	v.Target = managePathPrefix + token
	if t.page != nil {
		s.statusManageView(w, ctx, t, v, status, heading, undo)
		return
	}
	if heading != "" {
		v.Heading = heading
		if len(undo) > 0 {
			f := pageForm{Action: "undo", Button: v.W.Undo, Secondary: true}
			for _, id := range undo {
				f.Lists = append(f.Lists, id.String())
			}
			v.Forms = append(v.Forms, f)
		}
		v.Forms = append(v.Forms, pageForm{Action: "show", Button: v.W.ManageLink, Secondary: true})
		writeListPage(w, status, v)
		return
	}
	v.Heading = v.W.ManageTitle
	own := t.subsOf(t.list.ID)
	if activeState(own) {
		v.Texts = []string{fill(v.W.SubscribedAs, ln, in, maskAddress(own[0].Email))}
		v.Forms = append(v.Forms, pageForm{Action: "unsubscribe", Button: fill(v.W.UnsubscribeFrom, ln, in, "")})
	} else {
		v.Texts = []string{fill(v.W.NotSubscribed, ln, in, "")}
	}
	choices := pageForm{Action: "save", Button: v.W.Save, Legend: fill(v.W.Lists, ln, in, ""), Secondary: true}
	for _, l := range t.offered() {
		choices.Choices = append(choices.Choices, pageChoice{ID: l.ID.String(), Name: l.Name, Description: l.Description, Checked: activeState(t.subsOf(l.ID))})
	}
	v.Forms = append(v.Forms, choices,
		pageForm{Action: "all", Button: fill(v.W.UnsubscribeAll, ln, in, ""), Note: fill(v.W.UnsubscribeAllText, ln, in, ""), Secondary: true})
	writeListPage(w, status, v)
}

func (s *Server) statusManageView(w http.ResponseWriter, ctx context.Context, t *manageTarget, v listPageView, status int, heading string, undo []uuid.UUID) {
	ln, in := t.page.Name, t.inbox.Name
	statusPageColor(&v, t.page)
	v.Links = []pageLink{{URL: s.pageURL(*t.page, ""), Label: v.W.StatusBack}}
	own := t.subsOf(t.list.ID)
	if heading != "" {
		v.Heading = heading
		if len(undo) > 0 {
			v.Forms = append(v.Forms, pageForm{Action: "undo", Button: v.W.Undo, Secondary: true, Lists: []string{t.list.ID.String()}})
		}
		v.Forms = append(v.Forms, pageForm{Action: "show", Button: v.W.ManageLink, Secondary: true})
		writeListPage(w, status, v)
		return
	}
	v.Heading = v.W.ManageTitle
	if !activeState(own) {
		v.Texts = []string{fill(v.W.StatusNotSubscribed, ln, in, "")}
		writeListPage(w, status, v)
		return
	}
	v.Texts = []string{fill(v.W.StatusSubscribedAs, ln, in, maskAddress(own[0].Email))}
	v.Forms = append(v.Forms, pageForm{Action: "unsubscribe", Button: fill(v.W.UnsubscribeFrom, ln, in, "")})
	if len(t.components) > 1 {
		var chosen []uuid.UUID
		for _, sub := range own {
			ids, err := subscriptionComponentIDs(ctx, s.st.Queries, sub)
			if err != nil {
				s.log.ErrorContext(ctx, "status components", slog.Any("error", err))
			}
			chosen = append(chosen, ids...)
		}
		f := pageForm{Action: "components", Button: v.W.StatusSave, Legend: v.W.StatusLegend, Field: "components", Secondary: true}
		for _, c := range t.components {
			f.Choices = append(f.Choices, pageChoice{ID: c.ID.String(), Name: c.Name, Checked: len(chosen) == 0 || slices.Contains(chosen, c.ID)})
		}
		v.Forms = append(v.Forms, f)
	}
	writeListPage(w, status, v)
}

// serveManagePage shows the unsubscribe and preference page; opening it changes nothing.
// testManagePage answers the unsubscribe link of a test send, which names no list.
func (s *Server) testManagePage(w http.ResponseWriter, r *http.Request, token string) bool {
	ids, _, ok := s.openToken(token, manageTokenContext, 4, false)
	if !ok || ids[1] != uuid.Nil() || ids[3] == uuid.Nil() {
		return false
	}
	if r.Method == http.MethodPost && r.PostFormValue("List-Unsubscribe") == "One-Click" {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		return true
	}
	b, err := s.st.GetBroadcast(r.Context(), store.GetBroadcastParams{WorkspaceID: ids[0], ID: ids[3]})
	if err != nil {
		goneListPage(w)
		return true
	}
	in, err := s.st.GetInbox(r.Context(), store.GetInboxParams{WorkspaceID: ids[0], ID: b.InboxID})
	if err != nil {
		goneListPage(w)
		return true
	}
	v := s.listPageView(in, nil)
	v.Heading = v.W.TestMail
	writeListPage(w, http.StatusOK, v)
	return true
}

func (s *Server) serveManagePage(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if s.testManagePage(w, r, token) {
		return
	}
	t, err := s.manageTarget(r.Context(), s.st.Queries, token, false)
	if err != nil {
		s.listPageError(w, r, err)
		return
	}
	if t == nil {
		goneListPage(w)
		return
	}
	s.manageView(w, r.Context(), t, token, http.StatusOK, "", nil)
}

func formLists(values []string) []uuid.UUID {
	var out []uuid.UUID
	for _, v := range values {
		if id, err := uuid.Parse(v); err == nil && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// serveManagePost applies the page's choices, or RFC 8058 one-click: a POST whose body is
// `List-Unsubscribe=One-Click` unsubscribes the link's list and answers 200 with no body.
func (s *Server) serveManagePost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxListFormBytes)
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		_ = r.ParseMultipartForm(maxListFormBytes)
	} else {
		_ = r.ParseForm()
	}
	token := r.PathValue("token")
	if s.testManagePage(w, r, token) {
		return
	}
	action := r.PostFormValue("action")
	oneClick := action == "" && r.PostFormValue("List-Unsubscribe") == "One-Click"
	ids, _, ok := s.openToken(token, manageTokenContext, 4, false)
	if !ok {
		if oneClick {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		goneListPage(w)
		return
	}
	var (
		t       *manageTarget
		heading string
		undo    []uuid.UUID
	)
	err := s.inTx(r.Context(), ids[0], func(q *store.Queries, events *eventBatch) error {
		var err error
		if t, err = s.manageTarget(r.Context(), q, token, true); err != nil || t == nil {
			return err
		}
		words := s.listPageView(t.inbox, &t.contact).W
		ln, in := t.list.Name, t.inbox.Name
		if t.page != nil {
			ln = t.page.Name
		}
		switch {
		case oneClick:
			_, err = s.leaveLists(r.Context(), q, events, r, t, []uuid.UUID{t.list.ID}, string(oas.UnsubscribeReasonOneClick))
		case action == "unsubscribe":
			undo, err = s.leaveLists(r.Context(), q, events, r, t, []uuid.UUID{t.list.ID}, string(oas.UnsubscribeReasonLink))
			heading = fill(words.Unsubscribed, ln, in, "")
		case action == "save" && t.page == nil:
			heading = words.Saved
			undo, err = s.saveChoices(r.Context(), q, events, r, t, formLists(r.PostForm["lists"]))
		case action == "components" && t.page != nil:
			heading = words.Saved
			err = s.saveComponents(r.Context(), q, t, formLists(r.PostForm["components"]))
			if errors.Is(err, errNoComponents) {
				heading, err = words.StatusNoneChosen, nil
			}
		case action == "all" && t.page == nil:
			heading = fill(words.AllGone, ln, in, "")
			if undo, err = s.leaveAll(r.Context(), q, events, r, t); err != nil {
				return err
			}
		case action == "undo":
			heading = words.Restored
			err = s.undoLeave(r.Context(), q, events, r, t, formLists(r.PostForm["lists"]))
		}
		return err
	})
	switch {
	case err != nil && oneClick:
		s.log.ErrorContext(r.Context(), "one-click unsubscribe", slog.Any("error", err))
		w.WriteHeader(http.StatusInternalServerError)
	case err != nil:
		s.listPageError(w, r, err)
	case t == nil && oneClick:
		w.WriteHeader(http.StatusNotFound)
	case t == nil:
		goneListPage(w)
	case oneClick:
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
	default:
		if heading != "" {
			if fresh, ferr := s.manageTarget(r.Context(), s.st.Queries, token, false); ferr == nil && fresh != nil {
				t = fresh
			}
		}
		s.manageView(w, r.Context(), t, token, http.StatusOK, heading, undo)
	}
}

var errNoComponents = errors.New("no components chosen")

// Component choices are preferences, so no consent event is written.
func (s *Server) saveComponents(ctx context.Context, q *store.Queries, t *manageTarget, chosen []uuid.UUID) error {
	visible := make([]uuid.UUID, len(t.components))
	for i, c := range t.components {
		visible[i] = c.ID
	}
	if !slices.ContainsFunc(chosen, func(id uuid.UUID) bool { return slices.Contains(visible, id) }) {
		return errNoComponents
	}
	for _, sub := range t.subsOf(t.list.ID) {
		if sub.State == subUnsubscribed {
			continue
		}
		if err := setSubscriptionComponents(ctx, q, sub, visible, chosen); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) pageProof(r *http.Request, t *manageTarget, source string) consentProof {
	pr := s.requestProof(r, source)
	if t.broadcast != uuid.Nil() {
		b := t.broadcast
		pr.broadcastID = &b
	}
	return pr
}

func listByID(lists []store.MailingList, id uuid.UUID) (store.MailingList, bool) {
	i := slices.IndexFunc(lists, func(l store.MailingList) bool { return l.ID == id })
	if i < 0 {
		return store.MailingList{}, false
	}
	return lists[i], true
}

// leaveLists unsubscribes the contact from the lists and reports the ones it left.
func (s *Server) leaveLists(ctx context.Context, q *store.Queries, events *eventBatch, r *http.Request, t *manageTarget, ids []uuid.UUID, reason string) ([]uuid.UUID, error) {
	pr := s.pageProof(r, t, reason)
	var left []uuid.UUID
	for _, id := range ids {
		l, ok := listByID(t.lists, id)
		if !ok {
			continue
		}
		for _, sub := range t.subsOf(id) {
			if sub.State == subUnsubscribed {
				continue
			}
			if _, err := s.unsubscribe(ctx, q, events, l, sub, reason, consentUnsubscribed, pr); err != nil {
				return nil, err
			}
			if !slices.Contains(left, id) {
				left = append(left, id)
			}
		}
	}
	return left, nil
}

// joinLists subscribes the contact at once: the person asked on a page only their mailbox links
// to. An objection they made before is lifted.
func (s *Server) joinLists(ctx context.Context, q *store.Queries, events *eventBatch, r *http.Request, t *manageTarget, ids []uuid.UUID) error {
	addr, err := t.address(ctx, q)
	if err != nil || addr == "" {
		return err
	}
	if suppressed, err := q.IsEmailSuppressedAnyScope(ctx, store.IsEmailSuppressedAnyScopeParams{WorkspaceID: t.ws, Email: addr}); err != nil || suppressed {
		return err
	}
	src := string(oas.SubscriptionSourcePreferences)
	for _, id := range ids {
		l, ok := listByID(t.lists, id)
		if !ok || activeState(t.subsOf(id)) {
			continue
		}
		pr := s.pageProof(r, t, src)
		label := l.Name
		if l.Description != "" {
			label += " — " + l.Description
		}
		pr.text = optionalText(label, 2000)
		cur, err := q.LockSubscriptionByEmail(ctx, store.LockSubscriptionByEmailParams{WorkspaceID: t.ws, ListID: l.ID, Email: addr})
		switch {
		case err == nil:
			_, err = s.subscribeNow(ctx, q, events, l, cur, s.now(), pr)
		case store.IsNotFound(err):
			var sub store.ListSubscription
			if sub, err = s.newSubscription(ctx, q, events, l, t.contact.ID, addr, subSubscribed, src, new(s.now())); err == nil {
				if err = s.consentEvent(ctx, q, sub, consentSubscribed, pr); err == nil {
					s.queueWelcome(events, l, sub)
				}
			}
		}
		if err != nil {
			return err
		}
	}
	if t.page != nil {
		return nil
	}
	return s.forgetOptOut(ctx, q, t.ws, addr)
}

func (s *Server) saveChoices(ctx context.Context, q *store.Queries, events *eventBatch, r *http.Request, t *manageTarget, chosen []uuid.UUID) ([]uuid.UUID, error) {
	var join, leave []uuid.UUID
	for _, l := range t.offered() {
		active := activeState(t.subsOf(l.ID))
		want := slices.Contains(chosen, l.ID)
		switch {
		case want && !active:
			join = append(join, l.ID)
		case !want && active:
			leave = append(leave, l.ID)
		}
	}
	left, err := s.leaveLists(ctx, q, events, r, t, leave, string(oas.UnsubscribeReasonPreferences))
	if err != nil {
		return nil, err
	}
	if len(join) > 0 {
		if err := s.joinLists(ctx, q, events, r, t, join); err != nil {
			return nil, err
		}
	}
	return left, nil
}

// leaveAll unsubscribes the contact from every list of the inbox and remembers the objection for
// the contact's addresses in the workspace.
func (s *Server) leaveAll(ctx context.Context, q *store.Queries, events *eventBatch, r *http.Request, t *manageTarget) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, len(t.lists))
	for i, l := range t.lists {
		ids[i] = l.ID
	}
	left, err := s.leaveLists(ctx, q, events, r, t, ids, string(oas.UnsubscribeReasonPreferences))
	if err != nil {
		return nil, err
	}
	var addrs []string
	for _, sub := range t.subs {
		if !slices.Contains(addrs, sub.Email) {
			addrs = append(addrs, sub.Email)
		}
	}
	emails, err := q.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: t.ws, ContactIds: []uuid.UUID{t.contact.ID}})
	if err != nil {
		return nil, err
	}
	for _, e := range emails {
		if !slices.Contains(addrs, e.Email) {
			addrs = append(addrs, e.Email)
		}
	}
	return left, s.rememberOptOuts(ctx, q, t.ws, addrs, optOutAll)
}

// undoLeave subscribes again to lists the person left from this page in the last ten minutes.
func (s *Server) undoLeave(ctx context.Context, q *store.Queries, events *eventBatch, r *http.Request, t *manageTarget, ids []uuid.UUID) error {
	var back []uuid.UUID
	for _, id := range ids {
		for _, sub := range t.subsOf(id) {
			if sub.State == subUnsubscribed && sub.UnsubscribedAt != nil && s.now().Sub(*sub.UnsubscribedAt) <= undoWindow &&
				sub.UnsubscribeReason != nil && slices.Contains([]string{"link", "one_click", "preferences"}, *sub.UnsubscribeReason) {
				back = append(back, id)
				break
			}
		}
	}
	if len(back) == 0 {
		return nil
	}
	return s.joinLists(ctx, q, events, r, t, back)
}

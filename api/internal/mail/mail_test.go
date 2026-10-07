package mail_test

import (
	"testing"

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

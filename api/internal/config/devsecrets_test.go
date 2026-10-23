package config

import "testing"

func TestDevSecretsOnlyOnLoopback(t *testing.T) {
	cases := []struct {
		url     string
		key     string
		ingress string
		ok      bool
	}{
		{"http://localhost:8809", devSecrets["YUVA_MASTER_KEY"], devSecrets["YUVA_INGRESS_SECRET"], true},
		{"http://127.0.0.1:8809", devSecrets["YUVA_MASTER_KEY"], "", true},
		{"https://help.example.com", devSecrets["YUVA_MASTER_KEY"], "", false},
		{"https://help.example.com", "", devSecrets["YUVA_INGRESS_SECRET"], false},
		{"https://help.example.com", "cmVhbC1rZXk=", "real", true},
	}
	for _, tc := range cases {
		t.Setenv("YUVA_PUBLIC_URL", tc.url)
		t.Setenv("YUVA_MASTER_KEY", tc.key)
		t.Setenv("YUVA_INGRESS_SECRET", tc.ingress)
		_, err := Load("test")
		if (err == nil) != tc.ok {
			t.Errorf("%s key=%q ingress=%q: err = %v, want ok=%v", tc.url, tc.key, tc.ingress, err, tc.ok)
		}
	}
}

func TestDevVAPIDKeyRejectedOnPublicHost(t *testing.T) {
	t.Setenv("YUVA_PUBLIC_URL", "https://help.example.com")
	t.Setenv("YUVA_VAPID_PUBLIC_KEY", "pub")
	t.Setenv("YUVA_VAPID_PRIVATE_KEY", devSecrets["YUVA_VAPID_PRIVATE_KEY"])
	t.Setenv("YUVA_VAPID_SUBJECT", "mailto:a@example.com")
	if _, err := Load("test"); err == nil {
		t.Fatal("dev VAPID private key accepted on a public host")
	}
}

func TestFullMailLogOnlyOnLoopback(t *testing.T) {
	cases := []struct {
		url, mode string
		ok, full  bool
	}{
		{"http://localhost:8080", "", true, false},
		{"http://localhost:8080", "full", true, true},
		{"http://127.0.0.1:8080", "FULL", true, true},
		{"https://help.example.com", "redacted", true, false},
		{"https://help.example.com", "full", false, false},
		{"http://localhost:8080", "everything", false, false},
	}
	for _, tc := range cases {
		t.Setenv("YUVA_PUBLIC_URL", tc.url)
		t.Setenv("YUVA_MAIL_LOG", tc.mode)
		c, err := Load("test")
		if (err == nil) != tc.ok || (err == nil && c.MailLogFull != tc.full) {
			t.Errorf("%s YUVA_MAIL_LOG=%q: full = %v, err = %v; want ok=%v full=%v", tc.url, tc.mode, c.MailLogFull, err, tc.ok, tc.full)
		}
	}
}

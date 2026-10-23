package config

import (
	"strings"
	"testing"
)

func TestAnalyticsMonthlyEventsDefault(t *testing.T) {
	for _, tc := range []struct {
		env    string
		signup bool
		want   int64
	}{
		{"", false, 1000000},
		{"", true, 1000000},
		{"250000", false, 250000},
		{"0", false, 0},
	} {
		t.Setenv("YUVA_ANALYTICS_MONTHLY_EVENTS", tc.env)
		c := Config{SignupOpen: tc.signup}
		if err := c.loadAnalytics(); err != nil {
			t.Fatal(err)
		}
		if c.Analytics.MonthlyEvents != tc.want {
			t.Errorf("env %q signup %v: %d, want %d", tc.env, tc.signup, c.Analytics.MonthlyEvents, tc.want)
		}
	}
}

func TestSESSettings(t *testing.T) {
	const secret = "s3cr3t/value+that-must-not-leak"
	full := map[string]string{
		"YUVA_SES_REGION": "eu-west-1", "YUVA_SES_ACCESS_KEY_ID": "AKIAEXAMPLEEXAMPLE12",
		"YUVA_SES_SECRET_ACCESS_KEY": secret, "YUVA_SES_CONFIGURATION_SET": "yuva-events", "YUVA_SES_TENANT_PREFIX": "",
	}
	for _, tc := range []struct {
		name    string
		change  map[string]string
		enabled bool
		prefix  string
		fails   bool
	}{
		{name: "none", change: map[string]string{"YUVA_SES_REGION": "", "YUVA_SES_ACCESS_KEY_ID": "", "YUVA_SES_SECRET_ACCESS_KEY": "", "YUVA_SES_CONFIGURATION_SET": ""}},
		{name: "all", enabled: true, prefix: "yuva-"},
		{name: "own prefix", change: map[string]string{"YUVA_SES_TENANT_PREFIX": "prod_"}, enabled: true, prefix: "prod_"},
		{name: "no secret", change: map[string]string{"YUVA_SES_SECRET_ACCESS_KEY": ""}, fails: true},
		{name: "only secret", change: map[string]string{"YUVA_SES_REGION": "", "YUVA_SES_ACCESS_KEY_ID": "", "YUVA_SES_CONFIGURATION_SET": ""}, fails: true},
		{name: "no configuration set", change: map[string]string{"YUVA_SES_CONFIGURATION_SET": ""}, fails: true},
		{name: "bad region", change: map[string]string{"YUVA_SES_REGION": "europe"}, fails: true},
		{name: "bad configuration set", change: map[string]string{"YUVA_SES_CONFIGURATION_SET": "has space"}, fails: true},
		{name: "long prefix", change: map[string]string{"YUVA_SES_TENANT_PREFIX": "abcdefghijklmnopqrstuvwxyz0123456"}, fails: true},
		{name: "bad prefix", change: map[string]string{"YUVA_SES_TENANT_PREFIX": "yuva:"}, fails: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range full {
				if c, ok := tc.change[k]; ok {
					v = c
				}
				t.Setenv(k, v)
			}
			var c Config
			err := c.loadSES()
			if tc.fails {
				if err == nil {
					t.Fatal("no error")
				}
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("the error shows the secret: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.SES.Enabled() != tc.enabled || (tc.enabled && c.SES.TenantPrefix != tc.prefix) {
				t.Fatalf("got %+v", c.SES.Enabled())
			}
		})
	}
}

func TestStatusCanaryURLs(t *testing.T) {
	for _, tc := range []struct {
		env   string
		off   bool
		urls  int
		fails bool
	}{
		{env: ""},
		{env: "off", off: true},
		{env: " OFF ", off: true},
		{env: "https://a.example/ping, http://b.example/", urls: 2},
		{env: "off,https://a.example/", fails: true},
		{env: "a.example", fails: true},
	} {
		t.Setenv("YUVA_STATUS_CANARY_URLS", tc.env)
		t.Setenv("YUVA_STATUS_MIN_INTERVAL", "")
		var c Config
		err := c.loadStatusChecks()
		if (err != nil) != tc.fails || c.StatusCanariesOff != tc.off || len(c.StatusCanaryURLs) != tc.urls {
			t.Errorf("env %q: off %v, urls %v, error %v", tc.env, c.StatusCanariesOff, c.StatusCanaryURLs, err)
		}
	}
}

func TestIngressAcceptsOnlySignedHeadersByDefault(t *testing.T) {
	t.Setenv("YUVA_PUBLIC_URL", "http://localhost:8809")
	t.Setenv("YUVA_MASTER_KEY", devSecrets["YUVA_MASTER_KEY"])
	for _, tc := range []struct {
		env  string
		want bool
	}{{"", false}, {"true", true}, {"false", false}} {
		t.Setenv("YUVA_INGRESS_ACCEPT_V2", tc.env)
		c, err := Load("test")
		if err != nil {
			t.Fatal(err)
		}
		if c.IngressAcceptV1 || c.IngressAcceptV2 != tc.want {
			t.Errorf("YUVA_INGRESS_ACCEPT_V2=%q: v1 %v v2 %v", tc.env, c.IngressAcceptV1, c.IngressAcceptV2)
		}
	}
}

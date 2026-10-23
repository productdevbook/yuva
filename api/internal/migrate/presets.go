// Package migrate holds what moving a mailbox into Yuva needs before any mail is read: the
// provider presets, detection from an address, the address checks for the connection, and the
// connection test.
package migrate

import (
	"strings"

	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk"
	"github.com/productdevbook/yuva/api/internal/migrate/microsoft"
)

type Server struct {
	Host string
	Port int
	TLS  string
}

type Budget struct {
	Connections    int
	BytesPerDay    int64
	BytesPerSecond int64
	// RequestsPerMinute, IncrementalPerMinute and RequestsPerDay are a help desk's, until it
	// reports its limits.
	RequestsPerMinute    int
	IncrementalPerMinute int
	RequestsPerDay       int
}

type Preset struct {
	ID          string
	Source      string
	Name        string
	Servers     []Server
	Auth        []string
	AppPassword bool
	Username    string
	Folders     []string
	Budget      Budget
	Guide       string
	Available   bool
	// Scopes are what a help desk's or Microsoft's sign-in asks for; Subdomain, that it needs the
	// account's; TokenFields, what a token desk's connect form asks for. Microsoft's Available is
	// the server's to tell (Sign in with Microsoft).
	Scopes      []string
	Subdomain   bool
	TokenFields []string
}

const (
	AuthPassword  = "password"
	AuthMicrosoft = "microsoft"
	AuthOAuth     = helpdesk.AuthOAuth
	AuthToken     = helpdesk.AuthToken

	SourceZendesk   = "zendesk"
	SourceHelpScout = "helpscout"
	SourceIntercom  = "intercom"

	UsernameAddress   = "address"
	UsernameLocalPart = "local_part"

	FolderUser = "user"

	PresetGmail           = "gmail"
	PresetGoogleWorkspace = "google_workspace"
	PresetMicrosoft       = "microsoft"
	PresetYandex          = "yandex"
	PresetICloud          = "icloud"
	PresetZoho            = "zoho"
	PresetYahoo           = "yahoo"
	PresetFastmail        = "fastmail"
	PresetGMX             = "gmx"
	PresetIMAP            = "imap"
	PresetTakeout         = "takeout"
	PresetFile            = "file"
	PresetZendesk         = "zendesk"
	PresetHelpScout       = "helpscout"
	PresetIntercom        = "intercom"
)

func tls993(hosts ...string) []Server {
	out := make([]Server, len(hosts))
	for i, h := range hosts {
		out[i] = Server{Host: h, Port: 993, TLS: "tls"}
	}
	return out
}

var (
	gmailBudget   = Budget{Connections: 1, BytesPerDay: 1500 * 1000 * 1000}
	defaultBudget = Budget{Connections: 1, BytesPerSecond: 4 * 1000 * 1000}
	inboxSentUser = []string{"INBOX", `\Sent`, FolderUser}
	withArchive   = []string{"INBOX", `\Sent`, `\Archive`, FolderUser}
)

var zohoHosts = func() []string {
	var out []string
	for _, tld := range []string{"com", "eu", "in", "com.au", "jp", "ca", "sa"} {
		out = append(out, "imappro.zoho."+tld, "imap.zoho."+tld)
	}
	return out
}()

// Presets are the providers in the order the panel shows them.
var Presets = []Preset{
	{ID: PresetGmail, Name: "Gmail", Servers: tls993("imap.gmail.com"), Auth: []string{AuthPassword}, AppPassword: true,
		Username: UsernameAddress, Folders: []string{`\All`}, Budget: gmailBudget, Guide: "migrate-gmail", Available: true},
	{ID: PresetGoogleWorkspace, Name: "Google Workspace", Servers: tls993("imap.gmail.com"), Auth: []string{AuthPassword}, AppPassword: true,
		Username: UsernameAddress, Folders: []string{`\All`}, Budget: gmailBudget, Guide: "migrate-google-workspace", Available: true},
	{ID: PresetMicrosoft, Name: "Outlook.com and Microsoft 365", Servers: tls993("outlook.office365.com"), Auth: []string{AuthMicrosoft},
		Username: UsernameAddress, Folders: withArchive, Budget: defaultBudget, Guide: "migrate-microsoft", Scopes: microsoft.Scopes},
	{ID: PresetYandex, Name: "Yandex", Servers: tls993("imap.yandex.com"), Auth: []string{AuthPassword}, AppPassword: true,
		Username: UsernameAddress, Folders: inboxSentUser, Budget: defaultBudget, Guide: "migrate-yandex", Available: true},
	{ID: PresetICloud, Name: "iCloud Mail", Servers: tls993("imap.mail.me.com"), Auth: []string{AuthPassword}, AppPassword: true,
		Username: UsernameLocalPart, Folders: withArchive, Budget: defaultBudget, Guide: "migrate-icloud", Available: true},
	{ID: PresetZoho, Name: "Zoho Mail", Servers: tls993(zohoHosts...), Auth: []string{AuthPassword},
		Username: UsernameAddress, Folders: inboxSentUser, Budget: defaultBudget, Guide: "migrate-zoho", Available: true},
	{ID: PresetYahoo, Name: "Yahoo Mail", Servers: tls993("imap.mail.yahoo.com"), Auth: []string{AuthPassword}, AppPassword: true,
		Username: UsernameAddress, Folders: withArchive, Budget: defaultBudget, Guide: "migrate-yahoo", Available: true},
	{ID: PresetFastmail, Name: "Fastmail", Servers: tls993("imap.fastmail.com"), Auth: []string{AuthPassword}, AppPassword: true,
		Username: UsernameAddress, Folders: withArchive, Budget: defaultBudget, Guide: "migrate-fastmail", Available: true},
	{ID: PresetGMX, Name: "GMX", Servers: tls993("imap.gmx.net", "imap.gmx.com", "imap.mail.com"), Auth: []string{AuthPassword},
		Username: UsernameAddress, Folders: inboxSentUser, Budget: defaultBudget, Guide: "migrate-gmx", Available: true},
	{ID: PresetIMAP, Name: "Other IMAP server", Auth: []string{AuthPassword},
		Username: UsernameAddress, Folders: withArchive, Budget: defaultBudget, Guide: "migrate-imap", Available: true},
	{ID: PresetTakeout, Source: "file", Name: "Google Takeout", Auth: []string{}, Username: UsernameAddress, Folders: []string{`\All`},
		Guide: "migrate-files", Available: true},
	{ID: PresetFile, Source: "file", Name: "Export file", Auth: []string{}, Username: UsernameAddress, Folders: withArchive,
		Guide: "migrate-files", Available: true},
}

// deskPreset is a help desk's preset, read from its registry entry: the preset's id is the source.
func deskPreset(d helpdesk.Desk) Preset {
	p := Preset{
		ID: d.Source, Source: d.Source, Name: d.Name, Auth: []string{d.Auth}, Username: UsernameAddress, Folders: []string{"INBOX"},
		Budget: Budget{Connections: 1, RequestsPerMinute: d.PerMinute(), IncrementalPerMinute: d.IncrementalMax, RequestsPerDay: d.PerDay()},
		Guide:  d.Guide, Available: true, Subdomain: d.Account == helpdesk.AccountSubdomain && d.Auth == AuthOAuth, TokenFields: d.Fields(),
	}
	if d.Auth == AuthOAuth {
		p.Scopes = d.Scopes
	}
	if d.Account == helpdesk.AccountNone || d.Account == helpdesk.AccountWebsite {
		p.Servers = []Server{{Host: d.Host(helpdesk.Account{}), Port: 443, TLS: "tls"}}
	}
	return p
}

// Helpdesk tells a help-desk source.
func Helpdesk(source string) bool {
	_, ok := helpdesk.Lookup(source)
	return ok
}

func init() {
	for _, d := range helpdesk.Desks() {
		Presets = append(Presets, deskPreset(d))
	}
	for i := range Presets {
		if Presets[i].Source == "" {
			Presets[i].Source = "imap"
		}
	}
}

func LookupPreset(id string) (Preset, bool) {
	for _, p := range Presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// ServerFor picks the preset's server named by host, or its first when host is empty.
func (p Preset) ServerFor(host string) (Server, bool) {
	if len(p.Servers) == 0 {
		return Server{}, false
	}
	if host == "" {
		return p.Servers[0], true
	}
	for _, s := range p.Servers {
		if strings.EqualFold(s.Host, host) {
			return s, true
		}
	}
	return Server{}, false
}

// MicrosoftHost reports a Microsoft server, which accepts no passwords.
func MicrosoftHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, s := range []string{"office365.com", "outlook.com", "hotmail.com", "live.com"} {
		if host == s || strings.HasSuffix(host, "."+s) {
			return true
		}
	}
	return false
}

package api

import (
	"context"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/migrate"
	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
	"github.com/productdevbook/yuva/api/internal/webhook"
)

const (
	eventMigrationUpdated = string(oas.MigrationUpdated)

	migrationDraft          = string(oas.MigrationDraft)
	migrationConnected      = string(oas.MigrationConnected)
	migrationScanning       = string(oas.MigrationScanning)
	migrationReady          = string(oas.MigrationReady)
	migrationCopying        = string(oas.MigrationCopying)
	migrationPaused         = string(oas.MigrationPaused)
	migrationNeedsAttention = string(oas.MigrationNeedsAttention)
	migrationSyncing        = string(oas.MigrationSyncing)
	migrationCompleted      = string(oas.MigrationCompleted)
	migrationCancelled      = string(oas.MigrationCancelled)
	migrationRemoving       = string(oas.MigrationRemoving)
	migrationRemoved        = string(oas.MigrationRemoved)

	testsPerMigration   = 10
	testsPerWorkspace   = 30
	testWindow          = time.Hour
	authFailuresToLock  = 3
	authFailureLockout  = 15 * time.Minute
	defaultCloseAfter   = 14
	defaultQuietPeriod  = 24
	maxMigrationFolders = 1000
)

var (
	errMigrationGone         = problem(http.StatusNotFound, "not_found", "no such migration")
	errProviderNotConfigured = problem(http.StatusConflict, "provider_not_configured", "Outlook.com and Microsoft 365 connect with Sign in with Microsoft, which this server cannot offer: it has no Microsoft app of its own (YUVA_MICROSOFT_CLIENT_ID) and its public URL is not one Microsoft accepts for a team's own app (HTTPS, or http://localhost)")
	errMigrationNoCreds      = problem(http.StatusConflict, "migration_state", "set the mailbox's credentials before testing the connection")
	errFileNoCredentials     = problem(http.StatusConflict, "migration_state", "an export file migration reads its uploaded file; it has no mailbox to connect to")
	errMicrosoftNoPassword   = problem(http.StatusConflict, "migration_state", "Outlook.com and Microsoft 365 accept no passwords: sign in with Microsoft (POST .../sign-in)")
	builtMigrationSources    = func() []string {
		out := []string{string(oas.MigrationSourceImap), string(oas.MigrationSourceFile)}
		for _, d := range helpdesk.Desks() {
			out = append(out, d.Source)
		}
		return out
	}()
	runningMigrationStates = []string{migrationScanning, migrationCopying, migrationSyncing, migrationRemoving}
)

func errMigrationState(detail string) *apiError {
	return problem(http.StatusConflict, "migration_state", detail)
}

func errHostedLimit(limit oas.HostedLimit, detail string) *apiError {
	e := problem(http.StatusUnprocessableEntity, "hosted_limit", detail)
	e.Limit = limit
	return e
}

// MigrationSettings: Sources are the sources the operator allows (YUVA_MIGRATION_SOURCES);
// AllowPrivate lets mailboxes and autoconfig live on private addresses (YUVA_MIGRATION_ALLOW_PRIVATE).
// MaxActive, MaxBytes and MaxMessageBytes are YUVA_MIGRATION_MAX_ACTIVE, _MAX_BYTES and
// _MAX_MESSAGE_BYTES. Resolver, HTTP and RootCAs replace the system's, for tests.
type MigrationSettings struct {
	Sources         []string
	AllowPrivate    bool
	MaxActive       int
	MaxBytes        int64
	MaxMessageBytes int64
	Resolver        migrate.Resolver
	HTTP            migrate.HTTPDoer
	RootCAs         *x509.CertPool
	TestTimeout     time.Duration
	// SyncMaxDays and AuditDays are YUVA_MIGRATION_SYNC_MAX_DAYS and YUVA_MIGRATION_AUDIT_DAYS.
	SyncMaxDays int
	AuditDays   int
	// ProbeSMTP and ProbeFrom are the server's own mailer (YUVA_SMTP_*), which sends the test
	// message of a switch; without it the channel's SMTP account sends it.
	ProbeSMTP *email.SMTPConfig
	ProbeFrom string
	// SyncEvery is how often catch-up sync reads the old mailbox; 0 means two minutes.
	SyncEvery time.Duration
	// UploadMaxBytes is the largest export file (YUVA_MIGRATION_UPLOAD_MAX_BYTES); 0 means 20 GiB.
	// An operator turns file imports off by leaving file out of Sources.
	UploadMaxBytes int64
	// HelpdeskDial replaces the dialer help-desk readers connect with, for tests.
	HelpdeskDial func(ctx context.Context, network, addr string) (net.Conn, error)
	// HelpdeskSleep replaces how a help-desk reader waits for its request budget, for tests.
	HelpdeskSleep func(ctx context.Context, d time.Duration) error
	// MicrosoftClientID, MicrosoftClientSecret and MicrosoftTenant are the server's own Microsoft
	// app registration (YUVA_MICROSOFT_*); without them a team signs in with its own.
	MicrosoftClientID     string
	MicrosoftClientSecret string
	MicrosoftTenant       string
	// MicrosoftDial replaces the dialer Microsoft's token endpoint and IMAP server are reached
	// with, for tests.
	MicrosoftDial func(ctx context.Context, network, addr string) (net.Conn, error)
}

func migrationSettings(in MigrationSettings) MigrationSettings {
	if in.Sources == nil {
		in.Sources = builtMigrationSources
	}
	if in.Resolver == nil {
		in.Resolver = net.DefaultResolver
	}
	if in.HTTP == nil {
		in.HTTP = webhook.NewClient(in.AllowPrivate, in.Resolver)
	}
	if in.MaxMessageBytes <= 0 {
		in.MaxMessageBytes = defaultMigrationMaxMessage
	}
	if in.SyncMaxDays <= 0 {
		in.SyncMaxDays = defaultSyncMaxDays
	}
	if in.AuditDays <= 0 {
		in.AuditDays = defaultAuditDays
	}
	if in.SyncEvery <= 0 {
		in.SyncEvery = defaultSyncEvery
	}
	if in.UploadMaxBytes <= 0 {
		in.UploadMaxBytes = defaultUploadMaxBytes
	}
	return in
}

func migrationSecretContext(workspaceID, migrationID uuid.UUID) []byte {
	return append(append([]byte("migration:"), workspaceID[:]...), migrationID[:]...)
}

type migrationSecret struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func requireMigrationManager(p principal) error {
	return requireManagerOrFullKey(p)
}

func visibleMigration(ctx context.Context, q *store.Queries, p principal, id uuid.UUID, lock bool) (store.Migration, error) {
	if err := requireMigrationManager(p); err != nil {
		return store.Migration{}, err
	}
	get := q.GetMigration
	if lock {
		get = func(ctx context.Context, arg store.GetMigrationParams) (store.Migration, error) {
			return q.LockMigration(ctx, store.LockMigrationParams(arg))
		}
	}
	m, err := get(ctx, store.GetMigrationParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return m, errMigrationGone
	}
	return m, err
}

func defaultMigrationOptions() oas.MigrationOptions {
	return oas.MigrationOptions{
		SkipAutomatic: true, CloseAfterDays: defaultCloseAfter, KeepOriginals: true, QuietPeriodHours: defaultQuietPeriod,
		TeamAddresses: []oas.Email{}, People: []oas.MigrationPerson{}, Labels: []oas.MigrationLabelMapping{},
	}
}

func migrationOptions(raw []byte) oas.MigrationOptions {
	out := defaultMigrationOptions()
	_ = json.Unmarshal(raw, &out)
	if out.TeamAddresses == nil {
		out.TeamAddresses = []oas.Email{}
	}
	if out.People == nil {
		out.People = []oas.MigrationPerson{}
	}
	if out.Labels == nil {
		out.Labels = []oas.MigrationLabelMapping{}
	}
	return out
}

func migrationCounts(raw []byte) oas.MigrationCounts {
	var out oas.MigrationCounts
	_ = json.Unmarshal(raw, &out)
	return out
}

func migrationActions(m store.Migration, hasCreds bool) []oas.MigrationAction {
	out := []oas.MigrationAction{}
	file := fileMigration(m)
	if file {
		hasCreds = uploadReady(m)
	}
	desk := helpdeskMigration(m) || microsoftMigration(m)
	connect := oas.MigrationActionSignIn
	if tokenDesk(m) {
		connect = oas.MigrationActionSetCredentials
	}
	switch {
	case desk && (m.State == migrationDraft || m.State == migrationConnected):
		out = append(out, oas.MigrationActionUpdate, connect)
	case desk && m.State == migrationReady:
		out = append(out, oas.MigrationActionUpdate)
	case desk && m.State == migrationNeedsAttention:
		out = append(out, connect)
	case m.State == migrationDraft && file:
		out = append(out, oas.MigrationActionUpdate, oas.MigrationActionUpload)
	case m.State == migrationConnected && file, m.State == migrationReady:
		out = append(out, oas.MigrationActionUpdate)
	case m.State == migrationDraft, m.State == migrationConnected:
		out = append(out, oas.MigrationActionUpdate, oas.MigrationActionSetCredentials)
	case m.State == migrationNeedsAttention && !file:
		out = append(out, oas.MigrationActionSetCredentials)
	}
	if hasCreds && testableMigration(m.State) && !file {
		out = append(out, oas.MigrationActionTestConnection)
	}
	switch m.State {
	case migrationConnected:
		if hasCreds {
			out = append(out, oas.MigrationActionScan)
		}
	case migrationReady:
		out = append(out, oas.MigrationActionScan, oas.MigrationActionStart)
	case migrationCopying:
		out = append(out, oas.MigrationActionPause)
	case migrationPaused, migrationNeedsAttention:
		out = append(out, oas.MigrationActionResume)
	case migrationSyncing:
		out = append(out, oas.MigrationActionProbe, oas.MigrationActionFinish)
	case migrationCompleted, migrationCancelled:
		out = append(out, oas.MigrationActionRemoveHistory)
	}
	if m.State == migrationNeedsAttention && m.LiveFrom != nil {
		out = append(out, oas.MigrationActionFinish)
	}
	if c := migrationCounts(m.Counts); c.Failed > 0 && slices.Contains([]string{migrationCopying, migrationPaused, migrationSyncing}, m.State) {
		out = append(out, oas.MigrationActionRetry)
	}
	if cancellableMigration(m.State) {
		out = append(out, oas.MigrationActionCancel)
	}
	if !slices.Contains(runningMigrationStates, m.State) {
		out = append(out, oas.MigrationActionDelete)
	}
	return out
}

func cancellableMigration(state string) bool {
	switch state {
	case migrationConnected, migrationScanning, migrationReady, migrationCopying, migrationPaused, migrationNeedsAttention, migrationSyncing:
		return true
	}
	return false
}

func testableMigration(state string) bool {
	switch state {
	case migrationDraft, migrationConnected, migrationReady, migrationPaused, migrationNeedsAttention:
		return true
	}
	return false
}

// migrationExtra is what a migration's body shows from other tables.
type migrationExtra struct {
	cred        *store.ListMigrationCredentialInfosRow
	history     store.CountMigrationHistoryRow
	forwarding  string
	uploadParts int32
	uploadBytes int64
}

func (s *Server) migrationBody(m store.Migration, x migrationExtra) oas.Migration {
	cred := x.cred
	out := oas.Migration{
		Id: m.ID, Source: oas.MigrationSource(m.Source), Preset: oas.MigrationPresetId(m.Preset), SourceAddress: m.SourceAddress,
		InboxId: m.InboxID, ChannelId: m.ChannelID, State: oas.MigrationState(m.State), WaitingUntil: m.WaitingUntil,
		Options: migrationOptions(m.Options), Counts: migrationCounts(m.Counts), Actions: migrationActions(m, cred != nil),
		MemberId: m.CreatedByMemberID, ApiKeyId: m.CreatedByApiKeyID, Via: m.CreatedVia, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
		StartedAt: m.StartedAt, FinishedAt: m.FinishedAt, CredentialsDeletedAt: m.CredentialsDeletedAt, FollowsMigrationId: m.FollowsMigrationID,
	}
	if fileMigration(m) {
		out.Upload = fileBody(m, x.uploadParts, x.uploadBytes)
	}
	if m.StateReason != "" {
		out.StateReason = &m.StateReason
	}
	if doc := readSourceState(m.SourceState); doc.SignInResult != "" && (helpdeskMigration(m) || microsoftMigration(m)) {
		out.SignInResult = new(oas.MigrationSignInResult(doc.SignInResult))
		if doc.AdminConsentURL != "" && doc.SignInResult == string(oas.SignInAdminConsentRequired) {
			out.AdminConsentUrl = new(doc.AdminConsentURL)
		}
	}
	out.LiveFrom, out.SyncedAt, out.ProbeSentAt, out.ProbeSyncedAt, out.VerifiedAt = m.LiveFrom, m.SyncedAt, m.ProbeSentAt, m.ProbeSyncedAt, m.VerifiedAt
	if m.VerifiedRoute != nil {
		out.VerifiedRoute = new(oas.MigrationVerifiedRoute(*m.VerifiedRoute))
	}
	if m.State == migrationSyncing && m.LiveFrom != nil {
		out.SyncUntil = new(m.LiveFrom.AddDate(0, 0, s.migrations.SyncMaxDays))
	}
	if m.State == migrationSyncing {
		out.QuietUntil = quietUntil(m)
	}
	if x.forwarding != "" {
		out.ForwardingAddress = &x.forwarding
	}
	if c := readForwardingConfirmation(m.ForwardingConfirmation); c != nil {
		out.ForwardingConfirmation = c
	}
	if m.StartedAt != nil {
		out.ImportedConversations, out.RemovableConversations = &x.history.Conversations, &x.history.Removable
	}
	if doc := readEstimate(m.Estimate); doc != nil {
		out.Estimate = &doc.DryRun
		suggestions := doc.TeamSuggestions
		if suggestions == nil {
			suggestions = []oas.MigrationTeamSuggestion{}
		}
		out.TeamSuggestions = &suggestions
	}
	if m.State == migrationCopying {
		out.RatePerMinute, out.SecondsLeft = migrationRate(readCheckpoint(m.Checkpoint), out.Counts)
	}
	if cred != nil {
		c := oas.MigrationCredentials{
			Kind:              oas.MigrationCredentialsKind(cred.Kind),
			Server:            oas.MigrationServer{Host: cred.Host, Port: cred.Port, Tls: oas.MigrationTls(cred.Tls)},
			CertificateSha256: cred.CertificateSha256, ExpiresAt: cred.ExpiresAt, UpdatedAt: cred.UpdatedAt,
		}
		if len(cred.Scopes) > 0 {
			c.Scopes = &cred.Scopes
		}
		out.Credentials = &c
	}
	return out
}

func (s *Server) migrationBodies(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, rows []store.Migration) ([]oas.Migration, error) {
	ids := make([]uuid.UUID, len(rows))
	var channels, files []uuid.UUID
	for i, r := range rows {
		ids[i] = r.ID
		if r.ChannelID != nil {
			channels = append(channels, *r.ChannelID)
		}
		if fileMigration(r) {
			files = append(files, r.ID)
		}
	}
	creds, err := q.ListMigrationCredentialInfos(ctx, store.ListMigrationCredentialInfosParams{WorkspaceID: workspaceID, MigrationIds: ids})
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]migrationExtra, len(rows))
	for i := range creds {
		x := byID[creds[i].MigrationID]
		x.cred = &creds[i]
		byID[creds[i].MigrationID] = x
	}
	history, err := q.CountMigrationHistory(ctx, store.CountMigrationHistoryParams{WorkspaceID: workspaceID, MigrationIds: ids})
	if err != nil {
		return nil, err
	}
	for _, h := range history {
		x := byID[h.MigrationID]
		x.history = h
		byID[h.MigrationID] = x
	}
	if len(files) > 0 {
		totals, err := q.ListMigrationUploadTotals(ctx, store.ListMigrationUploadTotalsParams{WorkspaceID: workspaceID, MigrationIds: files})
		if err != nil {
			return nil, err
		}
		for _, t := range totals {
			x := byID[t.MigrationID]
			x.uploadParts, x.uploadBytes = t.Parts, t.Bytes
			byID[t.MigrationID] = x
		}
	}
	var emails map[uuid.UUID]store.EmailChannel
	if s.ingress.ForwardingDomain != "" && len(channels) > 0 {
		if emails, err = emailChannelsByID(ctx, q, workspaceID, channels); err != nil {
			return nil, err
		}
	}
	out := make([]oas.Migration, len(rows))
	for i, r := range rows {
		x := byID[r.ID]
		if r.ChannelID != nil {
			if e, ok := emails[*r.ChannelID]; ok {
				x.forwarding = s.forwardingAddress(e)
			}
		}
		out[i] = s.migrationBody(r, x)
	}
	return out, nil
}

func (s *Server) oneMigration(ctx context.Context, q *store.Queries, m store.Migration) (oas.Migration, error) {
	out, err := s.migrationBodies(ctx, q, m.WorkspaceID, []store.Migration{m})
	if err != nil {
		return oas.Migration{}, err
	}
	return out[0], nil
}

func (s *Server) migrationChanged(ctx context.Context, q *store.Queries, events *eventBatch, m store.Migration, previous *string) error {
	if previous != nil && *previous == m.State {
		return nil
	}
	return s.migrationEvent(ctx, q, events, m, previous)
}

// migrationEvent writes migration.updated with the migration as it is now.
func (s *Server) migrationEvent(ctx context.Context, q *store.Queries, events *eventBatch, m store.Migration, previous *string) error {
	body, err := s.oneMigration(ctx, q, m)
	if err != nil {
		return err
	}
	data := oas.MigrationEventData{Migration: body}
	if previous != nil {
		data.PreviousState = new(oas.MigrationState(*previous))
	}
	events.add(eventMigrationUpdated, nil, nil, data)
	if m.InboxID == nil || (previous != nil && *previous == m.State) || (!importState(m.State) && (previous == nil || !importState(*previous))) {
		return nil
	}
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: m.WorkspaceID, ID: *m.InboxID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	inbox, err := memberInbox(ctx, q, in)
	if err != nil {
		return err
	}
	events.add(realtime.InboxUpdated, &inbox.Id, nil, inbox)
	return nil
}

func importState(state string) bool {
	return slices.Contains([]string{migrationScanning, migrationCopying, migrationPaused, migrationNeedsAttention, migrationSyncing}, state)
}

func (s *Server) migrationAudit(ctx context.Context, q *store.Queries, p principal, migrationID uuid.UUID, action string, detail any) (int64, error) {
	var keyID *uuid.UUID
	if p.isKey() {
		keyID = &p.keyID
	}
	if detail == nil {
		detail = map[string]any{}
	}
	return q.AddMigrationAudit(ctx, store.AddMigrationAuditParams{
		WorkspaceID: p.workspaceID, MigrationID: migrationID, Action: action, MemberID: p.memberRef(), ApiKeyID: keyID,
		Via: p.viaClient(), Detail: mustJSON(detail), Now: s.now(),
	})
}

func (s *Server) migrationSourceAllowed(source string) bool {
	return slices.Contains(builtMigrationSources, source) && slices.Contains(s.migrations.Sources, source)
}

// migrationTarget checks the inbox and the e-mail channel a migration writes to: the channel must
// belong to the inbox and receive mail at the mailbox address.
func migrationTarget(ctx context.Context, q *store.Queries, p principal, inboxID, channelID *uuid.UUID, address string) (*uuid.UUID, *uuid.UUID, error) {
	if channelID != nil {
		ch, err := q.GetChannel(ctx, store.GetChannelParams{WorkspaceID: p.workspaceID, ID: *channelID})
		if store.IsNotFound(err) {
			return nil, nil, errChannelGone
		}
		if err != nil {
			return nil, nil, err
		}
		if inboxID != nil && *inboxID != ch.InboxID {
			return nil, nil, errValidation("channel_id is not a channel of inbox_id")
		}
		if ch.Kind != string(oas.ChannelKindEmail) {
			return nil, nil, errValidation("channel_id must be an e-mail channel")
		}
		e, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: p.workspaceID, ChannelID: ch.ID})
		if store.IsNotFound(err) {
			return nil, nil, errValidation("channel_id must be an e-mail channel")
		}
		if err != nil {
			return nil, nil, err
		}
		if !strings.EqualFold(e.Address, address) {
			return nil, nil, errValidation("the channel must receive mail at the mailbox address " + address)
		}
		inbox := ch.InboxID
		return &inbox, &ch.ID, nil
	}
	if inboxID != nil {
		if _, err := visibleInbox(ctx, q, p, *inboxID); err != nil {
			return nil, nil, err
		}
	}
	return inboxID, nil, nil
}

func applyMigrationOptions(ctx context.Context, q *store.Queries, p principal, cur oas.MigrationOptions, in *oas.MigrationOptionsInput) (oas.MigrationOptions, bool, error) {
	if in == nil {
		return cur, false, nil
	}
	out := cur
	if in.Folders.IsSpecified() {
		if in.Folders.IsNull() {
			out.Folders = nil
		} else {
			v, _ := in.Folders.Get()
			if len(v) > maxMigrationFolders {
				return out, false, errValidation("options.folders names at most 1,000 folders")
			}
			var folders []string
			for _, f := range v {
				if n := len([]rune(f)); n < 1 || n > 1000 {
					return out, false, errValidation("a folder name is 1 to 1,000 characters")
				}
				if !slices.Contains(folders, f) {
					folders = append(folders, f)
				}
			}
			if folders == nil {
				folders = []string{}
			}
			out.Folders = &folders
		}
	}
	if in.Since.IsSpecified() {
		if in.Since.IsNull() {
			out.Since = nil
		} else {
			v, _ := in.Since.Get()
			v = v.UTC()
			out.Since = &v
		}
	}
	if in.SkipAutomatic != nil {
		out.SkipAutomatic = *in.SkipAutomatic
	}
	if in.CloseAfterDays != nil {
		if *in.CloseAfterDays < 0 || *in.CloseAfterDays > 3650 {
			return out, false, errValidation("options.close_after_days must be 0 to 3650")
		}
		out.CloseAfterDays = *in.CloseAfterDays
	}
	if in.KeepOriginals != nil {
		out.KeepOriginals = *in.KeepOriginals
	}
	if in.QuietPeriodHours != nil {
		if *in.QuietPeriodHours < 1 || *in.QuietPeriodHours > 336 {
			return out, false, errValidation("options.quiet_period_hours must be 1 to 336")
		}
		out.QuietPeriodHours = *in.QuietPeriodHours
	}
	if in.TeamAddresses != nil {
		if len(*in.TeamAddresses) > 100 {
			return out, false, errValidation("options.team_addresses holds at most 100 addresses")
		}
		addrs := []oas.Email{}
		for _, a := range *in.TeamAddresses {
			n, err := normalizeEmail(a)
			if err != nil {
				return out, false, errValidation("options.team_addresses holds an address that is not valid")
			}
			if !slices.Contains(addrs, oas.Email(n)) {
				addrs = append(addrs, oas.Email(n))
			}
		}
		out.TeamAddresses = addrs
	}
	if in.People != nil {
		if len(*in.People) > 200 {
			return out, false, errValidation("options.people holds at most 200 addresses")
		}
		people := []oas.MigrationPerson{}
		seen := map[string]bool{}
		for _, person := range *in.People {
			n, err := normalizeEmail(person.Address)
			if err != nil {
				return out, false, errValidation("options.people holds an address that is not valid")
			}
			if seen[n] {
				return out, false, errValidation("options.people names " + n + " twice")
			}
			seen[n] = true
			if person.MemberId != nil {
				if _, err := q.GetMember(ctx, store.GetMemberParams{WorkspaceID: p.workspaceID, ID: *person.MemberId}); store.IsNotFound(err) {
					return out, false, errValidation("options.people names a member who is not in the workspace")
				} else if err != nil {
					return out, false, err
				}
			}
			people = append(people, oas.MigrationPerson{Address: oas.Email(n), MemberId: person.MemberId})
		}
		out.People = people
	}
	if in.Labels != nil {
		if len(*in.Labels) > maxMigrationFolders {
			return out, false, errValidation("options.labels holds at most 1,000 folders")
		}
		labels := []oas.MigrationLabelMapping{}
		seen := map[string]bool{}
		for _, l := range *in.Labels {
			if n := len([]rune(l.Folder)); n < 1 || n > 1000 {
				return out, false, errValidation("a folder name is 1 to 1,000 characters")
			}
			if seen[l.Folder] {
				return out, false, errValidation("options.labels names " + l.Folder + " twice")
			}
			seen[l.Folder] = true
			switch l.Action {
			case oas.MigrationLabelExisting:
				if l.LabelId == nil {
					return out, false, errValidation("an existing label needs label_id")
				}
				n, err := q.CountLabels(ctx, store.CountLabelsParams{WorkspaceID: p.workspaceID, Ids: []uuid.UUID{*l.LabelId}})
				if err != nil {
					return out, false, err
				}
				if n != 1 {
					return out, false, errValidation("options.labels names a label that does not exist")
				}
			case oas.MigrationLabelCreate, oas.MigrationLabelNone:
				l.LabelId = nil
			default:
				return out, false, errValidation("a label action is create, existing or none")
			}
			labels = append(labels, l)
		}
		out.Labels = labels
	}
	if in.Inboxes != nil {
		if len(*in.Inboxes) > maxMigrationFolders {
			return out, false, errValidation("options.inboxes holds at most 1,000 brands or mailboxes")
		}
		inboxes := []oas.MigrationInboxMapping{}
		seen := map[string]bool{}
		for _, ib := range *in.Inboxes {
			if n := len([]rune(ib.Folder)); n < 1 || n > 1000 {
				return out, false, errValidation("a folder name is 1 to 1,000 characters")
			}
			if seen[ib.Folder] {
				return out, false, errValidation("options.inboxes names " + ib.Folder + " twice")
			}
			seen[ib.Folder] = true
			if _, err := visibleInbox(ctx, q, p, ib.InboxId); err != nil {
				return out, false, errValidation("options.inboxes names an inbox that does not exist")
			}
			inboxes = append(inboxes, ib)
		}
		out.Inboxes = &inboxes
	}
	readChanged := !sameFolders(cur.Folders, out.Folders) || !sameTime(cur.Since, out.Since) || cur.SkipAutomatic != out.SkipAutomatic
	return out, readChanged, nil
}

func sameFolders(a, b *[]string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return slices.Equal(*a, *b)
}

func (s *Server) ListMigrationPresets(ctx context.Context, req oas.ListMigrationPresetsRequestObject) (oas.ListMigrationPresetsResponseObject, error) {
	if err := requireMigrationManager(principalFrom(ctx)); err != nil {
		return nil, err
	}
	out := oas.ListMigrationPresets200JSONResponse{Items: []oas.MigrationPreset{}}
	for _, p := range migrate.Presets {
		if !s.migrationSourceAllowed(p.Source) {
			continue
		}
		body := presetBody(p)
		body.Available = s.presetAvailable(p)
		if slices.Contains(p.Auth, migrate.AuthOAuth) || slices.Contains(p.Auth, migrate.AuthMicrosoft) {
			body.RedirectUrl, body.Scopes, body.TeamClient = new(s.signInRedirectURL()), new(slices.Clone(p.Scopes)), new(true)
			if p.Subdomain {
				body.Subdomain = new(true)
			}
		}
		if len(p.TokenFields) > 0 {
			fields := make([]oas.MigrationPresetTokenFields, len(p.TokenFields))
			for i, f := range p.TokenFields {
				fields[i] = oas.MigrationPresetTokenFields(f)
			}
			body.TokenFields = &fields
		}
		if _, ok := s.microsoftServerApp(); ok && p.ID == migrate.PresetMicrosoft {
			body.TeamClient = new(false)
		}
		out.Items = append(out.Items, body)
	}
	return out, nil
}

func presetBody(p migrate.Preset) oas.MigrationPreset {
	out := oas.MigrationPreset{
		Id: oas.MigrationPresetId(p.ID), Source: oas.MigrationSource(p.Source), Name: p.Name, Servers: []oas.MigrationServer{},
		Auth: authWays(p.Auth), AppPassword: p.AppPassword, Username: oas.MigrationPresetUsername(p.Username),
		Folders: p.Folders, Guide: p.Guide, Available: p.Available,
		Budget: oas.MigrationBudget{Connections: int32(p.Budget.Connections)},
	}
	if p.Budget.RequestsPerMinute > 0 {
		out.Budget.RequestsPerMinute = new(int32(p.Budget.RequestsPerMinute))
	}
	if p.Budget.IncrementalPerMinute > 0 {
		out.Budget.IncrementalPerMinute = new(int32(p.Budget.IncrementalPerMinute))
	}
	if p.Budget.RequestsPerDay > 0 {
		out.Budget.RequestsPerDay = new(int32(p.Budget.RequestsPerDay))
	}
	for _, sv := range p.Servers {
		out.Servers = append(out.Servers, serverBody(sv))
	}
	if p.Budget.BytesPerDay > 0 {
		out.Budget.BytesPerDay = &p.Budget.BytesPerDay
	}
	if p.Budget.BytesPerSecond > 0 {
		out.Budget.BytesPerSecond = &p.Budget.BytesPerSecond
	}
	return out
}

func serverBody(sv migrate.Server) oas.MigrationServer {
	return oas.MigrationServer{Host: sv.Host, Port: int32(sv.Port), Tls: oas.MigrationTls(sv.TLS)}
}

func authWays(in []string) []oas.MigrationAuth {
	out := make([]oas.MigrationAuth, len(in))
	for i, a := range in {
		out[i] = oas.MigrationAuth(a)
	}
	return out
}

func (s *Server) DetectMigrationSource(ctx context.Context, req oas.DetectMigrationSourceRequestObject) (oas.DetectMigrationSourceResponseObject, error) {
	if err := requireMigrationManager(principalFrom(ctx)); err != nil {
		return nil, err
	}
	addr, err := normalizeEmail(req.Body.Address)
	if err != nil {
		return nil, errValidation("address is not a valid address")
	}
	ctx, cancel := context.WithTimeout(ctx, migrate.TestTimeout)
	defer cancel()
	d := migrate.Detector{
		Resolver: s.migrations.Resolver, HTTP: s.migrations.HTTP, AllowPrivate: s.migrations.AllowPrivate, MicrosoftAvailable: s.microsoftAvailable(),
	}.Detect(ctx, addr)
	out := oas.DetectMigrationSource200JSONResponse{
		Address: d.Address, Domain: d.Domain, Preset: oas.MigrationPresetId(d.Preset), Name: d.Name, Available: d.Available,
		Auth: authWays(d.Auth), Guide: d.Guide, Mx: d.MX, FoundBy: oas.MigrationDetectionFoundBy(d.FoundBy),
	}
	if d.Note != "" {
		out.Note = new(oas.MigrationDetectionNote(d.Note))
	}
	if d.Server != nil {
		out.Server = new(serverBody(*d.Server))
	}
	return out, nil
}

func (s *Server) ListMigrations(ctx context.Context, req oas.ListMigrationsRequestObject) (oas.ListMigrationsResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireMigrationManager(p); err != nil {
		return nil, err
	}
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	states, err := enumList(req.Params.State, "state must be a migration state")
	if err != nil {
		return nil, err
	}
	if states == nil {
		states = []string{}
	}
	rows, err := s.st.ListMigrations(ctx, store.ListMigrationsParams{WorkspaceID: p.workspaceID, States: states, CursorAt: at, CursorID: id, Lim: lim + 1})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(rows[lim-1].CreatedAt, rows[lim-1].ID)
		next = &c
	}
	items, err := s.migrationBodies(ctx, s.st.Queries, p.workspaceID, rows)
	if err != nil {
		return nil, err
	}
	return oas.ListMigrations200JSONResponse{Items: items, NextCursor: next}, nil
}

func (s *Server) GetMigration(ctx context.Context, req oas.GetMigrationRequestObject) (oas.GetMigrationResponseObject, error) {
	p := principalFrom(ctx)
	m, err := visibleMigration(ctx, s.st.Queries, p, req.MigrationId, false)
	if err != nil {
		return nil, err
	}
	out, err := s.oneMigration(ctx, s.st.Queries, m)
	if err != nil {
		return nil, err
	}
	return oas.GetMigration200JSONResponse(out), nil
}

func (s *Server) checkMigrationPreset(source, preset string) error {
	if !s.migrationSourceAllowed(source) {
		return errHostedLimit(oas.LimitMigrationSources, "this server does not offer "+source+" migrations (YUVA_MIGRATION_SOURCES)")
	}
	p, ok := migrate.LookupPreset(preset)
	if !ok || p.Source != source {
		return errValidation("preset must be a preset of source " + source)
	}
	if !s.presetAvailable(p) {
		return errProviderNotConfigured
	}
	return nil
}

func (s *Server) CreateMigration(ctx context.Context, req oas.CreateMigrationRequestObject) (oas.CreateMigrationResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireMigrationManager(p); err != nil {
		return nil, err
	}
	b := req.Body
	source := string(oas.MigrationSourceImap)
	if b.Source != nil {
		if !b.Source.Valid() {
			return nil, errValidation("source must be imap, file, zendesk, helpscout, chatwoot or front")
		}
		source = string(*b.Source)
	}
	if !b.Preset.Valid() {
		return nil, errValidation("preset is not a known preset")
	}
	if err := s.checkMigrationPreset(source, string(b.Preset)); err != nil {
		return nil, err
	}
	address, err := normalizeEmail(b.SourceAddress)
	if err != nil {
		return nil, errValidation("source_address is not a valid address")
	}
	var out oas.Migration
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		inbox, channel, err := migrationTarget(ctx, q, p, b.InboxId, b.ChannelId, address)
		if err != nil {
			return err
		}
		opts, _, err := applyMigrationOptions(ctx, q, p, defaultMigrationOptions(), b.Options)
		if err != nil {
			return err
		}
		var follows *uuid.UUID
		if source == string(oas.MigrationSourceImap) && (b.Options == nil || !b.Options.Since.IsSpecified()) {
			var since *time.Time
			if follows, since, err = followFile(ctx, q, p.workspaceID, channel, address); err != nil {
				return err
			}
			if since != nil {
				opts.Since = since
			}
		}
		var keyID *uuid.UUID
		if p.isKey() {
			keyID = &p.keyID
		}
		m, err := q.CreateMigration(ctx, store.CreateMigrationParams{
			ID: newID(), WorkspaceID: p.workspaceID, Source: source, Preset: string(b.Preset), SourceAddress: address,
			InboxID: inbox, ChannelID: channel, Options: mustJSON(opts), MemberID: p.memberRef(), ApiKeyID: keyID,
			Via: p.viaClient(), Now: s.now(),
		})
		if err != nil {
			return err
		}
		if follows != nil {
			if err := q.SetMigrationFollows(ctx, store.SetMigrationFollowsParams{WorkspaceID: m.WorkspaceID, ID: m.ID, FollowsMigrationID: follows}); err != nil {
				return err
			}
			m.FollowsMigrationID = follows
		}
		if _, err := s.migrationAudit(ctx, q, p, m.ID, "created", map[string]any{"source": source, "preset": m.Preset}); err != nil {
			return err
		}
		if out, err = s.oneMigration(ctx, q, m); err != nil {
			return err
		}
		return s.migrationChanged(ctx, q, events, m, nil)
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateMigration201JSONResponse(out), nil
}

func (s *Server) UpdateMigration(ctx context.Context, req oas.UpdateMigrationRequestObject) (oas.UpdateMigrationResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	var out oas.Migration
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		cur, err := visibleMigration(ctx, q, p, req.MigrationId, true)
		if err != nil {
			return err
		}
		if b.Preset == nil && b.SourceAddress == nil && !b.InboxId.IsSpecified() && !b.ChannelId.IsSpecified() && b.Options == nil {
			return errValidation("name at least one change: preset, source_address, inbox_id, channel_id or options")
		}
		if cur.State != migrationDraft && cur.State != migrationConnected && cur.State != migrationReady {
			return errMigrationState("a migration can be changed only while draft, connected or ready")
		}
		preset, address := cur.Preset, cur.SourceAddress
		if b.Preset != nil || b.SourceAddress != nil {
			if cur.State != migrationDraft {
				return errMigrationState("the mailbox and its preset can be changed only while the migration is a draft")
			}
		}
		if b.Preset != nil {
			if !b.Preset.Valid() {
				return errValidation("preset is not a known preset")
			}
			if err := s.checkMigrationPreset(cur.Source, string(*b.Preset)); err != nil {
				return err
			}
			preset = string(*b.Preset)
		}
		if b.SourceAddress != nil {
			if address, err = normalizeEmail(*b.SourceAddress); err != nil {
				return errValidation("source_address is not a valid address")
			}
		}
		inboxID, channelID := cur.InboxID, cur.ChannelID
		if b.InboxId.IsSpecified() {
			inboxID = nil
			if v, err := b.InboxId.Get(); err == nil {
				inboxID = &v
			}
			if inboxID == nil || (!b.ChannelId.IsSpecified() && !sameID(inboxID, cur.InboxID)) {
				channelID = nil
			}
		}
		if b.ChannelId.IsSpecified() {
			channelID = nil
			if v, err := b.ChannelId.Get(); err == nil {
				channelID = &v
			}
		}
		if inboxID, channelID, err = migrationTarget(ctx, q, p, inboxID, channelID, address); err != nil {
			return err
		}
		opts, readChanged, err := applyMigrationOptions(ctx, q, p, migrationOptions(cur.Options), b.Options)
		if err != nil {
			return err
		}
		follows := cur.FollowsMigrationID
		if cur.Source == string(oas.MigrationSourceImap) && follows == nil && opts.Since == nil && !sameID(channelID, cur.ChannelID) &&
			(b.Options == nil || !b.Options.Since.IsSpecified()) {
			var since *time.Time
			if follows, since, err = followFile(ctx, q, p.workspaceID, channelID, address); err != nil {
				return err
			}
			if since != nil {
				opts.Since, readChanged = since, true
			}
		}
		state := cur.State
		if state == migrationReady && (readChanged || b.Options != nil || b.InboxId.IsSpecified() || b.ChannelId.IsSpecified()) {
			state = migrationConnected
		}
		m, err := q.UpdateMigration(ctx, store.UpdateMigrationParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, Preset: preset, SourceAddress: address, InboxID: inboxID, ChannelID: channelID,
			Options: mustJSON(opts), State: state, Now: s.now(),
		})
		if err != nil {
			return err
		}
		if !sameID(follows, cur.FollowsMigrationID) {
			if err := q.SetMigrationFollows(ctx, store.SetMigrationFollowsParams{WorkspaceID: m.WorkspaceID, ID: m.ID, FollowsMigrationID: follows}); err != nil {
				return err
			}
			m.FollowsMigrationID = follows
		}
		if out, err = s.oneMigration(ctx, q, m); err != nil {
			return err
		}
		return s.migrationChanged(ctx, q, events, m, &cur.State)
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateMigration200JSONResponse(out), nil
}

func (s *Server) DeleteMigration(ctx context.Context, req oas.DeleteMigrationRequestObject) (oas.DeleteMigrationResponseObject, error) {
	p := principalFrom(ctx)
	var parts []string
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		m, err := visibleMigration(ctx, q, p, req.MigrationId, true)
		if err != nil {
			return err
		}
		if slices.Contains(runningMigrationStates, m.State) {
			return errMigrationState("a migration that is scanning, copying, syncing or removing history cannot be deleted; pause or cancel it first")
		}
		rows, err := q.ListMigrationUploadParts(ctx, store.ListMigrationUploadPartsParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID})
		if err != nil {
			return err
		}
		for _, r := range rows {
			parts = append(parts, r.StorageKey)
		}
		if _, err := q.DeleteMigration(ctx, store.DeleteMigrationParams{WorkspaceID: p.workspaceID, ID: m.ID}); err != nil {
			return err
		}
		_, err = s.migrationAudit(ctx, q, p, m.ID, "deleted", map[string]any{"state": m.State})
		return err
	})
	if err != nil {
		return nil, err
	}
	s.deleteObjects(ctx, parts)
	return oas.DeleteMigration204Response{}, nil
}

func normalizeFingerprint(v string) (string, bool) {
	v = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(v), ":", ""))
	b, err := hex.DecodeString(v)
	return v, err == nil && len(b) == 32
}

// migrationServer picks the server credentials are for: one of the preset's servers, or for
// other servers the one given.
func migrationServer(preset migrate.Preset, b *oas.MigrationCredentialsInput) (migrate.Server, error) {
	host := ""
	if b.Host != nil {
		host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(*b.Host), "."))
	}
	if len(preset.Servers) > 0 {
		sv, ok := preset.ServerFor(host)
		if !ok {
			return sv, errValidation("host must be one of the preset's servers")
		}
		if (b.Port != nil && int(*b.Port) != sv.Port) || (b.Tls != nil && string(*b.Tls) != sv.TLS) {
			return sv, errValidation("the preset's server takes its own port and TLS")
		}
		return sv, nil
	}
	if host == "" || !migrate.ValidHost(host) {
		return migrate.Server{}, errValidation("host must be the IMAP server's name or address")
	}
	sv := migrate.Server{Host: host, Port: 993, TLS: string(oas.ImapTls)}
	if b.Tls != nil {
		if !b.Tls.Valid() {
			return sv, errValidation("tls must be tls or starttls")
		}
		sv.TLS = string(*b.Tls)
		if sv.TLS == string(oas.ImapStarttls) {
			sv.Port = 143
		}
	}
	if b.Port != nil {
		if *b.Port < 1 || *b.Port > 65535 {
			return sv, errValidation("port must be 1 to 65535")
		}
		sv.Port = int(*b.Port)
	}
	return sv, nil
}

func (s *Server) SetMigrationCredentials(ctx context.Context, req oas.SetMigrationCredentialsRequestObject) (oas.SetMigrationCredentialsResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	if b.Kind != nil && *b.Kind == oas.CredentialsInputToken {
		return s.setTokenCredentials(ctx, p, req.MigrationId, b)
	}
	if b.Kind != nil && *b.Kind != oas.CredentialsInputPassword {
		return nil, errValidation("kind must be password or token")
	}
	if b.Token != nil || b.Secret != nil || b.Subdomain != nil || b.BaseUrl != nil {
		return nil, errValidation("token, secret, subdomain and base_url go with kind token")
	}
	username := strings.TrimSpace(deref(b.Username))
	if n := len([]rune(username)); n < 1 || n > 320 {
		return nil, errValidation("username must be 1 to 320 characters")
	}
	if b.Password == nil || len(*b.Password) < 1 || len(*b.Password) > 1000 {
		return nil, errValidation("password must be 1 to 1,000 characters")
	}
	var pinned *string
	if b.CertificateSha256 != nil && strings.TrimSpace(*b.CertificateSha256) != "" {
		fp, ok := normalizeFingerprint(*b.CertificateSha256)
		if !ok {
			return nil, errValidation("certificate_sha256 must be a SHA-256 in hexadecimal")
		}
		pinned = &fp
	}
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		m, err := visibleMigration(ctx, q, p, req.MigrationId, true)
		if err != nil {
			return err
		}
		if fileMigration(m) {
			return errFileNoCredentials
		}
		if d, ok := deskOf(m); ok && d.Auth == helpdesk.AuthToken {
			return errMigrationState(d.Name + " connects with an API token: kind token, not a password")
		}
		if helpdeskMigration(m) {
			return errHelpdeskNoPassword
		}
		if microsoftMigration(m) {
			return errMicrosoftNoPassword
		}
		preset, ok := migrate.LookupPreset(m.Preset)
		if !ok || !preset.Available || !slices.Contains(preset.Auth, migrate.AuthPassword) {
			return errProviderNotConfigured
		}
		sv, err := migrationServer(preset, b)
		if err != nil {
			return err
		}
		switch m.State {
		case migrationDraft, migrationConnected:
		case migrationNeedsAttention:
			old, err := q.GetMigrationCredentials(ctx, store.GetMigrationCredentialsParams{WorkspaceID: p.workspaceID, MigrationID: m.ID})
			if err != nil && !store.IsNotFound(err) {
				return err
			}
			if err == nil {
				plain, err := s.secrets.Open(old.Sealed, migrationSecretContext(p.workspaceID, m.ID))
				var prev migrationSecret
				if err != nil || json.Unmarshal(plain, &prev) != nil || !strings.EqualFold(prev.Username, username) || !strings.EqualFold(old.Host, sv.Host) {
					return errMigrationState("reconnect the same account on the same server; start a new migration for another mailbox")
				}
			}
		default:
			return errMigrationState("credentials can be set while the migration is a draft, connected or needs attention")
		}
		sealed := s.secrets.Seal(mustJSON(migrationSecret{Username: username, Password: *b.Password}), migrationSecretContext(p.workspaceID, m.ID))
		if err := q.SetMigrationCredentials(ctx, store.SetMigrationCredentialsParams{
			WorkspaceID: p.workspaceID, MigrationID: m.ID, Kind: string(oas.CredentialsPassword), Sealed: sealed,
			Host: sv.Host, Port: int32(sv.Port), Tls: sv.TLS, CertificateSha256: pinned, Scopes: []string{}, Now: s.now(),
		}); err != nil {
			return err
		}
		if _, err := s.migrationAudit(ctx, q, p, m.ID, "credentials_set", map[string]any{"host": sv.Host, "port": sv.Port, "tls": sv.TLS, "pinned": pinned != nil}); err != nil {
			return err
		}
		if m.State != migrationConnected {
			return nil
		}
		updated, err := q.SetMigrationState(ctx, store.SetMigrationStateParams{WorkspaceID: p.workspaceID, ID: m.ID, State: migrationDraft, Now: s.now()})
		if err != nil {
			return err
		}
		return s.migrationChanged(ctx, q, events, updated, &m.State)
	})
	if err != nil {
		return nil, err
	}
	return oas.SetMigrationCredentials204Response{}, nil
}

// claimMigrationTest refuses a test past the limits and records the one it lets through, so a
// server never becomes a tool for guessing passwords.
func (s *Server) claimMigrationTest(ctx context.Context, q *store.Queries, p principal, m store.Migration) (int64, error) {
	now := s.now()
	counts, err := q.CountMigrationTests(ctx, store.CountMigrationTestsParams{WorkspaceID: p.workspaceID, MigrationID: m.ID, Since: now.Add(-testWindow)})
	if err != nil {
		return 0, err
	}
	if counts.Migration >= testsPerMigration {
		return 0, rateLimited(counts.MigrationOldest.Add(testWindow).Sub(now))
	}
	if counts.Workspace >= testsPerWorkspace {
		return 0, rateLimited(counts.WorkspaceOldest.Add(testWindow).Sub(now))
	}
	recent, err := q.ListRecentMigrationTests(ctx, store.ListRecentMigrationTestsParams{WorkspaceID: p.workspaceID, MigrationID: m.ID, Lim: authFailuresToLock})
	if err != nil {
		return 0, err
	}
	if len(recent) == authFailuresToLock && now.Before(recent[0].CreatedAt.Add(authFailureLockout)) &&
		!slices.ContainsFunc(recent, func(r store.ListRecentMigrationTestsRow) bool { return r.Code != migrate.CodeAuthFailed }) {
		return 0, rateLimited(recent[0].CreatedAt.Add(authFailureLockout).Sub(now))
	}
	return s.migrationAudit(ctx, q, p, m.ID, "tested", map[string]any{})
}

func (s *Server) TestMigrationConnection(ctx context.Context, req oas.TestMigrationConnectionRequestObject) (oas.TestMigrationConnectionResponseObject, error) {
	p := principalFrom(ctx)
	if m, err := visibleMigration(ctx, s.st.Queries, p, req.MigrationId, false); err != nil {
		return nil, err
	} else if helpdeskMigration(m) {
		return s.testHelpdeskConnection(ctx, p, req.MigrationId)
	}
	var (
		auditID   int64
		target    migrate.Target
		credAt    time.Time
		migration store.Migration
		oauthCred *store.MigrationCredential
	)
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		m, err := visibleMigration(ctx, q, p, req.MigrationId, true)
		if err != nil {
			return err
		}
		if fileMigration(m) {
			return errFileNoCredentials
		}
		if !testableMigration(m.State) {
			return errMigrationState("the connection can be tested before the copy starts, or while it is paused or needs attention")
		}
		cred, err := q.GetMigrationCredentials(ctx, store.GetMigrationCredentialsParams{WorkspaceID: p.workspaceID, MigrationID: m.ID})
		if store.IsNotFound(err) {
			return errMigrationNoCreds
		}
		if err != nil {
			return err
		}
		preset, ok := migrate.LookupPreset(m.Preset)
		switch {
		case ok && microsoftMigration(m) && cred.Kind == string(oas.CredentialsOauth):
			oauthCred, migration = &cred, m
			target = migrate.Target{Preset: preset, Server: migrate.Server{Host: cred.Host, Port: int(cred.Port), TLS: cred.Tls}, Username: m.SourceAddress}
		case !ok || cred.Kind != string(oas.CredentialsPassword):
			return errProviderNotConfigured
		default:
			plain, err := s.secrets.Open(cred.Sealed, migrationSecretContext(p.workspaceID, m.ID))
			if err != nil {
				return err
			}
			var sec migrationSecret
			if err := json.Unmarshal(plain, &sec); err != nil {
				return err
			}
			target = migrate.Target{
				Preset: preset, Server: migrate.Server{Host: cred.Host, Port: int(cred.Port), TLS: cred.Tls},
				Username: sec.Username, Password: sec.Password,
			}
		}
		if cred.CertificateSha256 != nil {
			target.CertificateSHA256 = *cred.CertificateSha256
		}
		credAt = cred.UpdatedAt
		auditID, err = s.claimMigrationTest(ctx, q, p, m)
		return err
	})
	if err != nil {
		return nil, err
	}
	tester := migrate.Tester{
		Resolver: s.migrations.Resolver, AllowPrivate: s.migrations.AllowPrivate, RootCAs: s.migrations.RootCAs, Timeout: s.migrations.TestTimeout,
		Dial: s.sourceDial(target.Server.Host),
	}
	var res migrate.TestResult
	if oauthCred != nil {
		sec, err := s.microsoftCredentials(context.WithoutCancel(ctx), migration, *oauthCred)
		if err != nil {
			res = microsoftTestFailure(err, sec)
		} else {
			target.Token = sec.Tokens.Access
			res = tester.Run(context.WithoutCancel(ctx), target)
		}
	} else {
		res = tester.Run(context.WithoutCancel(ctx), target)
	}
	target = migrate.Target{}
	failed := res.Failed()
	detail := map[string]any{"ok": res.OK}
	if !res.OK {
		detail["step"], detail["code"] = failed.Step, failed.Code
	}
	var out oas.Migration
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if err := q.SetMigrationAuditDetail(ctx, store.SetMigrationAuditDetailParams{WorkspaceID: p.workspaceID, ID: auditID, Detail: mustJSON(detail)}); err != nil {
			return err
		}
		m, err := visibleMigration(ctx, q, p, req.MigrationId, true)
		if err != nil {
			return err
		}
		if res.OK && (m.State == migrationDraft || m.State == migrationConnected) {
			listed := make([]listedFolder, len(res.List))
			for i, f := range res.List {
				listed[i] = listedFolder{Name: f.Name, Delim: f.Delim, Kind: f.Kind, Messages: f.Messages}
			}
			if err := s.recordFolders(ctx, q, m, listed); err != nil {
				return err
			}
		}
		if res.OK && m.State == migrationDraft {
			cred, err := q.GetMigrationCredentials(ctx, store.GetMigrationCredentialsParams{WorkspaceID: p.workspaceID, MigrationID: m.ID})
			if err == nil && cred.UpdatedAt.Equal(credAt) {
				updated, err := q.SetMigrationState(ctx, store.SetMigrationStateParams{WorkspaceID: p.workspaceID, ID: m.ID, State: migrationConnected, Now: s.now()})
				if err != nil {
					return err
				}
				if err := s.migrationChanged(ctx, q, events, updated, &m.State); err != nil {
					return err
				}
				m = updated
			} else if err != nil && !store.IsNotFound(err) {
				return err
			}
		}
		out, err = s.oneMigration(ctx, q, m)
		return err
	})
	if err != nil {
		return nil, err
	}
	body := testResultBody(res, out)
	if res.Certificate != "" && failed.Code == migrate.CodeTLSFailed {
		body.CertificateSha256 = new(res.Certificate)
	}
	return body, nil
}

func testResultBody(res migrate.TestResult, out oas.Migration) oas.TestMigrationConnection200JSONResponse {
	body := oas.TestMigrationConnection200JSONResponse{Ok: res.OK, Steps: make([]oas.MigrationTestStep, len(res.Steps)), Migration: out}
	for i, st := range res.Steps {
		step := oas.MigrationTestStep{Step: oas.MigrationTestStepStep(st.Step), Ok: st.OK}
		if st.Code != "" {
			step.Code = new(oas.MigrationTestCode(st.Code))
		}
		if st.Detail != "" {
			step.Detail = new(st.Detail)
		}
		body.Steps[i] = step
	}
	if res.OK {
		body.Folders = new(int32(res.Folders))
		body.Messages = new(res.Messages)
	}
	return body
}

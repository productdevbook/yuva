package api

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/productdevbook/yuva/api/internal/awsses"
	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/metrics"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

// SESClient is the AWS side of tenant management: awsses.Client on a server with YUVA_SES_* set.
type SESClient interface {
	CallerAccount(ctx context.Context) (string, error)
	AccessKeyAccount(ctx context.Context, keyID string) (string, error)
	CreateTenant(ctx context.Context, name string, tags map[string]string) (awsses.Tenant, error)
	GetTenant(ctx context.Context, name string) (awsses.Tenant, error)
	DeleteTenant(ctx context.Context, name string) error
	Associate(ctx context.Context, tenant, resourceARN string) error
	Dissociate(ctx context.Context, tenant, resourceARN string) error
	TenantResources(ctx context.Context, tenant string) ([]awsses.Resource, error)
	Identities(ctx context.Context) ([]awsses.Identity, error)
	ReputationEntities(ctx context.Context, prefix string) ([]awsses.Entity, error)
	Reinstate(ctx context.Context, tenantARN string) error
}

// SESSettings: with a Client the server manages one Amazon SES tenant per inbox that sends through
// its own SES account in Region.
type SESSettings struct {
	Client           SESClient
	Region           string
	ConfigurationSet string
	TenantPrefix     string
}

const (
	sesStateNone            = "none"
	sesStatePending         = "pending"
	sesStateIsolated        = "isolated"
	sesStateIdentityMissing = "identity_missing"
	sesStateNotIsolated     = "not_isolated"
	sesStateError           = "error"

	tenantCreating = "creating"
	tenantReady    = "ready"
	tenantError    = "error"
	tenantMissing  = "missing"

	sendingEnabled    = "enabled"
	sendingReinstated = "reinstated"
	sendingDisabled   = "disabled"

	resourceIdentity         = "identity"
	resourceConfigurationSet = "configuration_set"

	noticeSESTenantPaused = "ses_tenant_paused"

	sesTenantHeader           = "X-SES-TENANT"
	sesConfigurationSetHeader = "X-SES-CONFIGURATION-SET"
	sesTenantNameTag          = "ses:tenant-name"
	sesSyncRetry              = 5 * time.Second
	sesSetupWait              = 15 * time.Second
)

var (
	errSESTenantBlocked = problem(http.StatusConflict, "ses_tenant_blocked", "Amazon SES paused the inbox's tenant or the channel lost its tenant isolation; see the review item ses_tenant")
	ErrSESNotConfigured = errors.New("YUVA_SES_* is not set")

	sesNamePattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	accessKeyPattern = regexp.MustCompile(`^[A-Z0-9]{16,128}$`)
	tenantNameChars  = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)
	sesUniqueStates  = []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled, rivertype.JobStateRetryable,
	}
	sesEventTypes = []string{"Sending Status Enabled", "Sending Status Disabled", "Advisor Recommendation Status Open", "Advisor Recommendation Status Resolved"}
	unknownARNs   arnLog
)

func (s *Server) sesOn() bool { return s.ses.Client != nil }

// SESEnabled reports whether the server manages Amazon SES tenants (YUVA_SES_* is set).
func (s *Server) SESEnabled() bool { return s.sesOn() }

// sesErr counts a refused or failed AWS call and returns err.
func sesErr(op string, err error) error {
	if err == nil {
		return nil
	}
	code := "error"
	var e *awsses.Error
	if errors.As(err, &e) {
		op, code = e.Operation, e.Code
	}
	metrics.SESAPIErrors.WithLabelValues(op, code).Inc()
	return err
}

func newTenantName(prefix string) string {
	b := make([]byte, 13)
	_, _ = rand.Read(b)
	return prefix + tenantNameChars.EncodeToString(b)[:20]
}

// arnParts are the partition and account of an ARN such as arn:aws:ses:eu-west-1:123456789012:tenant/x/y.
func arnParts(arn string) (partition, account string) {
	p := strings.SplitN(arn, ":", 6)
	if len(p) < 6 {
		return "", ""
	}
	return p[1], p[4]
}

func regionPartition(region string) string {
	switch {
	case strings.HasPrefix(region, "cn-"):
		return "aws-cn"
	case strings.HasPrefix(region, "us-gov-"):
		return "aws-us-gov"
	}
	return "aws"
}

func (s *Server) rememberSESCaller(account string) {
	if account != "" {
		s.sesCaller.Store(&account)
	}
}

func (s *Server) sesCallerAccount(ctx context.Context) (string, error) {
	acct, err := s.ses.Client.CallerAccount(ctx)
	if err != nil {
		return "", sesErr("GetCallerIdentity", err)
	}
	s.rememberSESCaller(acct)
	return acct, nil
}

// accountManaged reports whether an SMTP account (its host and the AWS account of its user name)
// sends through the server's own SES account in the region whose tenants it manages.
func (s *Server) accountManaged(acct *string, host string, t *store.SesTenant) bool {
	if !s.sesOn() || acct == nil || email.SESRegion(host) != s.ses.Region {
		return false
	}
	if t != nil && t.Arn != nil {
		_, a := arnParts(*t.Arn)
		return a == *acct && t.Region == s.ses.Region
	}
	c := s.sesCaller.Load()
	return c != nil && *c == *acct
}

func channelUsesSES(e store.EmailChannel) bool {
	return email.IsSESHost(e.SmtpHost) || (e.BroadcastSmtpHost != nil && email.IsSESHost(*e.BroadcastSmtpHost))
}

// broadcastAccountOf is the host, user name and stored AWS account of the account broadcasts use.
func broadcastAccountOf(e store.EmailChannel) (string, string, *string) {
	if e.BroadcastSmtpHost != nil && *e.BroadcastSmtpHost != "" {
		return *e.BroadcastSmtpHost, deref(e.BroadcastSmtpUsername), e.BroadcastSmtpAwsAccount
	}
	return e.SmtpHost, e.SmtpUsername, e.SmtpAwsAccount
}

func inboxTenant(ctx context.Context, q *store.Queries, ws, inbox uuid.UUID) (*store.SesTenant, error) {
	t, err := q.GetSESTenant(ctx, store.GetSESTenantParams{WorkspaceID: ws, InboxID: inbox})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Server) channelTenant(ctx context.Context, q *store.Queries, e store.EmailChannel) (*store.SesTenant, error) {
	if !channelUsesSES(e) {
		return nil, nil
	}
	c, err := q.GetChannel(ctx, store.GetChannelParams{WorkspaceID: e.WorkspaceID, ID: e.ChannelID})
	if err != nil {
		return nil, err
	}
	return inboxTenant(ctx, q, e.WorkspaceID, c.InboxID)
}

// sesHeaders are the SES headers of mail through cfg, an SMTP account of e. Through the server's own
// SES account: the shared configuration set always, so its event destinations see the mail, and the
// tenant of e's inbox once the channel is isolated in a ready tenant. Through another SES account:
// the tenant typed on the channel. None otherwise.
func (s *Server) sesHeaders(ctx context.Context, e store.EmailChannel, cfg email.SMTPConfig) ([]email.Header, error) {
	if !email.IsSESHost(cfg.Host) {
		return nil, nil
	}
	acct := e.SmtpAwsAccount
	if cfg.Host != e.SmtpHost || cfg.Username != e.SmtpUsername {
		acct = e.BroadcastSmtpAwsAccount
	}
	if s.sesOn() && acct != nil && email.SESRegion(cfg.Host) == s.ses.Region {
		t, err := s.channelTenant(ctx, s.st.Queries, e)
		if err != nil {
			return nil, err
		}
		if s.accountManaged(acct, cfg.Host, t) {
			h := []email.Header{{Name: sesConfigurationSetHeader, Value: s.ses.ConfigurationSet}}
			if e.SesState == sesStateIsolated && t != nil && t.State == tenantReady {
				h = append([]email.Header{{Name: sesTenantHeader, Value: t.Name}}, h...)
			}
			return h, nil
		}
	}
	if e.SesTenant == nil {
		return nil, nil
	}
	h := []email.Header{{Name: sesTenantHeader, Value: *e.SesTenant}}
	if e.SesConfigurationSet != nil {
		h = append(h, email.Header{Name: sesConfigurationSetHeader, Value: *e.SesConfigurationSet})
	}
	return h, nil
}

// emailSesBody is how the channel's mail through SES is isolated, for the API; nil without SES.
func (s *Server) emailSesBody(e store.EmailChannel, t *store.SesTenant) *oas.EmailSes {
	bcHost := deref(e.BroadcastSmtpHost)
	supportSES, bcSES := email.IsSESHost(e.SmtpHost), bcHost != "" && email.IsSESHost(bcHost)
	if !supportSES && !bcSES {
		return nil
	}
	managed := (supportSES && s.accountManaged(e.SmtpAwsAccount, e.SmtpHost, t)) || (bcSES && s.accountManaged(e.BroadcastSmtpAwsAccount, bcHost, t))
	out := &oas.EmailSes{ChannelTenant: e.SesTenant, ChannelConfigurationSet: e.SesConfigurationSet}
	switch {
	case managed && e.SesState == sesStateIsolated && t != nil && t.State == tenantReady:
		out.Source, out.State = oas.SesSourceServer, oas.SesIsolated
		out.Tenant, out.ConfigurationSet = new(t.Name), new(s.ses.ConfigurationSet)
		if t.SendingStatus == sendingDisabled {
			out.State, out.Detail = oas.SesPaused, t.Cause
		}
	case e.SesState == sesStateIdentityMissing || e.SesState == sesStateError:
		out.Source, out.State, out.Detail = oas.SesSourceServer, oas.EmailSesState(e.SesState), e.SesDetail
	case managed || (s.sesOn() && e.SesState == sesStatePending):
		out.Source, out.State = oas.SesSourceServer, oas.SesPending
	case e.SesTenant != nil:
		out.Source, out.State = oas.SesSourceChannel, oas.SesIsolated
		out.Tenant, out.ConfigurationSet = e.SesTenant, e.SesConfigurationSet
	default:
		out.Source, out.State, out.Detail = oas.SesSourceNone, oas.SesNotIsolated, e.SesDetail
	}
	return out
}

func sesTenantStatusBody(t store.SesTenant) *oas.SesTenantStatus {
	out := &oas.SesTenantStatus{
		State: oas.SesTenantStatusState(t.State), SendingStatus: oas.SesTenantStatusSendingStatus(t.SendingStatus),
		ReputationImpact: oas.SesTenantStatusReputationImpact(t.ReputationImpact), Cause: t.Cause, StatusAt: t.StatusAt, CheckedAt: t.CheckedAt,
	}
	if t.State == tenantCreating {
		out.State = oas.SesTenantPending
	}
	if t.SendingStatus == sendingDisabled {
		by := oas.SesPausedByPolicy
		if deref(t.AwsStatus) == sendingDisabled {
			by = oas.SesPausedByAws
		}
		out.PausedBy = &by
	}
	return out
}

// withSESTenants adds each inbox's tenant status for p; SES's cause only for those who manage
// inboxes. Realtime events carry no tenant status, since every member of the inbox reads them.
func withSESTenants(ctx context.Context, q *store.Queries, p principal, items []oas.Inbox) error {
	if len(items) == 0 {
		return nil
	}
	rows, err := q.ListSESTenantsOfWorkspace(ctx, p.workspaceID)
	if err != nil {
		return err
	}
	full := seesSESDetails(p)
	for _, t := range rows {
		for i := range items {
			if items[i].Id == t.InboxID {
				items[i].SesTenant = sesTenantStatusBody(t)
				if !full {
					items[i].SesTenant.Cause = nil
				}
			}
		}
	}
	return nil
}

// seesSESDetails reports who may read tenant names, configuration sets and SES's diagnostics:
// owners and admins, and keys or grants with inboxes:manage.
func seesSESDetails(p principal) bool {
	return requireManagerOrKey(p) == nil && requireScope(p, oas.InboxesManage) == nil
}

// redactChannelSES leaves agents the SES source and state of a channel, not the names or details.
func redactChannelSES(p principal, ch *oas.Channel) {
	if ch.Email == nil || ch.Email.Ses == nil || seesSESDetails(p) {
		return
	}
	e := ch.Email.Ses
	e.Tenant, e.ConfigurationSet, e.Detail, e.ChannelTenant, e.ChannelConfigurationSet = nil, nil, nil, nil, nil
}

// localSESState is a channel's SES state where no AWS call decides it: on a server without keys.
func localSESState(e store.EmailChannel) string {
	switch {
	case !channelUsesSES(e):
		return sesStateNone
	case e.SesTenant != nil:
		return sesStateIsolated
	}
	return sesStateNotIsolated
}

// applySESInput stores the tenant typed on a channel: absent keeps, empty removes.
func applySESInput(ctx context.Context, q *store.Queries, e store.EmailChannel, in *oas.EmailSesInput) (store.EmailChannel, error) {
	if in == nil {
		return e, nil
	}
	tenant, set := e.SesTenant, e.SesConfigurationSet
	pick := func(v *string, field string, cur *string) (*string, error) {
		if v == nil {
			return cur, nil
		}
		x := strings.TrimSpace(*v)
		if x == "" {
			return nil, nil
		}
		if !sesNamePattern.MatchString(x) {
			return nil, errValidation(field + " must be 1 to 64 letters, digits, - or _")
		}
		return &x, nil
	}
	var err error
	if tenant, err = pick(in.Tenant, "email.ses.tenant", tenant); err != nil {
		return e, err
	}
	if set, err = pick(in.ConfigurationSet, "email.ses.configuration_set", set); err != nil {
		return e, err
	}
	return q.SetEmailChannelSESInput(ctx, store.SetEmailChannelSESInputParams{WorkspaceID: e.WorkspaceID, ChannelID: e.ChannelID, Tenant: tenant, ConfigurationSet: set})
}

// sesChannelSaved resets what a saved channel's SES state was decided on and queues the sync of its
// inbox: a changed host or user name forgets the stored AWS account, and a changed address, sending
// address or typed tenant asks for a new verdict.
func (s *Server) sesChannelSaved(ctx context.Context, q *store.Queries, events *eventBatch, prev *store.EmailChannel, cur store.EmailChannel, inboxID uuid.UUID) (store.EmailChannel, error) {
	clearSMTP, clearBC, changed := true, true, true
	wasSES := false
	if prev != nil {
		clearSMTP = prev.SmtpHost != cur.SmtpHost || prev.SmtpUsername != cur.SmtpUsername
		clearBC = deref(prev.BroadcastSmtpHost) != deref(cur.BroadcastSmtpHost) || deref(prev.BroadcastSmtpUsername) != deref(cur.BroadcastSmtpUsername)
		changed = clearSMTP || clearBC || prev.Address != cur.Address || deref(prev.FromAddress) != deref(cur.FromAddress) ||
			deref(prev.SesTenant) != deref(cur.SesTenant) || deref(prev.SesConfigurationSet) != deref(cur.SesConfigurationSet)
		wasSES = channelUsesSES(*prev)
	}
	isSES := channelUsesSES(cur)
	if !changed || (!wasSES && !isSES && cur.SesState == sesStateNone) {
		return cur, nil
	}
	state := localSESState(cur)
	if s.sesOn() && isSES {
		state = sesStatePending
	}
	out, err := q.ResetEmailChannelSES(ctx, store.ResetEmailChannelSESParams{
		WorkspaceID: cur.WorkspaceID, ChannelID: cur.ChannelID, State: state,
		ClearSmtp: clearSMTP && cur.SmtpAwsAccount != nil, ClearBroadcast: clearBC && cur.BroadcastSmtpAwsAccount != nil,
	})
	if err != nil {
		return cur, err
	}
	if s.sesOn() && (wasSES || isSES) {
		events.jobOnce(SESTenantSyncArgs{WorkspaceID: cur.WorkspaceID, InboxID: inboxID}, nil)
	}
	return out, nil
}

// SESTenantSyncArgs reconciles an inbox's Amazon SES tenant with its channels.
type SESTenantSyncArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	InboxID     uuid.UUID `json:"inbox_id"`
}

func (SESTenantSyncArgs) Kind() string { return "ses_tenant_sync" }

func (SESTenantSyncArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: sesUniqueStates}}
}

// SESTenantDeleteArgs deletes a tenant of a deleted inbox or workspace; it carries the name because
// the rows are gone by the time it runs.
type SESTenantDeleteArgs struct {
	Name   string `json:"name"`
	Region string `json:"region"`
}

func (SESTenantDeleteArgs) Kind() string { return "ses_tenant_delete" }

type sesTenantSyncWorker struct {
	river.WorkerDefaults[SESTenantSyncArgs]
	s *Server
}

func (w *sesTenantSyncWorker) Timeout(*river.Job[SESTenantSyncArgs]) time.Duration {
	return 3 * time.Minute
}

func (w *sesTenantSyncWorker) Work(ctx context.Context, job *river.Job[SESTenantSyncArgs]) error {
	res, err := w.s.SyncSESTenant(ctx, job.Args.WorkspaceID, job.Args.InboxID, false)
	if err != nil {
		return err
	}
	if res.Retry {
		return river.JobSnooze(sesSyncRetry)
	}
	return nil
}

type sesTenantStatusWorker struct {
	river.WorkerDefaults[jobs.SESTenantStatusArgs]
	s *Server
}

func (w *sesTenantStatusWorker) Work(ctx context.Context, job *river.Job[jobs.SESTenantStatusArgs]) error {
	return w.s.RefreshSESTenants(ctx, job.Args.WorkspaceID, job.Args.InboxID)
}

type sesTenantDeleteWorker struct {
	river.WorkerDefaults[SESTenantDeleteArgs]
	s *Server
}

func (w *sesTenantDeleteWorker) Work(ctx context.Context, job *river.Job[SESTenantDeleteArgs]) error {
	return w.s.DeleteSESTenant(ctx, job.Args)
}

func (s *Server) addSESWorkers(workers *river.Workers) {
	river.AddWorker(workers, &sesTenantSyncWorker{s: s})
	river.AddWorker(workers, &sesTenantStatusWorker{s: s})
	river.AddWorker(workers, &sesTenantDeleteWorker{s: s})
}

// DeleteSESTenant deletes the AWS tenant of a deleted inbox or workspace after dissociating its
// resources; an error asks River to try again.
func (s *Server) DeleteSESTenant(ctx context.Context, a SESTenantDeleteArgs) error {
	log := s.log.With(slog.String("tenant", a.Name))
	if !s.sesOn() || a.Region != s.ses.Region || !strings.HasPrefix(a.Name, s.ses.TenantPrefix) {
		log.ErrorContext(ctx, "ses tenant of a deleted inbox not deleted: this server does not manage it (YUVA_SES_* off, or another region or tenant prefix)",
			slog.String("region", a.Region))
		return nil
	}
	retry := func(err error) bool { return awsses.Temporary(err) || awsses.IsCode(err, "ConflictException") }
	resources, err := s.ses.Client.TenantResources(ctx, a.Name)
	err = sesErr("ListTenantResources", err)
	if awsses.IsCode(err, "NotFoundException") {
		log.InfoContext(ctx, "ses tenant already deleted")
		return nil
	}
	if err != nil {
		if retry(err) {
			return err
		}
		log.ErrorContext(ctx, "ses tenant not deleted: its resources cannot be listed", slog.Any("error", err))
		return nil
	}
	for _, r := range resources {
		if err := sesErr("DeleteTenantResourceAssociation", s.ses.Client.Dissociate(ctx, a.Name, r.ARN)); err != nil {
			if retry(err) {
				return err
			}
			log.ErrorContext(ctx, "ses tenant not deleted: a resource cannot be dissociated", slog.String("resource", r.ARN), slog.Any("error", err))
			return nil
		}
	}
	err = sesErr("DeleteTenant", s.ses.Client.DeleteTenant(ctx, a.Name))
	switch {
	case err == nil, awsses.IsCode(err, "NotFoundException"):
		log.InfoContext(ctx, "ses tenant deleted")
		return nil
	case retry(err):
		return err
	}
	log.ErrorContext(ctx, "ses tenant not deleted", slog.Any("error", err))
	return nil
}

// SESChannelVerdict is what a sync decided for one e-mail channel.
type SESChannelVerdict struct {
	ChannelID uuid.UUID
	Address   string
	State     string
	Detail    string
}

// SESSyncResult reports a sync: the tenant, what changed (or would, on a dry run) in AWS, the
// channels' states, and Retry when a channel changed while it ran.
type SESSyncResult struct {
	Tenant   string
	Actions  []string
	Channels []SESChannelVerdict
	Retry    bool
}

type sesSync struct {
	s      *Server
	ws     uuid.UUID
	inbox  uuid.UUID
	dry    bool
	res    *SESSyncResult
	caller string
	keys   map[string]string
}

type sesVerdict struct {
	state  string
	detail string
}

// account is the AWS account the user name of an SES SMTP account belongs to, "" when it is not an
// access key or STS does not know it.
func (y *sesSync) account(ctx context.Context, user string, stored *string) (string, error) {
	if stored != nil {
		return *stored, nil
	}
	if !accessKeyPattern.MatchString(user) {
		return "", nil
	}
	if a, ok := y.keys[user]; ok {
		return a, nil
	}
	a, err := y.s.ses.Client.AccessKeyAccount(ctx, user)
	if err != nil {
		err = sesErr("GetAccessKeyInfo", err)
		if awsses.Temporary(err) {
			return "", err
		}
		y.s.log.WarnContext(ctx, "ses access key account unknown", slog.Any("error", err))
		a = ""
	}
	y.keys[user] = a
	return a, nil
}

// SyncSESTenant reconciles an inbox's tenant with its e-mail channels: it learns the AWS account of
// each SES SMTP account, creates the inbox's tenant when a channel sends through the server's own
// SES account, associates the configuration set and the channels' verified identities, dissociates
// identities no channel uses any more and records each channel's state. dry changes nothing and
// lists in Actions what it would do in AWS.
func (s *Server) SyncSESTenant(ctx context.Context, ws, inboxID uuid.UUID, dry bool) (SESSyncResult, error) {
	var res SESSyncResult
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live {
		return res, err
	}
	if _, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: inboxID}); store.IsNotFound(err) {
		return res, nil
	} else if err != nil {
		return res, err
	}
	chans, err := s.st.ListInboxEmailChannelsForSES(ctx, store.ListInboxEmailChannelsForSESParams{WorkspaceID: ws, InboxID: inboxID})
	if err != nil {
		return res, err
	}
	y := &sesSync{s: s, ws: ws, inbox: inboxID, dry: dry, res: &res, keys: map[string]string{}}
	if s.sesOn() {
		if y.caller, err = s.sesCallerAccount(ctx); err != nil {
			return res, err
		}
	}
	verdicts := map[uuid.UUID]sesVerdict{}
	var managed []int
	for i := range chans {
		e := &chans[i]
		if !channelUsesSES(*e) {
			verdicts[e.ChannelID] = sesVerdict{state: sesStateNone}
			continue
		}
		if !s.sesOn() {
			verdicts[e.ChannelID] = sesVerdict{state: localSESState(*e)}
			continue
		}
		isManaged, why, err := y.classify(ctx, e)
		if err != nil {
			return res, err
		}
		switch {
		case isManaged:
			managed = append(managed, i)
		case e.SesTenant != nil:
			verdicts[e.ChannelID] = sesVerdict{state: sesStateIsolated}
		default:
			verdicts[e.ChannelID] = sesVerdict{state: sesStateNotIsolated, detail: why}
		}
	}
	t, err := inboxTenant(ctx, s.st.Queries, ws, inboxID)
	if err != nil {
		return res, err
	}
	if len(managed) == 0 {
		if t != nil {
			res.Tenant = t.Name
			if err := y.dissociateUnused(ctx, *t, nil); err != nil {
				if !awsses.Temporary(err) {
					s.log.WarnContext(ctx, "ses tenant identity not dissociated", slog.String("tenant", t.Name), slog.Any("error", err))
				} else {
					return res, err
				}
			}
		}
		return res, y.record(ctx, chans, verdicts)
	}
	fail := func(err error) (SESSyncResult, error) {
		if awsses.Temporary(err) {
			return res, err
		}
		for _, i := range managed {
			verdicts[chans[i].ChannelID] = sesVerdict{state: sesStateError, detail: truncateRunes(err.Error(), 2000)}
		}
		return res, y.record(ctx, chans, verdicts)
	}
	tenant, err := y.ensureTenant(ctx, t)
	if err != nil {
		return fail(err)
	}
	res.Tenant = tenant.Name
	if tenant.Arn == nil {
		for _, i := range managed {
			verdicts[chans[i].ChannelID] = sesVerdict{state: sesStatePending}
		}
		return res, nil
	}
	if err := y.associateConfigurationSet(ctx, tenant); err != nil {
		return fail(err)
	}
	idents, err := s.ses.Client.Identities(ctx)
	if err != nil {
		return fail(sesErr("ListEmailIdentities", err))
	}
	have := verifiedIdentities(idents)
	want := map[string]string{}
	for _, i := range managed {
		e := chans[i]
		ids, missing := have.forChannel(e)
		if missing != "" {
			verdicts[e.ChannelID] = sesVerdict{state: sesStateIdentityMissing, detail: missing + " is not a verified identity in the server's Amazon SES account; mail goes out without tenant isolation and broadcasts are blocked"}
			continue
		}
		for _, id := range ids {
			want[y.identityARN(tenant, id)] = id
		}
		verdicts[e.ChannelID] = sesVerdict{state: sesStateIsolated}
	}
	if err := y.associateIdentities(ctx, tenant, want); err != nil {
		return fail(err)
	}
	if err := y.dissociateUnused(ctx, tenant, want); err != nil {
		return fail(err)
	}
	if err := y.record(ctx, chans, verdicts); err != nil {
		return res, err
	}
	if !dry {
		if _, err := s.jobs.Insert(ctx, jobs.SESTenantStatusArgs{WorkspaceID: &ws, InboxID: &inboxID}, nil); err != nil {
			return res, err
		}
	}
	return res, nil
}

// classify tells whether one of the channel's SES accounts is the server's own SES account in its
// region, and else why not. It stores the AWS accounts it learns.
func (y *sesSync) classify(ctx context.Context, e *store.EmailChannel) (bool, string, error) {
	s := y.s
	isManaged := false
	var why []string
	look := func(label, host, user string, stored *string) (*string, error) {
		if !email.IsSESHost(host) {
			return stored, nil
		}
		acct, err := y.account(ctx, user, stored)
		if err != nil {
			return nil, err
		}
		region := email.SESRegion(host)
		switch {
		case region != s.ses.Region:
			why = append(why, fmt.Sprintf("the %s account sends through Amazon SES in %s; the server manages tenants in %s", label, region, s.ses.Region))
		case acct == "":
			why = append(why, "the "+label+" account's user name is not an access key of the server's AWS account")
		case acct != y.caller:
			why = append(why, "the "+label+" account sends through another AWS account")
		default:
			isManaged = true
		}
		if acct == "" {
			return nil, nil
		}
		return &acct, nil
	}
	smtpAcct, err := look("SMTP", e.SmtpHost, e.SmtpUsername, e.SmtpAwsAccount)
	if err != nil {
		return false, "", err
	}
	bcAcct := e.BroadcastSmtpAwsAccount
	if e.BroadcastSmtpHost != nil && *e.BroadcastSmtpHost != "" {
		if bcAcct, err = look("broadcast SMTP", *e.BroadcastSmtpHost, deref(e.BroadcastSmtpUsername), e.BroadcastSmtpAwsAccount); err != nil {
			return false, "", err
		}
	}
	if !y.dry && (deref(smtpAcct) != deref(e.SmtpAwsAccount) || deref(bcAcct) != deref(e.BroadcastSmtpAwsAccount)) {
		if err := s.st.SetEmailChannelSESAccounts(ctx, store.SetEmailChannelSESAccountsParams{
			WorkspaceID: e.WorkspaceID, ChannelID: e.ChannelID, SmtpAwsAccount: smtpAcct, BroadcastSmtpAwsAccount: bcAcct,
			SmtpHost: e.SmtpHost, SmtpUsername: e.SmtpUsername, BroadcastSmtpHost: deref(e.BroadcastSmtpHost), BroadcastSmtpUsername: deref(e.BroadcastSmtpUsername),
		}); err != nil {
			return false, "", err
		}
	}
	e.SmtpAwsAccount, e.BroadcastSmtpAwsAccount = smtpAcct, bcAcct
	if len(why) == 0 && !isManaged {
		why = append(why, "no tenant is named on the channel")
	}
	return isManaged, strings.Join(why, "; "), nil
}

func (y *sesSync) action(format string, args ...any) {
	y.res.Actions = append(y.res.Actions, fmt.Sprintf(format, args...))
}

// ensureTenant creates the inbox's tenant in Yuva and in AWS once. A name AWS already knows is
// taken over only when its tags say this server created it for this inbox (a crash after
// CreateTenant); any other tenant is never used.
func (y *sesSync) ensureTenant(ctx context.Context, t *store.SesTenant) (store.SesTenant, error) {
	s := y.s
	if t == nil {
		if y.dry {
			y.action("create a tenant for the inbox")
			return store.SesTenant{Name: "(new tenant)", Region: s.ses.Region}, nil
		}
		row, err := s.st.CreateSESTenant(ctx, store.CreateSESTenantParams{WorkspaceID: y.ws, InboxID: y.inbox, Name: newTenantName(s.ses.TenantPrefix), Region: s.ses.Region, Now: s.now()})
		if store.IsNotFound(err) {
			row, err = s.st.GetSESTenant(ctx, store.GetSESTenantParams{WorkspaceID: y.ws, InboxID: y.inbox})
		}
		if err != nil {
			return store.SesTenant{}, err
		}
		t = &row
	}
	if t.Arn != nil && t.State == tenantReady {
		return *t, nil
	}
	if y.dry {
		y.action("create tenant %s", t.Name)
		return *t, nil
	}
	tags := map[string]string{"yuva-workspace": y.ws.String(), "yuva-inbox": y.inbox.String(), "yuva-server": s.publicHost()}
	at, err := s.ses.Client.CreateTenant(ctx, t.Name, tags)
	err = sesErr("CreateTenant", err)
	if awsses.IsCode(err, "AlreadyExistsException") {
		if at, err = s.ses.Client.GetTenant(ctx, t.Name); err == nil && (at.Tags["yuva-workspace"] != y.ws.String() || at.Tags["yuva-inbox"] != y.inbox.String()) {
			err = &awsses.Error{Operation: "GetTenant", Code: "ForeignTenant", Message: "a tenant named " + t.Name + " exists in Amazon SES that this server did not create for the inbox"}
		}
		err = sesErr("GetTenant", err)
	}
	if err == nil && at.ARN == "" {
		err = &awsses.Error{Operation: "CreateTenant", Code: "NoArn", Message: "Amazon SES answered without the tenant's ARN"}
	}
	if err != nil {
		if awsses.Temporary(err) {
			return *t, err
		}
		if _, serr := s.st.SetSESTenantError(ctx, store.SetSESTenantErrorParams{WorkspaceID: y.ws, InboxID: y.inbox, Error: optionalText(err.Error(), 2000)}); serr != nil {
			return *t, serr
		}
		return *t, err
	}
	row, err := s.st.SetSESTenantReady(ctx, store.SetSESTenantReadyParams{WorkspaceID: y.ws, InboxID: y.inbox, Arn: &at.ARN})
	if err != nil {
		return *t, err
	}
	y.action("created tenant %s", row.Name)
	return row, nil
}

func (s *Server) publicHost() string {
	if u, err := url.Parse(s.auth.PublicURL); err == nil && u.Host != "" {
		return u.Hostname()
	}
	return s.auth.PublicURL
}

func (y *sesSync) resourceARN(t store.SesTenant, kind, name string) string {
	partition, account := arnParts(deref(t.Arn))
	return "arn:" + partition + ":ses:" + t.Region + ":" + account + ":" + kind + "/" + name
}

func (y *sesSync) identityARN(t store.SesTenant, name string) string {
	return y.resourceARN(t, "identity", name)
}

func (y *sesSync) associateConfigurationSet(ctx context.Context, t store.SesTenant) error {
	s := y.s
	arn := y.resourceARN(t, "configuration-set", s.ses.ConfigurationSet)
	recorded, err := s.st.ListSESTenantResources(ctx, store.ListSESTenantResourcesParams{WorkspaceID: y.ws, InboxID: y.inbox})
	if err != nil {
		return err
	}
	if slices.ContainsFunc(recorded, func(r store.SesTenantResource) bool { return r.Arn == arn }) {
		return nil
	}
	if y.dry {
		y.action("associate configuration set %s with %s", s.ses.ConfigurationSet, t.Name)
		return nil
	}
	if err := sesErr("CreateTenantResourceAssociation", s.ses.Client.Associate(ctx, t.Name, arn)); err != nil {
		return err
	}
	y.action("associated configuration set %s with %s", s.ses.ConfigurationSet, t.Name)
	return s.st.AddSESTenantResource(ctx, store.AddSESTenantResourceParams{
		WorkspaceID: y.ws, InboxID: y.inbox, Arn: arn, Kind: resourceConfigurationSet, Name: s.ses.ConfigurationSet, Now: s.now(),
	})
}

func (y *sesSync) associateIdentities(ctx context.Context, t store.SesTenant, want map[string]string) error {
	s := y.s
	recorded, err := s.st.ListSESTenantResources(ctx, store.ListSESTenantResourcesParams{WorkspaceID: y.ws, InboxID: y.inbox})
	if err != nil {
		return err
	}
	for _, arn := range slices.Sorted(mapsKeys(want)) {
		if slices.ContainsFunc(recorded, func(r store.SesTenantResource) bool { return r.Arn == arn }) {
			continue
		}
		if y.dry {
			y.action("associate identity %s with %s", want[arn], t.Name)
			continue
		}
		if err := sesErr("CreateTenantResourceAssociation", s.ses.Client.Associate(ctx, t.Name, arn)); err != nil {
			return err
		}
		y.action("associated identity %s with %s", want[arn], t.Name)
		if err := s.st.AddSESTenantResource(ctx, store.AddSESTenantResourceParams{
			WorkspaceID: y.ws, InboxID: y.inbox, Arn: arn, Kind: resourceIdentity, Name: want[arn], Now: s.now(),
		}); err != nil {
			return err
		}
	}
	return nil
}

// dissociateUnused removes the identities Yuva associated that no channel needs any more; the
// configuration set stays.
func (y *sesSync) dissociateUnused(ctx context.Context, t store.SesTenant, want map[string]string) error {
	s := y.s
	recorded, err := s.st.ListSESTenantResources(ctx, store.ListSESTenantResourcesParams{WorkspaceID: y.ws, InboxID: y.inbox})
	if err != nil {
		return err
	}
	for _, r := range recorded {
		if r.Kind != resourceIdentity {
			continue
		}
		if _, ok := want[r.Arn]; ok {
			continue
		}
		if y.dry {
			y.action("dissociate identity %s from %s", r.Name, t.Name)
			continue
		}
		if err := sesErr("DeleteTenantResourceAssociation", s.ses.Client.Dissociate(ctx, t.Name, r.Arn)); err != nil {
			return err
		}
		y.action("dissociated identity %s from %s", r.Name, t.Name)
		if err := s.st.DeleteSESTenantResource(ctx, store.DeleteSESTenantResourceParams{WorkspaceID: y.ws, InboxID: y.inbox, Arn: r.Arn}); err != nil {
			return err
		}
	}
	return nil
}

// record writes each channel's verdict, but only for a channel unchanged since it was read; a
// changed one asks for another run.
func (y *sesSync) record(ctx context.Context, chans []store.EmailChannel, verdicts map[uuid.UUID]sesVerdict) error {
	s := y.s
	for _, e := range chans {
		v, ok := verdicts[e.ChannelID]
		if !ok {
			continue
		}
		y.res.Channels = append(y.res.Channels, SESChannelVerdict{ChannelID: e.ChannelID, Address: e.Address, State: v.state, Detail: v.detail})
		if y.dry {
			continue
		}
		n, err := s.st.SetEmailChannelSESState(ctx, store.SetEmailChannelSESStateParams{
			WorkspaceID: e.WorkspaceID, ChannelID: e.ChannelID, State: v.state, Detail: optionalText(v.detail, 2000), Now: new(s.now()),
			SmtpHost: e.SmtpHost, SmtpUsername: e.SmtpUsername, BroadcastSmtpHost: deref(e.BroadcastSmtpHost), BroadcastSmtpUsername: deref(e.BroadcastSmtpUsername),
			Address: e.Address, FromAddress: deref(e.FromAddress), SesTenant: deref(e.SesTenant), SesConfigurationSet: deref(e.SesConfigurationSet),
		})
		if err != nil {
			return err
		}
		if n == 0 {
			y.res.Retry = true
		}
	}
	return nil
}

type identitySet struct{ addresses, domains map[string]bool }

func verifiedIdentities(list []awsses.Identity) identitySet {
	out := identitySet{addresses: map[string]bool{}, domains: map[string]bool{}}
	for _, i := range list {
		if !i.Verified || !i.SendingEnabled {
			continue
		}
		name := strings.ToLower(strings.TrimSuffix(i.Name, "."))
		switch i.Type {
		case "EMAIL_ADDRESS":
			out.addresses[name] = true
		case "DOMAIN", "MANAGED_DOMAIN":
			out.domains[name] = true
		}
	}
	return out
}

// domainFor is the most specific verified domain identity among domain and its parents.
func (h identitySet) domainFor(domain string) string {
	for d := strings.ToLower(domain); d != ""; {
		if h.domains[d] {
			return d
		}
		_, rest, ok := strings.Cut(d, ".")
		if !ok || !strings.Contains(rest, ".") {
			return ""
		}
		d = rest
	}
	return ""
}

// forChannel are the identities the channel's mail is sent under: per sending address its own
// identity and the most specific domain identity, which a catch-all channel needs for its domain.
// missing names a sending address or domain with neither.
func (h identitySet) forChannel(e store.EmailChannel) ([]string, string) {
	var senders []string
	var domainOnly []string
	if isCatchAll(e.Address) {
		domainOnly = append(domainOnly, email.Domain(e.Address))
		if e.FromAddress != nil {
			senders = append(senders, *e.FromAddress)
		}
	} else if e.FromAddress != nil {
		senders = append(senders, *e.FromAddress)
	} else {
		senders = append(senders, e.Address)
	}
	var out []string
	add := func(v string) {
		if v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	for _, d := range domainOnly {
		dom := h.domainFor(d)
		if dom == "" {
			return nil, d
		}
		add(dom)
	}
	for _, a := range senders {
		a = strings.ToLower(a)
		addr := ""
		if h.addresses[a] {
			addr = a
		}
		dom := h.domainFor(email.Domain(a))
		if addr == "" && dom == "" {
			return nil, email.Domain(a)
		}
		add(addr)
		add(dom)
	}
	return out, ""
}

func mapsKeys(m map[string]string) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// RefreshSESTenants reads the sending status of the managed tenants from SES, of one inbox or of
// all, and acts on a tenant that SES paused.
func (s *Server) RefreshSESTenants(ctx context.Context, ws, inboxID *uuid.UUID) error {
	if !s.sesOn() {
		return nil
	}
	var rows []store.SesTenant
	prefix := ""
	all := ws == nil || inboxID == nil
	if all {
		var err error
		if rows, err = s.st.ListSESTenantsAll(ctx); err != nil {
			return err
		}
		if len(rows) == 0 {
			metrics.SESTenants.Reset()
			return nil
		}
		acct, err := s.sesCallerAccount(ctx)
		if err != nil {
			return err
		}
		prefix = "arn:" + regionPartition(s.ses.Region) + ":ses:" + s.ses.Region + ":" + acct + ":tenant/" + s.ses.TenantPrefix
	} else {
		t, err := inboxTenant(ctx, s.st.Queries, *ws, *inboxID)
		if err != nil || t == nil || t.Arn == nil {
			return err
		}
		rows, prefix = []store.SesTenant{*t}, *t.Arn
	}
	entities, err := s.ses.Client.ReputationEntities(ctx, prefix)
	if err != nil {
		return sesErr("ListReputationEntities", err)
	}
	byARN := make(map[string]awsses.Entity, len(entities))
	for _, e := range entities {
		byARN[e.Reference] = e
	}
	counts := map[string]float64{}
	for _, t := range rows {
		e, ok := byARN[*t.Arn]
		switch {
		case ok:
			delete(byARN, *t.Arn)
			if t, err = s.applySESStatus(ctx, t, e); err != nil {
				return err
			}
		case t.State == tenantReady && strings.HasPrefix(*t.Arn, prefix):
			if err := s.checkSESTenantGone(ctx, t); err != nil {
				return err
			}
		}
		counts[t.SendingStatus]++
	}
	if all {
		for ref := range byARN {
			if unknownARNs.first(ref) {
				s.log.InfoContext(ctx, "ses tenant under the prefix that this server does not know", slog.String("arn", ref))
			}
		}
		for _, st := range []string{sendingEnabled, sendingReinstated, sendingDisabled} {
			metrics.SESTenants.WithLabelValues(st).Set(counts[st])
		}
	}
	return nil
}

// checkSESTenantGone marks a ready tenant missing when the status listing left it out and SES no
// longer knows it, forgets its associations and queues the sync that sets it up again.
func (s *Server) checkSESTenantGone(ctx context.Context, t store.SesTenant) error {
	_, err := s.ses.Client.GetTenant(ctx, t.Name)
	if err == nil {
		return nil
	}
	if err = sesErr("GetTenant", err); !awsses.IsCode(err, "NotFoundException") {
		return err
	}
	marked := false
	err = s.inTx(ctx, t.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
		n, err := q.SetSESTenantMissing(ctx, store.SetSESTenantMissingParams{WorkspaceID: t.WorkspaceID, InboxID: t.InboxID, Arn: t.Arn, Now: new(s.now())})
		if err != nil || n == 0 {
			return err
		}
		marked = true
		if err := q.DeleteSESTenantResources(ctx, store.DeleteSESTenantResourcesParams{WorkspaceID: t.WorkspaceID, InboxID: t.InboxID}); err != nil {
			return err
		}
		events.jobOnce(SESTenantSyncArgs{WorkspaceID: t.WorkspaceID, InboxID: t.InboxID}, nil)
		return nil
	})
	if err == nil && marked {
		s.log.ErrorContext(ctx, "ses tenant missing in Amazon SES; setting it up again", slog.String("tenant", t.Name),
			slog.String("workspace_id", t.WorkspaceID.String()), slog.String("inbox_id", t.InboxID.String()))
	}
	return err
}

// arnLog remembers which ARNs were logged, up to a bound, so a poll logs each unknown one once.
type arnLog struct {
	mu   sync.Mutex
	seen map[string]bool
}

const arnLogMax = 1000

func (l *arnLog) first(arn string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.seen[arn] {
		return false
	}
	if l.seen == nil || len(l.seen) >= arnLogMax {
		l.seen = map[string]bool{}
	}
	l.seen[arn] = true
	return true
}

func sesStatus(v string) *string {
	v = strings.ToLower(v)
	switch v {
	case sendingEnabled, sendingReinstated, sendingDisabled:
		return &v
	}
	return nil
}

// applySESStatus stores what SES reports of a tenant; on a pause it pauses the inbox's running
// broadcasts through the server's SES account and notes it for the operator.
func (s *Server) applySESStatus(ctx context.Context, t store.SesTenant, e awsses.Entity) (store.SesTenant, error) {
	agg := sesStatus(e.Aggregate)
	if agg == nil {
		return t, nil
	}
	customer, aws := sesStatus(e.Customer.Status), sesStatus(e.AWS.Status)
	impact := strings.ToLower(e.Impact)
	if impact != "low" && impact != "high" {
		impact = "none"
	}
	cause := e.Customer.Cause
	pausedBy := "the reputation policy or the operator"
	if deref(aws) == sendingDisabled {
		cause, pausedBy = e.AWS.Cause, "Amazon SES"
	}
	var at *time.Time
	for _, u := range []*time.Time{e.Customer.UpdatedAt, e.AWS.UpdatedAt} {
		if u != nil && (at == nil || u.After(*at)) {
			at = u
		}
	}
	paused := false
	var pausedIDs []uuid.UUID
	err := s.inTx(ctx, t.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
		prev, err := q.SetSESTenantStatus(ctx, store.SetSESTenantStatusParams{
			WorkspaceID: t.WorkspaceID, InboxID: t.InboxID, SendingStatus: *agg, CustomerStatus: customer, AwsStatus: aws,
			Cause: optionalText(cause, 2000), ReputationImpact: impact, StatusAt: at, Now: new(s.now()),
		})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if prev == sendingDisabled || *agg != sendingDisabled {
			return nil
		}
		paused = true
		running, err := q.ListRunningBroadcastsOfInbox(ctx, store.ListRunningBroadcastsOfInboxParams{WorkspaceID: t.WorkspaceID, InboxID: t.InboxID})
		if err != nil {
			return err
		}
		detail := "Amazon SES paused the inbox's tenant"
		if c := strings.TrimSpace(cause); c != "" {
			detail += ": " + c
		}
		for _, b := range running {
			if b.ChannelID == nil {
				continue
			}
			ch, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: t.WorkspaceID, ChannelID: *b.ChannelID})
			if store.IsNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			host, _, acct := broadcastAccountOf(ch)
			if !s.accountManaged(acct, host, &t) {
				continue
			}
			ok, err := s.pauseBroadcast(ctx, q, events, b.ID, t.WorkspaceID, oas.BroadcastPauseReasonSesTenant, detail)
			if err != nil {
				return err
			}
			if ok {
				pausedIDs = append(pausedIDs, b.ID)
			}
		}
		w, err := q.GetWorkspace(ctx, t.WorkspaceID)
		if err != nil {
			return err
		}
		inboxName := t.InboxID.String()
		if in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: t.WorkspaceID, ID: t.InboxID}); err == nil {
			inboxName = in.Name
		}
		note := fmt.Sprintf("inbox %s, tenant %s: paused by %s", inboxName, t.Name, pausedBy)
		if c := strings.TrimSpace(cause); c != "" {
			note += ": " + c
		}
		return s.noteBroadcastOperator(ctx, q, noticeSESTenantPaused, "", w.Name, note)
	})
	if err != nil {
		return t, err
	}
	if paused {
		s.log.WarnContext(ctx, "ses tenant paused", slog.String("workspace_id", t.WorkspaceID.String()), slog.String("inbox_id", t.InboxID.String()),
			slog.String("tenant", t.Name), slog.Int("broadcasts_paused", len(pausedIDs)))
	}
	t.SendingStatus, t.CustomerStatus, t.AwsStatus, t.ReputationImpact, t.Cause, t.StatusAt = *agg, customer, aws, impact, optionalText(cause, 2000), at
	return t, nil
}

// sesCheck is the review item ses_tenant for broadcasts through a channel. applies is false when
// the broadcast account is not Amazon SES; wait means the server is still setting the tenant up,
// which holds sending back for a moment instead of pausing it.
type sesCheck struct {
	status  oas.BroadcastReviewStatus
	detail  string
	applies bool
	wait    bool
}

const sesSetupDetail = "the server is still setting up the inbox's Amazon SES tenant; sending waits until it is ready"

func (s *Server) sesBroadcastCheck(ctx context.Context, q *store.Queries, e store.EmailChannel, inboxID uuid.UUID) (sesCheck, error) {
	host, _, acct := broadcastAccountOf(e)
	if !email.IsSESHost(host) {
		return sesCheck{status: oas.ReviewOk}, nil
	}
	t, err := inboxTenant(ctx, q, e.WorkspaceID, inboxID)
	if err != nil {
		return sesCheck{}, err
	}
	if !s.accountManaged(acct, host, t) {
		if s.sesOn() && e.SesTenant == nil {
			return sesCheck{status: oas.ReviewWarning, applies: true, detail: "the channel sends through Amazon SES outside the server's account and names no SES tenant, so its reputation is not kept apart"}, nil
		}
		return sesCheck{status: oas.ReviewOk, applies: true}, nil
	}
	block := func(detail string) (sesCheck, error) {
		return sesCheck{status: oas.ReviewBlocking, detail: detail, applies: true}, nil
	}
	switch {
	case t != nil && t.State == tenantError:
		return block("the inbox's Amazon SES tenant could not be set up: " + deref(t.Error))
	case t != nil && t.State == tenantMissing:
		return block("the inbox's Amazon SES tenant no longer exists in Amazon SES; the server is setting it up again")
	case t != nil && t.SendingStatus == sendingDisabled:
		detail := "Amazon SES paused the inbox's tenant"
		if c := deref(t.Cause); c != "" {
			detail += ": " + c
		}
		return block(detail)
	case e.SesState == sesStateIdentityMissing || e.SesState == sesStateError:
		detail := deref(e.SesDetail)
		if detail == "" {
			detail = "the channel is not isolated in the inbox's Amazon SES tenant"
		}
		return block(detail)
	case e.SesState != sesStateIsolated || t == nil || t.State != tenantReady:
		return sesCheck{status: oas.ReviewWarning, detail: sesSetupDetail, applies: true, wait: true}, nil
	case t.ReputationImpact == "high":
		return sesCheck{status: oas.ReviewWarning, detail: "Amazon SES reports a high-severity reputation finding for this inbox", applies: true}, nil
	}
	return sesCheck{status: oas.ReviewOk, applies: true}, nil
}

// applySESTenantEvent handles an SES tenant event from EventBridge through SNS: it only asks for a
// fresh status of a tenant it knows, because the event's status is per origin.
func (s *Server) applySESTenantEvent(ctx context.Context, detailType string, resources []string) error {
	if !slices.Contains(sesEventTypes, detailType) || len(resources) == 0 || !s.sesOn() {
		return nil
	}
	t, err := s.st.FindSESTenantByARN(ctx, &resources[0])
	if store.IsNotFound(err) {
		s.log.InfoContext(ctx, "ses tenant event for an unknown tenant", slog.String("detail_type", detailType))
		return nil
	}
	if err != nil {
		return err
	}
	_, err = s.jobs.Insert(ctx, jobs.SESTenantStatusArgs{WorkspaceID: &t.WorkspaceID, InboxID: &t.InboxID}, nil)
	return err
}

// sesTenantForeign reports whether an SES notification's ses:tenant-name provably names another
// tenant than the one of the inbox whose channel sent the mail: that inbox has its own tenant under
// another name. A sender that cannot be resolved, an inbox without a tenant or a name outside the
// server's prefix is not proof, and the notification applies as it always did.
func (s *Server) sesTenantForeign(ctx context.Context, ws uuid.UUID, channelID *uuid.UUID, name string) (bool, error) {
	if name == "" || channelID == nil || !s.sesOn() || !strings.HasPrefix(name, s.ses.TenantPrefix) {
		return false, nil
	}
	c, err := s.st.GetChannel(ctx, store.GetChannelParams{WorkspaceID: ws, ID: *channelID})
	if store.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	t, err := inboxTenant(ctx, s.st.Queries, ws, c.InboxID)
	if err != nil {
		return false, err
	}
	return t != nil && t.Name != name, nil
}

// SESInboxReport is what the operator sees of an inbox's tenant.
type SESInboxReport struct {
	InboxID   uuid.UUID
	InboxName string
	InboxSlug string
	Tenant    *store.SesTenant
	Resources []store.SesTenantResource
	Channels  []SESChannelReport
}

type SESChannelReport struct {
	ChannelID uuid.UUID
	Name      string
	Address   string
	SES       *oas.EmailSes
}

// OperatorSESTenants reports the stored tenants and channel states of a workspace's inboxes, or of
// one; it calls no AWS operation.
func (s *Server) OperatorSESTenants(ctx context.Context, ws uuid.UUID, inboxID *uuid.UUID) ([]SESInboxReport, error) {
	inboxes, err := s.st.ListInboxes(ctx, store.ListInboxesParams{WorkspaceID: ws, AllInboxes: true, WithPersonal: true})
	if err != nil {
		return nil, err
	}
	var out []SESInboxReport
	for _, in := range inboxes {
		if inboxID != nil && in.ID != *inboxID {
			continue
		}
		r := SESInboxReport{InboxID: in.ID, InboxName: in.Name, InboxSlug: in.Slug}
		if r.Tenant, err = inboxTenant(ctx, s.st.Queries, ws, in.ID); err != nil {
			return nil, err
		}
		if r.Tenant != nil {
			if r.Resources, err = s.st.ListSESTenantResources(ctx, store.ListSESTenantResourcesParams{WorkspaceID: ws, InboxID: in.ID}); err != nil {
				return nil, err
			}
		}
		chans, err := s.st.ListInboxEmailChannelsForSES(ctx, store.ListInboxEmailChannelsForSESParams{WorkspaceID: ws, InboxID: in.ID})
		if err != nil {
			return nil, err
		}
		for _, e := range chans {
			c, err := s.st.GetChannel(ctx, store.GetChannelParams{WorkspaceID: ws, ID: e.ChannelID})
			if err != nil {
				return nil, err
			}
			r.Channels = append(r.Channels, SESChannelReport{ChannelID: e.ChannelID, Name: c.Name, Address: e.Address, SES: s.emailSesBody(e, r.Tenant)})
		}
		out = append(out, r)
	}
	return out, nil
}

// SESInboxRef names an inbox of a workspace.
type SESInboxRef struct{ WorkspaceID, InboxID uuid.UUID }

// OperatorSESInboxes are the inboxes of every workspace with an SES channel or a tenant, for the
// operator's back-fill.
func (s *Server) OperatorSESInboxes(ctx context.Context) ([]SESInboxRef, error) {
	rows, err := s.st.ListInboxesWithSESChannels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SESInboxRef, len(rows))
	for i, r := range rows {
		out[i] = SESInboxRef{WorkspaceID: r.WorkspaceID, InboxID: r.InboxID}
	}
	return out, nil
}

// OperatorSyncSESTenant syncs an inbox's tenant and then reads its status, inline.
func (s *Server) OperatorSyncSESTenant(ctx context.Context, ws, inboxID uuid.UUID, dry bool) (SESSyncResult, error) {
	if !s.sesOn() {
		return SESSyncResult{}, ErrSESNotConfigured
	}
	var res SESSyncResult
	var err error
	for range 3 {
		if res, err = s.SyncSESTenant(ctx, ws, inboxID, dry); err != nil || !res.Retry {
			break
		}
	}
	if err != nil || dry {
		return res, err
	}
	return res, s.RefreshSESTenants(ctx, &ws, &inboxID)
}

// OperatorReinstateSESTenant sets a tenant paused by its reputation policy (or by hand) to
// REINSTATED. A pause by Amazon SES itself is refused: only an AWS Support case lifts it.
func (s *Server) OperatorReinstateSESTenant(ctx context.Context, ws, inboxID uuid.UUID) (store.SesTenant, error) {
	if !s.sesOn() {
		return store.SesTenant{}, ErrSESNotConfigured
	}
	if err := s.RefreshSESTenants(ctx, &ws, &inboxID); err != nil {
		return store.SesTenant{}, err
	}
	t, err := inboxTenant(ctx, s.st.Queries, ws, inboxID)
	if err != nil {
		return store.SesTenant{}, err
	}
	switch {
	case t == nil || t.Arn == nil:
		return store.SesTenant{}, errors.New("the inbox has no Amazon SES tenant")
	case deref(t.AwsStatus) == sendingDisabled:
		return *t, errors.New("Amazon SES itself paused this tenant; only an AWS Support case lifts it")
	case t.SendingStatus != sendingDisabled || deref(t.CustomerStatus) != sendingDisabled:
		return *t, fmt.Errorf("the tenant is not paused (sending status %s)", t.SendingStatus)
	}
	if err := sesErr("UpdateReputationEntityCustomerManagedStatus", s.ses.Client.Reinstate(ctx, *t.Arn)); err != nil {
		return *t, err
	}
	if err := s.RefreshSESTenants(ctx, &ws, &inboxID); err != nil {
		return *t, err
	}
	cur, err := inboxTenant(ctx, s.st.Queries, ws, inboxID)
	if err != nil || cur == nil {
		return *t, err
	}
	return *cur, nil
}

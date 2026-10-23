package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/oapi-codegen/nullable"
	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/issues"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
	"github.com/productdevbook/yuva/api/internal/webhook"
)

const (
	maxIssueIntegrations   = 20
	maxIssuesPerTarget     = 10
	maxLinksPerIssue       = 1000
	maxIssueWebhookBytes   = 1 << 20
	issueSyncAttempts      = 5
	issueStaleAfter        = time.Hour
	issueSweepHold         = 15 * time.Minute
	issueBackoffMax        = 24 * time.Hour
	issueIngressPathPrefix = "/ingress/issues/"

	eventIssueLinkCreated = string(oas.IssueLinkEventTypeIssueLinkCreated)
	eventIssueLinkUpdated = string(oas.IssueLinkEventTypeIssueLinkUpdated)
	eventIssueLinkDeleted = string(oas.IssueLinkEventTypeIssueLinkDeleted)
)

var (
	errIssueIntegrationGone = problem(http.StatusNotFound, "not_found", "no such issue tracker integration")
	errIssueLinkGone        = problem(http.StatusNotFound, "not_found", "no such issue link")
	errTooManyIntegrations  = problem(http.StatusUnprocessableEntity, "too_many_integrations", "a workspace has at most 20 issue tracker integrations")
	errTooManyIssues        = problem(http.StatusUnprocessableEntity, "too_many_links", "a feedback or idea links at most 10 issues")
	errTooManyIssueLinks    = problem(http.StatusUnprocessableEntity, "too_many_links", "an issue has at most 1,000 links")
	errIssueBadSignature    = problem(http.StatusUnauthorized, "invalid_signature", "the webhook signature is missing or wrong")

	limitIssueCallsPerWorkspace = limit{120, time.Minute}

	issueSweepBatch          = 500
	issueSweepPerIntegration = 50
	limitIssueWebhook        = limit{600, time.Minute}
)

// IssueTrackerSettings points the GitHub and Linear clients elsewhere, for tests; empty calls
// api.github.com and api.linear.app through the outbound address checks.
type IssueTrackerSettings struct {
	GitHubAPI  string
	LinearAPI  string
	HTTPClient *http.Client
}

type issueProgressKeyType struct{}

var issueProgressKey issueProgressKeyType

func issueProgressFrom(ctx context.Context) *uuid.UUID {
	id, ok := ctx.Value(issueProgressKey).(uuid.UUID)
	if !ok {
		return nil
	}
	return &id
}

func issueTokenContext(ws, id uuid.UUID) []byte {
	return append(append([]byte("issue-token:"), ws[:]...), id[:]...)
}

func issueSecretContext(ws, id uuid.UUID) []byte {
	return append(append([]byte("issue-webhook:"), ws[:]...), id[:]...)
}

func trackerProblem(err error) error {
	var e *issues.Error
	if !errors.As(err, &e) {
		return err
	}
	switch e.Kind {
	case issues.KindAuth:
		return problem(http.StatusUnprocessableEntity, "tracker_auth_failed", e.Msg)
	case issues.KindNotFound:
		return problem(http.StatusUnprocessableEntity, "issue_not_found", e.Msg)
	case issues.KindRejected:
		return problem(http.StatusUnprocessableEntity, "tracker_rejected", e.Msg)
	case issues.KindInvalid:
		return errValidation(e.Msg)
	}
	return problem(http.StatusServiceUnavailable, "tracker_unavailable", e.Msg)
}

func (s *Server) trackerBase(provider string, apiURL *string) string {
	if provider == issues.Linear {
		return orDefault(s.issueTrackers.LinearAPI, issues.LinearAPI)
	}
	if apiURL != nil {
		return *apiURL
	}
	return orDefault(s.issueTrackers.GitHubAPI, issues.GitHubAPI)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func (s *Server) trackerClient(provider string, apiURL *string, token string) *issues.Client {
	var hc issues.Doer = s.hooks
	if s.issueTrackers.HTTPClient != nil {
		hc = s.issueTrackers.HTTPClient
	}
	cl := &issues.Client{HTTP: hc, Provider: provider, Base: s.trackerBase(provider, apiURL), Token: token}
	if apiURL != nil {
		cl.WebHost = urlHost(*apiURL)
	}
	return cl
}

func (s *Server) integrationClient(in store.IssueIntegration) (*issues.Client, error) {
	tok, err := s.secrets.Open(in.Token, issueTokenContext(in.WorkspaceID, in.ID))
	if err != nil {
		return nil, err
	}
	return s.trackerClient(in.Provider, in.ApiUrl, string(tok)), nil
}

func newIssueWebhookSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) issueIntegrationBody(in store.IssueIntegration) oas.IssueIntegration {
	return oas.IssueIntegration{
		Id: in.ID, Provider: oas.IssueProvider(in.Provider), Name: in.Name, Account: in.Account, ApiUrl: in.ApiUrl, DefaultTarget: in.DefaultTarget,
		WebhookUrl: s.auth.PublicURL + issueIngressPathPrefix + in.ID.String(), WebhookSecretSet: in.WebhookSecret != nil,
		LastError: in.LastError, LastErrorAt: in.LastErrorAt, CreatedAt: in.CreatedAt, UpdatedAt: in.UpdatedAt,
	}
}

func requireIntegrationManager(p principal) error { return requireManagerOrFullKey(p) }

func (s *Server) ListIssueIntegrations(ctx context.Context, _ oas.ListIssueIntegrationsRequestObject) (oas.ListIssueIntegrationsResponseObject, error) {
	p := principalFrom(ctx)
	rows, err := s.st.ListIssueIntegrations(ctx, p.workspaceID)
	if err != nil {
		return nil, err
	}
	out := oas.ListIssueIntegrations200JSONResponse{Items: make([]oas.IssueIntegration, len(rows))}
	for i, r := range rows {
		out.Items[i] = s.issueIntegrationBody(r)
	}
	return out, nil
}

func (s *Server) GetIssueIntegration(ctx context.Context, req oas.GetIssueIntegrationRequestObject) (oas.GetIssueIntegrationResponseObject, error) {
	p := principalFrom(ctx)
	in, err := s.st.GetIssueIntegration(ctx, store.GetIssueIntegrationParams{WorkspaceID: p.workspaceID, ID: req.IntegrationId})
	if store.IsNotFound(err) {
		return nil, errIssueIntegrationGone
	}
	if err != nil {
		return nil, err
	}
	return oas.GetIssueIntegration200JSONResponse(s.issueIntegrationBody(in)), nil
}

func (s *Server) checkTrackerAPIURL(raw string) (string, error) {
	u, err := webhook.CheckURL(raw, s.webhooks.AllowPrivate)
	if err != nil || u.Scheme != "https" || u.RawQuery != "" || u.Fragment != "" {
		return "", errValidation("api_url must be an https URL of a GitHub Enterprise Server's API on a public address")
	}
	return strings.TrimSuffix(u.String(), "/"), nil
}

func webhookSecretInput(v *string) (string, error) {
	sec := strings.TrimSpace(*v)
	if len(sec) < 16 || len(sec) > 200 {
		return "", errValidation("webhook_secret must be 16 to 200 characters")
	}
	return sec, nil
}

func defaultTargetInput(provider string, v string) (*string, error) {
	t := strings.TrimSpace(v)
	if !issues.ValidTarget(provider, t) {
		if provider == issues.Linear {
			return nil, errValidation("default_target must be a Linear team key or id")
		}
		return nil, errValidation("default_target must be owner/repo")
	}
	return &t, nil
}

func (s *Server) CreateIssueIntegration(ctx context.Context, req oas.CreateIssueIntegrationRequestObject) (oas.CreateIssueIntegrationResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireIntegrationManager(p); err != nil {
		return nil, err
	}
	b := req.Body
	if !b.Provider.Valid() {
		return nil, errValidation("provider must be github or linear")
	}
	provider := string(b.Provider)
	if b.Token == nil || strings.TrimSpace(*b.Token) == "" || len(*b.Token) > 1000 {
		return nil, errValidation("token is required, at most 1,000 characters")
	}
	token := strings.TrimSpace(*b.Token)
	var apiURL, target *string
	if b.ApiUrl != nil {
		if provider != issues.GitHub {
			return nil, errValidation("api_url is only for github")
		}
		u, err := s.checkTrackerAPIURL(*b.ApiUrl)
		if err != nil {
			return nil, err
		}
		apiURL = &u
	}
	if b.DefaultTarget != nil {
		var err error
		if target, err = defaultTargetInput(provider, *b.DefaultTarget); err != nil {
			return nil, err
		}
	}
	secret, made := newIssueWebhookSecret(), true
	if b.WebhookSecret != nil {
		var err error
		if secret, err = webhookSecretInput(b.WebhookSecret); err != nil {
			return nil, err
		}
		made = false
	}
	n, err := s.st.CountIssueIntegrations(ctx, p.workspaceID)
	if err != nil {
		return nil, err
	}
	if n >= maxIssueIntegrations {
		return nil, errTooManyIntegrations
	}
	if err := s.rateLimit(ctx, rateCheck{"issues:" + p.workspaceID.String(), limitIssueCallsPerWorkspace}); err != nil {
		return nil, err
	}
	account, err := s.trackerClient(provider, apiURL, token).Verify(ctx)
	if err != nil {
		return nil, trackerProblem(err)
	}
	name := providerName(provider) + " · " + account
	if b.Name != nil {
		if name, err = trimmed(*b.Name, 1, 100, "name"); err != nil {
			return nil, err
		}
	}
	id := newID()
	var in store.IssueIntegration
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		arg := store.CreateIssueIntegrationParams{
			ID: id, WorkspaceID: p.workspaceID, Provider: provider, Name: name, Account: truncateRunes(account, 200), ApiUrl: apiURL, DefaultTarget: target,
			Token: s.secrets.Seal([]byte(token), issueTokenContext(p.workspaceID, id)), WebhookSecret: s.secrets.Seal([]byte(secret), issueSecretContext(p.workspaceID, id)),
			Now: s.now(),
		}
		if !p.isKey() && p.memberID != uuid.Nil() {
			arg.CreatedByMemberID = new(p.memberID)
		}
		var err error
		if in, err = q.CreateIssueIntegration(ctx, arg); err != nil {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditIssueIntegrationConnected, oas.AuditTargetIssueIntegration, id, integrationAuditDetails(in))
	})
	if err != nil {
		return nil, err
	}
	out := oas.IssueIntegrationSecret{Integration: s.issueIntegrationBody(in)}
	if made {
		out.WebhookSecret = &secret
	}
	return oas.CreateIssueIntegration201JSONResponse(out), nil
}

func providerName(provider string) string {
	if provider == issues.Linear {
		return "Linear"
	}
	return "GitHub"
}

func integrationAuditDetails(in store.IssueIntegration) auditDetails {
	d := auditDetails{"provider": in.Provider, "name": in.Name, "account": in.Account}
	if in.ApiUrl != nil {
		d["host"] = urlHost(*in.ApiUrl)
	}
	return d
}

func (s *Server) UpdateIssueIntegration(ctx context.Context, req oas.UpdateIssueIntegrationRequestObject) (oas.UpdateIssueIntegrationResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireIntegrationManager(p); err != nil {
		return nil, err
	}
	b := req.Body
	if b.Name == nil && b.Token == nil && !b.DefaultTarget.IsSpecified() && b.WebhookSecret == nil {
		return nil, errValidation("send name, token, default_target or webhook_secret")
	}
	cur, err := s.st.GetIssueIntegration(ctx, store.GetIssueIntegrationParams{WorkspaceID: p.workspaceID, ID: req.IntegrationId})
	if store.IsNotFound(err) {
		return nil, errIssueIntegrationGone
	}
	if err != nil {
		return nil, err
	}
	var fields []string
	var name, secret, token, account string
	var target *string
	if b.Name != nil {
		if name, err = trimmed(*b.Name, 1, 100, "name"); err != nil {
			return nil, err
		}
		fields = append(fields, "name")
	}
	if b.DefaultTarget.IsSpecified() {
		if !b.DefaultTarget.IsNull() {
			if target, err = defaultTargetInput(cur.Provider, b.DefaultTarget.MustGet()); err != nil {
				return nil, err
			}
		}
		fields = append(fields, "default_target")
	}
	if b.WebhookSecret != nil {
		if secret, err = webhookSecretInput(b.WebhookSecret); err != nil {
			return nil, err
		}
		fields = append(fields, "webhook_secret")
	}
	if b.Token != nil {
		token = strings.TrimSpace(*b.Token)
		if token == "" || len(token) > 1000 {
			return nil, errValidation("token must be 1 to 1,000 characters")
		}
		if err := s.rateLimit(ctx, rateCheck{"issues:" + p.workspaceID.String(), limitIssueCallsPerWorkspace}); err != nil {
			return nil, err
		}
		if account, err = s.trackerClient(cur.Provider, cur.ApiUrl, token).Verify(ctx); err != nil {
			return nil, trackerProblem(err)
		}
		fields = append(fields, "token")
	}
	var in store.IssueIntegration
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		locked, err := q.LockIssueIntegration(ctx, store.LockIssueIntegrationParams{WorkspaceID: p.workspaceID, ID: cur.ID})
		if store.IsNotFound(err) {
			return errIssueIntegrationGone
		}
		if err != nil {
			return err
		}
		arg := store.UpdateIssueIntegrationParams{
			WorkspaceID: p.workspaceID, ID: locked.ID, Name: locked.Name, Account: locked.Account, DefaultTarget: locked.DefaultTarget,
			Token: locked.Token, WebhookSecret: locked.WebhookSecret, Now: s.now(),
		}
		if b.Name != nil {
			arg.Name = name
		}
		if b.DefaultTarget.IsSpecified() {
			arg.DefaultTarget = target
		}
		if b.WebhookSecret != nil {
			arg.WebhookSecret = s.secrets.Seal([]byte(secret), issueSecretContext(p.workspaceID, locked.ID))
		}
		if b.Token != nil {
			arg.Account, arg.Token, arg.TokenChanged = truncateRunes(account, 200), s.secrets.Seal([]byte(token), issueTokenContext(p.workspaceID, locked.ID)), true
		}
		if in, err = q.UpdateIssueIntegration(ctx, arg); err != nil {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditIssueIntegrationUpdated, oas.AuditTargetIssueIntegration, in.ID, auditDetails{"fields": fields})
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateIssueIntegration200JSONResponse(s.issueIntegrationBody(in)), nil
}

func (s *Server) DeleteIssueIntegration(ctx context.Context, req oas.DeleteIssueIntegrationRequestObject) (oas.DeleteIssueIntegrationResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireIntegrationManager(p); err != nil {
		return nil, err
	}
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		in, err := q.GetIssueIntegration(ctx, store.GetIssueIntegrationParams{WorkspaceID: p.workspaceID, ID: req.IntegrationId})
		if store.IsNotFound(err) {
			return errIssueIntegrationGone
		}
		if err != nil {
			return err
		}
		views, err := issueLinkViews(ctx, q, p.workspaceID, issueLinkFilter{integration: &in.ID})
		if err != nil {
			return err
		}
		if _, err := q.DeleteIssueIntegration(ctx, store.DeleteIssueIntegrationParams{WorkspaceID: p.workspaceID, ID: in.ID}); err != nil {
			return err
		}
		for _, v := range views {
			addIssueLinkEvent(events, eventIssueLinkDeleted, v, nil)
		}
		return s.audit(ctx, q, p, oas.AuditIssueIntegrationDisconnected, oas.AuditTargetIssueIntegration, in.ID, integrationAuditDetails(in))
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteIssueIntegration204Response{}, nil
}

func (s *Server) RotateIssueWebhookSecret(ctx context.Context, req oas.RotateIssueWebhookSecretRequestObject) (oas.RotateIssueWebhookSecretResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireIntegrationManager(p); err != nil {
		return nil, err
	}
	secret := newIssueWebhookSecret()
	var in store.IssueIntegration
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		cur, err := q.LockIssueIntegration(ctx, store.LockIssueIntegrationParams{WorkspaceID: p.workspaceID, ID: req.IntegrationId})
		if store.IsNotFound(err) {
			return errIssueIntegrationGone
		}
		if err != nil {
			return err
		}
		in, err = q.UpdateIssueIntegration(ctx, store.UpdateIssueIntegrationParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, Name: cur.Name, Account: cur.Account, DefaultTarget: cur.DefaultTarget, Token: cur.Token,
			WebhookSecret: s.secrets.Seal([]byte(secret), issueSecretContext(p.workspaceID, cur.ID)), Now: s.now(),
		})
		if err != nil {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditIssueIntegrationSecretRotated, oas.AuditTargetIssueIntegration, in.ID, nil)
	})
	if err != nil {
		return nil, err
	}
	return oas.RotateIssueWebhookSecret200JSONResponse(oas.IssueIntegrationSecret{Integration: s.issueIntegrationBody(in), WebhookSecret: &secret}), nil
}

type issueLinkFilter struct {
	ids, conversations, ideas []uuid.UUID
	issue, integration        *uuid.UUID
}

func issueLinkViews(ctx context.Context, q *store.Queries, ws uuid.UUID, f issueLinkFilter) ([]store.ListIssueLinkViewsRow, error) {
	nz := func(v []uuid.UUID) []uuid.UUID {
		if v == nil {
			return []uuid.UUID{}
		}
		return v
	}
	return q.ListIssueLinkViews(ctx, store.ListIssueLinkViewsParams{
		WorkspaceID: ws, Ids: nz(f.ids), ConversationIds: nz(f.conversations), IdeaIds: nz(f.ideas), IssueID: f.issue, IntegrationID: f.integration,
	})
}

func issueLinkBody(v store.ListIssueLinkViewsRow) oas.IssueLink {
	l, t := v.IssueLink, v.TrackedIssue
	inbox := v.InboxID
	out := oas.IssueLink{
		Id: l.ID, IntegrationId: t.IntegrationID, Provider: oas.IssueProvider(v.Provider), Key: t.Key, Url: t.Url, Title: t.Title, State: t.State,
		Progress: oas.FeedbackProgress(t.Progress), ConversationId: l.ConversationID, IdeaId: l.IdeaID, InboxId: &inbox, LinkedAt: l.CreatedAt,
		StateChangedAt: t.StateChangedAt, SyncedAt: &t.SyncedAt,
	}
	if l.LinkedByMemberID != nil || l.LinkedByApiKeyID != nil {
		out.LinkedBy = &oas.IssueLinkAuthor{MemberId: l.LinkedByMemberID, ApiKeyId: l.LinkedByApiKeyID}
	}
	return out
}

func addIssueLinkEvent(events *eventBatch, typ string, v store.ListIssueLinkViewsRow, previous *string) {
	inbox := v.InboxID
	events.add(typ, &inbox, v.IssueLink.ConversationID, oas.IssueLinkEventData{IssueLink: issueLinkBody(v), PreviousProgress: (*oas.FeedbackProgress)(previous)})
}

func fillIssues(ctx context.Context, q *store.Queries, ws uuid.UUID, convs ...*oas.Conversation) error {
	var ids []uuid.UUID
	for _, c := range convs {
		if c.Kind == oas.ConversationKindFeedback {
			ids = append(ids, c.Id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := issueLinkViews(ctx, q, ws, issueLinkFilter{conversations: ids})
	if err != nil {
		return err
	}
	for _, r := range rows {
		for _, c := range convs {
			if r.IssueLink.ConversationID != nil && c.Id == *r.IssueLink.ConversationID {
				list := append(derefIssues(c.Issues), issueLinkBody(r))
				c.Issues = &list
			}
		}
	}
	return nil
}

func derefIssues(v *[]oas.IssueLink) []oas.IssueLink {
	if v == nil {
		return nil
	}
	return *v
}

func fillIdeaIssues(ctx context.Context, q *store.Queries, ws uuid.UUID, d *oas.Discussion) error {
	rows, err := issueLinkViews(ctx, q, ws, issueLinkFilter{ideas: []uuid.UUID{d.Id}})
	if err != nil || len(rows) == 0 {
		return err
	}
	list := make([]oas.IssueLink, len(rows))
	for i, r := range rows {
		list[i] = issueLinkBody(r)
	}
	d.Issues = &list
	return nil
}

func issueList(rows []store.ListIssueLinkViewsRow) oas.IssueLinkList {
	out := oas.IssueLinkList{Items: make([]oas.IssueLink, len(rows))}
	for i, r := range rows {
		out.Items[i] = issueLinkBody(r)
	}
	return out
}

func (s *Server) ListConversationIssues(ctx context.Context, req oas.ListConversationIssuesRequestObject) (oas.ListConversationIssuesResponseObject, error) {
	p := principalFrom(ctx)
	if _, err := visibleConversation(ctx, s.st.Queries, p, req.ConversationId, false); err != nil {
		return nil, err
	}
	rows, err := issueLinkViews(ctx, s.st.Queries, p.workspaceID, issueLinkFilter{conversations: []uuid.UUID{req.ConversationId}})
	if err != nil {
		return nil, err
	}
	return oas.ListConversationIssues200JSONResponse(issueList(rows)), nil
}

func (s *Server) ListIdeaIssues(ctx context.Context, req oas.ListIdeaIssuesRequestObject) (oas.ListIdeaIssuesResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if _, err := visibleIdea(ctx, s.st.Queries, p, req.DiscussionId, false); err != nil {
		return nil, err
	}
	rows, err := issueLinkViews(ctx, s.st.Queries, p.workspaceID, issueLinkFilter{ideas: []uuid.UUID{req.DiscussionId}})
	if err != nil {
		return nil, err
	}
	return oas.ListIdeaIssues200JSONResponse(issueList(rows)), nil
}

// issueSubject is what an issue is linked to: a feedback conversation or an idea, checked for the
// caller in q.
type issueSubject struct {
	conversation, idea *uuid.UUID
	path               string
}

func (s *Server) feedbackSubject(p principal, id uuid.UUID) func(ctx context.Context, q *store.Queries, lock bool) (issueSubject, error) {
	return func(ctx context.Context, q *store.Queries, lock bool) (issueSubject, error) {
		c, err := visibleConversation(ctx, q, p, id, lock)
		if err != nil {
			return issueSubject{}, err
		}
		if c.Kind != string(oas.ConversationKindFeedback) {
			return issueSubject{}, errNotFeedbackLink
		}
		return issueSubject{conversation: &c.ID, path: conversationPath(p.workspaceID, c.ID)}, nil
	}
}

func (s *Server) ideaSubject(p principal, id uuid.UUID) func(ctx context.Context, q *store.Queries, lock bool) (issueSubject, error) {
	return func(ctx context.Context, q *store.Queries, lock bool) (issueSubject, error) {
		t, err := teamThread(ctx, q, p, id, lock)
		if err != nil {
			return issueSubject{}, err
		}
		if t.d.Kind != kindIdea {
			return issueSubject{}, errNotAnIdea
		}
		return issueSubject{idea: &t.d.ID, path: "/board/ideas/" + t.d.ID.String() + "?workspace_id=" + p.workspaceID.String()}, nil
	}
}

func (s *Server) LinkConversationIssue(ctx context.Context, req oas.LinkConversationIssueRequestObject) (oas.LinkConversationIssueResponseObject, error) {
	p := principalFrom(ctx)
	link, created, err := s.linkIssue(ctx, p, *req.Body, s.feedbackSubject(p, req.ConversationId))
	if err != nil {
		return nil, err
	}
	if created {
		return oas.LinkConversationIssue201JSONResponse(link), nil
	}
	return oas.LinkConversationIssue200JSONResponse(link), nil
}

func (s *Server) LinkIdeaIssue(ctx context.Context, req oas.LinkIdeaIssueRequestObject) (oas.LinkIdeaIssueResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	link, created, err := s.linkIssue(ctx, p, *req.Body, s.ideaSubject(p, req.DiscussionId))
	if err != nil {
		return nil, err
	}
	if created {
		return oas.LinkIdeaIssue201JSONResponse(link), nil
	}
	return oas.LinkIdeaIssue200JSONResponse(link), nil
}

func issueBody(body *string, link string) string {
	text := ""
	if body != nil {
		text = strings.TrimSpace(*body)
	}
	back := "From Yuva: " + link
	if text == "" {
		return back
	}
	return text + "\n\n---\n" + back
}

// linkIssue finds or creates the issue in the tracker, then links it in one transaction that
// stores the issue, writes the link, its event and the progress or stage the issue's state sets.
func (s *Server) linkIssue(ctx context.Context, p principal, b oas.IssueLinkCreate, subject func(context.Context, *store.Queries, bool) (issueSubject, error)) (oas.IssueLink, bool, error) {
	var out oas.IssueLink
	if (b.Url == nil) == (b.Title == nil) {
		return out, false, errValidation("send url to link an issue or title to create one")
	}
	var title string
	if b.Title != nil {
		var err error
		if title, err = trimmed(*b.Title, 1, 256, "title"); err != nil {
			return out, false, err
		}
		if b.Body != nil && len([]rune(*b.Body)) > 20000 {
			return out, false, errValidation("body must be at most 20,000 characters")
		}
	} else if b.Body != nil || b.Target != nil {
		return out, false, errValidation("body and target are only for creating an issue")
	}
	sub, err := subject(ctx, s.st.Queries, false)
	if err != nil {
		return out, false, err
	}
	in, err := s.st.GetIssueIntegration(ctx, store.GetIssueIntegrationParams{WorkspaceID: p.workspaceID, ID: b.IntegrationId})
	if store.IsNotFound(err) {
		return out, false, errIssueIntegrationGone
	}
	if err != nil {
		return out, false, err
	}
	n, err := s.st.CountIssueLinksOfTarget(ctx, store.CountIssueLinksOfTargetParams{WorkspaceID: p.workspaceID, ConversationID: sub.conversation, IdeaID: sub.idea})
	if err != nil {
		return out, false, err
	}
	if n >= maxIssuesPerTarget {
		return out, false, errTooManyIssues
	}
	if err := s.rateLimit(ctx, rateCheck{"issues:" + p.workspaceID.String(), limitIssueCallsPerWorkspace}); err != nil {
		return out, false, err
	}
	cl, err := s.integrationClient(in)
	if err != nil {
		return out, false, err
	}
	var iss issues.Issue
	if b.Url != nil {
		iss, err = cl.Find(ctx, *b.Url)
	} else {
		target := in.DefaultTarget
		if b.Target != nil {
			t := strings.TrimSpace(*b.Target)
			target = &t
		}
		if target == nil || *target == "" {
			return out, false, errValidation("target is required: the integration has no default_target")
		}
		iss, err = cl.Create(ctx, *target, title, issueBody(b.Body, s.auth.PublicURL+sub.path))
	}
	if err != nil {
		return out, false, trackerProblem(err)
	}
	fetched := s.now()
	var linkID uuid.UUID
	created := false
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		sub, err := subject(ctx, q, true)
		if err != nil {
			return err
		}
		t, moved, err := s.storeIssue(ctx, q, events, in, iss, fetched)
		if err != nil {
			return err
		}
		if moved {
			events.job(IssueSyncArgs{WorkspaceID: p.workspaceID, IssueID: t.ID}, nil)
		}
		if l, err := q.FindIssueLink(ctx, store.FindIssueLinkParams{WorkspaceID: p.workspaceID, IssueID: t.ID, ConversationID: sub.conversation, IdeaID: sub.idea}); err == nil {
			linkID = l.ID
			return nil
		} else if !store.IsNotFound(err) {
			return err
		}
		if n, err := q.CountIssueLinksOfTarget(ctx, store.CountIssueLinksOfTargetParams{WorkspaceID: p.workspaceID, ConversationID: sub.conversation, IdeaID: sub.idea}); err != nil {
			return err
		} else if n >= maxIssuesPerTarget {
			return errTooManyIssues
		}
		if n, err := q.CountIssueLinksOfIssue(ctx, store.CountIssueLinksOfIssueParams{WorkspaceID: p.workspaceID, IssueID: t.ID}); err != nil {
			return err
		} else if n >= maxLinksPerIssue {
			return errTooManyIssueLinks
		}
		arg := store.InsertIssueLinkParams{ID: newID(), WorkspaceID: p.workspaceID, IssueID: t.ID, ConversationID: sub.conversation, IdeaID: sub.idea, Via: p.via, Now: s.now(),
			SsoSettingsID: p.ssoSettings}
		arg.LinkedByMemberID, arg.LinkedByApiKeyID = statusAuthor(p)
		if p.isGrant() {
			arg.LinkedByGrantID = new(p.grantID)
		}
		l, err := q.InsertIssueLink(ctx, arg)
		if err != nil {
			return err
		}
		linkID, created = l.ID, true
		views, err := issueLinkViews(ctx, q, p.workspaceID, issueLinkFilter{ids: []uuid.UUID{l.ID}})
		if err != nil || len(views) == 0 {
			return err
		}
		addIssueLinkEvent(events, eventIssueLinkCreated, views[0], nil)
		return s.applyIssueLink(ctx, q, events, p, l, t)
	})
	if err != nil {
		return out, false, err
	}
	views, err := issueLinkViews(ctx, s.st.Queries, p.workspaceID, issueLinkFilter{ids: []uuid.UUID{linkID}})
	if err != nil {
		return out, false, err
	}
	if len(views) == 0 {
		return out, false, errIssueLinkGone
	}
	return issueLinkBody(views[0]), created, nil
}

// storeIssue saves what the tracker answered at fetched, unless a newer answer is stored already.
// moved reports that the issue's progress changed for links that were there before; each of
// them gets issue_link.updated when anything they show changed.
func (s *Server) storeIssue(ctx context.Context, q *store.Queries, events *eventBatch, in store.IssueIntegration, iss issues.Issue, fetched time.Time) (store.TrackedIssue, bool, error) {
	cur, err := q.GetTrackedIssueByExternal(ctx, store.GetTrackedIssueByExternalParams{WorkspaceID: in.WorkspaceID, IntegrationID: in.ID, ExternalID: iss.ExternalID})
	if store.IsNotFound(err) {
		t, err := q.InsertTrackedIssue(ctx, store.InsertTrackedIssueParams{
			ID: newID(), WorkspaceID: in.WorkspaceID, IntegrationID: in.ID, ExternalID: iss.ExternalID, Key: truncateRunes(iss.Key, 300),
			Url: truncateRunes(iss.URL, 2000), Title: truncateRunes(iss.Title, 1000), State: truncateRunes(iss.State, 100), Progress: iss.Progress, Now: fetched, NextSyncAt: fetched.Add(issueStaleAfter),
		})
		return t, false, err
	}
	if err != nil {
		return cur, false, err
	}
	return s.refreshIssue(ctx, q, events, cur, iss, fetched)
}

func (s *Server) refreshIssue(ctx context.Context, q *store.Queries, events *eventBatch, cur store.TrackedIssue, iss issues.Issue, fetched time.Time) (store.TrackedIssue, bool, error) {
	if !fetched.After(cur.SyncedAt) {
		return cur, false, nil
	}
	t, err := q.UpdateTrackedIssue(ctx, store.UpdateTrackedIssueParams{
		WorkspaceID: cur.WorkspaceID, ID: cur.ID, Key: truncateRunes(iss.Key, 300), Url: truncateRunes(iss.URL, 2000), Title: truncateRunes(iss.Title, 1000),
		State: truncateRunes(iss.State, 100), Progress: iss.Progress, Now: fetched, SyncedAt: fetched, NextSyncAt: fetched.Add(issueStaleAfter),
	})
	if err != nil {
		return cur, false, err
	}
	moved := t.Progress != cur.Progress
	if !moved && t.Key == cur.Key && t.Url == cur.Url && t.Title == cur.Title && t.State == cur.State {
		return t, false, nil
	}
	views, err := issueLinkViews(ctx, q, cur.WorkspaceID, issueLinkFilter{issue: &cur.ID})
	if err != nil {
		return t, moved, err
	}
	var prev *string
	if moved {
		prev = &cur.Progress
	}
	for _, v := range views {
		v.TrackedIssue = t
		addIssueLinkEvent(events, eventIssueLinkUpdated, v, prev)
	}
	return t, moved, nil
}

// applyIssueLink sets the issue's progress on the link's feedback, as p, and records it on the link
// so it is not set again until the progress changes. An idea only shows the issue's state: moving
// its stage stays a deliberate action.
func (s *Server) applyIssueLink(ctx context.Context, q *store.Queries, events *eventBatch, p principal, l store.IssueLink, t store.TrackedIssue) error {
	switch {
	case requireScope(p, oas.IssuesWrite) != nil:
	case l.ConversationID != nil:
		c, err := visibleConversation(ctx, q, p, *l.ConversationID, true)
		if err != nil {
			return err
		}
		if c.FeedbackProgress == nil || *c.FeedbackProgress != t.Progress {
			ictx := context.WithValue(ctx, issueProgressKey, l.ID)
			if _, err := s.changeConversation(ictx, q, events, p, c.ID, oas.ConversationUpdate{
				FeedbackProgress: nullable.NewNullableWithValue(oas.FeedbackProgress(t.Progress)),
			}, nil, nil); err != nil {
				return err
			}
		}
	}
	return q.SetIssueLinkApplied(ctx, store.SetIssueLinkAppliedParams{WorkspaceID: l.WorkspaceID, ID: l.ID, Progress: &t.Progress})
}

func (s *Server) UnlinkConversationIssue(ctx context.Context, req oas.UnlinkConversationIssueRequestObject) (oas.UnlinkConversationIssueResponseObject, error) {
	p := principalFrom(ctx)
	if err := s.unlinkIssue(ctx, p, req.IssueLinkId, s.feedbackSubject(p, req.ConversationId)); err != nil {
		return nil, err
	}
	return oas.UnlinkConversationIssue204Response{}, nil
}

func (s *Server) UnlinkIdeaIssue(ctx context.Context, req oas.UnlinkIdeaIssueRequestObject) (oas.UnlinkIdeaIssueResponseObject, error) {
	if err := s.roadmapOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := s.unlinkIssue(ctx, p, req.IssueLinkId, s.ideaSubject(p, req.DiscussionId)); err != nil {
		return nil, err
	}
	return oas.UnlinkIdeaIssue204Response{}, nil
}

func (s *Server) unlinkIssue(ctx context.Context, p principal, linkID uuid.UUID, subject func(context.Context, *store.Queries, bool) (issueSubject, error)) error {
	return s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		sub, err := subject(ctx, q, true)
		if err != nil {
			return err
		}
		views, err := issueLinkViews(ctx, q, p.workspaceID, issueLinkFilter{ids: []uuid.UUID{linkID}})
		if err != nil {
			return err
		}
		l, err := q.DeleteIssueLink(ctx, store.DeleteIssueLinkParams{WorkspaceID: p.workspaceID, ID: linkID, ConversationID: sub.conversation, IdeaID: sub.idea})
		if store.IsNotFound(err) || len(views) == 0 {
			return errIssueLinkGone
		}
		if err != nil {
			return err
		}
		addIssueLinkEvent(events, eventIssueLinkDeleted, views[0], nil)
		return q.DeleteOrphanTrackedIssue(ctx, store.DeleteOrphanTrackedIssueParams{WorkspaceID: p.workspaceID, ID: l.IssueID})
	})
}

type IssueSyncArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	IssueID     uuid.UUID `json:"issue_id"`
}

func (IssueSyncArgs) Kind() string { return "issue_sync" }

func (IssueSyncArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.OutboundQueue, MaxAttempts: issueSyncAttempts}
}

type issueSyncWorker struct {
	river.WorkerDefaults[IssueSyncArgs]
	s *Server
}

func (w *issueSyncWorker) Work(ctx context.Context, job *river.Job[IssueSyncArgs]) error {
	return w.s.SyncIssue(ctx, job.Args)
}

// SyncIssue reads a linked issue from its tracker, stores it and sets its progress on every link
// that does not have it yet, each link in its own transaction as the member or key that made it.
func (s *Server) SyncIssue(ctx context.Context, a IssueSyncArgs) error {
	t, err := s.st.GetTrackedIssue(ctx, store.GetTrackedIssueParams{WorkspaceID: a.WorkspaceID, ID: a.IssueID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	in, err := s.st.GetIssueIntegration(ctx, store.GetIssueIntegrationParams{WorkspaceID: a.WorkspaceID, ID: t.IntegrationID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	cl, err := s.integrationClient(in)
	if err != nil {
		return err
	}
	iss, err := cl.Get(ctx, t.ExternalID)
	fetched := s.now()
	if err != nil {
		s.issueIntegrationError(ctx, in, err)
		return s.st.TrackedIssueFailed(ctx, store.TrackedIssueFailedParams{WorkspaceID: a.WorkspaceID, ID: t.ID, NextSyncAt: fetched.Add(issueBackoff(t.SyncFailures))})
	}
	s.issueIntegrationError(ctx, in, nil)
	err = s.inTx(ctx, a.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
		cur, err := q.LockTrackedIssue(ctx, store.LockTrackedIssueParams{WorkspaceID: a.WorkspaceID, ID: t.ID})
		if err != nil {
			return err
		}
		iss.ExternalID = cur.ExternalID
		_, _, err = s.refreshIssue(ctx, q, events, cur, iss, fetched)
		return err
	})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.applyIssueLinks(ctx, a.WorkspaceID, t.ID)
}

// issueBackoff is the wait after a failed read that followed failures earlier ones: 15 minutes,
// doubling up to a day.
func issueBackoff(failures int32) time.Duration {
	d := issueSweepHold << min(max(failures, 0), 10)
	return min(d, issueBackoffMax)
}

func (s *Server) issueIntegrationError(ctx context.Context, in store.IssueIntegration, err error) {
	var msg *string
	if err != nil {
		m := truncateRunes(err.Error(), 1000)
		msg = &m
	}
	if e := s.st.SetIssueIntegrationError(ctx, store.SetIssueIntegrationErrorParams{WorkspaceID: in.WorkspaceID, ID: in.ID, Error: msg, Now: s.now()}); e != nil {
		s.log.WarnContext(ctx, "issue integration error not recorded", slog.String("integration_id", in.ID.String()), slog.Any("error", e))
	}
}

func (s *Server) applyIssueLinks(ctx context.Context, ws, issueID uuid.UUID) error {
	links, err := s.st.IssueLinksToApply(ctx, store.IssueLinksToApplyParams{WorkspaceID: ws, IssueID: issueID})
	if err != nil {
		return err
	}
	for _, l := range links {
		p, refused, err := s.callerPrincipal(ctx, ws, l.LinkedByMemberID, l.LinkedByApiKeyID, l.LinkedByGrantID, l.SsoSettingsID, l.Via, false)
		if err != nil {
			return err
		}
		if refused != "" && refused != reasonRevoked {
			s.log.WarnContext(ctx, "issue progress not set", slog.String("issue_link_id", l.ID.String()), slog.String("code", refused))
		}
		ok := refused == ""
		err = s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
			cur, err := q.LockIssueLink(ctx, store.LockIssueLinkParams{WorkspaceID: ws, ID: l.ID})
			if err != nil {
				return err
			}
			t, err := q.GetTrackedIssue(ctx, store.GetTrackedIssueParams{WorkspaceID: ws, ID: issueID})
			if err != nil {
				return err
			}
			if cur.AppliedProgress != nil && *cur.AppliedProgress == t.Progress {
				return nil
			}
			if !ok {
				return q.SetIssueLinkApplied(ctx, store.SetIssueLinkAppliedParams{WorkspaceID: ws, ID: l.ID, Progress: &t.Progress})
			}
			return s.applyIssueLink(ctx, q, events, p, cur, t)
		})
		var e *apiError
		switch {
		case err == nil || store.IsNotFound(err):
		case errors.As(err, &e) && e.Status < http.StatusInternalServerError && e.Status != http.StatusTooManyRequests:
			s.log.WarnContext(ctx, "issue progress not set", slog.String("issue_link_id", l.ID.String()), slog.String("code", e.Code))
			if err := s.markIssueLinkApplied(ctx, ws, l.ID, issueID); err != nil {
				return err
			}
		default:
			return err
		}
	}
	return nil
}

func (s *Server) markIssueLinkApplied(ctx context.Context, ws, linkID, issueID uuid.UUID) error {
	t, err := s.st.GetTrackedIssue(ctx, store.GetTrackedIssueParams{WorkspaceID: ws, ID: issueID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.st.SetIssueLinkApplied(ctx, store.SetIssueLinkAppliedParams{WorkspaceID: ws, ID: linkID, Progress: &t.Progress})
}

type issueSweepWorker struct {
	river.WorkerDefaults[jobs.IssueSweepArgs]
	s *Server
}

func (w *issueSweepWorker) Work(ctx context.Context, _ *river.Job[jobs.IssueSweepArgs]) error {
	_, err := w.s.SweepIssues(ctx)
	return err
}

// SweepIssues queues a read of the linked issues, in every workspace, that are due: an hour after
// the last read, later after each failure. Each integration gets at most issueSweepPerIntegration of
// a sweep, so a tracker that keeps failing never takes the others' turn.
func (s *Server) SweepIssues(ctx context.Context) (int, error) {
	now := s.now()
	rows, err := s.st.DueTrackedIssues(ctx, store.DueTrackedIssuesParams{
		Now: now, HoldUntil: now.Add(issueSweepHold), PerIntegration: int64(issueSweepPerIntegration), Lim: int32(issueSweepBatch),
	})
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	batch := make([]river.InsertManyParams, len(rows))
	for i, r := range rows {
		batch[i] = river.InsertManyParams{Args: IssueSyncArgs{WorkspaceID: r.WorkspaceID, IssueID: r.ID}}
	}
	if _, err := s.jobs.InsertMany(ctx, batch); err != nil {
		return 0, err
	}
	return len(rows), nil
}

// serveIssueWebhook finds the integration by its id (before the workspace is known), checks the
// signature with its secret and queues a read of each linked issue the event names.
func (s *Server) serveIssueWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(r.PathValue("integrationId"))
	if err != nil {
		writeProblem(w, errIssueIntegrationGone)
		return
	}
	in, err := s.st.FindIssueIntegration(ctx, id)
	if store.IsNotFound(err) {
		writeProblem(w, errIssueIntegrationGone)
		return
	}
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if err := s.rateLimit(ctx, rateCheck{"issue-webhook:" + id.String(), limitIssueWebhook}); err != nil {
		s.writeError(w, r, err)
		return
	}
	body, err := readLimited(r, maxIssueWebhookBytes)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if in.WebhookSecret == nil {
		writeProblem(w, errIssueBadSignature)
		return
	}
	secret, err := s.secrets.Open(in.WebhookSecret, issueSecretContext(in.WorkspaceID, in.ID))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var ids []string
	if in.Provider == issues.Linear {
		if !issues.VerifyLinear(secret, body, r.Header.Get("Linear-Signature"), s.now()) {
			writeProblem(w, errIssueBadSignature)
			return
		}
		ids = issues.LinearEventIssues(body)
	} else {
		if !issues.VerifyGitHub(secret, body, r.Header.Get("X-Hub-Signature-256")) {
			writeProblem(w, errIssueBadSignature)
			return
		}
		ids = issues.GitHubEventIssues(r.Header.Get("X-GitHub-Event"), body)
	}
	if len(ids) > 0 {
		if live, err := s.workspaceLive(ctx, in.WorkspaceID); err != nil || !live {
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			writeProblem(w, errIssueIntegrationGone)
			return
		}
		found, err := s.st.FindTrackedIssues(ctx, store.FindTrackedIssuesParams{WorkspaceID: in.WorkspaceID, IntegrationID: in.ID, ExternalIds: ids})
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		if len(found) > 0 {
			batch := make([]river.InsertManyParams, len(found))
			for i, issueID := range found {
				batch[i] = river.InsertManyParams{Args: IssueSyncArgs{WorkspaceID: in.WorkspaceID, IssueID: issueID}}
			}
			if _, err := s.jobs.InsertMany(ctx, batch); err != nil {
				s.writeError(w, r, err)
				return
			}
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

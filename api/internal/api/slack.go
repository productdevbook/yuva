package api

import (
	"cmp"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/slack"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	slackCallbackPath = "/slack/oauth/callback"
	slackEventsPath   = "/ingress/slack"
	slackConnectTTL   = 10 * time.Minute
	slackLinkTTL      = 10 * time.Minute
	slackLinkTries    = 5
	slackEventsKept   = 24 * time.Hour
	slackMaxBody      = 1 << 20
	slackChannelLimit = 1000
	slackPreviewRunes = 300
	slackPostAttempts = 5
	slackCallTimeout  = 30 * time.Second
	slackVia          = "Slack"

	slackNewConversation = "new_conversation"
	slackMention         = "mention"
)

// SlackSettings is the Slack app the operator registered (YUVA_SLACK_*); Slack is off unless all
// three are set. APIBase and HTTPClient point the client elsewhere, for tests.
type SlackSettings struct {
	ClientID      string
	ClientSecret  string
	SigningSecret string
	APIBase       string
	HTTPClient    *http.Client
}

func (d SlackSettings) on() bool {
	return d.ClientID != "" && d.ClientSecret != "" && d.SigningSecret != ""
}

var (
	errSlackNotConfigured = problem(http.StatusConflict, "slack_not_configured", "this server has no Slack app (YUVA_SLACK_CLIENT_ID, YUVA_SLACK_CLIENT_SECRET, YUVA_SLACK_SIGNING_SECRET)")
	errSlackNotConnected  = problem(http.StatusConflict, "slack_not_connected", "this workspace has not connected Slack")
	errSlackUserNotFound  = problem(http.StatusNotFound, "slack_user_not_found", "the connected Slack workspace has no user with that e-mail address")
	errSlackUserTaken     = problem(http.StatusConflict, "slack_user_taken", "another member has linked that Slack user")
	errSlackSignature     = problem(http.StatusUnauthorized, "invalid_signature", "the Slack signature or timestamp is not valid")
	slackReplyCommandRe   = regexp.MustCompile(`^![a-z]{1,20}$`)
	slackChannelRe        = regexp.MustCompile(`^[CG][A-Z0-9]{1,40}$`)
	slackTeamRe           = regexp.MustCompile(`^[A-Z0-9]{1,40}$`)
	slackUserRe           = regexp.MustCompile(`^[UW][A-Z0-9]{1,40}$`)
	slackCodeRe           = regexp.MustCompile(`^[0-9]{6}$`)
)

func errSlackFailed(err error) *apiError {
	var se *slack.Error
	if errors.As(err, &se) {
		e := problem(http.StatusBadGateway, "slack_failed", "Slack answered "+se.Code)
		if se.RetryAfter > 0 {
			e.RetryAfter = int(se.RetryAfter.Seconds())
		}
		return e
	}
	return problem(http.StatusBadGateway, "slack_failed", "Slack did not answer")
}

func newSlackClient(d SlackSettings, sleep func(ctx context.Context, d time.Duration) error) *slack.Client {
	hc := d.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &slack.Client{HTTP: hc, Base: d.APIBase, Sleep: sleep}
}

func slackTokenContext(ws uuid.UUID) []byte { return append([]byte("slack:"), ws[:]...) }

func (s *Server) slackRedirectURL() string {
	return strings.TrimRight(s.auth.PublicURL, "/") + slackCallbackPath
}

func (s *Server) slackEventsURL() string {
	return strings.TrimRight(s.auth.PublicURL, "/") + slackEventsPath
}

func (s *Server) slackToken(inst store.SlackInstallation) (string, error) {
	plain, err := s.secrets.Open(inst.BotToken, slackTokenContext(inst.WorkspaceID))
	if err != nil {
		return "", fmt.Errorf("slack token cannot be decrypted: %w", err)
	}
	return string(plain), nil
}

func (s *Server) slackConnectionBody(inst *store.SlackInstallation) oas.SlackConnection {
	out := oas.SlackConnection{Available: s.slack.on(), ReplyCommand: "!reply", RedirectUrl: s.slackRedirectURL(), EventsUrl: s.slackEventsURL()}
	if inst == nil {
		return out
	}
	out.Connected, out.ReplyCommand = true, inst.ReplyCommand
	out.TeamId, out.TeamName, out.BotUserId = &inst.TeamID, &inst.TeamName, &inst.BotUserID
	out.ConnectedAt, out.ConnectedBy, out.LastError, out.LastErrorAt = &inst.CreatedAt, inst.ConnectedBy, inst.LastError, inst.LastErrorAt
	return out
}

func slackInstallation(ctx context.Context, q *store.Queries, ws uuid.UUID) (*store.SlackInstallation, error) {
	inst, err := q.GetSlackInstallation(ctx, ws)
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

// connectedSlack is the workspace's installation and its token, or why there is none.
func (s *Server) connectedSlack(ctx context.Context, ws uuid.UUID) (store.SlackInstallation, string, error) {
	if !s.slack.on() {
		return store.SlackInstallation{}, "", errSlackNotConfigured
	}
	inst, err := s.st.GetSlackInstallation(ctx, ws)
	if store.IsNotFound(err) {
		return inst, "", errSlackNotConnected
	}
	if err != nil {
		return inst, "", err
	}
	token, err := s.slackToken(inst)
	return inst, token, err
}

func (s *Server) GetSlack(ctx context.Context, _ oas.GetSlackRequestObject) (oas.GetSlackResponseObject, error) {
	p := principalFrom(ctx)
	if !s.slack.on() {
		return oas.GetSlack200JSONResponse(s.slackConnectionBody(nil)), nil
	}
	inst, err := slackInstallation(ctx, s.st.Queries, p.workspaceID)
	if err != nil {
		return nil, err
	}
	return oas.GetSlack200JSONResponse(s.slackConnectionBody(inst)), nil
}

func (s *Server) UpdateSlack(ctx context.Context, req oas.UpdateSlackRequestObject) (oas.UpdateSlackResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	if !s.slack.on() {
		return nil, errSlackNotConfigured
	}
	inst, err := s.st.GetSlackInstallation(ctx, p.workspaceID)
	if store.IsNotFound(err) {
		return nil, errSlackNotConnected
	}
	if err != nil {
		return nil, err
	}
	if c := req.Body.ReplyCommand; c != nil {
		if !slackReplyCommandRe.MatchString(*c) {
			return nil, errValidation("reply_command must be ! and 1 to 20 lowercase letters, such as !reply")
		}
		inst, err = s.st.SetSlackReplyCommand(ctx, store.SetSlackReplyCommandParams{WorkspaceID: p.workspaceID, ReplyCommand: *c, Now: s.now()})
		if store.IsNotFound(err) {
			return nil, errSlackNotConnected
		}
		if err != nil {
			return nil, err
		}
	}
	return oas.UpdateSlack200JSONResponse(s.slackConnectionBody(&inst)), nil
}

func clearSlack(ctx context.Context, q *store.Queries, ws uuid.UUID) error {
	if err := q.DeleteSlackInboxSettings(ctx, ws); err != nil {
		return err
	}
	if err := q.DeleteSlackThreads(ctx, ws); err != nil {
		return err
	}
	if err := q.DeleteSlackPosts(ctx, ws); err != nil {
		return err
	}
	if err := q.DeleteSlackMemberLinks(ctx, ws); err != nil {
		return err
	}
	return q.DeleteSlackLinkCodes(ctx, ws)
}

func (s *Server) DisconnectSlack(ctx context.Context, _ oas.DisconnectSlackRequestObject) (oas.DisconnectSlackResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	var gone store.SlackInstallation
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		var err error
		gone, err = q.DeleteSlackInstallation(ctx, p.workspaceID)
		if store.IsNotFound(err) {
			return errSlackNotConnected
		}
		if err != nil {
			return err
		}
		if err := clearSlack(ctx, q, p.workspaceID); err != nil {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditWorkspaceSlackDisconnected, oas.AuditTargetWorkspace, p.workspaceID, auditDetails{"team_id": gone.TeamID})
	})
	if err != nil {
		return nil, err
	}
	s.revokeSlack(ctx, gone)
	return oas.DisconnectSlack204Response{}, nil
}

// revokeSlack ends a removed installation's token at Slack; the connection is gone either way.
func (s *Server) revokeSlack(ctx context.Context, inst store.SlackInstallation) {
	token, err := s.slackToken(inst)
	if err == nil {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), slackCallTimeout)
		defer cancel()
		err = s.slackAPI.Revoke(cctx, token)
	}
	if err != nil {
		s.log.WarnContext(ctx, "slack token not revoked", slog.String("workspace_id", inst.WorkspaceID.String()), slog.Any("error", err))
	}
}

func (s *Server) ConnectSlack(ctx context.Context, _ oas.ConnectSlackRequestObject) (oas.ConnectSlackResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	if !s.slack.on() {
		return nil, errSlackNotConfigured
	}
	state := signInState(p.workspaceID, p.memberID)
	expires := s.now().Add(slackConnectTTL)
	if err := s.st.SetSlackConnectState(ctx, store.SetSlackConnectStateParams{
		WorkspaceID: p.workspaceID, MemberID: p.memberID, StateHash: stateHash(state), ExpiresAt: expires,
	}); err != nil {
		return nil, err
	}
	q := url.Values{
		"client_id": {s.slack.ClientID}, "scope": {strings.Join(slack.Scopes, ",")},
		"redirect_uri": {s.slackRedirectURL()}, "state": {state},
	}
	return oas.ConnectSlack200JSONResponse{AuthorizeUrl: slack.AuthorizeURL + "?" + q.Encode(), RedirectUrl: s.slackRedirectURL(), ExpiresAt: expires}, nil
}

func (s *Server) serveSlackCallback(w http.ResponseWriter, r *http.Request) {
	if !s.slack.on() {
		writeProblem(w, errNotFound)
		return
	}
	ws, result := s.finishSlackConnect(r)
	q := url.Values{"result": {string(result)}}
	if ws != uuid.Nil() {
		q.Set("workspace", ws.String())
	}
	w.Header().Set("X-Yuva-Slack-Result", string(result))
	http.Redirect(w, r, strings.TrimRight(s.auth.PublicURL, "/")+"/settings/slack?"+q.Encode(), http.StatusSeeOther)
}

func (s *Server) finishSlackConnect(r *http.Request) (uuid.UUID, oas.SlackConnectResult) {
	ctx := r.Context()
	query := r.URL.Query()
	state := query.Get("state")
	ws, _, ok := readSignInState(state)
	if !ok {
		return uuid.Nil(), oas.SlackExpired
	}
	memberID, err := s.st.TakeSlackConnectState(ctx, store.TakeSlackConnectStateParams{WorkspaceID: ws, StateHash: stateHash(state), Now: s.now()})
	if err != nil {
		if !store.IsNotFound(err) {
			s.log.ErrorContext(ctx, "slack connect", slog.Any("error", err))
		}
		return ws, oas.SlackExpired
	}
	if !s.sameMemberSession(ctx, r, ws, memberID) {
		return ws, oas.SlackSessionMismatch
	}
	code := query.Get("code")
	if query.Get("error") != "" || code == "" {
		return ws, oas.SlackDenied
	}
	cctx, cancel := context.WithTimeout(ctx, slackCallTimeout)
	defer cancel()
	in, err := s.slackAPI.Exchange(cctx, s.slack.ClientID, s.slack.ClientSecret, code, s.slackRedirectURL())
	if err != nil {
		s.log.WarnContext(ctx, "slack connect", slog.String("workspace_id", ws.String()), slog.Any("error", err))
		return ws, oas.SlackFailed
	}
	if in.TokenType != "bot" || in.BotToken == "" || in.Enterprise || !slackTeamRe.MatchString(in.TeamID) || !slackUserRe.MatchString(in.BotUserID) || in.AppID == "" || len(in.AppID) > 40 {
		s.log.WarnContext(ctx, "slack connect: unexpected install", slog.String("workspace_id", ws.String()), slog.String("token_type", in.TokenType), slog.Bool("enterprise", in.Enterprise))
		return ws, oas.SlackFailed
	}
	if missing := slack.MissingScopes(in.Scopes); len(missing) > 0 {
		s.revokeSlack(ctx, store.SlackInstallation{WorkspaceID: ws, BotToken: s.secrets.Seal([]byte(in.BotToken), slackTokenContext(ws))})
		return ws, oas.SlackScopeMissing
	}
	result := oas.SlackConnected
	var replaced *store.SlackInstallation
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		other, err := q.FindSlackInstallationByTeam(ctx, in.TeamID)
		if err == nil && other.WorkspaceID != ws {
			result = oas.SlackTeamTaken
			return nil
		}
		if err != nil && !store.IsNotFound(err) {
			return err
		}
		m, err := q.GetMember(ctx, store.GetMemberParams{WorkspaceID: ws, ID: memberID})
		if store.IsNotFound(err) || (err == nil && m.Role != roleOwner && m.Role != roleAdmin) {
			result = oas.SlackSessionMismatch
			return nil
		}
		if err != nil {
			return err
		}
		old, err := q.LockSlackInstallation(ctx, ws)
		switch {
		case err == nil && old.TeamID != in.TeamID:
			if err := clearSlack(ctx, q, ws); err != nil {
				return err
			}
			replaced = &old
		case err != nil && !store.IsNotFound(err):
			return err
		}
		if _, err := q.SaveSlackInstallation(ctx, store.SaveSlackInstallationParams{
			WorkspaceID: ws, TeamID: in.TeamID, TeamName: truncateRunes(in.TeamName, 200), AppID: in.AppID, BotUserID: in.BotUserID,
			BotToken: s.secrets.Seal([]byte(in.BotToken), slackTokenContext(ws)), Scopes: in.Scopes, ConnectedBy: &memberID, Now: s.now(),
		}); err != nil {
			return err
		}
		p := principal{workspaceID: ws, memberID: memberID, personID: m.PersonID, role: m.Role}
		return s.audit(ctx, q, p, oas.AuditWorkspaceSlackConnected, oas.AuditTargetWorkspace, ws, auditDetails{"team_id": in.TeamID, "team_name": in.TeamName})
	})
	if err != nil {
		s.log.ErrorContext(ctx, "slack connect", slog.Any("error", err))
		return ws, oas.SlackFailed
	}
	if replaced != nil {
		s.revokeSlack(ctx, *replaced)
	}
	return ws, result
}

func (s *Server) slackChannels(ctx context.Context, token string) ([]slack.Channel, error) {
	cctx, cancel := context.WithTimeout(ctx, slackCallTimeout)
	defer cancel()
	chs, err := s.slackAPI.Channels(cctx, token, slackChannelLimit)
	if err != nil {
		return nil, errSlackFailed(err)
	}
	slices.SortFunc(chs, func(a, b slack.Channel) int { return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID)) })
	return chs, nil
}

func (s *Server) ListSlackChannels(ctx context.Context, _ oas.ListSlackChannelsRequestObject) (oas.ListSlackChannelsResponseObject, error) {
	p := principalFrom(ctx)
	_, token, err := s.connectedSlack(ctx, p.workspaceID)
	if err != nil {
		return nil, err
	}
	chs, err := s.slackChannels(ctx, token)
	if err != nil {
		return nil, err
	}
	out := oas.ListSlackChannels200JSONResponse{Items: make([]oas.SlackChannel, len(chs))}
	for i, c := range chs {
		out.Items[i] = oas.SlackChannel{Id: c.ID, Name: c.Name, Private: c.Private}
	}
	return out, nil
}

func inboxSlackBody(inboxID uuid.UUID, r *store.SlackInboxSetting) oas.InboxSlack {
	out := oas.InboxSlack{InboxId: inboxID}
	if r != nil {
		out.NewConversationsChannelId, out.MentionsChannelId, out.SlaChannelId = r.NewConversationsChannel, r.MentionsChannel, r.SlaChannel
		out.SlaWarnings, out.IncludeContent, out.UpdatedAt = r.SlaWarnings, r.IncludeContent, &r.UpdatedAt
	}
	return out
}

func (s *Server) GetInboxSlack(ctx context.Context, req oas.GetInboxSlackRequestObject) (oas.GetInboxSlackResponseObject, error) {
	p := principalFrom(ctx)
	if _, err := visibleInbox(ctx, s.st.Queries, p, req.InboxId); err != nil {
		return nil, err
	}
	r, err := s.st.GetSlackInboxSettings(ctx, store.GetSlackInboxSettingsParams{WorkspaceID: p.workspaceID, InboxID: req.InboxId})
	if store.IsNotFound(err) {
		return oas.GetInboxSlack200JSONResponse(inboxSlackBody(req.InboxId, nil)), nil
	}
	if err != nil {
		return nil, err
	}
	return oas.GetInboxSlack200JSONResponse(inboxSlackBody(req.InboxId, &r)), nil
}

func slackChannelInput(v interface {
	IsNull() bool
	IsSpecified() bool
	Get() (string, error)
}, field string) (*string, error) {
	if !v.IsSpecified() || v.IsNull() {
		return nil, nil
	}
	id, _ := v.Get()
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	if !slackChannelRe.MatchString(id) {
		return nil, errValidation(field + " must be a Slack channel id such as C0123ABCD")
	}
	return &id, nil
}

func (s *Server) SetInboxSlack(ctx context.Context, req oas.SetInboxSlackRequestObject) (oas.SetInboxSlackResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	if in, err := visibleInbox(ctx, s.st.Queries, p, req.InboxId); err != nil {
		return nil, err
	} else if in.Personal {
		return nil, errPersonalMailbox("a personal mailbox posts nothing to Slack")
	}
	b := req.Body
	newConv, err := slackChannelInput(b.NewConversationsChannelId, "new_conversations_channel_id")
	if err != nil {
		return nil, err
	}
	mentions, err := slackChannelInput(b.MentionsChannelId, "mentions_channel_id")
	if err != nil {
		return nil, err
	}
	slaCh, err := slackChannelInput(b.SlaChannelId, "sla_channel_id")
	if err != nil {
		return nil, err
	}
	keepSLA := !b.SlaChannelId.IsSpecified()
	var stored *store.SlackInboxSetting
	if keepSLA || b.SlaWarnings == nil {
		cur, err := s.st.GetSlackInboxSettings(ctx, store.GetSlackInboxSettingsParams{WorkspaceID: p.workspaceID, InboxID: req.InboxId})
		if err != nil && !store.IsNotFound(err) {
			return nil, err
		}
		if err == nil {
			stored = &cur
		}
	}
	slaWarnings := b.SlaWarnings != nil && *b.SlaWarnings
	if stored != nil {
		if keepSLA {
			slaCh = stored.SlaChannel
		}
		if b.SlaWarnings == nil {
			slaWarnings = stored.SlaWarnings
		}
	}
	_, token, err := s.connectedSlack(ctx, p.workspaceID)
	if err != nil {
		return nil, err
	}
	if newConv != nil || mentions != nil || slaCh != nil {
		chs, err := s.slackChannels(ctx, token)
		if err != nil {
			return nil, err
		}
		for _, c := range []*string{newConv, mentions, slaCh} {
			if c != nil && !slices.ContainsFunc(chs, func(ch slack.Channel) bool { return ch.ID == *c }) {
				return nil, errValidation("the Yuva bot is not in channel " + *c + "; invite it there in Slack first")
			}
		}
	}
	r, err := s.st.SaveSlackInboxSettings(ctx, store.SaveSlackInboxSettingsParams{
		WorkspaceID: p.workspaceID, InboxID: req.InboxId, NewConversationsChannel: newConv, MentionsChannel: mentions,
		SlaChannel: slaCh, SlaWarnings: slaWarnings, IncludeContent: b.IncludeContent != nil && *b.IncludeContent, Now: s.now(),
	})
	if err != nil {
		return nil, err
	}
	return oas.SetInboxSlack200JSONResponse(inboxSlackBody(req.InboxId, &r)), nil
}

func (s *Server) mySlack(ctx context.Context, q *store.Queries, p principal) (oas.SlackMemberLink, error) {
	var out oas.SlackMemberLink
	link, err := q.GetSlackMemberLink(ctx, store.GetSlackMemberLinkParams{WorkspaceID: p.workspaceID, MemberID: p.memberID})
	switch {
	case err == nil:
		out.Linked, out.SlackUserId, out.SlackUserName, out.LinkedAt = true, &link.SlackUserID, &link.SlackUserName, &link.LinkedAt
	case !store.IsNotFound(err):
		return out, err
	}
	code, err := q.GetSlackLinkCode(ctx, store.GetSlackLinkCodeParams{WorkspaceID: p.workspaceID, MemberID: p.memberID})
	switch {
	case err == nil && code.ExpiresAt.After(s.now()) && code.Attempts < slackLinkTries:
		out.CodeExpiresAt = &code.ExpiresAt
		if !out.Linked {
			out.SlackUserId, out.SlackUserName = &code.SlackUserID, &code.SlackUserName
		}
	case err != nil && !store.IsNotFound(err):
		return out, err
	}
	return out, nil
}

func (s *Server) GetMySlack(ctx context.Context, _ oas.GetMySlackRequestObject) (oas.GetMySlackResponseObject, error) {
	out, err := s.mySlack(ctx, s.st.Queries, principalFrom(ctx))
	if err != nil {
		return nil, err
	}
	return oas.GetMySlack200JSONResponse(out), nil
}

func (s *Server) UnlinkMySlack(ctx context.Context, _ oas.UnlinkMySlackRequestObject) (oas.UnlinkMySlackResponseObject, error) {
	p := principalFrom(ctx)
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		if err := q.DeleteSlackLinkCode(ctx, store.DeleteSlackLinkCodeParams{WorkspaceID: p.workspaceID, MemberID: p.memberID}); err != nil {
			return err
		}
		link, err := q.DeleteSlackMemberLink(ctx, store.DeleteSlackMemberLinkParams{WorkspaceID: p.workspaceID, MemberID: p.memberID})
		if store.IsNotFound(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditMemberSlackUnlinked, oas.AuditTargetMember, p.memberID, auditDetails{"slack_user_id": link.SlackUserID})
	})
	if err != nil {
		return nil, err
	}
	return oas.UnlinkMySlack204Response{}, nil
}

const slackCodeInfo = "yuva slack link codes"

func slackCodeHash(key []byte, ws, member uuid.UUID, slackUser, code string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(ws[:])
	m.Write(member[:])
	m.Write([]byte(slackUser + ":" + code))
	return m.Sum(nil)
}

func (s *Server) StartMySlackLink(ctx context.Context, req oas.StartMySlackLinkRequestObject) (oas.StartMySlackLinkResponseObject, error) {
	p := principalFrom(ctx)
	inst, token, err := s.connectedSlack(ctx, p.workspaceID)
	if err != nil {
		return nil, err
	}
	m, err := s.st.GetMember(ctx, store.GetMemberParams{WorkspaceID: p.workspaceID, ID: p.memberID})
	if err != nil {
		return nil, err
	}
	email := m.Email
	if req.Body != nil && req.Body.Email != nil {
		email = strings.TrimSpace(string(*req.Body.Email))
	}
	if email == "" || len(email) > 320 {
		return nil, errValidation("email must be the Slack account's e-mail address")
	}
	if err := s.rateLimit(ctx, rateCheck{"slack-link:" + p.memberID.String(), limit{5, time.Hour}}); err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, slackCallTimeout)
	defer cancel()
	user, err := s.slackAPI.UserByEmail(cctx, token, email)
	var se *slack.Error
	if errors.As(err, &se) && se.Code == "users_not_found" {
		return nil, errSlackUserNotFound
	}
	if err != nil {
		return nil, errSlackFailed(err)
	}
	if !slackUserRe.MatchString(user.ID) {
		return nil, errSlackUserNotFound
	}
	if taken, err := s.st.GetSlackMemberLinkByUser(ctx, store.GetSlackMemberLinkByUserParams{WorkspaceID: p.workspaceID, SlackUserID: user.ID}); err == nil && taken.MemberID != p.memberID {
		return nil, errSlackUserTaken
	} else if err != nil && !store.IsNotFound(err) {
		return nil, err
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return nil, err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	name := truncateRunes(user.Name, 200)
	if _, err := s.st.SetSlackLinkCode(ctx, store.SetSlackLinkCodeParams{
		WorkspaceID: p.workspaceID, MemberID: p.memberID, SlackUserID: user.ID, SlackUserName: name,
		CodeHash: slackCodeHash(s.secrets.Derive(slackCodeInfo), p.workspaceID, p.memberID, user.ID, code), ExpiresAt: s.now().Add(slackLinkTTL),
	}); err != nil {
		return nil, err
	}
	who := m.Name
	if who == "" {
		who = m.Email
	}
	text := fmt.Sprintf("Your code to link this Slack account to %s in Yuva (%s): *%s*\nIt expires in 10 minutes. If you did not ask for it, ignore this message.",
		slack.Escape(who), slack.Escape(inst.TeamName), code)
	if err := s.slackAPI.DirectMessage(cctx, token, user.ID, text); err != nil {
		return nil, errSlackFailed(err)
	}
	out, err := s.mySlack(ctx, s.st.Queries, p)
	if err != nil {
		return nil, err
	}
	out.SlackUserId, out.SlackUserName = &user.ID, &name
	return oas.StartMySlackLink202JSONResponse(out), nil
}

func (s *Server) ConfirmMySlackLink(ctx context.Context, req oas.ConfirmMySlackLinkRequestObject) (oas.ConfirmMySlackLinkResponseObject, error) {
	p := principalFrom(ctx)
	if !s.slack.on() {
		return nil, errSlackNotConfigured
	}
	code := strings.TrimSpace(req.Body.Code)
	if !slackCodeRe.MatchString(code) {
		return nil, errValidation("code must be the 6 digits the Yuva bot sent in Slack")
	}
	var wrong bool
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		row, err := q.LockSlackLinkCode(ctx, store.LockSlackLinkCodeParams{WorkspaceID: p.workspaceID, MemberID: p.memberID})
		if store.IsNotFound(err) {
			return errCodeExpired
		}
		if err != nil {
			return err
		}
		if !row.ExpiresAt.After(s.now()) || row.Attempts >= slackLinkTries {
			return errCodeExpired
		}
		if !s.anyDerived(slackCodeInfo, func(k []byte) bool {
			return hmac.Equal(row.CodeHash, slackCodeHash(k, p.workspaceID, p.memberID, row.SlackUserID, code))
		}) {
			wrong = true
			return q.CountSlackLinkCodeAttempt(ctx, store.CountSlackLinkCodeAttemptParams{WorkspaceID: p.workspaceID, MemberID: p.memberID})
		}
		taken, err := q.GetSlackMemberLinkByUser(ctx, store.GetSlackMemberLinkByUserParams{WorkspaceID: p.workspaceID, SlackUserID: row.SlackUserID})
		if err == nil && taken.MemberID != p.memberID {
			return errSlackUserTaken
		}
		if err != nil && !store.IsNotFound(err) {
			return err
		}
		if _, err := q.AddSlackMemberLink(ctx, store.AddSlackMemberLinkParams{
			WorkspaceID: p.workspaceID, MemberID: p.memberID, SlackUserID: row.SlackUserID, SlackUserName: row.SlackUserName, Now: s.now(),
		}); err != nil {
			return err
		}
		if err := q.DeleteSlackLinkCode(ctx, store.DeleteSlackLinkCodeParams{WorkspaceID: p.workspaceID, MemberID: p.memberID}); err != nil {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditMemberSlackLinked, oas.AuditTargetMember, p.memberID, auditDetails{"slack_user_id": row.SlackUserID})
	})
	if err != nil {
		return nil, err
	}
	if wrong {
		return nil, errInvalidCode
	}
	out, err := s.mySlack(ctx, s.st.Queries, p)
	if err != nil {
		return nil, err
	}
	return oas.ConfirmMySlackLink200JSONResponse(out), nil
}

type slackEnvelope struct {
	Type      string          `json:"type"`
	Challenge string          `json:"challenge"`
	TeamID    string          `json:"team_id"`
	APIAppID  string          `json:"api_app_id"`
	EventID   string          `json:"event_id"`
	EventTime int64           `json:"event_time"`
	Event     json.RawMessage `json:"event"`
}

type slackEvent struct {
	Type     string `json:"type"`
	Subtype  string `json:"subtype"`
	User     string `json:"user"`
	BotID    string `json:"bot_id"`
	Channel  string `json:"channel"`
	Text     string `json:"text"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts"`
}

// serveSlackEvents takes Slack's Events API requests: verified by the signing secret and the
// timestamp, kept once per event id, and handled by a job so Slack gets its answer at once.
func (s *Server) serveSlackEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.slack.on() {
		writeProblem(w, errNotFound)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, slackMaxBody))
	if err != nil {
		writeProblem(w, problem(http.StatusRequestEntityTooLarge, "too_large", "the request body is larger than 1 MiB"))
		return
	}
	if err := slack.Verify(s.slack.SigningSecret, r.Header.Get("X-Slack-Request-Timestamp"), r.Header.Get("X-Slack-Signature"), body, s.now()); err != nil {
		s.log.InfoContext(ctx, "slack event refused", slog.String("reason", err.Error()))
		writeProblem(w, errSlackSignature)
		return
	}
	var env slackEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		writeProblem(w, errValidation("the body is not a Slack event"))
		return
	}
	switch env.Type {
	case "url_verification":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"challenge": env.Challenge})
		return
	case "event_callback":
	default:
		w.WriteHeader(http.StatusOK)
		return
	}
	if env.EventID == "" || len(env.EventID) > 100 || !slackTeamRe.MatchString(env.TeamID) {
		w.WriteHeader(http.StatusOK)
		return
	}
	inst, err := s.st.FindSlackInstallationByTeam(ctx, env.TeamID)
	if store.IsNotFound(err) || (err == nil && inst.AppID != env.APIAppID) {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err != nil {
		s.log.ErrorContext(ctx, "slack event", slog.Any("error", err))
		writeProblem(w, errInternal)
		return
	}
	now := s.now()
	err = s.inTx(ctx, inst.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
		n, err := q.ClaimSlackEvent(ctx, store.ClaimSlackEventParams{WorkspaceID: inst.WorkspaceID, EventID: env.EventID, Now: now})
		if err != nil || n == 0 {
			return err
		}
		if err := q.PruneSlackEvents(ctx, store.PruneSlackEventsParams{WorkspaceID: inst.WorkspaceID, Before: now.Add(-slackEventsKept)}); err != nil {
			return err
		}
		events.job(SlackEventArgs{WorkspaceID: inst.WorkspaceID, TeamID: env.TeamID, EventID: env.EventID, EventTime: env.EventTime, Event: env.Event}, &river.InsertOpts{MaxAttempts: slackPostAttempts})
		return nil
	})
	if err != nil {
		s.log.ErrorContext(ctx, "slack event", slog.Any("error", err))
		writeProblem(w, errInternal)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// SlackEventArgs is one Slack event to handle.
type SlackEventArgs struct {
	WorkspaceID uuid.UUID       `json:"workspace_id"`
	TeamID      string          `json:"team_id"`
	EventID     string          `json:"event_id"`
	EventTime   int64           `json:"event_time"`
	Event       json.RawMessage `json:"event"`
}

func (SlackEventArgs) Kind() string { return "slack_event" }

func (SlackEventArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.OutboundQueue}
}

type slackEventWorker struct {
	river.WorkerDefaults[SlackEventArgs]
	s *Server
}

func (w *slackEventWorker) Work(ctx context.Context, job *river.Job[SlackEventArgs]) error {
	return w.s.HandleSlackEvent(ctx, job.Args)
}

func (w *slackEventWorker) Timeout(*river.Job[SlackEventArgs]) time.Duration { return 2 * time.Minute }

// HandleSlackEvent acts on one event: a removed app ends the connection, and a reply in a thread
// Yuva posted becomes a note or a customer reply by the linked member.
func (s *Server) HandleSlackEvent(ctx context.Context, a SlackEventArgs) error {
	if live, err := s.workspaceLive(ctx, a.WorkspaceID); err != nil || !live || !s.slack.on() {
		return err
	}
	var ev slackEvent
	if json.Unmarshal(a.Event, &ev) != nil {
		return nil
	}
	inst, err := s.st.GetSlackInstallation(ctx, a.WorkspaceID)
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if inst.TeamID != a.TeamID {
		return nil
	}
	switch ev.Type {
	case "app_uninstalled", "tokens_revoked":
		if a.EventTime == 0 || time.Unix(a.EventTime, 0).Before(inst.CreatedAt.Truncate(time.Second)) {
			return nil
		}
		return s.slackUninstalled(ctx, inst)
	case "message":
		return s.slackThreadReply(ctx, inst, ev)
	}
	return nil
}

func (s *Server) slackUninstalled(ctx context.Context, inst store.SlackInstallation) error {
	ws := inst.WorkspaceID
	return s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.LockSlackInstallation(ctx, ws)
		if store.IsNotFound(err) || (err == nil && (cur.TeamID != inst.TeamID || !cur.CreatedAt.Equal(inst.CreatedAt))) {
			return nil
		}
		if err != nil {
			return err
		}
		gone, err := q.DeleteSlackInstallation(ctx, ws)
		if err != nil {
			return err
		}
		if err := clearSlack(ctx, q, ws); err != nil {
			return err
		}
		return writeSystemAudit(ctx, q, ws, s.now(), oas.AuditWorkspaceSlackDisconnected, oas.AuditTargetWorkspace, ws, auditDetails{"team_id": gone.TeamID, "reason": "uninstalled"})
	})
}

const (
	slackTextUnlinked = "Yuva did not take this: your Slack account is not linked to a Yuva member. Link it in Yuva under your profile, then reply again."
	slackTextNoAccess = "Yuva did not take this: you cannot see this conversation in Yuva (no access to its inbox, or it was deleted)."
	slackTextEmpty    = "Yuva did not take this: write the reply after %s."
	slackTextSent     = "Sent to the customer."
	slackTextSpam     = "Yuva did not send this: the conversation is marked as spam. A reply without the command is kept as a note."
	slackTextSSO      = "Yuva did not take this: this workspace requires signing in through your organisation's single sign-on, which Slack cannot do. Sign in through SSO and answer in Yuva."
)

func (s *Server) slackEphemeral(ctx context.Context, token string, ev slackEvent, text string) {
	cctx, cancel := context.WithTimeout(ctx, slackCallTimeout)
	defer cancel()
	if err := s.slackAPI.PostEphemeral(cctx, token, slack.Message{Channel: ev.Channel, ThreadTS: ev.ThreadTS, Text: text}, ev.User); err != nil {
		s.log.InfoContext(ctx, "slack ephemeral", slog.Any("error", err))
	}
}

func (s *Server) slackThreadReply(ctx context.Context, inst store.SlackInstallation, ev slackEvent) error {
	if ev.Subtype != "" || ev.BotID != "" || ev.User == "" || ev.User == inst.BotUserID || ev.ThreadTS == "" || ev.ThreadTS == ev.TS || ev.TS == "" {
		return nil
	}
	ws := inst.WorkspaceID
	thread, err := s.st.GetSlackThread(ctx, store.GetSlackThreadParams{WorkspaceID: ws, ChannelID: ev.Channel, ThreadTs: ev.ThreadTS})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	token, err := s.slackToken(inst)
	if err != nil {
		return err
	}
	link, err := s.st.GetSlackMemberLinkByUser(ctx, store.GetSlackMemberLinkByUserParams{WorkspaceID: ws, SlackUserID: ev.User})
	if store.IsNotFound(err) {
		s.slackEphemeral(ctx, token, ev, slackTextUnlinked)
		return nil
	}
	if err != nil {
		return err
	}
	m, err := s.st.GetMember(ctx, store.GetMemberParams{WorkspaceID: ws, ID: link.MemberID})
	if store.IsNotFound(err) {
		s.slackEphemeral(ctx, token, ev, slackTextUnlinked)
		return nil
	}
	if err != nil {
		return err
	}
	var gate *apiError
	if err := s.ssoGate(ctx, ws, m.PersonID, m.Role, nil); errors.As(err, &gate) {
		s.slackEphemeral(ctx, token, ev, slackTextSSO)
		return nil
	} else if err != nil {
		return err
	}
	conv, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: ws, ID: thread.ConversationID})
	if store.IsNotFound(err) {
		s.slackEphemeral(ctx, token, ev, slackTextNoAccess)
		return nil
	}
	if err != nil {
		return err
	}
	text := strings.TrimSpace(ev.Text)
	kind := oas.MessageCreateKindNote
	if rest, ok := strings.CutPrefix(text, inst.ReplyCommand); ok && (rest == "" || rest[0] == ' ' || rest[0] == '\n') {
		kind, text = oas.MessageCreateKindMessage, strings.TrimSpace(rest)
	}
	if kind == oas.MessageCreateKindMessage && conv.Spam {
		s.slackEphemeral(ctx, token, ev, slackTextSpam)
		return nil
	}
	mentioned := map[string]uuid.UUID{}
	names := map[string]string{}
	var mentions []uuid.UUID
	for _, u := range slack.MentionedUsers(text) {
		l, err := s.st.GetSlackMemberLinkByUser(ctx, store.GetSlackMemberLinkByUserParams{WorkspaceID: ws, SlackUserID: u})
		if store.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		mm, err := s.st.GetMember(ctx, store.GetMemberParams{WorkspaceID: ws, ID: l.MemberID})
		if store.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		names[u] = cmp.Or(mm.Name, mm.Email)
		if _, dup := mentioned[u]; dup || kind != oas.MessageCreateKindNote || mm.ID == m.ID {
			continue
		}
		ok, err := memberHasInbox(ctx, s.st.Queries, ws, conv.InboxID, mm.ID)
		if err != nil {
			return err
		}
		if ok {
			mentioned[u] = mm.ID
			mentions = append(mentions, mm.ID)
		}
	}
	body := strings.TrimSpace(slack.PlainText(text, func(u string) string { return names[u] }))
	if body == "" {
		s.slackEphemeral(ctx, token, ev, fmt.Sprintf(slackTextEmpty, inst.ReplyCommand))
		return nil
	}
	p := principal{workspaceID: ws, memberID: m.ID, personID: m.PersonID, role: m.Role, via: slackVia}
	clientID := "slack:" + ev.Channel + ":" + ev.TS
	in := &oas.CreateMessageJSONRequestBody{Kind: kind, Body: &body, ClientId: &clientID}
	if len(mentions) > 0 {
		in.Mentions = &mentions
	}
	_, err = s.CreateMessage(context.WithValue(ctx, principalKey, p), oas.CreateMessageRequestObject{ConversationId: conv.ID, JSONBody: in})
	var ae *apiError
	switch {
	case errors.As(err, &ae) && ae.Status == http.StatusNotFound:
		s.slackEphemeral(ctx, token, ev, slackTextNoAccess)
		return nil
	case errors.As(err, &ae) && ae.Status < http.StatusInternalServerError:
		s.slackEphemeral(ctx, token, ev, "Yuva did not take this: "+ae.Detail)
		return nil
	case err != nil:
		return err
	}
	if kind == oas.MessageCreateKindMessage {
		s.slackEphemeral(ctx, token, ev, slackTextSent)
	}
	return nil
}

// SlackPostArgs posts one message's notification to the inbox's Slack channel.
type SlackPostArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	MessageID   uuid.UUID `json:"message_id"`
	Event       string    `json:"event"`
}

func (SlackPostArgs) Kind() string { return "slack_post" }

func (SlackPostArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.OutboundQueue}
}

type slackPostWorker struct {
	river.WorkerDefaults[SlackPostArgs]
	s *Server
}

func (w *slackPostWorker) Work(ctx context.Context, job *river.Job[SlackPostArgs]) error {
	wait, err := w.s.PostToSlack(ctx, job.Args)
	if err != nil {
		return err
	}
	if wait > 0 {
		return river.JobSnooze(wait)
	}
	return nil
}

func (w *slackPostWorker) Timeout(*river.Job[SlackPostArgs]) time.Duration { return 2 * time.Minute }

// queueSlack queues the Slack post a notification causes when the inbox sends that event to a
// channel.
func (s *Server) queueSlack(ctx context.Context, ws, inboxID, messageID uuid.UUID, ev notificationEvent) error {
	if !s.slack.on() {
		return nil
	}
	event := ""
	switch ev {
	case oas.NewLiveConversation, oas.NewAsyncConversation:
		event = slackNewConversation
	case oas.Mentioned:
		event = slackMention
	default:
		return nil
	}
	set, err := s.st.GetSlackInboxSettings(ctx, store.GetSlackInboxSettingsParams{WorkspaceID: ws, InboxID: inboxID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if slackChannelFor(set, event) == "" {
		return nil
	}
	_, err = s.jobs.Insert(ctx, SlackPostArgs{WorkspaceID: ws, MessageID: messageID, Event: event}, &river.InsertOpts{
		MaxAttempts: slackPostAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true},
	})
	return err
}

func slackChannelFor(set store.SlackInboxSetting, event string) string {
	if event == slackMention {
		return deref(set.MentionsChannel)
	}
	return deref(set.NewConversationsChannel)
}

// PostToSlack makes one attempt at a notification's Slack post. A rate limit returns how long to
// wait; a refusal that will not pass is recorded on the connection instead of retried.
func (s *Server) PostToSlack(ctx context.Context, a SlackPostArgs) (time.Duration, error) {
	ws := a.WorkspaceID
	if live, err := s.workspaceLive(ctx, ws); err != nil || !live || !s.slack.on() {
		return 0, err
	}
	inst, err := s.st.GetSlackInstallation(ctx, ws)
	if store.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	m, err := s.st.GetMessage(ctx, store.GetMessageParams{WorkspaceID: ws, ID: a.MessageID})
	if store.IsNotFound(err) || (err == nil && m.RedactedAt != nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	c, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: ws, ID: m.ConversationID})
	if store.IsNotFound(err) || (err == nil && (c.Spam || c.TrashedAt != nil)) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	set, err := s.st.GetSlackInboxSettings(ctx, store.GetSlackInboxSettingsParams{WorkspaceID: ws, InboxID: c.InboxID})
	if store.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	channel := slackChannelFor(set, a.Event)
	if channel == "" {
		return 0, nil
	}
	text, err := s.slackPostText(ctx, inst, set, a.Event, m, c)
	if err != nil {
		return 0, err
	}
	msg := slack.Message{Channel: channel, Text: text}
	if a.Event == slackMention {
		t, err := s.st.GetSlackThreadOfConversation(ctx, store.GetSlackThreadOfConversationParams{WorkspaceID: ws, ConversationID: c.ID, ChannelID: channel})
		switch {
		case err == nil:
			msg.ThreadTS = t.ThreadTs
		case !store.IsNotFound(err):
			return 0, err
		}
	}
	return s.sendSlackPost(ctx, inst, c.ID, &m.ID, set.IncludeContent, msg)
}

// sendSlackPost posts msg and records a top-level post as the conversation's thread. Every post is
// recorded with its message and whether it carries text, so redaction, trash and deletion can
// rewrite it.
func (s *Server) sendSlackPost(ctx context.Context, inst store.SlackInstallation, conversationID uuid.UUID, messageID *uuid.UUID, withContent bool, msg slack.Message) (time.Duration, error) {
	ws := inst.WorkspaceID
	token, err := s.slackToken(inst)
	if err != nil {
		return 0, s.slackError(ctx, ws, "the Slack token cannot be decrypted; connect Slack again")
	}
	cctx, cancel := context.WithTimeout(ctx, slackCallTimeout)
	defer cancel()
	ts, err := s.slackAPI.Post(cctx, token, msg)
	var se *slack.Error
	switch {
	case errors.As(err, &se) && se.RetryAfter > 0:
		return se.RetryAfter, nil
	case errors.As(err, &se) && !se.Temporary():
		return 0, s.slackError(ctx, ws, se.Code)
	case err != nil:
		return 0, err
	}
	if ts != "" {
		n, err := s.st.AddSlackPost(ctx, store.AddSlackPostParams{WorkspaceID: ws, ChannelID: msg.Channel, Ts: ts, ConversationID: conversationID, MessageID: messageID, WithContent: withContent, Now: s.now()})
		if err != nil && !store.IsForeignKeyViolation(err) {
			return 0, err
		}
		if err != nil || n == 0 {
			gone, err := s.scrubLatePost(ctx, ws, msg.Channel, ts, conversationID, messageID)
			if err != nil || gone {
				return 0, err
			}
		}
	}
	if msg.ThreadTS == "" && ts != "" {
		if err := s.st.AddSlackThread(ctx, store.AddSlackThreadParams{WorkspaceID: ws, ChannelID: msg.Channel, ThreadTs: ts, ConversationID: conversationID, Now: s.now()}); err != nil {
			return 0, err
		}
	}
	return 0, s.st.SetSlackError(ctx, store.SetSlackErrorParams{WorkspaceID: ws, Now: s.now()})
}

func (s *Server) slackError(ctx context.Context, ws uuid.UUID, reason string) error {
	reason = truncateRunes(reason, 500)
	s.log.WarnContext(ctx, "slack post failed", slog.String("workspace_id", ws.String()), slog.String("reason", reason))
	return s.st.SetSlackError(ctx, store.SetSlackErrorParams{WorkspaceID: ws, Error: &reason, Now: s.now()})
}

// slackPostText is the post: what happened, the subject, the contact and the inbox, a link, and
// the start of the text only when the inbox allows content. Everything from people is escaped,
// so no customer text becomes a Slack link or mention.
func (s *Server) slackPostText(ctx context.Context, inst store.SlackInstallation, set store.SlackInboxSetting, event string, m store.GetMessageRow, c store.Conversation) (string, error) {
	ws := c.WorkspaceID
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: c.InboxID})
	if err != nil {
		return "", err
	}
	contact, err := s.contactDisplayName(ctx, ws, c.ContactID)
	if err != nil {
		return "", err
	}
	subject := strings.TrimSpace(c.Subject)
	if subject == "" {
		subject = "(no subject)"
	}
	var b strings.Builder
	switch event {
	case slackMention:
		author, err := s.memberDisplayName(ctx, ws, m.AuthorMemberID)
		if err != nil {
			return "", err
		}
		links, err := s.st.ListSlackMemberLinks(ctx, store.ListSlackMemberLinksParams{WorkspaceID: ws, MemberIds: m.Mentions})
		if err != nil {
			return "", err
		}
		var who []string
		for _, id := range m.Mentions {
			if i := slices.IndexFunc(links, func(l store.SlackMemberLink) bool { return l.MemberID == id }); i >= 0 {
				who = append(who, "<@"+links[i].SlackUserID+">")
				continue
			}
			name, err := s.memberDisplayName(ctx, ws, &id)
			if err != nil {
				return "", err
			}
			if name != "" {
				who = append(who, slack.Escape(name))
			}
		}
		fmt.Fprintf(&b, "*%s* mentioned %s in *%s*", slack.Escape(cmp.Or(author, "Someone")), strings.Join(who, ", "), slack.Escape(subject))
	default:
		fmt.Fprintf(&b, "*New conversation*: *%s*", slack.Escape(subject))
	}
	fmt.Fprintf(&b, "\n%s · %s", slack.Escape(cmp.Or(contact, "Unknown contact")), slack.Escape(in.Name))
	if set.IncludeContent {
		for line := range strings.SplitSeq(excerpt(m.Body, slackPreviewRunes), "\n") {
			b.WriteString("\n> " + slack.Escape(line))
		}
	}
	link := strings.TrimRight(s.auth.PublicURL, "/") + conversationPath(ws, c.ID)
	fmt.Fprintf(&b, "\n<%s|Open in Yuva> · Reply in this thread to add a note; start with `%s` to answer the customer.", link, inst.ReplyCommand)
	return b.String(), nil
}

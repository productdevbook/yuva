package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	memberAddressCodeTTL      = 10 * time.Minute
	memberAddressMaxAttempts  = 5
	memberAddressesPerMember  = 10
	memberAddressCodeInfo     = "yuva member address codes v1"
	memberAddressCodeTemplate = "member_address_code"
	memberAddressCheckMaxAge  = 30 * 24 * time.Hour
)

var (
	limitMemberAddressCodes = limit{10, time.Hour}

	plainAddress = regexp.MustCompile("^[a-z0-9!#$%&'*+/=?^_`{|}~-]+(\\.[a-z0-9!#$%&'*+/=?^_`{|}~-]+)*@[a-z0-9]([a-z0-9-]*[a-z0-9])?(\\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$")

	errAddressTaken       = problem(http.StatusConflict, "address_taken", "this address belongs to a channel of this server or another member of the workspace")
	errAddressVerified    = problem(http.StatusConflict, "address_verified", "you verified this address already")
	errMemberAddressLimit = problem(http.StatusConflict, "limit_reached", fmt.Sprintf("a member has at most %d addresses", memberAddressesPerMember))
	errMemberAddressGone  = problem(http.StatusNotFound, "not_found", "no such address")
	errApprovalNotNeeded  = problem(http.StatusConflict, "approval_not_needed", "this address is verified by the code mailed to it")
	errFromNotMember      = errValidation("from_address: only a member's own session sends from the member's address")
	errFromNotEmailed     = errValidation("from_address is only for e-mailed replies; send a draft with POST /v1/messages/{id}/send and from_address")
)

func errFromNotAllowed(why string) *apiError {
	return problem(http.StatusConflict, "from_not_allowed", "from_address: "+why)
}

func newMemberAddressCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

func memberAddressCodeHash(key []byte, id uuid.UUID, code string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(id[:])
	m.Write([]byte(code))
	return m.Sum(nil)
}

func memberAddressInput(v string) (string, error) {
	a := strings.ToLower(strings.TrimSpace(v))
	if len(a) > 320 || !plainAddress.MatchString(a) || strings.HasPrefix(a, catchAllLocal+"@") {
		return "", errValidation("address: " + v + " is not a plain e-mail address")
	}
	return a, nil
}

func (s *Server) memberAddressTaken(ctx context.Context, q *store.Queries, ws uuid.UUID, addr string) error {
	if (s.ingress.ForwardingDomain != "" && email.Domain(addr) == s.ingress.ForwardingDomain) || slices.Contains(s.ingress.OwnAddresses, addr) {
		return errAddressTaken
	}
	own, err := q.IsEmailChannelSender(ctx, store.IsEmailChannelSenderParams{WorkspaceID: ws, Address: addr})
	if err != nil {
		return err
	}
	if own {
		return errAddressTaken
	}
	if _, err := q.FindEmailChannelByAddress(ctx, addr); err == nil {
		return errAddressTaken
	} else if !store.IsNotFound(err) {
		return err
	}
	return nil
}

// memberAddressVerification is approval when a channel of the workspace catches all mail to the
// address's domain: a code mailed there would land in an inbox every member may read.
func memberAddressVerification(ctx context.Context, q *store.Queries, ws uuid.UUID, addr string) (oas.MemberAddressVerification, error) {
	shared, err := q.IsEmailChannelSender(ctx, store.IsEmailChannelSenderParams{WorkspaceID: ws, Address: catchAllLocal + "@" + email.Domain(addr)})
	if err != nil {
		return "", err
	}
	if shared {
		return oas.MemberAddressVerificationApproval, nil
	}
	return oas.MemberAddressVerificationCode, nil
}

type sendChannel struct {
	ch    store.EmailChannel
	check *store.DeliverabilityCheck
}

func loadSendChannel(ctx context.Context, q *store.Queries, ch store.EmailChannel) (sendChannel, error) {
	row, err := q.GetDeliverabilityCheck(ctx, store.GetDeliverabilityCheckParams{WorkspaceID: ch.WorkspaceID, ChannelID: ch.ChannelID})
	if store.IsNotFound(err) {
		return sendChannel{ch: ch}, nil
	}
	if err != nil {
		return sendChannel{}, err
	}
	return sendChannel{ch: ch, check: &row}, nil
}

func (s *Server) memberAddressUnusable(ctx context.Context, q *store.Queries, ch store.EmailChannel, addr string) (string, error) {
	sc, err := loadSendChannel(ctx, q, ch)
	if err != nil {
		return "", err
	}
	return s.unusableThrough(sc, addr), nil
}

func (s *Server) unusableThrough(sc sendChannel, addr string) string {
	ch, domain := sc.ch, email.Domain(addr)
	if ch.SmtpHost == "" {
		return "the channel has no SMTP account"
	}
	if want := sendingDomain(ch); domain != want {
		return addr + " is not on the channel's sending domain " + want
	}
	if sc.check == nil {
		return "the channel's deliverability was never checked; run POST /v1/channels/{id}/deliverability"
	}
	row := *sc.check
	if row.CreatedAt.Before(s.now().Add(-memberAddressCheckMaxAge)) || (ch.SendingChangedAt != nil && !row.CreatedAt.After(*ch.SendingChangedAt)) {
		return "the channel's latest deliverability check is older than 30 days or than the channel's sending settings; run it again"
	}
	r := parseProbeResult(row.Result)
	if s.probeState(row) != string(oas.DeliverabilityReceived) || r.Domain != domain || r.Auth == nil {
		return "the channel's latest deliverability check of " + domain + " has no result; run it again"
	}
	if r.Auth.DKIM.Status != oas.AuthPass || !r.Auth.DKIM.Aligned || r.Auth.DMARC != oas.AuthPass {
		return "the channel's latest deliverability check of " + domain + " did not pass DKIM aligned with the domain and DMARC"
	}
	return ""
}

func (s *Server) memberAddressBody(a store.MemberAddress, channels []sendChannel) oas.MemberAddress {
	out := oas.MemberAddress{
		Id: a.ID, Address: oas.Email(a.Address), Verification: oas.MemberAddressVerification(a.Verification),
		Verified: a.VerifiedAt != nil, VerifiedAt: a.VerifiedAt, UsableChannelIds: []uuid.UUID{}, CreatedAt: a.CreatedAt,
	}
	if a.VerifiedAt == nil {
		if a.CodeHash != nil && a.CodeExpiresAt != nil && a.CodeExpiresAt.After(s.now()) {
			out.CodeSentAt, out.CodeExpiresAt = a.CodeSentAt, a.CodeExpiresAt
		}
		return out
	}
	for _, sc := range channels {
		if s.unusableThrough(sc, a.Address) == "" {
			out.UsableChannelIds = append(out.UsableChannelIds, sc.ch.ChannelID)
		}
	}
	return out
}

func memberSendChannels(ctx context.Context, q *store.Queries, ws, member uuid.UUID, allInboxes bool) ([]sendChannel, error) {
	inboxes, err := q.ListInboxes(ctx, store.ListInboxesParams{WorkspaceID: ws, AllInboxes: allInboxes, ViewerID: member})
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(inboxes))
	for _, in := range inboxes {
		if !in.Personal || (in.OwnerMemberID != nil && *in.OwnerMemberID == member) {
			ids = append(ids, in.ID)
		}
	}
	chs, err := q.ListInboxEmailChannels(ctx, store.ListInboxEmailChannelsParams{WorkspaceID: ws, InboxIds: ids})
	if err != nil {
		return nil, err
	}
	out := make([]sendChannel, len(chs))
	for i, ch := range chs {
		if out[i], err = loadSendChannel(ctx, q, ch); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Server) memberAddressList(ctx context.Context, q *store.Queries, ws, member uuid.UUID, allInboxes bool) ([]oas.MemberAddress, error) {
	rows, err := q.ListMemberAddresses(ctx, store.ListMemberAddressesParams{WorkspaceID: ws, MemberID: member})
	if err != nil {
		return nil, err
	}
	var channels []sendChannel
	if slices.ContainsFunc(rows, func(a store.MemberAddress) bool { return a.VerifiedAt != nil }) {
		if channels, err = memberSendChannels(ctx, q, ws, member, allInboxes); err != nil {
			return nil, err
		}
	}
	out := make([]oas.MemberAddress, len(rows))
	for i, r := range rows {
		out[i] = s.memberAddressBody(r, channels)
	}
	return out, nil
}

func (s *Server) oneMemberAddressBody(ctx context.Context, q *store.Queries, member uuid.UUID, allInboxes bool, a store.MemberAddress) (oas.MemberAddress, error) {
	var channels []sendChannel
	if a.VerifiedAt != nil {
		var err error
		if channels, err = memberSendChannels(ctx, q, a.WorkspaceID, member, allInboxes); err != nil {
			return oas.MemberAddress{}, err
		}
	}
	return s.memberAddressBody(a, channels), nil
}

func (s *Server) ListMyAddresses(ctx context.Context, _ oas.ListMyAddressesRequestObject) (oas.ListMyAddressesResponseObject, error) {
	p := principalFrom(ctx)
	items, err := s.memberAddressList(ctx, s.st.Queries, p.workspaceID, p.memberID, p.seesAllInboxes())
	if err != nil {
		return nil, err
	}
	return oas.ListMyAddresses200JSONResponse{Items: items}, nil
}

func (s *Server) AddMyAddress(ctx context.Context, req oas.AddMyAddressRequestObject) (oas.AddMyAddressResponseObject, error) {
	p := principalFrom(ctx)
	addr, err := memberAddressInput(string(req.Body.Address))
	if err != nil {
		return nil, err
	}
	if err := s.memberAddressTaken(ctx, s.st.Queries, p.workspaceID, addr); err != nil {
		return nil, err
	}
	verification, err := memberAddressVerification(ctx, s.st.Queries, p.workspaceID, addr)
	if err != nil {
		return nil, err
	}
	now := s.now()
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.LockMemberAddressByAddress(ctx, store.LockMemberAddressByAddressParams{WorkspaceID: p.workspaceID, Address: addr})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return checkMemberAddressClaim(p, cur, now)
	})
	if err != nil {
		return nil, err
	}
	if verification == oas.MemberAddressVerificationCode {
		if ok, wait := s.limits.takeEvery(ctx,
			rateTake{"member_address_code:member:" + p.memberID.String(), limitMemberAddressCodes, 1},
			rateTake{"address_code:to:" + rateAddress(addr), limitCodesPerAddress, 1}); !ok {
			return nil, rateLimited(wait)
		}
	}
	code := newMemberAddressCode()
	expires := now.Add(memberAddressCodeTTL)
	var out oas.MemberAddress
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		if err := q.LockEmailAddress(ctx, addr); err != nil {
			return err
		}
		if err := s.memberAddressTaken(ctx, q, p.workspaceID, addr); err != nil {
			return err
		}
		if verification, err = memberAddressVerification(ctx, q, p.workspaceID, addr); err != nil {
			return err
		}
		var row store.MemberAddress
		cur, err := q.LockMemberAddressByAddress(ctx, store.LockMemberAddressByAddressParams{WorkspaceID: p.workspaceID, Address: addr})
		switch {
		case err == nil && cur.MemberID == p.memberID:
			if err := checkMemberAddressClaim(p, cur, now); err != nil {
				return err
			}
			if verification == oas.MemberAddressVerificationApproval {
				row, err = q.StartMemberAddressApproval(ctx, store.StartMemberAddressApprovalParams{WorkspaceID: p.workspaceID, ID: cur.ID})
			} else {
				row, err = q.StartMemberAddressCode(ctx, store.StartMemberAddressCodeParams{
					WorkspaceID: p.workspaceID, ID: cur.ID, CodeHash: memberAddressCodeHash(s.secrets.Derive(memberAddressCodeInfo), cur.ID, code),
					CodeExpiresAt: &expires, Now: &now,
				})
			}
			if err != nil {
				return err
			}
		case err == nil || store.IsNotFound(err):
			if err == nil {
				if err := checkMemberAddressClaim(p, cur, now); err != nil {
					return err
				}
				if err := q.DeleteMemberAddressRow(ctx, store.DeleteMemberAddressRowParams{WorkspaceID: p.workspaceID, ID: cur.ID}); err != nil {
					return err
				}
			}
			n, err := q.CountMemberAddresses(ctx, store.CountMemberAddressesParams{WorkspaceID: p.workspaceID, MemberID: p.memberID})
			if err != nil {
				return err
			}
			if n >= memberAddressesPerMember {
				return errMemberAddressLimit
			}
			id := newID()
			arg := store.CreateMemberAddressParams{
				WorkspaceID: p.workspaceID, ID: id, MemberID: p.memberID, Address: addr, Verification: string(verification), Now: now,
			}
			if verification == oas.MemberAddressVerificationCode {
				arg.CodeHash, arg.CodeExpiresAt, arg.CodeSentAt = memberAddressCodeHash(s.secrets.Derive(memberAddressCodeInfo), id, code), &expires, &now
			}
			if row, err = q.CreateMemberAddress(ctx, arg); err != nil {
				return err
			}
			if err := s.audit(ctx, q, p, oas.AuditMemberAddressAdded, oas.AuditTargetMember, p.memberID, auditDetails{"address": addr}); err != nil {
				return err
			}
		default:
			return err
		}
		out = s.memberAddressBody(row, nil)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if verification == oas.MemberAddressVerificationApproval {
		return oas.AddMyAddress202JSONResponse(out), nil
	}
	locale := "en"
	if m, err := s.st.GetMemberAddress(ctx, store.GetMemberAddressParams{WorkspaceID: p.workspaceID, ID: p.memberID}); err == nil {
		locale = m.Locale
	}
	msg, err := mail.Render(memberAddressCodeTemplate, locale, map[string]any{"Address": addr, "Code": code, "Minutes": int(memberAddressCodeTTL / time.Minute)})
	if err != nil {
		return nil, err
	}
	msg.To = addr
	s.sendMail(ctx, msg)
	return oas.AddMyAddress202JSONResponse(out), nil
}

func checkMemberAddressClaim(p principal, cur store.MemberAddress, now time.Time) error {
	switch {
	case cur.VerifiedAt != nil && cur.MemberID == p.memberID:
		return errAddressVerified
	case cur.VerifiedAt != nil:
		return errAddressTaken
	case cur.MemberID == p.memberID && cur.CodeSentAt != nil && cur.CodeSentAt.After(now.Add(-verifyCodeGap)):
		return errVerifyRateLimited
	}
	return nil
}

func (s *Server) VerifyMyAddress(ctx context.Context, req oas.VerifyMyAddressRequestObject) (oas.VerifyMyAddressResponseObject, error) {
	p := principalFrom(ctx)
	code := strings.TrimSpace(req.Body.Code)
	now := s.now()
	wrong := false
	var out oas.MemberAddress
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.LockMemberAddress(ctx, store.LockMemberAddressParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ID: req.MemberAddressId})
		if store.IsNotFound(err) {
			return errMemberAddressGone
		}
		if err != nil {
			return err
		}
		if cur.VerifiedAt == nil {
			if cur.CodeHash == nil || cur.CodeExpiresAt == nil || !cur.CodeExpiresAt.After(now) ||
				!s.anyDerived(memberAddressCodeInfo, func(k []byte) bool { return hmac.Equal(cur.CodeHash, memberAddressCodeHash(k, cur.ID, code)) }) {
				wrong = true
				return q.FailMemberAddressCode(ctx, store.FailMemberAddressCodeParams{WorkspaceID: p.workspaceID, ID: cur.ID, MaxAttempts: memberAddressMaxAttempts})
			}
			if cur, err = s.markMemberAddressVerified(ctx, q, p, cur, now, auditDetails{"address": cur.Address}); err != nil {
				return err
			}
		}
		out, err = s.oneMemberAddressBody(ctx, q, p.memberID, p.seesAllInboxes(), cur)
		return err
	})
	if err != nil {
		return nil, err
	}
	if wrong {
		return nil, errVerifyCodeInvalid
	}
	return oas.VerifyMyAddress200JSONResponse(out), nil
}

func (s *Server) markMemberAddressVerified(ctx context.Context, q *store.Queries, p principal, cur store.MemberAddress, now time.Time, details auditDetails) (store.MemberAddress, error) {
	if err := q.LockEmailAddress(ctx, cur.Address); err != nil {
		return cur, err
	}
	if err := s.memberAddressTaken(ctx, q, cur.WorkspaceID, cur.Address); err != nil {
		return cur, err
	}
	row, err := q.MarkMemberAddressVerified(ctx, store.MarkMemberAddressVerifiedParams{WorkspaceID: cur.WorkspaceID, ID: cur.ID, Now: &now})
	if err != nil {
		return cur, err
	}
	return row, s.audit(ctx, q, p, oas.AuditMemberAddressVerified, oas.AuditTargetMember, cur.MemberID, details)
}

func (s *Server) DeleteMyAddress(ctx context.Context, req oas.DeleteMyAddressRequestObject) (oas.DeleteMyAddressResponseObject, error) {
	p := principalFrom(ctx)
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		row, err := q.DeleteMemberAddress(ctx, store.DeleteMemberAddressParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ID: req.MemberAddressId})
		if store.IsNotFound(err) {
			return errMemberAddressGone
		}
		if err != nil {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditMemberAddressRemoved, oas.AuditTargetMember, p.memberID, auditDetails{"address": row.Address})
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteMyAddress204Response{}, nil
}

func (s *Server) ListMemberAddresses(ctx context.Context, req oas.ListMemberAddressesRequestObject) (oas.ListMemberAddressesResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	m, err := s.st.GetMember(ctx, store.GetMemberParams{WorkspaceID: p.workspaceID, ID: req.MemberId})
	if store.IsNotFound(err) {
		return nil, errMemberGone
	}
	if err != nil {
		return nil, err
	}
	items, err := s.memberAddressList(ctx, s.st.Queries, p.workspaceID, m.ID, managerRole(m.Role))
	if err != nil {
		return nil, err
	}
	return oas.ListMemberAddresses200JSONResponse{Items: items}, nil
}

func (s *Server) ApproveMemberAddress(ctx context.Context, req oas.ApproveMemberAddressRequestObject) (oas.ApproveMemberAddressResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	now := s.now()
	var out oas.MemberAddress
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		m, err := q.GetMember(ctx, store.GetMemberParams{WorkspaceID: p.workspaceID, ID: req.MemberId})
		if store.IsNotFound(err) {
			return errMemberAddressGone
		}
		if err != nil {
			return err
		}
		cur, err := q.LockMemberAddress(ctx, store.LockMemberAddressParams{WorkspaceID: p.workspaceID, MemberID: m.ID, ID: req.MemberAddressId})
		if store.IsNotFound(err) {
			return errMemberAddressGone
		}
		if err != nil {
			return err
		}
		if cur.VerifiedAt == nil {
			if cur.Verification != string(oas.MemberAddressVerificationApproval) {
				return errApprovalNotNeeded
			}
			if cur, err = s.markMemberAddressVerified(ctx, q, p, cur, now, auditDetails{"address": cur.Address, "by": "approval"}); err != nil {
				return err
			}
		}
		out, err = s.oneMemberAddressBody(ctx, q, m.ID, managerRole(m.Role), cur)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.ApproveMemberAddress200JSONResponse(out), nil
}

func managerRole(role string) bool {
	return role == roleOwner || role == roleAdmin
}

func (s *Server) fromMemberAddress(ctx context.Context, q *store.Queries, p principal, ch store.EmailChannel, from string) (store.MemberAddress, error) {
	if p.isKey() || p.deliversAsBot() || p.memberID == uuid.Nil() {
		return store.MemberAddress{}, errFromNotMember
	}
	addr := strings.ToLower(strings.TrimSpace(from))
	member, whose := p.memberID, "your"
	owner, err := channelMailboxOwner(ctx, q, p.workspaceID, ch.ChannelID)
	if err != nil {
		return store.MemberAddress{}, err
	}
	if owner != nil && *owner != p.memberID {
		member, whose = *owner, "the mailbox owner's"
	}
	row, err := q.GetVerifiedMemberAddress(ctx, store.GetVerifiedMemberAddressParams{WorkspaceID: p.workspaceID, MemberID: member, Address: addr})
	if store.IsNotFound(err) {
		return row, errFromNotAllowed(addr + " is not one of " + whose + " verified addresses")
	}
	if err != nil {
		return row, err
	}
	why, err := s.memberAddressUnusable(ctx, q, ch, addr)
	if err != nil {
		return row, err
	}
	if why != "" {
		return row, errFromNotAllowed(why)
	}
	return row, nil
}

// channelMailboxOwner is the owner of the personal mailbox the channel belongs to, nil for a shared
// inbox.
func channelMailboxOwner(ctx context.Context, q *store.Queries, ws, channelID uuid.UUID) (*uuid.UUID, error) {
	c, err := q.GetChannel(ctx, store.GetChannelParams{WorkspaceID: ws, ID: channelID})
	if err != nil {
		return nil, err
	}
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: c.InboxID})
	return in.OwnerMemberID, err
}

func memberName(ctx context.Context, q *store.Queries, ws, member uuid.UUID) (string, error) {
	names, err := q.ListMemberNames(ctx, store.ListMemberNamesParams{WorkspaceID: ws, Ids: []uuid.UUID{member}})
	if err != nil || len(names) != 1 {
		return "", err
	}
	return strings.TrimSpace(names[0].Name), nil
}

func (s *Server) applyFromAddress(ctx context.Context, q *store.Queries, p principal, plan *emailPlan, from *string) error {
	if from == nil {
		return nil
	}
	a, err := s.fromMemberAddress(ctx, q, p, plan.channel, *from)
	if err != nil {
		return err
	}
	name, err := memberName(ctx, q, a.WorkspaceID, a.MemberID)
	if err != nil {
		return err
	}
	plan.from = email.Address{Name: name, Email: a.Address}
	plan.memberAddressID = &a.ID
	return nil
}

package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
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

	"github.com/productdevbook/yuva/api/internal/cfsaas"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	domainPending  = string(oas.StatusDomainPending)
	domainVerified = string(oas.StatusDomainVerified)
	domainFailed   = string(oas.StatusDomainFailed)
	domainOff      = string(oas.StatusDomainOff)

	statusDomainsAsk        = "ask"
	statusDomainsCloudflare = "cloudflare"
	statusDomainsOff        = "off"

	statusDomainRetry    = 10 * time.Minute
	statusDomainRecheck  = 24 * time.Hour
	statusDomainGrace    = 72 * time.Hour
	statusDomainClaim    = 7 * 24 * time.Hour
	statusDomainTXT      = "_yuva-status."
	statusDomainCacheTTL = 30 * time.Second
	statusDomainMissMax  = 10000
	statusDomainSweepMax = 500
	statusDomainLookup   = 10 * time.Second

	domainProblemTaken   = "taken"
	domainProblemExpired = "expired"

	helpSignalCenter       = string(oas.HelpCenterUpdated)
	changelogSignalUpdates = string(oas.ChangelogUpdated)

	kindStatus    = "status"
	kindHelp      = "help"
	kindRoadmap   = "roadmap"
	kindChangelog = "changelog"
)

var (
	errStatusDomainTaken = problem(http.StatusConflict, "domain_taken", "another page has this domain")
	errStatusDomainLimit = problem(http.StatusForbidden, "status_domain_limit", "this workspace has as many own domains as the server allows (YUVA_STATUS_DOMAIN_LIMIT)")

	domainLabelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	statusDomainPaths  = []string{"", "/history", "/feed.atom", "/badge.svg", "/subscribe", "/subscribe/sent"}
)

type StatusResolver interface {
	LookupCNAME(ctx context.Context, host string) (string, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// StatusDomainArgs checks the own domain of one status page, help center, roadmap board or changelog;
// Kind is empty for a status page.
type StatusDomainArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	PageID      uuid.UUID `json:"page_id"`
	PageKind    string    `json:"kind,omitempty"`
}

func (StatusDomainArgs) Kind() string { return "status_domain_check" }

func (StatusDomainArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled, rivertype.JobStateRetryable,
	}}}
}

func domainCheckArgs(kind string, ws, id uuid.UUID) StatusDomainArgs {
	if kind == kindStatus {
		kind = ""
	}
	return StatusDomainArgs{WorkspaceID: ws, PageID: id, PageKind: kind}
}

// StatusDomainReleaseArgs deletes a Cloudflare custom hostname whose domain was removed, turned off
// or whose page is gone.
type StatusDomainReleaseArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	HostnameID  string    `json:"hostname_id"`
	Domain      string    `json:"domain"`
}

func (StatusDomainReleaseArgs) Kind() string { return "status_domain_release" }

type ownDomain struct {
	kind      string
	workspace uuid.UUID
	id        uuid.UUID
	inbox     uuid.UUID
	slug      string
	live      bool
	row       any
	cols      store.WriteStatusPageDomainParams
}

func statusOwn(pg store.StatusPage) ownDomain {
	return ownDomain{kind: kindStatus, workspace: pg.WorkspaceID, id: pg.ID, inbox: pg.InboxID, slug: pg.Slug, live: pg.Published, row: pg,
		cols: store.WriteStatusPageDomainParams{WorkspaceID: pg.WorkspaceID, ID: pg.ID, Domain: pg.CustomDomain, Token: pg.DomainToken,
			State: pg.DomainState, Problem: pg.DomainProblem, CheckedAt: pg.DomainCheckedAt, CheckAt: pg.DomainCheckAt,
			VerifiedAt: pg.DomainVerifiedAt, MissingSince: pg.DomainMissingSince, ClaimedAt: pg.DomainClaimedAt,
			CfHostnameID: pg.CfHostnameID, CfStatus: pg.CfStatus, CfSslStatus: pg.CfSslStatus, CfErrors: pg.CfErrors, CfCheckedAt: pg.CfCheckedAt}}
}

func helpOwn(c store.HelpCenter) ownDomain {
	return ownDomain{kind: kindHelp, workspace: c.WorkspaceID, id: c.ID, inbox: c.InboxID, slug: c.Slug, live: c.Published, row: c,
		cols: store.WriteStatusPageDomainParams{WorkspaceID: c.WorkspaceID, ID: c.ID, Domain: c.CustomDomain, Token: c.DomainToken,
			State: c.DomainState, Problem: c.DomainProblem, CheckedAt: c.DomainCheckedAt, CheckAt: c.DomainCheckAt,
			VerifiedAt: c.DomainVerifiedAt, MissingSince: c.DomainMissingSince, ClaimedAt: c.DomainClaimedAt,
			CfHostnameID: c.CfHostnameID, CfStatus: c.CfStatus, CfSslStatus: c.CfSslStatus, CfErrors: c.CfErrors, CfCheckedAt: c.CfCheckedAt}}
}

func boardOwn(b store.RoadmapBoard) ownDomain {
	return ownDomain{kind: kindRoadmap, workspace: b.WorkspaceID, id: b.ID, inbox: b.InboxID, slug: b.Slug, live: b.Public, row: b,
		cols: store.WriteStatusPageDomainParams{WorkspaceID: b.WorkspaceID, ID: b.ID, Domain: b.CustomDomain, Token: b.DomainToken,
			State: b.DomainState, Problem: b.DomainProblem, CheckedAt: b.DomainCheckedAt, CheckAt: b.DomainCheckAt,
			VerifiedAt: b.DomainVerifiedAt, MissingSince: b.DomainMissingSince, ClaimedAt: b.DomainClaimedAt,
			CfHostnameID: b.CfHostnameID, CfStatus: b.CfStatus, CfSslStatus: b.CfSslStatus, CfErrors: b.CfErrors, CfCheckedAt: b.CfCheckedAt}}
}

func changelogOwn(c store.Changelog) ownDomain {
	return ownDomain{kind: kindChangelog, workspace: c.WorkspaceID, id: c.ID, inbox: c.InboxID, slug: c.Slug, live: c.Published, row: c,
		cols: store.WriteStatusPageDomainParams{WorkspaceID: c.WorkspaceID, ID: c.ID, Domain: c.CustomDomain, Token: c.DomainToken,
			State: c.DomainState, Problem: c.DomainProblem, CheckedAt: c.DomainCheckedAt, CheckAt: c.DomainCheckAt,
			VerifiedAt: c.DomainVerifiedAt, MissingSince: c.DomainMissingSince, ClaimedAt: c.DomainClaimedAt,
			CfHostnameID: c.CfHostnameID, CfStatus: c.CfStatus, CfSslStatus: c.CfSslStatus, CfErrors: c.CfErrors, CfCheckedAt: c.CfCheckedAt}}
}

type ownKind struct {
	get   func(context.Context, *store.Queries, uuid.UUID, uuid.UUID) (ownDomain, error)
	lock  func(context.Context, *store.Queries, uuid.UUID, uuid.UUID) (ownDomain, error)
	write func(context.Context, *store.Queries, store.WriteStatusPageDomainParams) (ownDomain, error)
	on    func() bool
	moved func(ctx context.Context, ws, inbox, id uuid.UUID)
}

func (s *Server) ownKind(kind string) (ownKind, bool) {
	switch kind {
	case "", kindStatus:
		return ownKind{
			get: func(ctx context.Context, q *store.Queries, ws, id uuid.UUID) (ownDomain, error) {
				pg, err := q.GetStatusPage(ctx, store.GetStatusPageParams{WorkspaceID: ws, ID: id})
				return statusOwn(pg), err
			},
			lock: func(ctx context.Context, q *store.Queries, ws, id uuid.UUID) (ownDomain, error) {
				pg, err := q.LockStatusPage(ctx, store.LockStatusPageParams{WorkspaceID: ws, ID: id})
				return statusOwn(pg), err
			},
			write: func(ctx context.Context, q *store.Queries, p store.WriteStatusPageDomainParams) (ownDomain, error) {
				pg, err := q.WriteStatusPageDomain(ctx, p)
				return statusOwn(pg), err
			},
			on: func() bool { return true },
			moved: func(ctx context.Context, ws, inbox, id uuid.UUID) {
				s.statusHint(ctx, ws, inbox)
				s.statusPageSignal(ctx, ws, inbox, id)
			},
		}, true
	case kindHelp:
		return ownKind{
			get: func(ctx context.Context, q *store.Queries, ws, id uuid.UUID) (ownDomain, error) {
				c, err := q.GetHelpCenter(ctx, store.GetHelpCenterParams{WorkspaceID: ws, ID: id})
				return helpOwn(c), err
			},
			lock: func(ctx context.Context, q *store.Queries, ws, id uuid.UUID) (ownDomain, error) {
				c, err := q.LockHelpCenter(ctx, store.LockHelpCenterParams{WorkspaceID: ws, ID: id})
				return helpOwn(c), err
			},
			write: func(ctx context.Context, q *store.Queries, p store.WriteStatusPageDomainParams) (ownDomain, error) {
				c, err := q.WriteHelpCenterDomain(ctx, store.WriteHelpCenterDomainParams(p))
				return helpOwn(c), err
			},
			on: func() bool { return !s.help.Off },
			moved: func(ctx context.Context, ws, inbox, id uuid.UUID) {
				s.statusHint(ctx, ws, inbox)
				s.helpCenterSignal(ctx, ws, inbox, id)
			},
		}, true
	case kindRoadmap:
		return ownKind{
			get: func(ctx context.Context, q *store.Queries, ws, id uuid.UUID) (ownDomain, error) {
				b, err := q.GetRoadmapBoard(ctx, store.GetRoadmapBoardParams{WorkspaceID: ws, ID: id})
				return boardOwn(b), err
			},
			lock: func(ctx context.Context, q *store.Queries, ws, id uuid.UUID) (ownDomain, error) {
				b, err := q.LockRoadmapBoard(ctx, store.LockRoadmapBoardParams{WorkspaceID: ws, ID: id})
				return boardOwn(b), err
			},
			write: func(ctx context.Context, q *store.Queries, p store.WriteStatusPageDomainParams) (ownDomain, error) {
				b, err := q.WriteRoadmapBoardDomain(ctx, store.WriteRoadmapBoardDomainParams(p))
				return boardOwn(b), err
			},
			on: func() bool { return !s.roadmap.Off },
			moved: func(ctx context.Context, ws, inbox, id uuid.UUID) {
				s.statusHint(ctx, ws, inbox)
				s.boardSignal(ctx, ws, inbox, id)
			},
		}, true
	case kindChangelog:
		return ownKind{
			get: func(ctx context.Context, q *store.Queries, ws, id uuid.UUID) (ownDomain, error) {
				c, err := q.GetChangelog(ctx, store.GetChangelogParams{WorkspaceID: ws, ID: id})
				return changelogOwn(c), err
			},
			lock: func(ctx context.Context, q *store.Queries, ws, id uuid.UUID) (ownDomain, error) {
				c, err := q.LockChangelog(ctx, store.LockChangelogParams{WorkspaceID: ws, ID: id})
				return changelogOwn(c), err
			},
			write: func(ctx context.Context, q *store.Queries, p store.WriteStatusPageDomainParams) (ownDomain, error) {
				c, err := q.WriteChangelogDomain(ctx, store.WriteChangelogDomainParams(p))
				return changelogOwn(c), err
			},
			on: func() bool { return !s.changelog.Off },
			moved: func(ctx context.Context, ws, inbox, id uuid.UUID) {
				s.statusHint(ctx, ws, inbox)
				s.changelogUpdatedSignal(ctx, ws, inbox, id)
			},
		}, true
	}
	return ownKind{}, false
}

func (s *Server) helpCenterSignal(ctx context.Context, ws, inboxID, centerID uuid.UUID) {
	s.signal(ctx, realtime.Event{Type: helpSignalCenter, WorkspaceID: ws, InboxID: &inboxID, Data: mustJSON(map[string]uuid.UUID{"center_id": centerID})})
}

func (s *Server) changelogUpdatedSignal(ctx context.Context, ws, inboxID, changelogID uuid.UUID) {
	s.signal(ctx, realtime.Event{Type: changelogSignalUpdates, WorkspaceID: ws, InboxID: &inboxID, Data: mustJSON(map[string]uuid.UUID{"changelog_id": changelogID})})
}

func (s *Server) statusDomainsMode() string {
	if s.status.Domains == "" {
		return statusDomainsAsk
	}
	return s.status.Domains
}

func (s *Server) statusDomainsOn() bool { return s.statusDomainsMode() != statusDomainsOff }

func hostnameOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func (s *Server) statusCNAME() string {
	if s.status.CNAME != "" {
		return strings.TrimSuffix(strings.ToLower(s.status.CNAME), ".")
	}
	return hostnameOf(s.auth.PublicURL)
}

func (s *Server) ownHosts() []string {
	var out []string
	for _, h := range []string{hostnameOf(s.auth.PublicURL), hostnameOf(s.status.URL), s.statusCNAME(), hostnameOf(s.help.URL)} {
		if h != "" && !slices.Contains(out, h) {
			out = append(out, h)
		}
	}
	return out
}

// domainLive is whether links name the page on its own domain: once verified, and with Cloudflare
// once the custom hostname and its certificate are active, since until then the domain has no TLS.
func (s *Server) domainLive(domain, state, cfStatus, cfSSL *string) bool {
	if !s.statusDomainsOn() || domain == nil || state == nil || *state != domainVerified {
		return false
	}
	if s.statusDomainsMode() == statusDomainsCloudflare {
		return cfStatus != nil && *cfStatus == "active" && cfSSL != nil && *cfSSL == "active"
	}
	return true
}

func (s *Server) statusURLFor(slug string, domain, state, cfStatus, cfSSL *string, path string) string {
	if s.domainLive(domain, state, cfStatus, cfSSL) {
		return s.domainURL(*domain, path)
	}
	return s.statusURL(slug, path)
}

func (s *Server) domainURL(domain, path string) string {
	scheme := "https"
	if u, err := url.Parse(s.auth.PublicURL); err == nil && u.Scheme == "http" {
		scheme = "http"
	}
	return scheme + "://" + domain + path
}

func (s *Server) pageURL(pg store.StatusPage, path string) string {
	return s.statusURLFor(pg.Slug, pg.CustomDomain, pg.DomainState, pg.CfStatus, pg.CfSslStatus, path)
}

func spelledDomain(raw string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
}

func domainSyntax(d string) bool {
	if len(d) < 4 || len(d) > 253 || net.ParseIP(d) != nil {
		return false
	}
	labels := strings.Split(d, ".")
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if !domainLabelPattern.MatchString(l) {
			return false
		}
	}
	return strings.Trim(labels[len(labels)-1], "0123456789") != ""
}

func (s *Server) normalStatusDomain(raw string) (string, error) {
	d := spelledDomain(raw)
	if !domainSyntax(d) {
		return "", errValidation("custom_domain must be a hostname such as status.example.com")
	}
	for _, h := range s.ownHosts() {
		if d == h || strings.HasSuffix(d, "."+h) || strings.HasSuffix(h, "."+d) {
			return "", errValidation("custom_domain must not be, contain or be inside one of this server's own hosts")
		}
	}
	return d, nil
}

func newDomainToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "yuva-verify-" + hex.EncodeToString(b)
}

func (s *Server) customDomainBody(d ownDomain) *oas.StatusCustomDomain {
	c := d.cols
	if !s.statusDomainsOn() || c.Domain == nil || c.State == nil || c.Token == nil {
		return nil
	}
	out := &oas.StatusCustomDomain{
		Domain: *c.Domain, State: oas.StatusDomainState(*c.State), CnameTarget: s.statusCNAME(),
		TxtName: statusDomainTXT + *c.Domain, TxtValue: *c.Token, Certificates: oas.StatusCertificatesAsk,
		Problem: c.Problem, CheckedAt: c.CheckedAt, NextCheckAt: c.CheckAt, VerifiedAt: c.VerifiedAt,
		MissingSince: c.MissingSince,
	}
	if s.statusDomainsMode() == statusDomainsCloudflare {
		out.Certificates = oas.StatusCertificatesCloudflare
		cf := &oas.StatusCloudflareHostname{Id: c.CfHostnameID, Status: c.CfStatus, SslStatus: c.CfSslStatus, CheckedAt: c.CfCheckedAt}
		if len(c.CfErrors) > 0 {
			errs := slices.Clone(c.CfErrors)
			cf.Errors = &errs
		}
		out.Cloudflare = cf
	}
	return out
}

func (s *Server) ownDomainsBody() *oas.StatusOwnDomains {
	switch s.statusDomainsMode() {
	case statusDomainsAsk:
		return &oas.StatusOwnDomains{Mode: oas.StatusOwnDomainsAsk}
	case statusDomainsCloudflare:
		return &oas.StatusOwnDomains{Mode: oas.StatusOwnDomainsCloudflare}
	}
	return nil
}

func releaseCloudflareHostname(ev *eventBatch, ws uuid.UUID, hostnameID, domain *string) {
	if hostnameID != nil && domain != nil {
		ev.job(StatusDomainReleaseArgs{WorkspaceID: ws, HostnameID: *hostnameID, Domain: *domain}, nil)
	}
}

func releaseCloudflare(ev *eventBatch, pg store.StatusPage) {
	releaseCloudflareHostname(ev, pg.WorkspaceID, pg.CfHostnameID, pg.CustomDomain)
}

func releaseOwnCloudflare(ev *eventBatch, d ownDomain) {
	releaseCloudflareHostname(ev, d.workspace, d.cols.CfHostnameID, d.cols.Domain)
}

// setStatusDomain applies custom_domain of a PATCH to the locked page.
func (s *Server) setStatusDomain(ctx context.Context, q *store.Queries, ev *eventBatch, cur store.StatusPage, raw *string) (store.StatusPage, error) {
	d, err := s.setOwnDomain(ctx, q, ev, statusOwn(cur), raw)
	return d.row.(store.StatusPage), err
}

func customDomainField(v interface {
	IsSpecified() bool
	IsNull() bool
	MustGet() string
}) (bool, *string) {
	if !v.IsSpecified() {
		return false, nil
	}
	if v.IsNull() {
		return true, nil
	}
	raw := v.MustGet()
	return true, &raw
}

func (s *Server) setOwnDomain(ctx context.Context, q *store.Queries, ev *eventBatch, cur ownDomain, raw *string) (ownDomain, error) {
	k, _ := s.ownKind(cur.kind)
	now := s.now()
	c := cur.cols
	if raw == nil || strings.TrimSpace(*raw) == "" {
		if c.Domain == nil {
			return cur, nil
		}
		releaseOwnCloudflare(ev, cur)
		return k.write(ctx, q, store.WriteStatusPageDomainParams{WorkspaceID: cur.workspace, ID: cur.id, Moved: true, Now: now})
	}
	if !s.statusDomainsOn() {
		if c.Domain != nil && *c.Domain == spelledDomain(*raw) {
			return cur, nil
		}
		return cur, errValidation("own domains are off on this server (YUVA_STATUS_DOMAINS=off)")
	}
	d, err := s.normalStatusDomain(*raw)
	if err != nil {
		return cur, err
	}
	check := func(o ownDomain) (ownDomain, error) {
		ev.job(domainCheckArgs(o.kind, o.workspace, o.id), nil)
		return o, nil
	}
	if c.Domain != nil && *c.Domain == d && *c.State == domainVerified {
		c.CheckAt, c.Moved, c.Now = &now, false, now
		o, err := k.write(ctx, q, c)
		if err != nil {
			return cur, err
		}
		return check(o)
	}
	if taken, err := q.OwnDomainTaken(ctx, store.OwnDomainTakenParams{Domain: d, ID: cur.id}); err != nil || taken {
		if err == nil {
			err = errStatusDomainTaken
		}
		return cur, err
	}
	if lim := s.status.DomainLimit; lim > 0 {
		if err := q.LockStatusDomainCount(ctx, cur.workspace); err != nil {
			return cur, err
		}
		n, err := q.CountOwnDomains(ctx, store.CountOwnDomainsParams{WorkspaceID: cur.workspace, ID: cur.id})
		if err != nil {
			return cur, err
		}
		if n >= int64(lim) {
			return cur, errStatusDomainLimit
		}
	}
	token := newDomainToken()
	if c.Domain != nil && *c.Domain == d {
		token = *c.Token
	}
	releaseOwnCloudflare(ev, cur)
	state := domainPending
	o, err := k.write(ctx, q, store.WriteStatusPageDomainParams{
		WorkspaceID: cur.workspace, ID: cur.id, Domain: &d, Token: &token, State: &state, CheckAt: &now, ClaimedAt: &now, Moved: true, Now: now,
	})
	if err != nil {
		return cur, err
	}
	return check(o)
}

type domainLookup struct {
	seen, wrong, unsure bool
	problem             string
}

func dnsMissing(err error) bool {
	var de *net.DNSError
	return errors.As(err, &de) && de.IsNotFound
}

func (s *Server) lookupStatusDomain(ctx context.Context, domain, token string) domainLookup {
	r := s.status.Resolver
	if r == nil {
		r = net.DefaultResolver
	}
	ctx, cancel := context.WithTimeout(ctx, statusDomainLookup)
	defer cancel()
	target := s.statusCNAME()
	var out domainLookup
	var problems []string
	cnameOK := false
	cname, err := r.LookupCNAME(ctx, domain)
	switch {
	case err != nil && !dnsMissing(err):
		out.unsure = true
		problems = append(problems, "DNS lookup of "+domain+" failed: "+err.Error())
	case err == nil && strings.TrimSuffix(strings.ToLower(cname), ".") == target:
		cnameOK = true
	case err == nil:
		mine, err1 := r.LookupHost(ctx, domain)
		theirs, err2 := r.LookupHost(ctx, target)
		if err1 == nil && err2 == nil && len(mine) > 0 && !slices.ContainsFunc(mine, func(a string) bool { return !slices.Contains(theirs, a) }) {
			cnameOK = true
		} else if len(mine) > 0 {
			out.wrong = true
			problems = append(problems, domain+" points at "+strings.TrimSuffix(cname, ".")+", not at "+target)
		} else {
			problems = append(problems, "no CNAME from "+domain+" to "+target)
		}
	default:
		problems = append(problems, "no CNAME from "+domain+" to "+target)
	}
	txtOK := false
	txts, err := r.LookupTXT(ctx, statusDomainTXT+domain)
	switch {
	case err != nil && !dnsMissing(err):
		out.unsure = true
		problems = append(problems, "DNS lookup of "+statusDomainTXT+domain+" failed: "+err.Error())
	case slices.ContainsFunc(txts, func(t string) bool { return strings.TrimSpace(t) == token }):
		txtOK = true
	case len(txts) > 0:
		out.wrong = true
		problems = append(problems, "the TXT record "+statusDomainTXT+domain+" does not hold the verification value")
	default:
		problems = append(problems, "no TXT record "+statusDomainTXT+domain)
	}
	out.seen = cnameOK && txtOK
	out.problem = strings.Join(problems, "; ")
	return out
}

func (s *Server) RunStatusDomainCheck(ctx context.Context, ws, id uuid.UUID) error {
	return s.RunOwnDomainCheck(ctx, kindStatus, ws, id)
}

// RunOwnDomainCheck looks the own domain of a status page, help center ("help"), roadmap board
// ("roadmap") or changelog ("changelog") up and moves its state: pending or failed to verified when
// both records are there and no other page of any kind holds the domain, verified to off when they
// have been gone for 3 days, and a claim not verified within 7 days to failed. A lookup that fails for
// another reason than a missing record changes nothing and is tried again in 10 minutes.
func (s *Server) RunOwnDomainCheck(ctx context.Context, kind string, ws, id uuid.UUID) error {
	k, ok := s.ownKind(kind)
	if !ok || !s.statusDomainsOn() {
		return nil
	}
	pg, err := k.get(ctx, s.st.Queries, ws, id)
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if pg.cols.Domain == nil || pg.cols.CheckAt == nil || *pg.cols.State == domainOff {
		return nil
	}
	domain, token := *pg.cols.Domain, *pg.cols.Token
	look := s.lookupStatusDomain(ctx, domain, token)
	var moved, verified bool
	var others []store.FailOwnDomainClaimsRow
	err = s.inTx(ctx, ws, func(q *store.Queries, ev *eventBatch) error {
		if err := q.LockStatusDomainName(ctx, domain); err != nil {
			return err
		}
		cur, err := k.lock(ctx, q, ws, id)
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		c := cur.cols
		if c.Domain == nil || *c.Domain != domain || *c.Token != token || *c.State == domainOff || c.CheckAt == nil {
			return nil
		}
		now := s.now()
		state := *c.State
		arg := c
		arg.Now, arg.CheckedAt, arg.Problem, arg.CheckAt = now, &now, nil, nil
		if look.problem != "" {
			arg.Problem = &look.problem
		}
		next := now.Add(statusDomainRetry)
		halt := false
		set := func(v string) { arg.State = &v }
		stop := func(problem string) {
			set(domainFailed)
			arg.Problem, halt = &problem, true
		}
		expired := c.ClaimedAt == nil || !now.Before(c.ClaimedAt.Add(statusDomainClaim))
		switch {
		case look.unsure:
		case look.seen && state == domainVerified:
			arg.MissingSince = nil
			next = now.Add(statusDomainRecheck)
		case look.seen:
			taken, err := q.OwnDomainTaken(ctx, store.OwnDomainTakenParams{Domain: domain, ID: id})
			if err != nil {
				return err
			}
			if taken {
				stop(domainProblemTaken)
				break
			}
			set(domainVerified)
			arg.VerifiedAt, arg.MissingSince = &now, nil
			next = now.Add(statusDomainRecheck)
		case state == domainVerified:
			if arg.MissingSince == nil {
				arg.MissingSince = &now
			}
			next = now.Add(statusDomainRecheck)
			if !now.Before(arg.MissingSince.Add(statusDomainGrace)) {
				set(domainOff)
			} else if until := arg.MissingSince.Add(statusDomainGrace); until.Before(next) {
				next = until
			}
		case expired:
			stop(domainProblemExpired)
		case look.wrong:
			set(domainFailed)
		default:
			set(domainPending)
		}
		if *arg.State != domainOff && !halt {
			arg.CheckAt = &next
		}
		moved = (*arg.State == domainVerified) != (state == domainVerified)
		arg.Moved = moved
		verified = *arg.State == domainVerified
		if *arg.State == domainOff && c.CfHostnameID != nil {
			releaseOwnCloudflare(ev, cur)
			arg.CfHostnameID, arg.CfStatus, arg.CfSslStatus, arg.CfErrors, arg.CfCheckedAt = nil, nil, nil, nil, &now
		}
		if _, err := k.write(ctx, q, arg); err != nil {
			return err
		}
		if verified && moved {
			if others, err = q.FailOwnDomainClaims(ctx, store.FailOwnDomainClaimsParams{Now: now, Domain: domain, ID: id}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if moved {
		s.statusDomainCache.forget(id, domain)
		k.moved(ctx, ws, pg.inbox, id)
	}
	for _, o := range others {
		if other, found := s.ownKind(o.Kind); found {
			s.statusDomainCache.forget(o.ID)
			other.moved(ctx, o.WorkspaceID, o.InboxID, o.ID)
		}
	}
	if !verified || s.statusDomainsMode() != statusDomainsCloudflare {
		return nil
	}
	pg, err = k.get(ctx, s.st.Queries, ws, id)
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if pg.cols.Domain == nil || *pg.cols.Domain != domain || *pg.cols.State != domainVerified {
		return nil
	}
	return s.syncCloudflareHostname(ctx, k, pg)
}

// syncCloudflareHostname creates the verified domain's custom hostname, or reads its state, and keeps
// checking every 10 minutes until Cloudflare reports both the hostname and its certificate active.
func (s *Server) syncCloudflareHostname(ctx context.Context, k ownKind, pg ownDomain) error {
	cf := s.status.Cloudflare
	if cf == nil {
		return nil
	}
	domain := *pg.cols.Domain
	var h cfsaas.Hostname
	var err error
	created := false
	if pg.cols.CfHostnameID != nil {
		h, err = cf.Get(ctx, *pg.cols.CfHostnameID)
	}
	if pg.cols.CfHostnameID == nil || errors.Is(err, cfsaas.ErrNotFound) {
		h, err = cf.Create(ctx, domain)
		created = err == nil
	}
	if err != nil {
		s.log.ErrorContext(ctx, "cloudflare custom hostname", slog.String("workspace_id", pg.workspace.String()),
			slog.String("domain", domain), slog.Any("error", err))
	}
	changed := false
	err = s.inTx(ctx, pg.workspace, func(q *store.Queries, ev *eventBatch) error {
		cur, lerr := k.lock(ctx, q, pg.workspace, pg.id)
		c := cur.cols
		gone := store.IsNotFound(lerr) || (lerr == nil && (c.Domain == nil || *c.Domain != domain || *c.State != domainVerified))
		if gone {
			if created {
				ev.job(StatusDomainReleaseArgs{WorkspaceID: pg.workspace, HostnameID: h.ID, Domain: domain}, nil)
			}
			return nil
		}
		if lerr != nil {
			return lerr
		}
		now := s.now()
		retry := now.Add(statusDomainRetry)
		arg := c
		arg.Now, arg.CfCheckedAt, arg.CheckAt = now, &now, &retry
		if err != nil {
			arg.CfErrors = []string{err.Error()}
		} else {
			arg.CfHostnameID, arg.CfStatus, arg.CfSslStatus, arg.CfErrors = &h.ID, &h.Status, &h.SSL, h.Errors
			if h.Status == "active" && h.SSL == "active" {
				arg.CheckAt = c.CheckAt
			}
		}
		changed = !sameString(arg.CfStatus, c.CfStatus) || !sameString(arg.CfSslStatus, c.CfSslStatus)
		arg.Moved = changed
		_, err := k.write(ctx, q, arg)
		return err
	})
	if err == nil && changed {
		s.statusDomainCache.forget(pg.id, domain)
		k.moved(ctx, pg.workspace, pg.inbox, pg.id)
	}
	return err
}

// ReleaseStatusDomain deletes a custom hostname at Cloudflare; an error asks River to try again.
func (s *Server) ReleaseStatusDomain(ctx context.Context, a StatusDomainReleaseArgs) error {
	if s.status.Cloudflare == nil {
		s.log.ErrorContext(ctx, "cloudflare custom hostname not deleted: YUVA_STATUS_DOMAINS is not cloudflare",
			slog.String("domain", a.Domain), slog.String("hostname_id", a.HostnameID))
		return nil
	}
	return s.status.Cloudflare.Delete(ctx, a.HostnameID)
}

// QueueStatusDomainChecks queues a check of every own domain that is due, of every kind.
func (s *Server) QueueStatusDomainChecks(ctx context.Context) error {
	if !s.statusDomainsOn() {
		return nil
	}
	due, err := s.st.ListDueOwnDomains(ctx, store.ListDueOwnDomainsParams{Now: s.now(), Lim: statusDomainSweepMax})
	if err != nil || len(due) == 0 {
		return err
	}
	params := make([]river.InsertManyParams, len(due))
	for i, d := range due {
		params[i] = river.InsertManyParams{Args: domainCheckArgs(d.Kind, d.WorkspaceID, d.ID)}
	}
	_, err = s.jobs.InsertMany(ctx, params)
	return err
}

type statusDomainCheckWorker struct {
	river.WorkerDefaults[StatusDomainArgs]
	s *Server
}

func (w *statusDomainCheckWorker) Work(ctx context.Context, job *river.Job[StatusDomainArgs]) error {
	return w.s.RunOwnDomainCheck(ctx, job.Args.PageKind, job.Args.WorkspaceID, job.Args.PageID)
}

type statusDomainReleaseWorker struct {
	river.WorkerDefaults[StatusDomainReleaseArgs]
	s *Server
}

func (w *statusDomainReleaseWorker) Work(ctx context.Context, job *river.Job[StatusDomainReleaseArgs]) error {
	return w.s.ReleaseStatusDomain(ctx, job.Args)
}

type statusDomainSweepWorker struct {
	river.WorkerDefaults[jobs.StatusDomainSweepArgs]
	s *Server
}

func (w *statusDomainSweepWorker) Work(ctx context.Context, _ *river.Job[jobs.StatusDomainSweepArgs]) error {
	return w.s.QueueStatusDomainChecks(ctx)
}

func (s *Server) addStatusDomainWorkers(workers *river.Workers) {
	river.AddWorker(workers, &statusDomainCheckWorker{s: s})
	river.AddWorker(workers, &statusDomainReleaseWorker{s: s})
	river.AddWorker(workers, &statusDomainSweepWorker{s: s})
}

func (s *Server) CheckStatusDomain(ctx context.Context, req oas.CheckStatusDomainRequestObject) (oas.CheckStatusDomainResponseObject, error) {
	if err := s.rateLimit(ctx, rateCheck{"status-domain-check:ip:" + rateIP(s.clientIP(requestFrom(ctx))), limitStatusPerIP}); err != nil {
		return nil, err
	}
	d := spelledDomain(req.Params.Domain)
	if s.statusDomainsMode() != statusDomainsAsk || !domainSyntax(d) {
		return nil, errNotFound
	}
	hit, err := s.st.FindOwnDomain(ctx, d)
	if store.IsNotFound(err) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	if k, ok := s.ownKind(hit.Kind); !ok || !hit.Live || !k.on() {
		return nil, errNotFound
	}
	return oas.CheckStatusDomain200JSONResponse{Domain: d}, nil
}

type statusDomainHit struct {
	kind            string
	workspace, page uuid.UUID
	slug            string
	at              time.Time
}

// statusDomainCache keeps the verified own domains apart from the hosts that are not one, so requests on
// arbitrary hosts never push a verified domain out.
type statusDomainCache struct {
	mu       sync.Mutex
	gen      uint64
	verified map[string]statusDomainHit
	misses   map[string]time.Time
}

func newStatusDomainCache() *statusDomainCache {
	return &statusDomainCache{verified: map[string]statusDomainHit{}, misses: map[string]time.Time{}}
}

func (c *statusDomainCache) forget(page uuid.UUID, hosts ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	for h, e := range c.verified {
		if e.page == page {
			delete(c.verified, h)
		}
	}
	for _, h := range hosts {
		delete(c.verified, h)
		delete(c.misses, h)
	}
}

func (c *statusDomainCache) get(host string, now time.Time) (statusDomainHit, bool, bool, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.verified[host]; ok && now.Sub(e.at) < statusDomainCacheTTL {
		return e, true, true, c.gen
	}
	if at, ok := c.misses[host]; ok && now.Sub(at) < statusDomainCacheTTL {
		return statusDomainHit{}, false, true, c.gen
	}
	return statusDomainHit{}, false, false, c.gen
}

func (c *statusDomainCache) put(host string, e statusDomainHit, ok bool, gen uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.gen {
		return
	}
	if ok {
		delete(c.misses, host)
		c.verified[host] = e
		return
	}
	delete(c.verified, host)
	if len(c.misses) >= statusDomainMissMax {
		for h, at := range c.misses {
			if e.at.Sub(at) >= statusDomainCacheTTL {
				delete(c.misses, h)
			}
		}
		if len(c.misses) >= statusDomainMissMax {
			c.misses = map[string]time.Time{}
		}
	}
	c.misses[host] = e.at
}

func (s *Server) domainPage(ctx context.Context, host string) (statusDomainHit, bool) {
	if !domainSyntax(host) {
		return statusDomainHit{}, false
	}
	c := s.statusDomainCache
	now := time.Now()
	e, ok, hit, gen := c.get(host, now)
	if hit {
		return e, ok
	}
	pg, err := s.st.FindOwnDomain(ctx, host)
	if err != nil && !store.IsNotFound(err) {
		s.log.ErrorContext(ctx, "own domain", slog.Any("error", err))
		return statusDomainHit{}, false
	}
	e = statusDomainHit{at: now}
	if ok = err == nil; ok {
		e.kind, e.workspace, e.page, e.slug = pg.Kind, pg.WorkspaceID, pg.ID, pg.Slug
	}
	c.put(host, e, ok, gen)
	return e, ok
}

// statusDomainPath maps a path on an own domain to the page's path under /status/{slug}; false for
// anything else, which the domain does not serve.
func statusDomainPath(slug, path string) (string, bool) {
	if path == "/" {
		path = ""
	}
	if slices.Contains(statusDomainPaths, path) {
		return statusPathPrefix + slug + path, true
	}
	if id, ok := strings.CutPrefix(path, "/incidents/"); ok && id != "" && !strings.Contains(id, "/") {
		return statusPathPrefix + slug + path, true
	}
	if name, ok := strings.CutPrefix(path, "/fonts/"); ok && name != "" && !strings.Contains(name, "/") {
		return statusPathPrefix + slug + path, true
	}
	return "", false
}

// ownDomainOrigin is whether origin is the verified own domain of a help center or roadmap board of
// workspace ws, whose pages embed the chat; uuid.Nil stands for any workspace, as a CORS preflight names
// no channel.
func (s *Server) ownDomainOrigin(ctx context.Context, ws uuid.UUID, origin string) bool {
	if !s.statusDomainsOn() {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Port() != "" || origin != s.domainURL(u.Host, "") {
		return false
	}
	hit, ok := s.domainPage(ctx, u.Host)
	return ok && (hit.kind == kindHelp || hit.kind == kindRoadmap) && (ws == uuid.Nil() || hit.workspace == ws)
}

type ownDomainKey struct{}

type ownOn struct {
	kind            string
	host            string
	workspace, page uuid.UUID
}

func ownDomainOn(ctx context.Context, kind string) (ownOn, bool) {
	on, ok := ctx.Value(ownDomainKey{}).(ownOn)
	return on, ok && on.kind == kind
}

func stillOn(domain, state *string, host string) bool {
	return domain != nil && *domain == host && state != nil && *state == domainVerified
}

var ownDomainPrefixes = map[string]string{kindHelp: helpPathPrefix, kindRoadmap: roadmapPathPrefix, kindChangelog: changelogPathPrefix}

// serveOwnDomain serves a help center, roadmap board or changelog at / of its verified own domain: every
// path maps under the page's own prefix, so nothing else of the server answers there.
func (s *Server) serveOwnDomain(w http.ResponseWriter, r *http.Request, next http.Handler, host string, hit statusDomainHit) {
	prefix := ownDomainPrefixes[hit.kind]
	if r.URL.Path == "/robots.txt" {
		body := "User-agent: *\nAllow: /\n"
		if hit.kind != kindRoadmap {
			body += "Sitemap: " + s.domainURL(host, "/sitemap.xml") + "\n"
		}
		h := w.Header()
		h.Set("Content-Type", "text/plain; charset=utf-8")
		h.Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write([]byte(body))
		return
	}
	path := strings.TrimSuffix(prefix, "/") + "/" + hit.slug
	if r.URL.Path != "/" {
		path += r.URL.Path
	}
	r2 := r.Clone(context.WithValue(r.Context(), ownDomainKey{}, ownOn{kind: hit.kind, host: host, workspace: hit.workspace, page: hit.page}))
	r2.URL.Path = path
	r2.URL.RawPath = ""
	next.ServeHTTP(w, r2)
}

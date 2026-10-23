package api

import (
	"bytes"
	"context"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

func (s *Server) ssoSettingsBody(ctx context.Context, q *store.Queries, ws uuid.UUID) (oas.SsoSettings, error) {
	out := oas.SsoSettings{Domains: []oas.SsoDomain{}, JitRole: oas.SsoJitAgent, RedirectUri: s.ssoRedirectURI(), SamlSp: s.samlSPBody(ws)}
	set, err := q.GetSsoSettings(ctx, ws)
	if err != nil && !store.IsNotFound(err) {
		return out, err
	}
	if err == nil {
		out.SsoRequired, out.Jit, out.JitRole = set.SsoRequired, set.Jit, oas.SsoJitRole(set.JitRole)
		if set.Protocol != nil {
			out.Protocol = new(oas.SsoProtocol(*set.Protocol))
		}
		switch deref(set.Protocol) {
		case protocolOIDC:
			out.Oidc = &oas.SsoOidcProvider{
				Issuer: deref(set.OidcIssuer), ClientId: deref(set.OidcClientID), Scopes: set.OidcScopes, GroupsClaim: set.GroupsClaim,
				AllowedGroups: set.AllowedGroups, UpdatedAt: set.UpdatedAt,
			}
		case protocolSAML:
			certs := make([]oas.SsoSamlCertificate, 0, len(set.SamlCertificates))
			for _, der := range set.SamlCertificates {
				certs = append(certs, samlCertificateBody(der))
			}
			out.Saml = &oas.SsoSamlProvider{
				EntityId: deref(set.SamlEntityID), SsoUrl: deref(set.SamlSsoUrl), Certificates: certs, MetadataUrl: set.SamlMetadataUrl,
				MetadataFetchedAt: set.SamlMetadataAt, EmailAttribute: set.SamlEmailAttribute, GroupsAttribute: set.GroupsClaim,
				AllowedGroups: set.AllowedGroups, UpdatedAt: set.UpdatedAt,
			}
		}
	}
	rows, err := q.ListSsoDomains(ctx, ws)
	if err != nil {
		return out, err
	}
	for _, d := range rows {
		out.Domains = append(out.Domains, ssoDomainBody(d))
	}
	return out, nil
}

func ssoDomainBody(d store.SsoDomain) oas.SsoDomain {
	state, problem := oas.SsoDomainPending, d.Problem
	if d.VerifiedAt != nil {
		state, problem = oas.SsoDomainVerified, nil
	}
	return oas.SsoDomain{
		Id: d.ID, Domain: d.Domain, State: state, TxtName: ssoTXT + d.Domain, TxtValue: d.Token, Problem: problem,
		CheckedAt: d.CheckedAt, VerifiedAt: d.VerifiedAt, CreatedAt: d.CreatedAt,
	}
}

func (s *Server) GetSsoSettings(ctx context.Context, _ oas.GetSsoSettingsRequestObject) (oas.GetSsoSettingsResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireOwner(p); err != nil {
		return nil, err
	}
	out, err := s.ssoSettingsBody(ctx, s.st.Queries, p.workspaceID)
	if err != nil {
		return nil, err
	}
	return oas.GetSsoSettings200JSONResponse(out), nil
}

func (s *Server) checkIssuer(raw string) (string, error) {
	issuer := strings.TrimSpace(raw)
	u, err := url.Parse(issuer)
	if err != nil || len(issuer) > 500 || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" ||
		(u.Scheme != "https" && !(u.Scheme == "http" && s.webhooks.AllowPrivate)) {
		return "", errValidation("issuer must be an https URL without credentials, query or fragment")
	}
	return issuer, nil
}

func cleanList(in *[]string, max, maxLen int, name string, valid func(string) bool) ([]string, error) {
	out := []string{}
	if in == nil {
		return out, nil
	}
	if len(*in) > max {
		return nil, errValidation(name + " holds too many entries")
	}
	for _, v := range *in {
		v = strings.TrimSpace(v)
		if v == "" || len([]rune(v)) > maxLen || (valid != nil && !valid(v)) {
			return nil, errValidation(name + " holds an entry that is empty, too long or not allowed")
		}
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out, nil
}

func (s *Server) SetSsoOidcProvider(ctx context.Context, req oas.SetSsoOidcProviderRequestObject) (oas.SetSsoOidcProviderResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireOwner(p); err != nil {
		return nil, err
	}
	b := req.Body
	issuer, err := s.checkIssuer(b.Issuer)
	if err != nil {
		return nil, err
	}
	clientID := strings.TrimSpace(b.ClientId)
	if clientID == "" || len(clientID) > 500 {
		return nil, errValidation("client_id must be 1 to 500 characters")
	}
	var clientSecret *string
	if b.ClientSecret != nil {
		v := strings.TrimSpace(*b.ClientSecret)
		if v == "" || len(v) > 2000 {
			return nil, errValidation("client_secret must be 1 to 2000 characters")
		}
		clientSecret = &v
	}
	scopes, err := cleanList(b.Scopes, 10, 100, "scopes", func(v string) bool {
		return scopeTokenPattern.MatchString(v) && v != "openid" && v != "email" && v != "profile"
	})
	if err != nil {
		return nil, err
	}
	groups, err := cleanList(b.AllowedGroups, 50, 200, "allowed_groups", nil)
	if err != nil {
		return nil, err
	}
	groupsClaim := ssoDefaultGroups
	if b.GroupsClaim != nil {
		if v := strings.TrimSpace(*b.GroupsClaim); v != "" {
			if len(v) > 100 {
				return nil, errValidation("groups_claim is longer than 100 characters")
			}
			groupsClaim = v
		}
	}
	if _, err := s.oidcProvider(ctx, issuer, true); err != nil {
		return nil, errSSODiscovery(err.Error())
	}
	now := s.now()
	var out oas.SsoSettings
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.LockSsoSettings(ctx, store.LockSsoSettingsParams{WorkspaceID: p.workspaceID, ID: newID(), Now: now})
		if err != nil {
			return err
		}
		sealed := cur.OidcClientSecret
		if clientSecret != nil {
			sealed = s.secrets.Seal([]byte(*clientSecret), ssoSecretContext(cur.WorkspaceID, cur.ID))
		} else if sealed == nil || deref(cur.Protocol) != protocolOIDC {
			return errValidation("client_secret is required")
		}
		created := deref(cur.Protocol) != protocolOIDC
		var fields []string
		if deref(cur.OidcIssuer) != issuer {
			fields = append(fields, "issuer")
		}
		if deref(cur.OidcClientID) != clientID {
			fields = append(fields, "client_id")
		}
		if clientSecret != nil {
			fields = append(fields, "client_secret")
		}
		if !slices.Equal(cur.OidcScopes, scopes) {
			fields = append(fields, "scopes")
		}
		if cur.GroupsClaim != groupsClaim {
			fields = append(fields, "groups_claim")
		}
		if !slices.Equal(cur.AllowedGroups, groups) {
			fields = append(fields, "allowed_groups")
		}
		if !created && len(fields) == 0 {
			out, err = s.ssoSettingsBody(ctx, q, p.workspaceID)
			return err
		}
		if _, err := q.SaveSsoOidc(ctx, store.SaveSsoOidcParams{
			WorkspaceID: p.workspaceID, Issuer: &issuer, ClientID: &clientID, ClientSecret: sealed, Scopes: scopes,
			GroupsClaim: groupsClaim, AllowedGroups: groups, Now: now,
		}); err != nil {
			return err
		}
		details := auditDetails{"protocol": protocolOIDC, "issuer_host": urlHost(issuer)}
		action := oas.AuditSsoProviderCreated
		if !created {
			action, details["fields"] = oas.AuditSsoProviderUpdated, fields
		} else if cur.Protocol != nil {
			details["replaced"] = *cur.Protocol
		}
		if err := s.audit(ctx, q, p, action, oas.AuditTargetSso, cur.ID, details); err != nil {
			return err
		}
		out, err = s.ssoSettingsBody(ctx, q, p.workspaceID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.SetSsoOidcProvider200JSONResponse(out), nil
}

func (s *Server) DeleteSsoOidcProvider(ctx context.Context, _ oas.DeleteSsoOidcProviderRequestObject) (oas.DeleteSsoOidcProviderResponseObject, error) {
	if err := s.deleteSSOProvider(ctx, protocolOIDC); err != nil {
		return nil, err
	}
	return oas.DeleteSsoOidcProvider204Response{}, nil
}

func (s *Server) DeleteSsoSamlProvider(ctx context.Context, _ oas.DeleteSsoSamlProviderRequestObject) (oas.DeleteSsoSamlProviderResponseObject, error) {
	if err := s.deleteSSOProvider(ctx, protocolSAML); err != nil {
		return nil, err
	}
	return oas.DeleteSsoSamlProvider204Response{}, nil
}

func (s *Server) deleteSSOProvider(ctx context.Context, protocol string) error {
	p := principalFrom(ctx)
	if err := requireOwner(p); err != nil {
		return err
	}
	return s.st.InTx(ctx, func(q *store.Queries) error {
		set, err := q.ClearSsoProvider(ctx, store.ClearSsoProviderParams{WorkspaceID: p.workspaceID, Protocol: &protocol, Now: s.now()})
		if store.IsNotFound(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditSsoProviderDeleted, oas.AuditTargetSso, set.ID, auditDetails{"protocol": protocol})
	})
}

func (s *Server) SetSsoSamlProvider(ctx context.Context, req oas.SetSsoSamlProviderRequestObject) (oas.SetSsoSamlProviderResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireOwner(p); err != nil {
		return nil, err
	}
	if !s.samlCookieWorks() {
		return nil, errSAMLNeedsHTTPS
	}
	b := req.Body
	if b.MetadataUrl != nil && b.MetadataXml != nil {
		return nil, errValidation("give metadata_url or metadata_xml, not both")
	}
	var metadataURL *string
	if b.MetadataUrl != nil {
		v := strings.TrimSpace(*b.MetadataUrl)
		u, err := url.Parse(v)
		if err != nil || len(v) > 2000 || u.Host == "" || u.User != nil || u.Fragment != "" ||
			(u.Scheme != "https" && !(u.Scheme == "http" && s.webhooks.AllowPrivate)) {
			return nil, errValidation("metadata_url must be an https URL without credentials or fragment")
		}
		metadataURL = &v
	}
	var emailAttr *string
	if b.EmailAttribute != nil {
		if v := strings.TrimSpace(*b.EmailAttribute); v != "" {
			if len(v) > 200 {
				return nil, errValidation("email_attribute is longer than 200 characters")
			}
			emailAttr = &v
		}
	}
	groupsAttr := ssoDefaultGroups
	if b.GroupsAttribute != nil {
		if v := strings.TrimSpace(*b.GroupsAttribute); v != "" {
			if len(v) > 100 {
				return nil, errValidation("groups_attribute is longer than 100 characters")
			}
			groupsAttr = v
		}
	}
	groups, err := cleanList(b.AllowedGroups, 50, 200, "allowed_groups", nil)
	if err != nil {
		return nil, err
	}
	cur, err := s.st.GetSsoSettings(ctx, p.workspaceID)
	if err != nil && !store.IsNotFound(err) {
		return nil, err
	}
	keep := b.MetadataUrl == nil && b.MetadataXml == nil
	if keep {
		if deref(cur.Protocol) != protocolSAML {
			return nil, errValidation("metadata_url or metadata_xml is required")
		}
		metadataURL = cur.SamlMetadataUrl
	}
	var idp samlIdP
	var fetchedAt *time.Time
	switch {
	case metadataURL != nil:
		if idp, err = s.fetchIdPMetadata(ctx, *metadataURL); err != nil {
			return nil, errSSOMetadata(err.Error())
		}
		fetchedAt = new(s.now())
	case b.MetadataXml != nil:
		if idp, err = s.parseIdPMetadata([]byte(*b.MetadataXml)); err != nil {
			return nil, errSSOMetadata(err.Error())
		}
	default:
		idp = samlIdP{entityID: deref(cur.SamlEntityID), ssoURL: deref(cur.SamlSsoUrl), certs: cur.SamlCertificates}
	}
	now := s.now()
	var out oas.SsoSettings
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.LockSsoSettings(ctx, store.LockSsoSettingsParams{WorkspaceID: p.workspaceID, ID: newID(), Now: now})
		if err != nil {
			return err
		}
		if keep && deref(cur.Protocol) != protocolSAML {
			return errValidation("metadata_url or metadata_xml is required")
		}
		created := deref(cur.Protocol) != protocolSAML
		var fields []string
		if deref(cur.SamlEntityID) != idp.entityID {
			fields = append(fields, "entity_id")
		}
		if deref(cur.SamlSsoUrl) != idp.ssoURL {
			fields = append(fields, "sso_url")
		}
		if !slices.EqualFunc(cur.SamlCertificates, idp.certs, bytes.Equal) {
			fields = append(fields, "certificates")
		}
		if deref(cur.SamlMetadataUrl) != deref(metadataURL) {
			fields = append(fields, "metadata_url")
		}
		if deref(cur.SamlEmailAttribute) != deref(emailAttr) {
			fields = append(fields, "email_attribute")
		}
		if cur.GroupsClaim != groupsAttr {
			fields = append(fields, "groups_attribute")
		}
		if !slices.Equal(cur.AllowedGroups, groups) {
			fields = append(fields, "allowed_groups")
		}
		if fetchedAt == nil && metadataURL != nil {
			fetchedAt = cur.SamlMetadataAt
		}
		if _, err := q.SaveSsoSaml(ctx, store.SaveSsoSamlParams{
			WorkspaceID: p.workspaceID, EntityID: &idp.entityID, SsoUrl: &idp.ssoURL, Certificates: idp.certs, MetadataUrl: metadataURL,
			MetadataAt: fetchedAt, EmailAttribute: emailAttr, GroupsClaim: groupsAttr, AllowedGroups: groups, Now: now,
		}); err != nil {
			return err
		}
		if !created && len(fields) == 0 {
			out, err = s.ssoSettingsBody(ctx, q, p.workspaceID)
			return err
		}
		details := auditDetails{"protocol": protocolSAML, "entity_id": idp.entityID}
		if metadataURL != nil {
			details["metadata_host"] = urlHost(*metadataURL)
		}
		action := oas.AuditSsoProviderCreated
		if !created {
			action, details["fields"] = oas.AuditSsoProviderUpdated, fields
		} else if cur.Protocol != nil {
			details["replaced"] = *cur.Protocol
		}
		if err := s.audit(ctx, q, p, action, oas.AuditTargetSso, cur.ID, details); err != nil {
			return err
		}
		out, err = s.ssoSettingsBody(ctx, q, p.workspaceID)
		return err
	})
	if err != nil {
		return nil, err
	}
	if len(idp.warnings) > 0 {
		out.Warnings = &idp.warnings
	}
	return oas.SetSsoSamlProvider200JSONResponse(out), nil
}

func (s *Server) UpdateSsoSettings(ctx context.Context, req oas.UpdateSsoSettingsRequestObject) (oas.UpdateSsoSettingsResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireOwner(p); err != nil {
		return nil, err
	}
	b := req.Body
	if b.JitRole != nil && !b.JitRole.Valid() {
		return nil, errValidation("jit_role must be admin or agent")
	}
	now := s.now()
	var out oas.SsoSettings
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.LockSsoSettings(ctx, store.LockSsoSettingsParams{WorkspaceID: p.workspaceID, ID: newID(), Now: now})
		if err != nil {
			return err
		}
		next := store.SaveSsoRulesParams{WorkspaceID: p.workspaceID, SsoRequired: cur.SsoRequired, Jit: cur.Jit, JitRole: cur.JitRole, Now: now}
		if b.SsoRequired != nil {
			next.SsoRequired = *b.SsoRequired
		}
		if b.Jit != nil {
			next.Jit = *b.Jit
		}
		if b.JitRole != nil {
			next.JitRole = string(*b.JitRole)
		}
		if next.SsoRequired && !cur.SsoRequired {
			verified, err := q.HasVerifiedSsoDomain(ctx, p.workspaceID)
			if err != nil {
				return err
			}
			if cur.Protocol == nil || !verified {
				return errSSONotReady
			}
		}
		from, to := auditDetails{}, auditDetails{}
		var fields []string
		if next.SsoRequired != cur.SsoRequired {
			fields, from["sso_required"], to["sso_required"] = append(fields, "sso_required"), cur.SsoRequired, next.SsoRequired
		}
		if next.Jit != cur.Jit {
			fields, from["jit"], to["jit"] = append(fields, "jit"), cur.Jit, next.Jit
		}
		if next.JitRole != cur.JitRole {
			fields, from["jit_role"], to["jit_role"] = append(fields, "jit_role"), cur.JitRole, next.JitRole
		}
		if len(fields) > 0 {
			if _, err := q.SaveSsoRules(ctx, next); err != nil {
				return err
			}
			if err := s.audit(ctx, q, p, oas.AuditSsoSettingsChanged, oas.AuditTargetSso, cur.ID, auditDetails{"fields": fields, "from": from, "to": to}); err != nil {
				return err
			}
		}
		out, err = s.ssoSettingsBody(ctx, q, p.workspaceID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateSsoSettings200JSONResponse(out), nil
}

func (s *Server) AddSsoDomain(ctx context.Context, req oas.AddSsoDomainRequestObject) (oas.AddSsoDomainResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireOwner(p); err != nil {
		return nil, err
	}
	domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(req.Body.Domain)), ".")
	if !domainSyntax(domain) {
		return nil, errValidation("domain must be an e-mail domain such as example.com")
	}
	var out store.SsoDomain
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		if _, err := q.LockSsoSettings(ctx, store.LockSsoSettingsParams{WorkspaceID: p.workspaceID, ID: newID(), Now: s.now()}); err != nil {
			return err
		}
		n, err := q.CountSsoDomains(ctx, p.workspaceID)
		if err != nil {
			return err
		}
		if n >= ssoDomainLimit {
			return errSSODomainLimit
		}
		taken, err := q.SsoDomainVerifiedElsewhere(ctx, store.SsoDomainVerifiedElsewhereParams{Domain: domain, WorkspaceID: p.workspaceID})
		if err != nil {
			return err
		}
		if taken {
			return errSSODomainTaken
		}
		out, err = q.AddSsoDomain(ctx, store.AddSsoDomainParams{ID: newID(), WorkspaceID: p.workspaceID, Domain: domain, Token: newDomainToken(), Now: s.now()})
		if store.IsUniqueViolation(err) {
			return errSSODomainExists
		}
		if err != nil {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditSsoDomainAdded, oas.AuditTargetSsoDomain, out.ID, auditDetails{"domain": domain})
	})
	if err != nil {
		return nil, err
	}
	return oas.AddSsoDomain201JSONResponse(ssoDomainBody(out)), nil
}

func (s *Server) lookupSSODomain(ctx context.Context, domain, token string) (bool, string) {
	r := s.status.Resolver
	if r == nil {
		r = net.DefaultResolver
	}
	ctx, cancel := context.WithTimeout(ctx, statusDomainLookup)
	defer cancel()
	name := ssoTXT + domain
	txts, err := r.LookupTXT(ctx, name)
	switch {
	case err != nil && !dnsMissing(err):
		return false, "DNS lookup of " + name + " failed: " + err.Error()
	case slices.ContainsFunc(txts, func(t string) bool { return strings.TrimSpace(t) == token }):
		return true, ""
	case len(txts) > 0:
		return false, "the TXT record " + name + " does not hold the verification value"
	}
	return false, "no TXT record " + name
}

func (s *Server) VerifySsoDomain(ctx context.Context, req oas.VerifySsoDomainRequestObject) (oas.VerifySsoDomainResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireOwner(p); err != nil {
		return nil, err
	}
	d, err := s.st.GetSsoDomain(ctx, store.GetSsoDomainParams{WorkspaceID: p.workspaceID, ID: req.SsoDomainId})
	if store.IsNotFound(err) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	if d.VerifiedAt != nil {
		return oas.VerifySsoDomain200JSONResponse(ssoDomainBody(d)), nil
	}
	seen, problem := s.lookupSSODomain(ctx, d.Domain, d.Token)
	now := s.now()
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		if seen {
			taken, err := q.SsoDomainVerifiedElsewhere(ctx, store.SsoDomainVerifiedElsewhereParams{Domain: d.Domain, WorkspaceID: p.workspaceID})
			if err != nil {
				return err
			}
			if taken {
				return errSSODomainTaken
			}
		}
		var prob *string
		if !seen {
			prob = &problem
		}
		d, err = q.MarkSsoDomainChecked(ctx, store.MarkSsoDomainCheckedParams{WorkspaceID: p.workspaceID, ID: d.ID, Now: now, Problem: prob, Verified: seen})
		if store.IsUniqueViolation(err) {
			return errSSODomainTaken
		}
		if store.IsNotFound(err) {
			return errNotFound
		}
		if err != nil || !seen {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditSsoDomainVerified, oas.AuditTargetSsoDomain, d.ID, auditDetails{"domain": d.Domain})
	})
	if err != nil {
		return nil, err
	}
	return oas.VerifySsoDomain200JSONResponse(ssoDomainBody(d)), nil
}

func (s *Server) DeleteSsoDomain(ctx context.Context, req oas.DeleteSsoDomainRequestObject) (oas.DeleteSsoDomainResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireOwner(p); err != nil {
		return nil, err
	}
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		d, err := q.DeleteSsoDomain(ctx, store.DeleteSsoDomainParams{WorkspaceID: p.workspaceID, ID: req.SsoDomainId})
		if store.IsNotFound(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if err := q.RelaxSsoWithoutDomains(ctx, store.RelaxSsoWithoutDomainsParams{WorkspaceID: p.workspaceID, Now: s.now()}); err != nil {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditSsoDomainDeleted, oas.AuditTargetSsoDomain, d.ID, auditDetails{"domain": d.Domain})
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteSsoDomain204Response{}, nil
}

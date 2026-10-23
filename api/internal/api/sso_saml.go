package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/crewjam/saml"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
	"github.com/productdevbook/yuva/api/internal/webhook"
)

const (
	samlCookie          = "yuva_saml"
	samlPathPrefix      = "/auth/saml/"
	samlACSPath         = "/auth/saml/acs"
	samlMetadataPath    = "/auth/saml/metadata/"
	samlRefreshAfter    = time.Hour
	samlRetryAfter      = 15 * time.Minute
	samlMaxResponse     = 256 << 10
	samlMaxMetadata     = 1 << 20
	samlMaxCertificates = 10
	protocolSAML        = string(oas.SsoProtocolSaml)
	samlNSAssertion     = "urn:oasis:names:tc:SAML:2.0:assertion"
	samlNSProtocol      = "urn:oasis:names:tc:SAML:2.0:protocol"
	samlBearer          = "urn:oasis:names:tc:SAML:2.0:cm:bearer"
)

var errSAMLNeedsHTTPS = problem(http.StatusBadRequest, "saml_requires_https",
	"SAML needs the server on https (YUVA_PUBLIC_URL): the identity provider's cross-site POST carries the sign-in cookie only when it is SameSite=None and Secure, which browsers keep on plain http only for localhost")

func errSSOMetadata(detail string) *apiError {
	return problem(http.StatusBadRequest, "sso_metadata_failed", "the identity provider's metadata could not be used: "+truncateRunes(detail, 300))
}

type samlIdP struct {
	entityID string
	ssoURL   string
	certs    [][]byte
	warnings []string
}

// safeXML refuses documents with a DTD or any other directive, so no entity is ever declared or
// expanded, and anything that is not well-formed.
func safeXML(raw []byte) error {
	d := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := d.RawToken()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.Directive:
			return errors.New("the document holds a DTD or another directive")
		case xml.ProcInst:
			if t.Target != "xml" {
				return errors.New("the document holds a processing instruction")
			}
		}
	}
}

func (s *Server) parseIdPMetadata(raw []byte) (samlIdP, error) {
	var out samlIdP
	if len(raw) > samlMaxMetadata {
		return out, errors.New("the metadata is larger than 1 MiB")
	}
	if err := safeXML(raw); err != nil {
		return out, err
	}
	now, current := s.now(), false
	var md saml.EntityDescriptor
	if err := xml.Unmarshal(raw, &md); err != nil {
		return out, errors.New("the document is not a SAML EntityDescriptor")
	}
	out.entityID = strings.TrimSpace(md.EntityID)
	if out.entityID == "" || len(out.entityID) > 1000 {
		return out, errors.New("the entityID is missing or longer than 1000 characters")
	}
	for _, d := range md.IDPSSODescriptors {
		for _, e := range d.SingleSignOnServices {
			if e.Binding == saml.HTTPRedirectBinding && out.ssoURL == "" {
				out.ssoURL = strings.TrimSpace(e.Location)
			}
		}
		for _, k := range d.KeyDescriptors {
			if k.Use != "" && k.Use != "signing" {
				continue
			}
			for _, c := range k.KeyInfo.X509Data.X509Certificates {
				der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(c.Data), ""))
				if err != nil {
					return out, errors.New("a signing certificate is not base64")
				}
				cert, err := x509.ParseCertificate(der)
				if err != nil {
					return out, fmt.Errorf("a signing certificate cannot be read: %v", err)
				}
				if slices.ContainsFunc(out.certs, func(b []byte) bool { return bytes.Equal(b, der) }) {
					continue
				}
				if now.After(cert.NotAfter) {
					out.warnings = append(out.warnings, fmt.Sprintf("the signing certificate %s (SHA-256 %s) expired on %s and was left out",
						cert.Subject, samlCertificateBody(der).Sha256, cert.NotAfter.UTC().Format(time.RFC3339)))
					continue
				}
				if !now.Before(cert.NotBefore) {
					current = true
				}
				out.certs = append(out.certs, der)
			}
		}
	}
	if len(md.IDPSSODescriptors) == 0 {
		return out, errors.New("the metadata holds no IDPSSODescriptor")
	}
	if out.ssoURL == "" {
		return out, errors.New("the metadata holds no SingleSignOnService with the HTTP-Redirect binding")
	}
	u, err := url.Parse(out.ssoURL)
	if err != nil || len(out.ssoURL) > 2000 || u.Host == "" || u.User != nil || u.Fragment != "" ||
		(u.Scheme != "https" && !(u.Scheme == "http" && s.webhooks.AllowPrivate)) {
		return out, errors.New("the single sign-on URL must be an https URL")
	}
	if len(out.certs) == 0 && len(out.warnings) == 0 {
		return out, errors.New("the metadata holds no signing certificate")
	}
	if !current {
		return out, errors.New("the metadata holds no signing certificate that is valid now")
	}
	if len(out.certs) > samlMaxCertificates {
		return out, errors.New("the metadata holds more than 10 signing certificates")
	}
	return out, nil
}

func (s *Server) fetchIdPMetadata(ctx context.Context, raw string) (samlIdP, error) {
	if _, err := webhook.CheckURL(raw, s.webhooks.AllowPrivate); err != nil {
		return samlIdP{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, ssoHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return samlIdP{}, err
	}
	req.Header.Set("Accept", "application/samlmetadata+xml, application/xml, text/xml")
	res, err := s.sso.http.Do(req)
	if err != nil {
		return samlIdP{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return samlIdP{}, fmt.Errorf("the metadata URL answered %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, samlMaxMetadata+1))
	if err != nil {
		return samlIdP{}, err
	}
	return s.parseIdPMetadata(body)
}

func (s *Server) samlACSURL() string {
	return strings.TrimRight(s.auth.PublicURL, "/") + samlACSPath
}

func (s *Server) samlEntityID(ws uuid.UUID) string {
	return strings.TrimRight(s.auth.PublicURL, "/") + samlMetadataPath + ws.String()
}

func (s *Server) samlSPBody(ws uuid.UUID) oas.SsoSamlServiceProvider {
	id := s.samlEntityID(ws)
	return oas.SsoSamlServiceProvider{EntityId: id, MetadataUrl: id, AcsUrl: s.samlACSURL(), NameIdFormat: string(saml.EmailAddressNameIDFormat)}
}

func samlCertificateBody(der []byte) oas.SsoSamlCertificate {
	sum := sha256.Sum256(der)
	hex := make([]string, len(sum))
	for i, b := range sum {
		hex[i] = fmt.Sprintf("%02X", b)
	}
	out := oas.SsoSamlCertificate{Sha256: strings.Join(hex, ":")}
	if c, err := x509.ParseCertificate(der); err == nil {
		out.Subject, out.NotBefore, out.NotAfter = c.Subject.String(), c.NotBefore, c.NotAfter
	}
	return out
}

func (s *Server) samlServiceProvider(set store.SsoSetting) (*saml.ServiceProvider, error) {
	acs, err := url.Parse(s.samlACSURL())
	if err != nil {
		return nil, err
	}
	certs := make([]saml.X509Certificate, 0, len(set.SamlCertificates))
	for _, der := range set.SamlCertificates {
		certs = append(certs, saml.X509Certificate{Data: base64.StdEncoding.EncodeToString(der)})
	}
	entityID := s.samlEntityID(set.WorkspaceID)
	sp := &saml.ServiceProvider{
		EntityID: entityID, AcsURL: *acs, AuthnNameIDFormat: saml.EmailAddressNameIDFormat,
		IDPMetadata: &saml.EntityDescriptor{
			EntityID: deref(set.SamlEntityID),
			IDPSSODescriptors: []saml.IDPSSODescriptor{{
				SSODescriptor: saml.SSODescriptor{RoleDescriptor: saml.RoleDescriptor{
					KeyDescriptors: []saml.KeyDescriptor{{Use: "signing", KeyInfo: saml.KeyInfo{X509Data: saml.X509Data{X509Certificates: certs}}}},
				}},
				SingleSignOnServices: []saml.Endpoint{{Binding: saml.HTTPRedirectBinding, Location: deref(set.SamlSsoUrl)}},
			}},
		},
		ValidateAudienceRestriction: func(a *saml.Assertion) error {
			if a.Conditions == nil || !slices.ContainsFunc(a.Conditions.AudienceRestrictions, func(r saml.AudienceRestriction) bool {
				return r.Audience.Value == entityID
			}) {
				return errors.New("the assertion's audience is not this workspace's entity id")
			}
			return nil
		},
	}
	return sp, nil
}

// samlRefresh reads metadata kept by URL again when the copy is older than an hour, so rotated
// certificates are used; a failure keeps the stored copy.
func (s *Server) samlRefresh(ctx context.Context, set store.SsoSetting) store.SsoSetting {
	now := s.now()
	if set.SamlMetadataUrl == nil || (set.SamlMetadataAt != nil && now.Sub(*set.SamlMetadataAt) < samlRefreshAfter) ||
		(set.SamlMetadataTriedAt != nil && now.Sub(*set.SamlMetadataTriedAt) < samlRetryAfter) {
		return set
	}
	idp, err := s.fetchIdPMetadata(ctx, *set.SamlMetadataUrl)
	if err != nil {
		s.log.WarnContext(ctx, "saml metadata refresh", "workspace", set.WorkspaceID, "error", err)
		if err := s.st.MarkSamlMetadataTried(ctx, store.MarkSamlMetadataTriedParams{
			WorkspaceID: set.WorkspaceID, ID: set.ID, MetadataUrl: set.SamlMetadataUrl, Now: &now,
		}); err != nil {
			s.log.WarnContext(ctx, "saml metadata refresh", "workspace", set.WorkspaceID, "error", err)
		}
		return set
	}
	var fields []string
	if deref(set.SamlEntityID) != idp.entityID {
		fields = append(fields, "entity_id")
	}
	if deref(set.SamlSsoUrl) != idp.ssoURL {
		fields = append(fields, "sso_url")
	}
	if !slices.EqualFunc(set.SamlCertificates, idp.certs, bytes.Equal) {
		fields = append(fields, "certificates")
	}
	next := set
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		var err error
		next, err = q.RefreshSsoSamlMetadata(ctx, store.RefreshSsoSamlMetadataParams{
			EntityID: &idp.entityID, SsoUrl: &idp.ssoURL, Certificates: idp.certs, Now: &now, Changed: len(fields) > 0,
			WorkspaceID: set.WorkspaceID, ID: set.ID, MetadataUrl: set.SamlMetadataUrl,
		})
		if err != nil || len(fields) == 0 {
			return err
		}
		return writeSystemAudit(ctx, q, set.WorkspaceID, now, oas.AuditSsoProviderUpdated, oas.AuditTargetSso, set.ID, auditDetails{
			"protocol": protocolSAML, "entity_id": idp.entityID, "metadata_host": urlHost(*set.SamlMetadataUrl), "fields": fields, "via": "metadata_refresh",
		})
	})
	if err != nil {
		if !store.IsNotFound(err) {
			s.log.WarnContext(ctx, "saml metadata refresh", "workspace", set.WorkspaceID, "error", err)
		}
		return set
	}
	return next
}

// samlCookie must reach the identity provider's cross-site form POST, so it is SameSite=None,
// which browsers accept only with Secure: https, or http on localhost.
func (s *Server) samlCookie(value string, maxAge int) string {
	c := http.Cookie{Name: samlCookie, Value: value, Path: samlACSPath, MaxAge: maxAge, HttpOnly: true, Secure: true, SameSite: http.SameSiteNoneMode}
	return c.String()
}

func (s *Server) samlCookieWorks() bool {
	u, err := url.Parse(s.auth.PublicURL)
	if err != nil {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	h := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	ip, err := netip.ParseAddr(h)
	return err == nil && ip.IsLoopback()
}

func (s *Server) startSAML(ctx context.Context, set store.SsoSetting, returnTo string) (string, string, error) {
	set = s.samlRefresh(ctx, set)
	sp, err := s.samlServiceProvider(set)
	if err != nil {
		return "", "", err
	}
	req, err := sp.MakeAuthenticationRequest(deref(set.SamlSsoUrl), saml.HTTPRedirectBinding, saml.HTTPPostBinding)
	if err != nil {
		return "", "", err
	}
	target, err := req.Redirect("", sp)
	if err != nil {
		return "", "", err
	}
	now := s.now()
	if err := s.st.DeleteExpiredSsoLogins(ctx, now); err != nil {
		return "", "", err
	}
	if err := s.st.CreateSsoLogin(ctx, store.CreateSsoLoginParams{
		StateHash: hashSecret(req.ID), WorkspaceID: set.WorkspaceID, SettingsID: set.ID, ReturnTo: returnTo, Now: now, ExpiresAt: now.Add(ssoLoginTTL),
	}); err != nil {
		return "", "", err
	}
	return target.String(), s.samlCookie(req.ID, int(ssoLoginTTL/time.Second)), nil
}

// samlShape is the unverified response's structure, checked before the signature: one plain
// assertion with a subject whose confirmations all carry data, and conditions with an audience.
type samlConfirmation struct {
	Method string    `xml:",attr"`
	Data   *struct{} `xml:"urn:oasis:names:tc:SAML:2.0:assertion SubjectConfirmationData"`
}

type samlShape struct {
	XMLName      xml.Name
	InResponseTo string `xml:",attr"`
	Destination  string `xml:",attr"`
	Assertions   []struct {
		Subject *struct {
			Confirmations []samlConfirmation `xml:"urn:oasis:names:tc:SAML:2.0:assertion SubjectConfirmation"`
		} `xml:"urn:oasis:names:tc:SAML:2.0:assertion Subject"`
		Conditions *struct {
			Audiences []struct{} `xml:"urn:oasis:names:tc:SAML:2.0:assertion AudienceRestriction"`
		} `xml:"urn:oasis:names:tc:SAML:2.0:assertion Conditions"`
	} `xml:"urn:oasis:names:tc:SAML:2.0:assertion Assertion"`
	Encrypted []struct{} `xml:"urn:oasis:names:tc:SAML:2.0:assertion EncryptedAssertion"`
}

func (sh samlShape) problem(acs string) string {
	switch {
	case sh.XMLName.Space != samlNSProtocol || sh.XMLName.Local != "Response":
		return "the document is not a SAML Response"
	case sh.Destination != acs:
		return "the response's Destination is not this server's ACS URL"
	case len(sh.Encrypted) > 0:
		return "encrypted assertions are not accepted"
	case len(sh.Assertions) != 1:
		return "the response must hold exactly one assertion"
	}
	a := sh.Assertions[0]
	if a.Subject == nil || a.Conditions == nil || len(a.Conditions.Audiences) == 0 {
		return "the assertion lacks a Subject, Conditions or an AudienceRestriction"
	}
	if len(a.Subject.Confirmations) == 0 || slices.ContainsFunc(a.Subject.Confirmations, func(c samlConfirmation) bool {
		return c.Method != samlBearer || c.Data == nil
	}) {
		return "the assertion's subject must be confirmed by bearer with SubjectConfirmationData"
	}
	return ""
}

func (s *Server) serveSAMLACS(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	panel := strings.TrimRight(s.auth.PublicURL, "/")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Add("Set-Cookie", s.samlCookie("", -1))
	fail := func(code string) {
		http.Redirect(w, r, panel+"/sign-in?"+url.Values{"sso_error": {code}}.Encode(), http.StatusSeeOther)
	}
	if err := s.rateLimit(ctx, rateCheck{"sso-callback:" + rateIP(s.clientIP(r)), limitSSOStartPerIP}); err != nil {
		fail(ssoFailFailed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, samlMaxResponse)
	if err := r.ParseForm(); err != nil {
		fail(ssoFailExpired)
		return
	}
	doc, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(r.PostForm.Get("SAMLResponse")), ""))
	if err != nil || len(doc) == 0 || safeXML(doc) != nil {
		fail(ssoFailExpired)
		return
	}
	var shape samlShape
	if err := xml.Unmarshal(doc, &shape); err != nil || shape.InResponseTo == "" {
		s.log.InfoContext(ctx, "saml response refused: not a response to a sign-in this server started")
		fail(ssoFailExpired)
		return
	}
	c, err := r.Cookie(samlCookie)
	if err != nil || subtle.ConstantTimeCompare([]byte(c.Value), []byte(shape.InResponseTo)) != 1 {
		fail(ssoFailExpired)
		return
	}
	login, err := s.st.TakeSsoLogin(ctx, hashSecret(shape.InResponseTo))
	if err != nil || !login.ExpiresAt.After(s.now()) {
		if err != nil && !store.IsNotFound(err) {
			s.log.ErrorContext(ctx, "saml acs", "error", err)
		}
		fail(ssoFailExpired)
		return
	}
	cookie, code, err := s.finishSAML(ctx, login, shape, doc)
	if err != nil {
		s.log.ErrorContext(ctx, "saml acs", "workspace", login.WorkspaceID, "error", err)
		fail(ssoFailFailed)
		return
	}
	if code != "" {
		fail(code)
		return
	}
	w.Header().Add("Set-Cookie", cookie)
	http.Redirect(w, r, panel+login.ReturnTo, http.StatusSeeOther)
}

func samlAttribute(a *saml.Assertion, name string) []string {
	var out []string
	for _, st := range a.AttributeStatements {
		for _, at := range st.Attributes {
			if at.Name != name && at.FriendlyName != name {
				continue
			}
			for _, v := range at.Values {
				if t := strings.TrimSpace(v.Value); t != "" {
					out = append(out, t)
				}
			}
		}
	}
	return out
}

var samlNameAttributes = []string{"displayName", "name", "http://schemas.microsoft.com/identity/claims/displayname"}

func (s *Server) finishSAML(ctx context.Context, login store.SsoLogin, shape samlShape, doc []byte) (string, string, error) {
	set, err := s.st.GetSsoSettings(ctx, login.WorkspaceID)
	if store.IsNotFound(err) || (err == nil && (set.ID != login.SettingsID || deref(set.Protocol) != protocolSAML)) {
		return "", ssoFailExpired, nil
	}
	if err != nil {
		return "", "", err
	}
	sp, err := s.samlServiceProvider(set)
	if err != nil {
		return "", "", err
	}
	if p := shape.problem(sp.AcsURL.String()); p != "" {
		return s.refuseSSO(ctx, set, "", ssoFailInvalidToken, p)
	}
	assertion, err := sp.ParseXMLResponse(doc, []string{shape.InResponseTo}, sp.AcsURL)
	if err != nil {
		var bad saml.ErrBadStatus
		var inv *saml.InvalidResponseError
		if errors.As(err, &inv) && inv.PrivateErr != nil {
			err = inv.PrivateErr
		}
		if errors.As(err, &bad) {
			return "", ssoFailDenied, nil
		}
		return s.refuseSSO(ctx, set, "", ssoFailInvalidToken, err.Error())
	}
	if assertion.ID == "" {
		return s.refuseSSO(ctx, set, "", ssoFailInvalidToken, "the assertion has no ID")
	}
	now := s.now()
	keep := assertion.IssueInstant.Add(saml.MaxIssueDelay)
	if assertion.Conditions != nil && assertion.Conditions.NotOnOrAfter.After(keep) {
		keep = assertion.Conditions.NotOnOrAfter
	}
	for _, sc := range assertion.Subject.SubjectConfirmations {
		if sc.SubjectConfirmationData != nil && sc.SubjectConfirmationData.NotOnOrAfter.After(keep) {
			keep = sc.SubjectConfirmationData.NotOnOrAfter
		}
	}
	keep = keep.Add(saml.MaxClockSkew)
	if keep.Before(now.Add(ssoLoginTTL)) {
		keep = now.Add(ssoLoginTTL)
	}
	if err := s.st.DeleteExpiredSamlAssertions(ctx, store.DeleteExpiredSamlAssertionsParams{WorkspaceID: set.WorkspaceID, Now: now}); err != nil {
		return "", "", err
	}
	n, err := s.st.RecordSamlAssertion(ctx, store.RecordSamlAssertionParams{WorkspaceID: set.WorkspaceID, IDHash: hashSecret(assertion.ID), ExpiresAt: keep})
	if err != nil {
		return "", "", err
	}
	if n == 0 {
		return s.refuseSSO(ctx, set, "", ssoFailInvalidToken, "the assertion was used before")
	}
	var rawEmail string
	if set.SamlEmailAttribute != nil {
		if v := samlAttribute(assertion, *set.SamlEmailAttribute); len(v) > 0 {
			rawEmail = v[0]
		}
	} else if assertion.Subject != nil && assertion.Subject.NameID != nil {
		rawEmail = strings.TrimSpace(assertion.Subject.NameID.Value)
	}
	email, err := normalizeEmail(oas.Email(rawEmail))
	if err != nil {
		return s.refuseSSO(ctx, set, "", ssoFailUnverified, "the assertion holds no valid e-mail address")
	}
	var name string
	for _, a := range samlNameAttributes {
		if v := samlAttribute(assertion, a); len(v) > 0 {
			name = v[0]
			break
		}
	}
	return s.completeSSO(ctx, set, ssoIdentity{email: email, name: name, groups: samlAttribute(assertion, set.GroupsClaim)})
}

func (s *Server) serveSAMLMetadata(w http.ResponseWriter, r *http.Request) {
	ws, err := uuid.Parse(r.PathValue("workspace"))
	if err != nil {
		writeProblem(w, errNotFound)
		return
	}
	row, err := s.st.GetWorkspace(r.Context(), ws)
	if err != nil || row.DeletedAt != nil {
		if err != nil && !store.IsNotFound(err) {
			s.log.ErrorContext(r.Context(), "saml metadata", "error", err)
		}
		writeProblem(w, errNotFound)
		return
	}
	sp, err := s.samlServiceProvider(store.SsoSetting{WorkspaceID: ws})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	md := sp.Metadata()
	md.SPSSODescriptors[0].AssertionConsumerServices = md.SPSSODescriptors[0].AssertionConsumerServices[:1]
	out, err := xml.MarshalIndent(md, "", "  ")
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(out)
}

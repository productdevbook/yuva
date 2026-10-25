package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/mail/design"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	designSnapshotEvery = 10 * time.Minute
	designVersionsKept  = 50
	sampleContactName   = "Ada Lovelace"
	sampleContactEmail  = "ada@example.com"
)

var (
	errDesignGone        = problem(http.StatusNotFound, "not_found", "no such e-mail design")
	errDesignVersionGone = problem(http.StatusNotFound, "not_found", "no such snapshot of the design")
	limitRender          = limit{120, time.Minute}
)

func designProblem(status int, code, detail string, issues []design.Issue) *apiError {
	e := problem(status, code, detail)
	if issues == nil {
		issues = []design.Issue{}
	}
	e.Extra = map[string]any{"errors": issues}
	return e
}

// issuesProblem is 422 invalid_design, or the one special code every issue shares.
func issuesProblem(issues []design.Issue) *apiError {
	code := "invalid_design"
	for _, special := range []string{"invalid_rich_text", "unknown_merge_field", "foreign_asset"} {
		all := true
		for _, i := range issues {
			all = all && i.Code == special
		}
		if all {
			code = special
		}
	}
	n := strconv.Itoa(len(issues))
	return designProblem(http.StatusUnprocessableEntity, code, "the design has "+n+" problem(s); see errors", issues)
}

// readDocument parses, upgrades and checks a document, and that its images are the workspace's own.
func (s *Server) readDocument(ctx context.Context, q *store.Queries, ws uuid.UUID, raw []byte, opt design.Options) (design.Document, error) {
	d, issues, err := design.Parse(raw)
	switch {
	case errors.Is(err, design.ErrTooLarge):
		return d, designProblem(http.StatusRequestEntityTooLarge, "design_too_large", "a design holds at most 200 blocks in 300 KiB of JSON, 1 MiB with its translations", nil)
	case errors.Is(err, design.ErrUnsupportedSchema):
		return d, designProblem(http.StatusUnprocessableEntity, "unsupported_schema", "the design was made by a newer Yuva; this server knows schema "+strconv.Itoa(design.Schema), nil)
	case err != nil:
		return d, err
	}
	if len(issues) == 0 {
		issues = design.Validate(d, opt)
	}
	if len(issues) == 0 {
		if issues, err = foreignImages(ctx, q, ws, d); err != nil {
			return d, err
		}
	}
	if len(issues) > 0 {
		return d, issuesProblem(issues)
	}
	return d, nil
}

func foreignImages(ctx context.Context, q *store.Queries, ws uuid.UUID, d design.Document) ([]design.Issue, error) {
	ids := d.ImageIDs()
	if len(ids) == 0 {
		return nil, nil
	}
	parsed := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if u, err := uuid.Parse(id); err == nil {
			parsed = append(parsed, u)
		}
	}
	rows, err := q.ListEmailImagesByIDs(ctx, store.ListEmailImagesByIDsParams{WorkspaceID: ws, Ids: parsed})
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, r := range rows {
		have[r.ID.String()] = true
	}
	var issues []design.Issue
	d.Walk(func(b design.Block) {
		if b.Type == "image" && b.ImageID != "" && !have[strings.ToLower(b.ImageID)] {
			issues = append(issues, design.Issue{BlockID: b.ID, Field: "image_id", Code: "foreign_asset", Detail: "not an image of this workspace"})
		}
	})
	return issues, nil
}

func requestDocument(doc oas.EmailDesignDocument) []byte {
	b, err := json.Marshal(doc)
	if err != nil {
		return nil
	}
	return b
}

func documentBody(d design.Document) (oas.EmailDesignDocument, error) {
	var out oas.EmailDesignDocument
	err := json.Unmarshal(design.Marshal(d), &out)
	return out, err
}

func blankDocument() design.Document {
	return design.Document{Schema: design.Schema, Blocks: []design.Block{}, Theme: design.Theme{
		Width: 600, Font: "system", Radius: 8,
		Colors: design.Colors{Page: "#f4f4f5", Card: "#ffffff", Text: "#18181b", Muted: "#5f5f68", Accent: "#2563eb", Link: "#1d4ed8", Border: "#e4e4e7"},
	}}
}

func mailLang(locale string) string {
	base, _, _ := strings.Cut(strings.ToLower(strings.ReplaceAll(locale, "_", "-")), "-")
	switch base {
	case "tr", "de":
		return base
	}
	return "en"
}

func (s *Server) imageURL(token string) string { return s.linkBase() + "/i/" + token }

func (s *Server) iconURL(network, variant string) string {
	return s.linkBase() + "/i/s/" + network + "-" + variant + ".png"
}

// renderScope is what a render knows besides the document: the inbox for the footer and the
// language, the list it names, and whose values fill the merge fields.
type renderScope struct {
	inbox   *store.Inbox
	list    *store.MailingList
	values  *design.Values
	subject string
	pre     string
	dark    bool
	// version is the language version to render; "" for a design without translations.
	version string
}

func (s *Server) renderDesign(ctx context.Context, q *store.Queries, ws uuid.UUID, d design.Document, sc renderScope) (design.Output, error) {
	doc := d
	if sc.version != "" {
		doc = d.Localized(sc.version)
	}
	in := design.Input{Doc: doc, Subject: sc.subject, Preheader: doc.Preheader, Lang: "en", Dark: sc.dark, Icon: s.iconURL}
	if sc.pre != "" {
		in.Preheader = sc.pre
	}
	v := design.Values{ContactName: sampleContactName, ContactEmail: sampleContactEmail}
	if sc.values != nil {
		v = *sc.values
	}
	if sc.inbox != nil {
		in.Lang = mailLang(sc.inbox.DefaultLocale)
		v.InboxName = sc.inbox.Name
		in.Sender = design.Sender{LegalName: deref(sc.inbox.SenderLegalName), PostalAddress: deref(sc.inbox.SenderPostalAddress),
			Registration: deref(sc.inbox.SenderRegistration), Contact: deref(sc.inbox.SenderContact)}
	}
	if sc.version != "" {
		in.Lang, _ = design.FooterLang(sc.version)
		in.HTMLLang = sc.version
	}
	if sc.list != nil {
		v.ListName = sc.list.Name
	}
	v.UnsubscribeURL = s.linkBase() + "/m/example"
	v.PreferencesURL = v.UnsubscribeURL
	in.Values = v
	img, err := s.designImages(ctx, q, ws, d)
	if err != nil {
		return design.Output{}, err
	}
	in.Image = img
	out, err := design.Render(in)
	if sc.version != "" {
		out.Warnings = append(out.Warnings, d.VersionIssues(sc.version)...)
	}
	if sc.inbox == nil {
		kept := out.Warnings[:0]
		for _, w := range out.Warnings {
			if w.Code != "sender_identity_missing" {
				kept = append(kept, w)
			}
		}
		out.Warnings = kept
	}
	return out, err
}

// designImages looks up the workspace's images a document uses, for the renderer.
func (s *Server) designImages(ctx context.Context, q *store.Queries, ws uuid.UUID, d design.Document) (func(design.Block) (design.Image, bool), error) {
	images := map[string]store.EmailImage{}
	if ids := d.ImageIDs(); len(ids) > 0 {
		var parsed []uuid.UUID
		for _, id := range ids {
			if u, err := uuid.Parse(id); err == nil {
				parsed = append(parsed, u)
			}
		}
		rows, err := q.ListEmailImagesByIDs(ctx, store.ListEmailImagesByIDsParams{WorkspaceID: ws, Ids: parsed})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			images[r.ID.String()] = r
		}
	}
	return func(b design.Block) (design.Image, bool) {
		img, ok := images[strings.ToLower(b.ImageID)]
		if !ok {
			return design.Image{}, false
		}
		return design.Image{URL: s.imageURL(img.Token), Width: int(img.Width), Height: int(img.Height)}, true
	}, nil
}

func issueBodies(is []design.Issue) []oas.EmailDesignIssue {
	out := make([]oas.EmailDesignIssue, len(is))
	for i, x := range is {
		out[i] = oas.EmailDesignIssue{Code: x.Code}
		if x.BlockID != "" {
			out[i].BlockId = new(x.BlockID)
		}
		if x.Field != "" {
			out[i].Field = new(x.Field)
		}
		if x.Detail != "" {
			out[i].Detail = new(x.Detail)
		}
	}
	return out
}

func (s *Server) designBody(ctx context.Context, q *store.Queries, row store.EmailDesign) (oas.EmailDesign, error) {
	d, issues, err := design.Parse(row.Document)
	if err != nil {
		return oas.EmailDesign{}, err
	}
	if len(issues) > 0 {
		return oas.EmailDesign{}, errors.New("stored design does not parse")
	}
	doc, err := documentBody(d)
	if err != nil {
		return oas.EmailDesign{}, err
	}
	sc := renderScope{}
	if row.InboxID != nil {
		in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: row.WorkspaceID, ID: *row.InboxID})
		if err != nil && !store.IsNotFound(err) {
			return oas.EmailDesign{}, err
		}
		if err == nil {
			sc.inbox = &in
		}
	}
	versions := d.Versions()
	if len(versions) > 0 {
		sc.version = versions[0]
	}
	out, err := s.renderDesign(ctx, q, row.WorkspaceID, d, sc)
	warnings := []oas.EmailDesignIssue{}
	switch {
	case errors.Is(err, design.ErrHTMLTooLarge):
		warnings = append(warnings, oas.EmailDesignIssue{Code: "html_too_large"})
	case err != nil:
		return oas.EmailDesign{}, err
	default:
		warnings = issueBodies(out.Warnings)
	}
	if len(versions) > 1 {
		for _, tag := range versions[1:] {
			warnings = append(warnings, issueBodies(d.VersionIssues(tag))...)
		}
	}
	return oas.EmailDesign{
		Id: row.ID, Name: row.Name, InboxId: row.InboxID, Version: row.Version, TemplateSlug: row.TemplateSlug,
		CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy, ArchivedAt: row.ArchivedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		Document: doc, Warnings: warnings,
	}, nil
}

// visibleDesign loads a design the caller may read: owners, admins and keys; a key limited to
// inboxes sees workspace-wide designs and those of its inboxes.
func visibleDesign(ctx context.Context, q *store.Queries, p principal, id uuid.UUID, lock bool) (store.EmailDesign, error) {
	if err := requireManagerOrKey(p); err != nil {
		return store.EmailDesign{}, err
	}
	get := q.GetEmailDesign
	if lock {
		get = func(ctx context.Context, arg store.GetEmailDesignParams) (store.EmailDesign, error) {
			return q.LockEmailDesign(ctx, store.LockEmailDesignParams(arg))
		}
	}
	d, err := get(ctx, store.GetEmailDesignParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return d, errDesignGone
	}
	if err != nil {
		return d, err
	}
	if d.InboxID != nil {
		ok, err := inboxObjectsVisible(ctx, q, p, *d.InboxID)
		if err != nil {
			return d, err
		}
		if !ok {
			return d, errDesignGone
		}
	}
	return d, nil
}

// designInbox checks the inbox a design is saved under: one the caller sees, and for a key limited
// to inboxes, one of them.
func designInbox(ctx context.Context, q *store.Queries, p principal, id *uuid.UUID) (*store.Inbox, error) {
	if id == nil {
		if p.isKey() && p.limited {
			return nil, problem(http.StatusForbidden, "forbidden", "a key limited to inboxes saves designs of its inboxes only")
		}
		return nil, nil
	}
	in, err := ownedInbox(ctx, q, p, *id)
	if err != nil {
		return nil, err
	}
	return &in, nil
}

func (p principal) memberRef() *uuid.UUID {
	if p.isKey() || p.memberID == uuid.Nil() {
		return nil
	}
	return new(p.memberID)
}

func (s *Server) snapshotDesign(ctx context.Context, q *store.Queries, p principal, row store.EmailDesign, reason string, force bool) error {
	if !force {
		last, err := q.LatestEmailDesignVersionAt(ctx, store.LatestEmailDesignVersionAtParams{WorkspaceID: row.WorkspaceID, DesignID: row.ID})
		if err != nil {
			return err
		}
		if s.now().Sub(last) < designSnapshotEvery {
			return nil
		}
	}
	if err := q.InsertEmailDesignVersion(ctx, store.InsertEmailDesignVersionParams{
		WorkspaceID: row.WorkspaceID, DesignID: row.ID, Version: row.Version, Document: row.Document, Reason: reason,
		MemberID: p.memberRef(), Now: s.now(),
	}); err != nil {
		return err
	}
	return q.PruneEmailDesignVersions(ctx, store.PruneEmailDesignVersionsParams{WorkspaceID: row.WorkspaceID, DesignID: row.ID, Keep: designVersionsKept})
}

// keepConflictVersion snapshots the version a stale save or restore lost against, so no choice the
// other tab makes next can drop it.
func (s *Server) keepConflictVersion(ctx context.Context, q *store.Queries, cur store.EmailDesign) error {
	if err := q.InsertEmailDesignConflictVersion(ctx, store.InsertEmailDesignConflictVersionParams{
		WorkspaceID: cur.WorkspaceID, DesignID: cur.ID, Version: cur.Version, Document: cur.Document,
		MemberID: cur.UpdatedBy, Now: s.now(),
	}); err != nil {
		return err
	}
	return q.PruneEmailDesignVersions(ctx, store.PruneEmailDesignVersionsParams{WorkspaceID: cur.WorkspaceID, DesignID: cur.ID, Keep: designVersionsKept})
}

func designName(name string) (string, error) { return trimmed(name, 1, 200, "name") }

func (s *Server) ListEmailDesigns(ctx context.Context, req oas.ListEmailDesignsRequestObject) (oas.ListEmailDesignsResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
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
	if req.Params.InboxId != nil {
		if _, err := ownedInbox(ctx, s.st.Queries, p, *req.Params.InboxId); err != nil {
			return nil, err
		}
	}
	var search *string
	if req.Params.Q != nil && strings.TrimSpace(*req.Params.Q) != "" {
		esc := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.TrimSpace(*req.Params.Q))
		search = &esc
	}
	rows, err := s.st.ListEmailDesigns(ctx, store.ListEmailDesignsParams{
		WorkspaceID: p.workspaceID, Archived: boolOr(req.Params.Archived, false), InboxID: req.Params.InboxId, Q: search,
		AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), CursorAt: at, CursorID: id, Lim: lim + 1,
	})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(rows[lim-1].UpdatedAt, rows[lim-1].ID)
		next = &c
	}
	items := make([]oas.EmailDesignSummary, len(rows))
	for i, r := range rows {
		items[i] = oas.EmailDesignSummary{
			Id: r.ID, Name: r.Name, InboxId: r.InboxID, Version: r.Version, TemplateSlug: r.TemplateSlug, CreatedBy: r.CreatedBy,
			UpdatedBy: r.UpdatedBy, ArchivedAt: r.ArchivedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	}
	return oas.ListEmailDesigns200JSONResponse{Items: items, NextCursor: next}, nil
}

func (s *Server) CreateEmailDesign(ctx context.Context, req oas.CreateEmailDesignRequestObject) (oas.CreateEmailDesignResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	name, err := designName(b.Name)
	if err != nil {
		return nil, err
	}
	if b.Document != nil && b.Template != nil {
		return nil, errValidation("send document or template, not both")
	}
	var row store.EmailDesign
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		if _, err := designInbox(ctx, q, p, b.InboxId); err != nil {
			return err
		}
		var d design.Document
		var slug *string
		switch {
		case b.Template != nil:
			var others []oas.EmailTemplateLocale
			if b.Template.Locales != nil {
				others = *b.Template.Locales
			}
			d, err = s.documentFromTemplate(ctx, q, p, b.Template.Slug, b.Template.Locale, others)
			if err != nil {
				return err
			}
			slug = new(b.Template.Slug)
		case b.Document != nil:
			if d, err = s.readDocument(ctx, q, p.workspaceID, requestDocument(*b.Document), design.Options{}); err != nil {
				return err
			}
		default:
			d = blankDocument()
		}
		row, err = q.CreateEmailDesign(ctx, store.CreateEmailDesignParams{
			ID: newID(), WorkspaceID: p.workspaceID, InboxID: b.InboxId, Name: name, Document: design.Marshal(d),
			TemplateSlug: slug, MemberID: p.memberRef(), Now: s.now(),
		})
		if err != nil {
			return err
		}
		return s.snapshotDesign(ctx, q, p, row, "autosave", true)
	})
	if err != nil {
		return nil, err
	}
	out, err := s.designBody(ctx, s.st.Queries, row)
	if err != nil {
		return nil, err
	}
	return oas.CreateEmailDesign201JSONResponse(out), nil
}

func (s *Server) GetEmailDesign(ctx context.Context, req oas.GetEmailDesignRequestObject) (oas.GetEmailDesignResponseObject, error) {
	p := principalFrom(ctx)
	row, err := visibleDesign(ctx, s.st.Queries, p, req.DesignId, false)
	if err != nil {
		return nil, err
	}
	out, err := s.designBody(ctx, s.st.Queries, row)
	if err != nil {
		return nil, err
	}
	return oas.GetEmailDesign200JSONResponse(out), nil
}

func (s *Server) versionConflict(ctx context.Context, cur store.EmailDesign) error {
	body, err := s.designBody(ctx, s.st.Queries, cur)
	if err != nil {
		return err
	}
	e := problem(http.StatusConflict, "version_conflict", "the design was saved by someone else since version "+strconv.Itoa(int(cur.Version)))
	e.Extra = map[string]any{"design": body}
	return e
}

// saveDesign writes a new version of a locked design when base matches its version.
func (s *Server) saveDesign(ctx context.Context, q *store.Queries, p principal, cur store.EmailDesign, base int32, name string, inboxID *uuid.UUID, d design.Document) (store.EmailDesign, error) {
	if base != cur.Version {
		return cur, errVersionConflict
	}
	return q.SaveEmailDesign(ctx, store.SaveEmailDesignParams{
		WorkspaceID: p.workspaceID, ID: cur.ID, Name: name, InboxID: inboxID, Document: design.Marshal(d), MemberID: p.memberRef(), Now: s.now(),
	})
}

var errVersionConflict = errors.New("version conflict")

// keepTranslations carries the stored language versions over to a document sent without the
// translations key, so a client that does not know them cannot erase them.
func keepTranslations(cur store.EmailDesign, d design.Document) (design.Document, error) {
	old, _, err := design.Parse(cur.Document)
	if err != nil {
		return d, err
	}
	if d.Locale == "" {
		d.Locale = old.Locale
	}
	if d.FallbackLocale == "" {
		d.FallbackLocale = old.FallbackLocale
	}
	d.Translations = old.Translations
	d.Prune()
	if issues := design.Validate(d, design.Options{}); len(issues) > 0 {
		return d, issuesProblem(issues)
	}
	return d, nil
}

func (s *Server) UpdateEmailDesign(ctx context.Context, req oas.UpdateEmailDesignRequestObject) (oas.UpdateEmailDesignResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	name, err := designName(b.Name)
	if err != nil {
		return nil, err
	}
	var row, conflict store.EmailDesign
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := visibleDesign(ctx, q, p, req.DesignId, true)
		if err != nil {
			return err
		}
		if _, err := designInbox(ctx, q, p, cur.InboxID); err != nil {
			return err
		}
		if _, err := designInbox(ctx, q, p, b.InboxId); err != nil {
			return err
		}
		d, err := s.readDocument(ctx, q, p.workspaceID, requestDocument(b.Document), design.Options{})
		if err != nil {
			return err
		}
		if b.Document.Translations == nil {
			if d, err = keepTranslations(cur, d); err != nil {
				return err
			}
		}
		row, err = s.saveDesign(ctx, q, p, cur, b.BaseVersion, name, b.InboxId, d)
		if errors.Is(err, errVersionConflict) {
			conflict = cur
			return s.keepConflictVersion(ctx, q, cur)
		}
		if err != nil {
			return err
		}
		if boolOr(b.Snapshot, false) {
			return s.snapshotDesign(ctx, q, p, row, "manual", true)
		}
		return s.snapshotDesign(ctx, q, p, row, "autosave", false)
	})
	if err != nil {
		return nil, err
	}
	if conflict.ID != uuid.Nil() {
		return nil, s.versionConflict(ctx, conflict)
	}
	out, err := s.designBody(ctx, s.st.Queries, row)
	if err != nil {
		return nil, err
	}
	return oas.UpdateEmailDesign200JSONResponse(out), nil
}

func (s *Server) ArchiveEmailDesign(ctx context.Context, req oas.ArchiveEmailDesignRequestObject) (oas.ArchiveEmailDesignResponseObject, error) {
	p := principalFrom(ctx)
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		return s.archiveEmailDesign(ctx, q, p, req.DesignId)
	})
	if err != nil {
		return nil, err
	}
	return oas.ArchiveEmailDesign204Response{}, nil
}

func (s *Server) archiveEmailDesign(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) error {
	cur, err := visibleDesign(ctx, q, p, id, true)
	if err != nil {
		return err
	}
	if _, err := designInbox(ctx, q, p, cur.InboxID); err != nil {
		return err
	}
	_, err = q.ArchiveEmailDesign(ctx, store.ArchiveEmailDesignParams{WorkspaceID: p.workspaceID, ID: cur.ID, Now: s.now()})
	return err
}

func (s *Server) DuplicateEmailDesign(ctx context.Context, req oas.DuplicateEmailDesignRequestObject) (oas.DuplicateEmailDesignResponseObject, error) {
	p := principalFrom(ctx)
	var row store.EmailDesign
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := visibleDesign(ctx, q, p, req.DesignId, false)
		if err != nil {
			return err
		}
		if _, err := designInbox(ctx, q, p, cur.InboxID); err != nil {
			return err
		}
		name := cur.Name + " (copy)"
		if r := []rune(name); len(r) > 200 {
			name = string(r[:200])
		}
		if req.Body != nil && req.Body.Name != nil {
			if name, err = designName(*req.Body.Name); err != nil {
				return err
			}
		}
		d, _, err := design.Parse(cur.Document)
		if err != nil {
			return err
		}
		row, err = q.CreateEmailDesign(ctx, store.CreateEmailDesignParams{
			ID: newID(), WorkspaceID: p.workspaceID, InboxID: cur.InboxID, Name: name, Document: design.Marshal(d),
			TemplateSlug: cur.TemplateSlug, MemberID: p.memberRef(), Now: s.now(),
		})
		if err != nil {
			return err
		}
		return s.snapshotDesign(ctx, q, p, row, "autosave", true)
	})
	if err != nil {
		return nil, err
	}
	out, err := s.designBody(ctx, s.st.Queries, row)
	if err != nil {
		return nil, err
	}
	return oas.DuplicateEmailDesign201JSONResponse(out), nil
}

func (s *Server) ListEmailDesignVersions(ctx context.Context, req oas.ListEmailDesignVersionsRequestObject) (oas.ListEmailDesignVersionsResponseObject, error) {
	p := principalFrom(ctx)
	cur, err := visibleDesign(ctx, s.st.Queries, p, req.DesignId, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListEmailDesignVersions(ctx, store.ListEmailDesignVersionsParams{WorkspaceID: p.workspaceID, DesignID: cur.ID})
	if err != nil {
		return nil, err
	}
	items := make([]oas.EmailDesignVersion, len(rows))
	for i, r := range rows {
		items[i] = oas.EmailDesignVersion{Version: r.Version, Reason: oas.EmailDesignVersionReason(r.Reason), MemberId: r.MemberID, CreatedAt: r.CreatedAt}
	}
	return oas.ListEmailDesignVersions200JSONResponse{Items: items}, nil
}

func (s *Server) GetEmailDesignVersion(ctx context.Context, req oas.GetEmailDesignVersionRequestObject) (oas.GetEmailDesignVersionResponseObject, error) {
	p := principalFrom(ctx)
	cur, err := visibleDesign(ctx, s.st.Queries, p, req.DesignId, false)
	if err != nil {
		return nil, err
	}
	v, err := s.st.GetEmailDesignVersion(ctx, store.GetEmailDesignVersionParams{WorkspaceID: p.workspaceID, DesignID: cur.ID, Version: req.Version})
	if store.IsNotFound(err) {
		return nil, errDesignVersionGone
	}
	if err != nil {
		return nil, err
	}
	d, _, err := design.Parse(v.Document)
	if err != nil {
		return nil, err
	}
	doc, err := documentBody(d)
	if err != nil {
		return nil, err
	}
	return oas.GetEmailDesignVersion200JSONResponse{
		Version: v.Version, Reason: oas.EmailDesignVersionReason(v.Reason), MemberId: v.MemberID, CreatedAt: v.CreatedAt, Document: doc,
	}, nil
}

func (s *Server) RestoreEmailDesignVersion(ctx context.Context, req oas.RestoreEmailDesignVersionRequestObject) (oas.RestoreEmailDesignVersionResponseObject, error) {
	p := principalFrom(ctx)
	var row, conflict store.EmailDesign
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := visibleDesign(ctx, q, p, req.DesignId, true)
		if err != nil {
			return err
		}
		if _, err := designInbox(ctx, q, p, cur.InboxID); err != nil {
			return err
		}
		v, err := q.GetEmailDesignVersion(ctx, store.GetEmailDesignVersionParams{WorkspaceID: p.workspaceID, DesignID: cur.ID, Version: req.Version})
		if store.IsNotFound(err) {
			return errDesignVersionGone
		}
		if err != nil {
			return err
		}
		d, err := s.readDocument(ctx, q, p.workspaceID, v.Document, design.Options{})
		if err != nil {
			return err
		}
		row, err = s.saveDesign(ctx, q, p, cur, req.Body.BaseVersion, cur.Name, cur.InboxID, d)
		if errors.Is(err, errVersionConflict) {
			conflict = cur
			return s.keepConflictVersion(ctx, q, cur)
		}
		if err != nil {
			return err
		}
		return s.snapshotDesign(ctx, q, p, row, "restored", true)
	})
	if err != nil {
		return nil, err
	}
	if conflict.ID != uuid.Nil() {
		return nil, s.versionConflict(ctx, conflict)
	}
	out, err := s.designBody(ctx, s.st.Queries, row)
	if err != nil {
		return nil, err
	}
	return oas.RestoreEmailDesignVersion200JSONResponse(out), nil
}

func (s *Server) RenderEmailDesign(ctx context.Context, req oas.RenderEmailDesignRequestObject) (oas.RenderEmailDesignResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	if err := s.rateLimit(ctx, rateCheck{"render:" + p.viewerID().String(), limitRender}); err != nil {
		return nil, err
	}
	b := req.Body
	q := s.st.Queries
	d, err := s.readDocument(ctx, q, p.workspaceID, requestDocument(b.Document), design.Options{})
	if err != nil {
		return nil, err
	}
	sc := renderScope{dark: boolOr(b.Dark, false)}
	if b.Subject != nil {
		sc.subject = *b.Subject
	}
	if b.Preheader != nil {
		sc.pre = *b.Preheader
	}
	if b.ListId != nil {
		l, err := visibleList(ctx, q, p, *b.ListId, false)
		if err != nil {
			return nil, err
		}
		sc.list = &l
		if b.InboxId == nil {
			in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: p.workspaceID, ID: l.InboxID})
			if err != nil {
				return nil, err
			}
			sc.inbox = &in
		}
	}
	if b.InboxId != nil {
		in, err := ownedInbox(ctx, q, p, *b.InboxId)
		if err != nil {
			return nil, err
		}
		sc.inbox = &in
	}
	want := ""
	if b.Locale != nil {
		if want, err = languageTag(string(*b.Locale), "locale"); err != nil {
			return nil, err
		}
	}
	if b.ContactId != nil {
		v, locale, err := s.contactValues(ctx, q, p, *b.ContactId)
		if err != nil {
			return nil, err
		}
		sc.values = &v
		if b.Locale == nil {
			want = locale
		}
	}
	sc.version, _ = d.Pick(want)
	out, err := s.renderDesign(ctx, q, p.workspaceID, d, sc)
	if errors.Is(err, design.ErrHTMLTooLarge) {
		return nil, designProblem(http.StatusUnprocessableEntity, "html_too_large", "the rendered mail is over 500 KB", nil)
	}
	if err != nil {
		return nil, err
	}
	res := oas.RenderEmailDesign200JSONResponse{Html: out.HTML, Text: out.Text, SizeBytes: int64(out.Size), Warnings: issueBodies(out.Warnings)}
	if sc.version != "" {
		res.Locale = new(oas.LanguageTag(sc.version))
	}
	return res, nil
}

// contactValues are a contact's merge field values: name, first address and top-level string and
// number attributes.
func (s *Server) contactValues(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) (design.Values, string, error) {
	if err := visibleContact(ctx, q, p, id); err != nil {
		return design.Values{}, "", err
	}
	c, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return design.Values{}, "", errContactGone
	}
	if err != nil {
		return design.Values{}, "", err
	}
	v := design.Values{ContactName: c.Name, Attributes: map[string]string{}}
	emails, err := q.ListContactEmails(ctx, store.ListContactEmailsParams{WorkspaceID: p.workspaceID, ContactIds: []uuid.UUID{id}})
	if err != nil {
		return v, "", err
	}
	if len(emails) > 0 {
		v.ContactEmail = emails[0].Email
	}
	var attrs map[string]any
	if len(c.Attributes) > 0 {
		_ = json.Unmarshal(c.Attributes, &attrs)
	}
	for k, a := range attrs {
		switch x := a.(type) {
		case string:
			v.Attributes[k] = x
		case float64:
			v.Attributes[k] = strconv.FormatFloat(x, 'f', -1, 64)
		}
	}
	return v, deref(c.Locale), nil
}

func languageTag(v, field string) (string, error) {
	tag, ok := design.CanonicalTag(strings.TrimSpace(v))
	if !ok {
		return "", errValidation(field + " must be a language tag such as en or es-MX")
	}
	return tag, nil
}

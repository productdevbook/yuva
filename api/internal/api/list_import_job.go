package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/csvimport"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	importBudget       = 30 * time.Second
	importBatch        = 1000
	importMaxDomains   = 5000
	importDNSTimeout   = 5 * time.Second
	importDNSWorkers   = 16
	importAttempts     = 10
	importAttrMaxRunes = 500

	reasonInvalidEmail = "invalid_email"
	reasonNoMailDomain = "no_mail_domain"
	reasonRoleAddress  = "role_address"
	reasonDuplicate    = "duplicate"
	reasonSuppressed   = "suppressed"
	reasonOptedOut     = "opted_out"
	reasonUnsubscribed = "unsubscribed"
)

var roleLocalParts = map[string]bool{"abuse": true, "postmaster": true, "noreply": true, "no-reply": true, "mailer-daemon": true}

type ListImportArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	ImportID    uuid.UUID `json:"import_id"`
}

func (ListImportArgs) Kind() string { return "list_import" }

// listImportOpts queues without uniqueness: every batch locks the import and continues from its
// rows_done, so a second job for the same import only shares the work.
func listImportOpts() *river.InsertOpts { return &river.InsertOpts{MaxAttempts: importAttempts} }

type listImportWorker struct {
	river.WorkerDefaults[ListImportArgs]
	s *Server
}

func (w *listImportWorker) Timeout(*river.Job[ListImportArgs]) time.Duration {
	return importBudget + 90*time.Second
}

func (w *listImportWorker) Work(ctx context.Context, job *river.Job[ListImportArgs]) error {
	done, err := w.s.RunListImport(ctx, job.Args, importBudget)
	if err != nil {
		if job.Attempt < job.MaxAttempts {
			return err
		}
		w.s.log.ErrorContext(ctx, "list import", slog.String("import_id", job.Args.ImportID.String()), slog.Any("error", err))
		return w.s.failListImport(ctx, job.Args.WorkspaceID, job.Args.ImportID, "the import stopped on a server error")
	}
	if !done {
		return river.JobSnooze(0)
	}
	return nil
}

// RunListImport validates or imports for about budget; done is false while work is left.
func (s *Server) RunListImport(ctx context.Context, a ListImportArgs, budget time.Duration) (bool, error) {
	if live, err := s.workspaceLive(ctx, a.WorkspaceID); err != nil || !live {
		return true, err
	}
	imp, err := s.st.GetListImport(ctx, store.GetListImportParams{WorkspaceID: a.WorkspaceID, ID: a.ImportID})
	if store.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	deadline := time.Now().Add(budget)
	switch imp.State {
	case importValidating:
		return s.validateListImport(ctx, imp, deadline)
	case importImporting:
		return s.importListImport(ctx, imp, deadline)
	}
	return true, nil
}

func (s *Server) failListImport(ctx context.Context, ws, id uuid.UUID, reason string) error {
	var (
		imp    store.ListImport
		key    *string
		failed bool
	)
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.LockListImport(ctx, store.LockListImportParams{WorkspaceID: ws, ID: id})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if cur.State != importValidating && cur.State != importImporting {
			return nil
		}
		key, failed = cur.StorageKey, true
		imp, err = q.FinishListImport(ctx, store.FinishListImportParams{WorkspaceID: ws, ID: id, State: importFailed, Error: new(truncateRunes(reason, 2000)), Now: new(s.now())})
		return err
	})
	if err != nil || !failed {
		return err
	}
	if key != nil {
		s.deleteObjects(ctx, []string{*key})
	}
	if list, err := s.st.GetMailingList(ctx, store.GetMailingListParams{WorkspaceID: ws, ID: imp.ListID}); err == nil {
		s.listImportSignal(ctx, list, imp)
	}
	return nil
}

// importFile reads and parses the upload; reason is set when the file itself is the problem.
func (s *Server) importFile(ctx context.Context, imp store.ListImport) (csvimport.File, string, error) {
	if imp.StorageKey == nil {
		return csvimport.File{}, "the uploaded file is gone", nil
	}
	rc, err := s.objects.Open(ctx, *imp.StorageKey)
	if err != nil {
		return csvimport.File{}, "", err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxImportBytes+1))
	if err != nil {
		return csvimport.File{}, "", err
	}
	f, err := csvimport.Parse(data, s.imports.MaxRows)
	if errors.Is(err, csvimport.ErrTooLarge) {
		return f, "the file has more than " + strconv.Itoa(s.imports.MaxRows) + " data rows", nil
	}
	if err != nil {
		return f, err.Error(), nil
	}
	return f, "", nil
}

func importColumn(cols []oas.ListImportColumn, field oas.ListImportField) int {
	for i, c := range cols {
		if c.Field == field {
			return i
		}
	}
	return -1
}

// guessImportColumns maps known headers to fields and the others to attributes; without an
// e-mail header, the first column whose sample values hold an @ is the address.
func guessImportColumns(f csvimport.File) []oas.ListImportColumn {
	cols := make([]oas.ListImportColumn, len(f.Header))
	taken := map[string]bool{}
	for i, h := range f.Header {
		col := oas.ListImportColumn{Index: int32(i), Header: h, Field: oas.ListImportFieldSkip}
		if field, ok := csvimport.GuessField(h); ok && !taken[field] {
			col.Field, taken[field] = oas.ListImportField(field), true
		} else if key := csvimport.AttributeKey(h); key != "" && !taken["attribute:"+key] && !ok {
			col.Field, col.Attribute, taken["attribute:"+key] = oas.ListImportFieldAttribute, &key, true
		}
		cols[i] = col
	}
	if importColumn(cols, oas.ListImportFieldEmail) >= 0 {
		return cols
	}
	for i := range cols {
		seen, at := 0, 0
		for _, r := range f.Rows[:min(len(f.Rows), importPreviewRows)] {
			if i < len(r.Fields) && strings.TrimSpace(r.Fields[i]) != "" {
				seen++
				if strings.Contains(r.Fields[i], "@") {
					at++
				}
			}
		}
		if seen > 0 && at*2 >= seen {
			cols[i].Field, cols[i].Attribute = oas.ListImportFieldEmail, nil
			break
		}
	}
	return cols
}

// fitImportColumns puts a stored mapping onto the file's header: every column named, mapped ones
// keeping their field.
func fitImportColumns(stored []oas.ListImportColumn, header []string) ([]oas.ListImportColumn, string) {
	if len(stored) > len(header) {
		for _, c := range stored[len(header):] {
			if c.Field != oas.ListImportFieldSkip {
				return nil, "the mapping names column " + strconv.Itoa(int(c.Index)+1) + " but the file has " + strconv.Itoa(len(header))
			}
		}
	}
	out := make([]oas.ListImportColumn, len(header))
	for i, h := range header {
		out[i] = oas.ListImportColumn{Index: int32(i), Header: h, Field: oas.ListImportFieldSkip}
		if i < len(stored) {
			out[i].Field, out[i].Attribute = stored[i].Field, stored[i].Attribute
		}
	}
	return out, ""
}

type importRow struct {
	line       int
	raw        string
	email      string
	domain     string
	name       string
	locale     string
	externalID string
	attrs      map[string]any
	reason     string
	existing   bool
}

func cell(fields []string, i int) string {
	if i < 0 || i >= len(fields) {
		return ""
	}
	return strings.TrimSpace(fields[i])
}

func importLocale(v string) string {
	v = strings.ReplaceAll(strings.TrimSpace(v), "_", "-")
	if head, tail, ok := strings.Cut(v, "-"); ok {
		v = strings.ToLower(head) + "-" + tail
	} else {
		v = strings.ToLower(v)
	}
	if !languagePattern.MatchString(v) {
		return ""
	}
	return v
}

// importRows reads every data row against the mapping and refuses what the file alone shows:
// bad addresses, repeats (the first one wins), role addresses and domains known to take no mail.
func importRows(f csvimport.File, cols []oas.ListImportColumn, domains map[string]bool) []importRow {
	at := func(field oas.ListImportField) int { return importColumn(cols, field) }
	emailCol, nameCol, firstCol, lastCol := at(oas.ListImportFieldEmail), at(oas.ListImportFieldName), at(oas.ListImportFieldFirstName), at(oas.ListImportFieldLastName)
	localeCol, extCol := at(oas.ListImportFieldLocale), at(oas.ListImportFieldExternalId)
	seen := make(map[string]bool, len(f.Rows))
	out := make([]importRow, len(f.Rows))
	for i, r := range f.Rows {
		row := importRow{line: r.Line, raw: truncateRunes(cell(r.Fields, emailCol), 320)}
		addr, err := subscriptionAddress(row.raw)
		switch {
		case err != nil:
			row.reason = reasonInvalidEmail
		case seen[addr]:
			row.reason = reasonDuplicate
		default:
			seen[addr] = true
			local, domain, _ := strings.Cut(addr, "@")
			row.email, row.domain = addr, domain
			if roleLocalParts[local] {
				row.reason = reasonRoleAddress
			} else if ok, known := domains[domain]; known && !ok {
				row.reason = reasonNoMailDomain
			}
		}
		if row.reason == "" {
			name := cell(r.Fields, nameCol)
			if name == "" {
				name = strings.TrimSpace(cell(r.Fields, firstCol) + " " + cell(r.Fields, lastCol))
			}
			row.name = truncateRunes(name, 200)
			row.locale = importLocale(cell(r.Fields, localeCol))
			if ext := cell(r.Fields, extCol); len([]rune(ext)) <= 200 {
				row.externalID = ext
			}
			for _, c := range cols {
				if c.Field != oas.ListImportFieldAttribute || c.Attribute == nil {
					continue
				}
				if v := cell(r.Fields, int(c.Index)); v != "" {
					if row.attrs == nil {
						row.attrs = map[string]any{}
					}
					row.attrs[*c.Attribute] = truncateRunes(v, importAttrMaxRunes)
				}
			}
			if row.attrs != nil && len(mustJSON(row.attrs)) > maxAttributesBytes {
				row.attrs = nil
			}
		}
		out[i] = row
	}
	return out
}

// checkDomains looks up the distinct domains of the rows still in, at most 5,000 per import, until
// deadline; complete is false when some are left. A domain that cannot be looked up now counts as
// taking mail.
func (s *Server) checkDomains(ctx context.Context, rows []importRow, known map[string]bool, deadline time.Time) bool {
	var todo []string
	listed := map[string]bool{}
	for d := range known {
		listed[d] = true
	}
	for _, r := range rows {
		if r.reason != "" || listed[r.domain] {
			continue
		}
		if len(listed) >= importMaxDomains {
			break
		}
		listed[r.domain] = true
		todo = append(todo, r.domain)
	}
	if len(todo) == 0 {
		return true
	}
	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		next = make(chan string)
	)
	for range importDNSWorkers {
		wg.Go(func() {
			for d := range next {
				ok := s.takesMail(ctx, d)
				mu.Lock()
				known[d] = ok
				mu.Unlock()
			}
		})
	}
	complete := true
	for _, d := range todo {
		if time.Now().After(deadline) || ctx.Err() != nil {
			complete = false
			break
		}
		next <- d
	}
	close(next)
	wg.Wait()
	return complete
}

func (s *Server) takesMail(ctx context.Context, domain string) bool {
	ctx, cancel := context.WithTimeout(ctx, importDNSTimeout)
	defer cancel()
	mx, err := s.imports.Resolver.LookupMX(ctx, domain)
	if err == nil && len(mx) > 0 {
		return !(len(mx) == 1 && strings.TrimSuffix(mx[0].Host, ".") == "")
	}
	var dnsErr *net.DNSError
	if err != nil && !(errors.As(err, &dnsErr) && dnsErr.IsNotFound) {
		return true
	}
	if _, err := s.imports.Resolver.LookupNetIP(ctx, "ip", domain); err != nil {
		return !(errors.As(err, &dnsErr) && dnsErr.IsNotFound)
	}
	return true
}

// classifyStored refuses rows the workspace's records rule out (suppressed, opted out,
// unsubscribed from the list) and marks the ones already on the list.
func (s *Server) classifyStored(ctx context.Context, q *store.Queries, ws, listID uuid.UUID, rows []*importRow) error {
	var emails []string
	for _, r := range rows {
		if r.reason == "" {
			emails = append(emails, r.email)
		}
	}
	if len(emails) == 0 {
		return nil
	}
	hashes, err := s.optOutHashes(ctx, q, ws, emails...)
	if err != nil {
		return err
	}
	hashOf := make(map[string][]byte, len(emails))
	for i, e := range emails {
		hashOf[e] = hashes[i]
	}
	suppressed, err := q.SuppressedEmailsIn(ctx, store.SuppressedEmailsInParams{WorkspaceID: ws, Emails: emails})
	if err != nil {
		return err
	}
	optedOut, err := q.OptedOutHashesIn(ctx, store.OptedOutHashesInParams{WorkspaceID: ws, Hashes: hashes})
	if err != nil {
		return err
	}
	subs, err := q.ListSubscriptionsByEmails(ctx, store.ListSubscriptionsByEmailsParams{WorkspaceID: ws, ListID: listID, Emails: emails})
	if err != nil {
		return err
	}
	bad := map[string]bool{}
	for _, e := range suppressed {
		bad[e] = true
	}
	opted := map[string]bool{}
	for _, h := range optedOut {
		opted[string(h)] = true
	}
	states := map[string]store.ListSubscriptionsByEmailsRow{}
	for _, sub := range subs {
		states[sub.Email] = sub
	}
	for _, r := range rows {
		if r.reason != "" {
			continue
		}
		sub, onList := states[r.email]
		switch {
		case bad[r.email]:
			r.reason = reasonSuppressed
		case opted[string(hashOf[r.email])]:
			r.reason = reasonOptedOut
		case onList && objected(store.ListSubscription{State: sub.State, UnsubscribeReason: sub.UnsubscribeReason}):
			r.reason = reasonUnsubscribed
		case onList:
			r.existing = true
		}
	}
	return nil
}

func countRefusal(c *oas.ListImportCounts, reason string) {
	switch reason {
	case reasonInvalidEmail:
		c.InvalidEmail++
	case reasonNoMailDomain:
		c.NoMailDomain++
	case reasonRoleAddress:
		c.RoleAddress++
	case reasonDuplicate:
		c.Duplicate++
	case reasonSuppressed:
		c.Suppressed++
	case reasonOptedOut:
		c.OptedOut++
	case reasonUnsubscribed:
		c.Unsubscribed++
	}
}

func importPreview(f csvimport.File) [][]string {
	out := make([][]string, 0, importPreviewRows)
	for _, r := range f.Rows[:min(len(f.Rows), importPreviewRows)] {
		row := make([]string, len(f.Header))
		for i := range row {
			row[i] = truncateRunes(cell(r.Fields, i), importPreviewCell)
		}
		out = append(out, row)
	}
	return out
}

func delimiterName(r rune) string {
	switch r {
	case ';':
		return string(oas.Semicolon)
	case '\t':
		return string(oas.Tab)
	}
	return string(oas.Comma)
}

// validateListImport reads the file, checks every row without writing and leaves the import
// ready with its counts and preview.
func (s *Server) validateListImport(ctx context.Context, imp store.ListImport, deadline time.Time) (bool, error) {
	ws := imp.WorkspaceID
	f, reason, err := s.importFile(ctx, imp)
	if err != nil {
		return false, err
	}
	if reason != "" {
		return true, s.failListImport(ctx, ws, imp.ID, reason)
	}
	cols := importColumns(imp.Columns)
	if len(cols) == 0 {
		cols = guessImportColumns(f)
	} else if cols, reason = fitImportColumns(cols, f.Header); reason != "" {
		return true, s.failListImport(ctx, ws, imp.ID, reason)
	}
	domains := map[string]bool{}
	_ = json.Unmarshal(imp.Domains, &domains)
	var counts oas.ListImportCounts
	if importColumn(cols, oas.ListImportFieldEmail) >= 0 {
		rows := importRows(f, cols, domains)
		if !s.checkDomains(ctx, rows, domains, deadline) {
			return false, s.st.SaveListImportDomains(ctx, store.SaveListImportDomainsParams{WorkspaceID: ws, ID: imp.ID, Domains: mustJSON(domains)})
		}
		rows = importRows(f, cols, domains)
		for start := 0; start < len(rows); start += importBatch {
			batch := make([]*importRow, 0, importBatch)
			for i := start; i < min(start+importBatch, len(rows)); i++ {
				batch = append(batch, &rows[i])
			}
			if err := s.classifyStored(ctx, s.st.Queries, ws, imp.ListID, batch); err != nil {
				return false, err
			}
		}
		for _, r := range rows {
			switch {
			case r.reason != "":
				countRefusal(&counts, r.reason)
			case r.existing:
				counts.Existing++
			default:
				counts.Valid++
			}
		}
	}
	out, err := s.st.SaveListImportValidation(ctx, store.SaveListImportValidationParams{
		WorkspaceID: ws, ID: imp.ID, Encoding: &f.Encoding, Delimiter: new(delimiterName(f.Delimiter)), Columns: mustJSON(cols),
		Preview: mustJSON(importPreview(f)), RowsTotal: int32(len(f.Rows)), Counts: mustJSON(counts), Domains: mustJSON(domains), Now: s.now(),
	})
	if store.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if list, err := s.st.GetMailingList(ctx, store.GetMailingListParams{WorkspaceID: ws, ID: imp.ListID}); err == nil {
		s.listImportSignal(ctx, list, out)
	}
	return true, nil
}

// importListImport adds the rows 1,000 per transaction from rows_done on, until deadline.
func (s *Server) importListImport(ctx context.Context, imp store.ListImport, deadline time.Time) (bool, error) {
	ws := imp.WorkspaceID
	f, reason, err := s.importFile(ctx, imp)
	if err != nil {
		return false, err
	}
	if reason != "" {
		return true, s.failListImport(ctx, ws, imp.ID, reason)
	}
	cols, reason := fitImportColumns(importColumns(imp.Columns), f.Header)
	if reason != "" {
		return true, s.failListImport(ctx, ws, imp.ID, reason)
	}
	domains := map[string]bool{}
	_ = json.Unmarshal(imp.Domains, &domains)
	rows := importRows(f, cols, domains)
	list, err := s.st.GetMailingList(ctx, store.GetMailingListParams{WorkspaceID: ws, ID: imp.ListID})
	if store.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	pr := consentProof{
		source: string(oas.SubscriptionSourceImport), memberID: imp.MemberID, apiKeyID: imp.ApiKeyID, importID: &imp.ID,
		text: &imp.Attestation, detail: map[string]any{"import_id": imp.ID.String(), "consent_source": imp.ConsentSource},
	}
	for {
		var (
			cur      store.ListImport
			finished bool
			key      *string
		)
		err := s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
			events.quiet = true
			var err error
			if cur, err = q.LockListImport(ctx, store.LockListImportParams{WorkspaceID: ws, ID: imp.ID}); err != nil {
				return err
			}
			if cur.State != importImporting {
				finished = true
				return nil
			}
			start := int(cur.RowsDone)
			if start >= len(rows) {
				finished, key = true, cur.StorageKey
				cur, err = q.FinishListImport(ctx, store.FinishListImportParams{WorkspaceID: ws, ID: imp.ID, State: importDone, Now: new(s.now())})
				return err
			}
			end := min(start+importBatch, len(rows))
			counts := importCounts(cur.Counts)
			if err := s.importBatch(ctx, q, events, list, cur, pr, rows[start:end], &counts); err != nil {
				return err
			}
			cur, err = q.SaveListImportProgress(ctx, store.SaveListImportProgressParams{WorkspaceID: ws, ID: imp.ID, RowsDone: int32(end), Counts: mustJSON(counts), Now: s.now()})
			return err
		})
		if store.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if finished {
			if cur.State == importDone && key != nil {
				s.deleteObjects(ctx, []string{*key})
				s.listImportSignal(ctx, list, cur)
			}
			return true, nil
		}
		s.listImportProgress(ctx, list, cur)
		if time.Now().After(deadline) {
			return false, nil
		}
	}
}

func (s *Server) importBatch(ctx context.Context, q *store.Queries, events *eventBatch, list store.MailingList, imp store.ListImport, pr consentProof, rows []importRow, counts *oas.ListImportCounts) error {
	ws := imp.WorkspaceID
	batch := make([]*importRow, len(rows))
	for i := range rows {
		r := rows[i]
		batch[i] = &r
	}
	if err := s.classifyStored(ctx, q, ws, list.ID, batch); err != nil {
		return err
	}
	fieldDefs, err := contactFieldDefs(ctx, q, ws)
	if err != nil {
		return err
	}
	var dailyLimit int64 = -1
	if imp.Reconfirm {
		w, err := q.GetWorkspace(ctx, ws)
		if err != nil {
			return err
		}
		if dailyLimit, _, err = s.dailyAllowance(ctx, q, w); err != nil {
			return err
		}
	}
	p := principal{workspaceID: ws}
	if imp.ApiKeyID != nil {
		p.keyID = *imp.ApiKeyID
	} else if imp.MemberID != nil {
		p.memberID = *imp.MemberID
	}
	var refused struct {
		rows    []int32
		emails  []string
		reasons []string
	}
	refuse := func(r *importRow, reason string) {
		countRefusal(counts, reason)
		refused.rows = append(refused.rows, int32(r.line))
		refused.emails = append(refused.emails, r.raw)
		refused.reasons = append(refused.reasons, reason)
	}
	for _, r := range batch {
		if r.reason != "" {
			refuse(r, r.reason)
			continue
		}
		b := oas.SubscriptionCreate{Email: &r.email}
		if r.name != "" {
			b.Name = &r.name
		}
		if r.locale != "" {
			b.Locale = new(oas.LanguageTag(r.locale))
		}
		if r.externalID != "" {
			b.ExternalId = &r.externalID
		}
		if r.attrs != nil {
			r.attrs, _ = keepValidContactFields(fieldDefs, r.attrs, true)
			b.Attributes = new(oas.Attributes(r.attrs))
		}
		contactID, email, created, err := s.subscriber(ctx, q, p, list, &b)
		var apiErr *apiError
		if errors.As(err, &apiErr) {
			refuse(r, reasonInvalidEmail)
			continue
		}
		if err != nil {
			return err
		}
		if created {
			counts.CreatedContacts++
		} else if r.attrs != nil {
			if err := s.fillContactAttributes(ctx, q, ws, contactID, r.attrs); err != nil {
				return err
			}
		}
		cur, err := q.LockSubscriptionByEmail(ctx, store.LockSubscriptionByEmailParams{WorkspaceID: ws, ListID: list.ID, Email: email})
		if err != nil && !store.IsNotFound(err) {
			return err
		}
		now := s.now()
		if err == nil {
			switch {
			case cur.State == subSubscribed:
				counts.Existing++
			case objected(cur):
				refuse(r, reasonUnsubscribed)
			case imp.Reconfirm:
				counts.Existing++
			default:
				if _, err := s.markSubscribed(ctx, q, events, list, cur, now, pr); err != nil {
					return err
				}
				counts.Added++
			}
			continue
		}
		if imp.Reconfirm {
			sub, err := s.newSubscription(ctx, q, events, list, contactID, email, subPending, pr.source, nil)
			if err != nil {
				return err
			}
			if err := s.consentEvent(ctx, q, sub, consentRequested, pr); err != nil {
				return err
			}
			var at time.Time
			if dailyLimit > 0 {
				at = now.Add(time.Duration(counts.Added/dailyLimit) * 24 * time.Hour)
			}
			if err := s.queueConfirmationAt(ctx, q, events, sub, at); err != nil {
				return err
			}
		} else {
			sub, err := s.newSubscription(ctx, q, events, list, contactID, email, subSubscribed, pr.source, &now)
			if err != nil {
				return err
			}
			if err := s.consentEvent(ctx, q, sub, consentSubscribed, pr); err != nil {
				return err
			}
		}
		counts.Added++
	}
	if len(refused.rows) == 0 {
		return nil
	}
	return q.AddListImportErrors(ctx, store.AddListImportErrorsParams{WorkspaceID: ws, ImportID: imp.ID, Rows: refused.rows, Emails: refused.emails, Reasons: refused.reasons})
}

// fillContactAttributes adds the keys a contact does not have yet; an import never overwrites what
// identity tokens or members set.
func (s *Server) fillContactAttributes(ctx context.Context, q *store.Queries, ws, contactID uuid.UUID, attrs map[string]any) error {
	c, err := q.LockContact(ctx, store.LockContactParams{WorkspaceID: ws, ID: contactID})
	if err != nil {
		return err
	}
	have := map[string]any{}
	_ = json.Unmarshal(c.Attributes, &have)
	changed := false
	for k, v := range attrs {
		if _, ok := have[k]; !ok {
			have[k], changed = v, true
		}
	}
	merged := mustJSON(have)
	if !changed || len(merged) > maxAttributesBytes {
		return nil
	}
	_, err = q.SetContactIdentity(ctx, store.SetContactIdentityParams{WorkspaceID: ws, ID: c.ID, Name: c.Name, Locale: c.Locale, Attributes: merged, Now: s.now()})
	return err
}

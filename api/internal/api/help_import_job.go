package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/mail/images"
	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	helpImportBudget   = 25 * time.Second
	helpImportAttempts = 10
	helpImportLinkPage = 50
	helpImportOrigin   = "helpscout-docs"

	helpImportItemArticle    = "article"
	helpImportItemCollection = "collection"
)

var errHelpImportLimit = errors.New("the workspace has as many help articles as the server allows")

type HelpImportArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	ImportID    uuid.UUID `json:"import_id"`
}

func (HelpImportArgs) Kind() string { return "help_import" }

// helpImportOpts queues without uniqueness: every slice locks nothing but continues from the
// checkpoint, and every write is keyed by the source's ids, so a second job only shares the work.
func helpImportOpts() *river.InsertOpts { return &river.InsertOpts{MaxAttempts: helpImportAttempts} }

type helpImportWorker struct {
	river.WorkerDefaults[HelpImportArgs]
	s *Server
}

func (w *helpImportWorker) Timeout(*river.Job[HelpImportArgs]) time.Duration {
	return helpImportBudget + 90*time.Second
}

func (w *helpImportWorker) Work(ctx context.Context, job *river.Job[HelpImportArgs]) error {
	wait, done, err := w.s.RunHelpImport(ctx, job.Args, helpImportBudget)
	if err != nil {
		if job.Attempt < job.MaxAttempts {
			return err
		}
		w.s.log.ErrorContext(ctx, "help import", slog.String("import_id", job.Args.ImportID.String()), slog.Any("error", err))
		return w.s.failHelpImport(ctx, job.Args.WorkspaceID, job.Args.ImportID, "the import stopped on a server error")
	}
	if !done {
		return river.JobSnooze(max(wait, 0))
	}
	return nil
}

type helpImportLocale struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type helpImportCheckpoint struct {
	Locales    []helpImportLocale     `json:"locales,omitempty"`
	Locale     int                    `json:"locale,omitempty"`
	Cursor     helpdesk.ArticleCursor `json:"cursor"`
	Budget     *helpdesk.Budget       `json:"budget,omitempty"`
	LinkSource string                 `json:"link_source,omitempty"`
	LinkLocale string                 `json:"link_locale,omitempty"`
}

type helpImportRun struct {
	s        *Server
	imp      store.HelpImport
	center   store.HelpCenter
	p        principal
	opts     helpImportOptions
	cp       helpImportCheckpoint
	counts   oas.HelpImportCounts
	skipped  []string
	origin   string
	src      helpdesk.ArticleSource
	budget   *helpdesk.Budget
	saveDesk func()
	deadline time.Time
	images   map[string]string
	stopped  bool
}

type helpImportFatal string

func (e helpImportFatal) Error() string { return string(e) }

func (s *Server) failHelpImport(ctx context.Context, ws, id uuid.UUID, reason string) error {
	var imp store.HelpImport
	failed := false
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		var err error
		imp, err = q.FinishHelpImport(ctx, store.FinishHelpImportParams{WorkspaceID: ws, ID: id, State: helpImportFailed, Error: new(truncateRunes(reason, 2000)), Now: new(s.now())})
		if store.IsNotFound(err) {
			return nil
		}
		failed = err == nil
		return err
	})
	if err != nil || !failed {
		return err
	}
	if c, err := s.st.GetHelpCenter(ctx, store.GetHelpCenterParams{WorkspaceID: ws, ID: imp.CenterID}); err == nil {
		s.helpImportSignal(ctx, c, imp)
	}
	return nil
}

// RunHelpImport works on an import for about budget. It returns done when the job may end, else
// the time to wait for the source's rate limit (zero to continue at once).
func (s *Server) RunHelpImport(ctx context.Context, a HelpImportArgs, budget time.Duration) (time.Duration, bool, error) {
	if live, err := s.workspaceLive(ctx, a.WorkspaceID); err != nil || !live {
		return 0, true, err
	}
	imp, err := s.st.StartHelpImportRun(ctx, store.StartHelpImportRunParams{WorkspaceID: a.WorkspaceID, ID: a.ImportID, Now: s.now()})
	if store.IsNotFound(err) {
		return 0, true, nil
	}
	if err != nil {
		return 0, false, err
	}
	center, err := s.st.GetHelpCenter(ctx, store.GetHelpCenterParams{WorkspaceID: a.WorkspaceID, ID: imp.CenterID})
	if store.IsNotFound(err) {
		return 0, true, nil
	}
	if err != nil {
		return 0, false, err
	}
	r := &helpImportRun{
		s: s, imp: imp, center: center, opts: readHelpImportOptions(imp.Options), counts: helpImportCounts(imp.Counts),
		skipped: imp.SkippedLocales, origin: imp.Origin, deadline: time.Now().Add(budget), images: map[string]string{},
		p: principal{workspaceID: imp.WorkspaceID},
	}
	if imp.ApiKeyID != nil {
		r.p.keyID = *imp.ApiKeyID
	} else if imp.MemberID != nil {
		r.p.memberID = *imp.MemberID
	}
	_ = json.Unmarshal(imp.Checkpoint, &r.cp)
	if imp.Phase == string(oas.HelpImportPhaseSource) {
		s.helpImportSignal(ctx, center, imp)
	}
	defer func() {
		if r.saveDesk != nil {
			r.saveDesk()
		}
	}()
	done, err := r.work(ctx)
	if err == nil {
		return 0, done, nil
	}
	var fatal helpImportFatal
	if errors.As(err, &fatal) {
		return 0, true, s.failHelpImport(ctx, imp.WorkspaceID, imp.ID, string(fatal))
	}
	if errors.Is(err, errHelpImportLimit) {
		return 0, true, s.failHelpImport(ctx, imp.WorkspaceID, imp.ID, "The workspace reached the number of help articles the server allows (YUVA_HELP_MAX_ARTICLES). Raise the limit or delete articles, then start the import again; it continues where it stopped.")
	}
	if wait, ok := r.sourceWait(err); ok {
		return wait, false, nil
	}
	if msg := r.sourceFatal(err); msg != "" {
		return 0, true, s.failHelpImport(ctx, imp.WorkspaceID, imp.ID, msg)
	}
	return 0, false, err
}

func (r *helpImportRun) sourceWait(err error) (time.Duration, bool) {
	var limited *helpdesk.LimitError
	var wait *helpdesk.WaitError
	switch {
	case errors.As(err, &limited):
		return max(limited.Until.Sub(r.s.now()), time.Second), true
	case errors.As(err, &wait):
		return max(wait.Until.Sub(r.s.now()), time.Second), true
	}
	return 0, false
}

func (r *helpImportRun) sourceFatal(err error) string {
	var auth *helpdesk.AuthError
	var token *helpdesk.TokenError
	var status *helpdesk.StatusError
	var refused *helpdesk.RefusedError
	switch {
	case errors.As(err, &auth), errors.As(err, &token):
		if r.imp.Source == helpImportSourceHelpScout {
			return "Help Scout refused the Docs API key. Check the key under Docs, Your Profile, API Keys, then start the import again."
		}
		return "Zendesk no longer accepts the sign-in of the migration. Sign in again on the migration, then start the import again."
	case errors.As(err, &status):
		return "The source answered " + strconv.Itoa(status.Status) + " for " + status.Path + ". For Zendesk the Guide must be set up and the signed-in user must be allowed to read it; for Help Scout the key must belong to a user of Docs."
	case errors.As(err, &refused):
		return "Yuva kept a request from leaving the server because it was not a GET to an allowed host; nothing was changed at the source."
	}
	return ""
}

// open connects to the source: a Zendesk import reuses the migration's sealed sign-in, refreshed
// when it is about to expire; a Help Scout import unseals its own Docs API key.
func (r *helpImportRun) open(ctx context.Context) error {
	s := r.s
	switch r.imp.Source {
	case helpImportSourceZendesk:
		if r.imp.MigrationID == nil {
			return helpImportFatal("The migration this import used is gone; start a new import from another Zendesk migration.")
		}
		m, err := s.st.GetMigration(ctx, store.GetMigrationParams{WorkspaceID: r.imp.WorkspaceID, ID: *r.imp.MigrationID})
		if store.IsNotFound(err) {
			return helpImportFatal("The migration this import used is gone; start a new import from another Zendesk migration.")
		}
		if err != nil {
			return err
		}
		budget := s.migrationBudget(m)
		r.budget = budget
		r.saveDesk = func() { s.saveBudget(context.WithoutCancel(ctx), m, budget) }
		src, sec, err := s.openHelpdesk(ctx, m, budget)
		if err != nil {
			return err
		}
		as, ok := src.(helpdesk.ArticleSource)
		if !ok {
			return helpImportFatal("This help desk has no articles to import.")
		}
		r.src = as
		if sec.Subdomain == "" {
			return helpImportFatal("The migration holds no Zendesk account.")
		}
		r.origin = helpdesk.ZendeskHost(sec.Subdomain)
	case helpImportSourceHelpScout:
		if r.imp.Sealed == nil {
			return helpImportFatal("The Docs API key of this import is gone; start the import again.")
		}
		key, err := s.secrets.Open(r.imp.Sealed, helpImportSecretContext(r.imp.WorkspaceID, r.imp.ID))
		if err != nil {
			return err
		}
		r.budget = r.cp.Budget
		if r.budget == nil {
			r.budget = &helpdesk.Budget{}
		}
		r.src = helpdesk.NewHelpScoutDocs(s.helpdeskConfig(), string(key), r.opts.Site, r.budget)
		r.origin = helpImportOrigin
	default:
		return helpImportFatal("Unknown source.")
	}
	return nil
}

func (r *helpImportRun) save(ctx context.Context, phase string) error {
	if r.skipped == nil {
		r.skipped = []string{}
	}
	if r.imp.Source == helpImportSourceHelpScout {
		r.cp.Budget = r.budget
	}
	imp, err := r.s.st.SaveHelpImportProgress(ctx, store.SaveHelpImportProgressParams{
		WorkspaceID: r.imp.WorkspaceID, ID: r.imp.ID, Phase: phase, Checkpoint: mustJSON(r.cp), Counts: mustJSON(r.counts), Origin: r.origin,
		SkippedLocales: r.skipped, Now: r.s.now(),
	})
	if store.IsNotFound(err) {
		r.stopped = true
		return nil
	}
	if err != nil {
		return err
	}
	changed := imp.Phase != r.imp.Phase
	r.imp = imp
	if changed {
		r.s.helpImportSignal(ctx, r.center, imp)
	} else {
		r.s.helpImportProgress(ctx, r.center, imp)
	}
	return nil
}

func (r *helpImportRun) timeLeft() bool { return time.Now().Before(r.deadline) }

func (r *helpImportRun) work(ctx context.Context) (bool, error) {
	if err := r.open(ctx); err != nil {
		return false, err
	}
	for !r.stopped {
		var err error
		switch r.imp.Phase {
		case string(oas.HelpImportPhaseSource):
			err = r.phaseSource(ctx)
		case string(oas.HelpImportPhaseCollections):
			err = r.phaseCollections(ctx)
		case string(oas.HelpImportPhaseArticles):
			err = r.phaseArticles(ctx)
		case string(oas.HelpImportPhaseLinks):
			var finished bool
			if finished, err = r.phaseLinks(ctx); err == nil && finished {
				return true, r.finish(ctx)
			}
		default:
			return true, r.s.failHelpImport(ctx, r.imp.WorkspaceID, r.imp.ID, "unknown phase")
		}
		if err != nil {
			return false, err
		}
		if !r.stopped && !r.timeLeft() {
			return false, nil
		}
	}
	return true, nil
}

func (r *helpImportRun) finish(ctx context.Context) error {
	var imp store.HelpImport
	done := false
	err := r.s.st.InTx(ctx, func(q *store.Queries) error {
		var err error
		imp, err = q.FinishHelpImport(ctx, store.FinishHelpImportParams{WorkspaceID: r.imp.WorkspaceID, ID: r.imp.ID, State: helpImportDone, Now: new(r.s.now())})
		if store.IsNotFound(err) {
			return nil
		}
		done = err == nil
		return err
	})
	if err != nil || !done {
		return err
	}
	imp.Counts = mustJSON(r.counts)
	r.s.helpImportSignal(ctx, r.center, imp)
	return nil
}

// matchHelpLocale finds the center's locale for a source language: the same tag, else the same
// base language when the center offers exactly one tag of it.
func matchHelpLocale(source string, offered []string) (string, bool) {
	norm := strings.ToLower(strings.ReplaceAll(source, "_", "-"))
	for _, o := range offered {
		if strings.EqualFold(o, norm) {
			return o, true
		}
	}
	base, _, _ := strings.Cut(norm, "-")
	found, n := "", 0
	for _, o := range offered {
		if ob, _, _ := strings.Cut(strings.ToLower(o), "-"); ob == base {
			found, n = o, n+1
		}
	}
	return found, n == 1
}

func (r *helpImportRun) phaseSource(ctx context.Context) error {
	loc, err := r.src.HelpLocales(ctx)
	if err != nil {
		return err
	}
	r.cp.Locales, r.skipped = nil, nil
	if len(loc.All) == 0 {
		r.cp.Locales = []helpImportLocale{{Source: "", Target: r.opts.Locale}}
		if r.opts.Locale == "" {
			r.cp.Locales[0].Target = r.center.DefaultLocale
		}
	} else {
		used := map[string]bool{}
		all := slices.Clone(loc.All)
		if i := slices.Index(all, loc.Default); i > 0 {
			all[0], all[i] = all[i], all[0]
		}
		for _, l := range all {
			target, ok := matchHelpLocale(l, r.center.Locales)
			if !ok || used[target] {
				r.skipped = append(r.skipped, l)
				continue
			}
			used[target] = true
			r.cp.Locales = append(r.cp.Locales, helpImportLocale{Source: l, Target: target})
		}
	}
	if len(r.cp.Locales) == 0 {
		return helpImportFatal("None of the languages of the source (" + strings.Join(r.skipped, ", ") + ") is offered by the help center. Add one to the center's locales, then start the import again.")
	}
	r.cp.Locale, r.cp.Cursor = 0, helpdesk.ArticleCursor{}
	return r.save(ctx, string(oas.HelpImportPhaseCollections))
}

func truncateHelpText(v string, n int) string {
	return truncateRunes(strings.TrimSpace(strings.Join(strings.Fields(v), " ")), n)
}

func sumHash(parts ...string) []byte {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return h.Sum(nil)
}

func (r *helpImportRun) phaseCollections(ctx context.Context) error {
	lists := make([]map[string]helpdesk.HelpCollection, len(r.cp.Locales))
	var structure []helpdesk.HelpCollection
	for i, l := range r.cp.Locales {
		list, err := r.src.HelpCollections(ctx, l.Source)
		if err != nil {
			return err
		}
		if i == 0 {
			structure = helpdesk.FlattenHelpCollections(list)
		}
		lists[i] = make(map[string]helpdesk.HelpCollection, len(list))
		for _, c := range list {
			lists[i][c.ID] = c
		}
	}
	slices.SortStableFunc(structure, func(a, b helpdesk.HelpCollection) int {
		return boolInt(a.ParentID != "") - boolInt(b.ParentID != "")
	})
	for _, c := range structure {
		if err := r.importCollection(ctx, c, lists); err != nil {
			return err
		}
	}
	if err := r.s.inTx(ctx, r.imp.WorkspaceID, func(q *store.Queries, _ *eventBatch) error {
		return r.s.touchHelpCenter(ctx, q, r.center)
	}); err != nil {
		return err
	}
	r.cp.Locale, r.cp.Cursor = 0, helpdesk.ArticleCursor{}
	return r.save(ctx, string(oas.HelpImportPhaseArticles))
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (r *helpImportRun) collectionTarget(ctx context.Context, q *store.Queries, sourceID string) (*store.HelpCollection, error) {
	id, err := q.FindHelpImportTarget(ctx, store.FindHelpImportTargetParams{
		WorkspaceID: r.imp.WorkspaceID, CenterID: r.center.ID, Origin: r.origin, Kind: helpImportItemCollection, SourceID: sourceID,
	})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	col, err := q.GetHelpCollection(ctx, store.GetHelpCollectionParams{WorkspaceID: r.imp.WorkspaceID, CenterID: r.center.ID, ID: id})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &col, nil
}

func (r *helpImportRun) importCollection(ctx context.Context, c helpdesk.HelpCollection, lists []map[string]helpdesk.HelpCollection) error {
	ws := r.imp.WorkspaceID
	skipped := false
	err := r.s.inTx(ctx, ws, func(q *store.Queries, _ *eventBatch) error {
		skipped = false
		if _, err := q.LockHelpCenter(ctx, store.LockHelpCenterParams{WorkspaceID: ws, ID: r.center.ID}); err != nil {
			return err
		}
		col, err := r.collectionTarget(ctx, q, c.ID)
		if err != nil {
			return err
		}
		if col == nil {
			n, err := q.CountHelpCollections(ctx, store.CountHelpCollectionsParams{WorkspaceID: ws, CenterID: r.center.ID})
			if err != nil {
				return err
			}
			if n >= maxHelpCollections {
				skipped = true
				return nil
			}
			var parent *uuid.UUID
			if c.ParentID != "" {
				if p, err := r.collectionTarget(ctx, q, c.ParentID); err != nil {
					return err
				} else if p != nil && p.ParentID == nil {
					parent = &p.ID
				}
			}
			number, err := q.TakeHelpCollectionNumber(ctx, store.TakeHelpCollectionNumberParams{WorkspaceID: ws, ID: r.center.ID})
			if err != nil {
				return err
			}
			created, err := q.CreateHelpCollection(ctx, store.CreateHelpCollectionParams{
				ID: newID(), WorkspaceID: ws, CenterID: r.center.ID, ParentID: parent, Number: number, Position: int32(max(c.Position, 0)), Now: r.s.now(),
			})
			if err != nil {
				return err
			}
			col = &created
		}
		current, err := q.ListHelpCollectionTranslations(ctx, store.ListHelpCollectionTranslationsParams{WorkspaceID: ws, CollectionIds: []uuid.UUID{col.ID}})
		if err != nil {
			return err
		}
		for i, l := range r.cp.Locales {
			lc, ok := lists[i][c.ID]
			if !ok {
				continue
			}
			if i == 0 {
				lc.Name = c.Name
			}
			name := truncateHelpText(lc.Name, 100)
			if name == "" {
				name = "Untitled"
			}
			desc := truncateHelpText(lc.Description, 500)
			hash := sumHash(name, desc)
			item, err := q.GetHelpImportItem(ctx, store.GetHelpImportItemParams{
				WorkspaceID: ws, CenterID: r.center.ID, Origin: r.origin, Kind: helpImportItemCollection, SourceID: c.ID, Locale: l.Target,
			})
			if err != nil && !store.IsNotFound(err) {
				return err
			}
			had := err == nil
			var cur *store.HelpCollectionTranslation
			for k := range current {
				if current[k].Locale == l.Target {
					cur = &current[k]
				}
			}
			keep := false
			if cur != nil && !r.opts.OverwriteEdited {
				keep = !had || !bytes.Equal(sumHash(cur.Name, cur.Description), item.ContentHash)
			}
			stored := hash
			if keep {
				r.counts.TranslationsKept++
				stored = item.ContentHash
			} else if err := q.UpsertHelpCollectionTranslation(ctx, store.UpsertHelpCollectionTranslationParams{
				WorkspaceID: ws, CollectionID: col.ID, Locale: l.Target, Name: name, Description: desc, Slug: helpSlug(name, "collection"), Now: r.s.now(),
			}); err != nil {
				return err
			}
			if err := q.UpsertHelpImportItem(ctx, store.UpsertHelpImportItemParams{
				WorkspaceID: ws, CenterID: r.center.ID, Origin: r.origin, Kind: helpImportItemCollection, SourceID: c.ID, Locale: l.Target, TargetID: col.ID,
				OldUrl: truncateRunes(lc.URL, 2000), ContentHash: stored, ImportID: r.imp.ID, Now: r.s.now(),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if skipped {
		r.counts.Failed++
	} else {
		r.counts.Collections++
	}
	return nil
}

func (r *helpImportRun) phaseArticles(ctx context.Context) error {
	for r.cp.Locale < len(r.cp.Locales) {
		l := r.cp.Locales[r.cp.Locale]
		page, err := r.src.HelpArticles(ctx, l.Source, r.cp.Cursor)
		if err != nil {
			return err
		}
		for _, a := range page.Articles {
			if err := r.importArticle(ctx, a, l); err != nil {
				return err
			}
			if r.stopped {
				return nil
			}
		}
		r.cp.Cursor = page.Next
		if page.End {
			r.cp.Locale++
			r.cp.Cursor = helpdesk.ArticleCursor{}
		}
		if err := r.save(ctx, string(oas.HelpImportPhaseArticles)); err != nil || r.stopped {
			return err
		}
		if !r.timeLeft() {
			return nil
		}
	}
	return r.save(ctx, string(oas.HelpImportPhaseLinks))
}

type helpImportAction int

const (
	helpActWrite helpImportAction = iota
	helpActUnchanged
	helpActKept
)

// decideHelpImport tells what a re-run does with a translation: write it, leave it because it is
// as the last import wrote it, or keep a draft somebody edited in Yuva since.
func decideHelpImport(item *store.HelpImportItem, tr *store.HelpArticleTranslation, hash []byte, overwrite bool) helpImportAction {
	switch {
	case tr == nil:
		return helpActWrite
	case item == nil || tr.Version != item.Version:
		if overwrite {
			return helpActWrite
		}
		return helpActKept
	case bytes.Equal(item.ContentHash, hash):
		return helpActUnchanged
	}
	return helpActWrite
}

func (r *helpImportRun) articleTarget(ctx context.Context, q *store.Queries, sourceID string) (*store.HelpArticle, error) {
	id, err := q.FindHelpImportTarget(ctx, store.FindHelpImportTargetParams{
		WorkspaceID: r.imp.WorkspaceID, CenterID: r.center.ID, Origin: r.origin, Kind: helpImportItemArticle, SourceID: sourceID,
	})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a, err := q.GetHelpArticle(ctx, store.GetHelpArticleParams{WorkspaceID: r.imp.WorkspaceID, CenterID: r.center.ID, ID: id})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *helpImportRun) readState(ctx context.Context, q *store.Queries, a helpdesk.HelpArticle, locale string) (*store.HelpImportItem, *store.HelpArticleTranslation, error) {
	var item *store.HelpImportItem
	it, err := q.GetHelpImportItem(ctx, store.GetHelpImportItemParams{
		WorkspaceID: r.imp.WorkspaceID, CenterID: r.center.ID, Origin: r.origin, Kind: helpImportItemArticle, SourceID: a.ID, Locale: locale,
	})
	if err == nil {
		item = &it
	} else if !store.IsNotFound(err) {
		return nil, nil, err
	}
	art, err := r.articleTarget(ctx, q, a.ID)
	if err != nil || art == nil {
		return item, nil, err
	}
	tr, err := q.GetHelpTranslation(ctx, store.GetHelpTranslationParams{WorkspaceID: r.imp.WorkspaceID, ArticleID: art.ID, Locale: locale})
	if store.IsNotFound(err) {
		return item, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return item, &tr, nil
}

// keepSeen records that the import saw an article it leaves alone, so its old URL still reaches
// the report; an unknown translation is remembered as somebody else's (version -1).
func (r *helpImportRun) keepSeen(ctx context.Context, q *store.Queries, a helpdesk.HelpArticle, locale string, item *store.HelpImportItem, tr *store.HelpArticleTranslation) error {
	if item != nil {
		return q.SeeHelpImportItem(ctx, store.SeeHelpImportItemParams{
			WorkspaceID: r.imp.WorkspaceID, CenterID: r.center.ID, Origin: r.origin, Kind: helpImportItemArticle, SourceID: a.ID, Locale: locale,
			ImportID: r.imp.ID, OldUrl: truncateRunes(a.URL, 2000), LinkKey: a.LinkKey, Now: r.s.now(),
		})
	}
	if tr == nil {
		return nil
	}
	return q.UpsertHelpImportItem(ctx, store.UpsertHelpImportItemParams{
		WorkspaceID: r.imp.WorkspaceID, CenterID: r.center.ID, Origin: r.origin, Kind: helpImportItemArticle, SourceID: a.ID, Locale: locale, LinkKey: a.LinkKey,
		TargetID: tr.ArticleID, OldUrl: truncateRunes(a.URL, 2000), Version: -1, ImportID: r.imp.ID, Now: r.s.now(),
	})
}

func (r *helpImportRun) count(act helpImportAction) {
	switch act {
	case helpActUnchanged:
		r.counts.TranslationsUnchanged++
	case helpActKept:
		r.counts.TranslationsKept++
	}
}

// image fetches an image of an article and stores it as the workspace's image; "" means it stayed
// a link to its original.
func (r *helpImportRun) image(ctx context.Context, raw string) (string, error) {
	if v, ok := r.images[raw]; ok {
		return v, nil
	}
	fail := func() (string, error) {
		r.counts.ImagesFailed++
		r.images[raw] = ""
		return "", nil
	}
	data, err := r.src.HelpImage(ctx, raw)
	if err != nil {
		var wait *helpdesk.WaitError
		if errors.As(err, &wait) || ctx.Err() != nil {
			return "", err
		}
		return fail()
	}
	if len(data) > images.MaxUploadBytes {
		return fail()
	}
	name := "image"
	if u, err := url.Parse(raw); err == nil {
		if b := path.Base(u.Path); b != "" && b != "." && b != "/" {
			name = b
		}
	}
	var row store.EmailImage
	err = r.s.st.InTx(ctx, func(q *store.Queries) error {
		var err error
		row, _, err = r.s.storeEmailImage(ctx, q, r.p, data, name)
		return err
	})
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return fail()
	}
	if err != nil {
		return "", err
	}
	u := r.s.imageURL(row.Token)
	r.images[raw] = u
	r.counts.ImagesStored++
	return u, nil
}

func (r *helpImportRun) importArticle(ctx context.Context, a helpdesk.HelpArticle, l helpImportLocale) error {
	ws := r.imp.WorkspaceID
	r.counts.Articles++
	title := truncateHelpText(a.Title, 200)
	if utf8.RuneCountInString(title) < 3 {
		title = strings.TrimSpace(title + " article")
	}
	keywords := truncateRunes(strings.Join(a.Keywords, ", "), maxHelpKeywords)
	hash := sumHash(title, a.HTML, keywords, a.CollectionID, strconv.Itoa(a.Position), l.Target)

	item, tr, err := r.readState(ctx, r.s.st.Queries, a, l.Target)
	if err != nil {
		return err
	}
	if act := decideHelpImport(item, tr, hash, r.opts.OverwriteEdited); act != helpActWrite {
		if err := r.keepSeen(ctx, r.s.st.Queries, a, l.Target, item, tr); err != nil {
			return err
		}
		r.count(act)
		return nil
	}

	base := r.src.HelpBase()
	stored := map[string]string{}
	for _, raw := range helpdesk.HTMLImages(a.HTML, base) {
		u, err := r.image(ctx, raw)
		if err != nil {
			return err
		}
		if u != "" {
			stored[raw] = u
		}
	}
	md := helpdesk.HTMLToMarkdown(a.HTML, helpdesk.Rewrite{Base: base, Image: stored})
	md, cut := helpdesk.CutMarkdown(md, maxHelpBody)
	d, err := helpDraftFields(title, &md, &keywords, nil)
	if err != nil {
		r.counts.Failed++
		return nil
	}
	pending := len(helpdesk.HTMLLinks(a.HTML, base)) > 0

	var (
		saved   store.HelpArticleTranslation
		act     helpImportAction
		created bool
	)
	err = r.s.inTx(ctx, ws, func(q *store.Queries, _ *eventBatch) error {
		created, act = false, helpActWrite
		if _, err := q.LockHelpCenter(ctx, store.LockHelpCenterParams{WorkspaceID: ws, ID: r.center.ID}); err != nil {
			return err
		}
		item, tr, err := r.readState(ctx, q, a, l.Target)
		if err != nil {
			return err
		}
		if act = decideHelpImport(item, tr, hash, r.opts.OverwriteEdited); act != helpActWrite {
			return r.keepSeen(ctx, q, a, l.Target, item, tr)
		}
		var colID *uuid.UUID
		if a.CollectionID != "" {
			col, err := r.collectionTarget(ctx, q, a.CollectionID)
			if err != nil {
				return err
			}
			if col != nil {
				colID = &col.ID
			}
		}
		art, err := r.articleTarget(ctx, q, a.ID)
		if err != nil {
			return err
		}
		member, key := helpCreatedBy(r.p)
		now := r.s.now()
		position := int32(max(a.Position, 0))
		if art == nil {
			if maxArticles := r.s.help.MaxArticles; maxArticles > 0 {
				if err := q.LockHelpArticleQuota(ctx, ws); err != nil {
					return err
				}
				n, err := q.CountWorkspaceHelpArticles(ctx, ws)
				if err != nil {
					return err
				}
				if int(n) >= maxArticles {
					return errHelpImportLimit
				}
			}
			number, err := q.TakeHelpArticleNumber(ctx, store.TakeHelpArticleNumberParams{WorkspaceID: ws, ID: r.center.ID})
			if err != nil {
				return err
			}
			fresh, err := q.CreateHelpArticle(ctx, store.CreateHelpArticleParams{
				ID: newID(), WorkspaceID: ws, CenterID: r.center.ID, CollectionID: colID, Number: number, Position: position, Paths: []string{},
				MemberID: member, ApiKeyID: key, Now: now,
			})
			if err != nil {
				return err
			}
			art = &fresh
		} else if colID != nil && (art.CollectionID == nil || *art.CollectionID != *colID) || art.Position != position {
			target := art.CollectionID
			if colID != nil {
				target = colID
			}
			if _, err := q.UpdateHelpArticle(ctx, store.UpdateHelpArticleParams{
				WorkspaceID: ws, ID: art.ID, CollectionID: target, Position: position, Paths: art.Paths, Now: now,
			}); err != nil {
				return err
			}
		}
		imgs, err := r.s.helpImages(ctx, q, ws, d.body)
		if err != nil {
			return err
		}
		if tr == nil {
			created = true
			saved, err = q.CreateHelpTranslation(ctx, store.CreateHelpTranslationParams{
				WorkspaceID: ws, ArticleID: art.ID, Locale: l.Target, Title: d.title, Body: d.body, Keywords: d.keywords, Slug: d.slug,
				ImageIds: imgs.ids(), SearchConfig: helpSearchConfig(l.Target), MemberID: member, ApiKeyID: key, Now: now,
			})
		} else {
			var locked store.HelpArticleTranslation
			if locked, err = lockedHelpTranslation(ctx, q, *art, l.Target); err == nil {
				saved, err = r.s.saveHelpDraft(ctx, q, r.p, locked, d)
			}
		}
		if err != nil {
			return err
		}
		if err := r.s.addHelpRevision(ctx, q, r.p, saved, oas.HelpRevisionSave, saved.Title, saved.Body, saved.Keywords); err != nil {
			return err
		}
		if err := q.TouchHelpArticle(ctx, store.TouchHelpArticleParams{WorkspaceID: ws, ID: art.ID, Now: now}); err != nil {
			return err
		}
		return q.UpsertHelpImportItem(ctx, store.UpsertHelpImportItemParams{
			WorkspaceID: ws, CenterID: r.center.ID, Origin: r.origin, Kind: helpImportItemArticle, SourceID: a.ID, Locale: l.Target, LinkKey: a.LinkKey,
			TargetID: art.ID, OldUrl: truncateRunes(a.URL, 2000), Version: saved.Version, ContentHash: hash, ImportID: r.imp.ID, LinksPending: pending, Now: now,
		})
	})
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		r.counts.Failed++
		return nil
	}
	if err != nil {
		return err
	}
	if act != helpActWrite {
		r.count(act)
		return nil
	}
	if cut {
		r.counts.Truncated++
	}
	if created {
		r.counts.TranslationsCreated++
	} else {
		r.counts.TranslationsUpdated++
	}
	r.s.helpArticleSignal(ctx, r.p, r.center, saved)
	return nil
}

var markdownDest = regexp.MustCompile(`\]\(([^()\s]+)\)`)

func (r *helpImportRun) phaseLinks(ctx context.Context) (bool, error) {
	for {
		rows, err := r.s.st.ListHelpImportPendingLinks(ctx, store.ListHelpImportPendingLinksParams{
			WorkspaceID: r.imp.WorkspaceID, ImportID: r.imp.ID, AfterSource: r.cp.LinkSource, AfterLocale: r.cp.LinkLocale, Lim: helpImportLinkPage,
		})
		if err != nil {
			return false, err
		}
		for _, it := range rows {
			if err := r.rewriteLinks(ctx, it); err != nil {
				return false, err
			}
			if r.stopped {
				return false, nil
			}
		}
		if len(rows) < helpImportLinkPage {
			return true, r.save(ctx, string(oas.HelpImportPhaseLinks))
		}
		last := rows[len(rows)-1]
		r.cp.LinkSource, r.cp.LinkLocale = last.SourceID, last.Locale
		if err := r.save(ctx, string(oas.HelpImportPhaseLinks)); err != nil || r.stopped {
			return false, err
		}
		if !r.timeLeft() {
			return false, nil
		}
	}
}

// resolveLink finds where an imported article now lives: by its old URL, else by the id the link
// names. The language is the link's, else the article it sits in, else the center's default.
func (r *helpImportRun) resolveLink(ctx context.Context, dest, locale string) (string, bool, error) {
	clean, fragment, _ := strings.Cut(dest, "#")
	if i := strings.IndexByte(clean, '?'); i >= 0 {
		clean = clean[:i]
	}
	key, linkLocale, _ := r.src.HelpLink(clean)
	if key == "" {
		key = "\x01"
	}
	rows, err := r.s.st.FindHelpImportArticlePages(ctx, store.FindHelpImportArticlePagesParams{
		WorkspaceID: r.imp.WorkspaceID, CenterID: r.center.ID, Origin: r.origin, LinkKey: key, OldUrl: clean,
	})
	if err != nil || len(rows) == 0 {
		return "", false, err
	}
	want := []string{locale, r.center.DefaultLocale}
	for _, l := range r.cp.Locales {
		if linkLocale != "" && strings.EqualFold(l.Source, linkLocale) {
			want = append([]string{l.Target}, want...)
		}
	}
	pick := rows[0]
	for i := len(want) - 1; i >= 0; i-- {
		for _, row := range rows {
			if row.Locale == want[i] {
				pick = row
			}
		}
	}
	out := r.s.helpArticleURL(r.center, pick.Number, pick.Locale, pick.Slug)
	if fragment != "" {
		out += "#" + fragment
	}
	return out, true, nil
}

func (r *helpImportRun) rewriteLinks(ctx context.Context, it store.HelpImportItem) error {
	ws := r.imp.WorkspaceID
	linked := func(version int32) error {
		return r.s.st.SetHelpImportItemLinked(ctx, store.SetHelpImportItemLinkedParams{
			WorkspaceID: ws, CenterID: r.center.ID, Origin: r.origin, SourceID: it.SourceID, Locale: it.Locale, Version: version, Now: r.s.now(),
		})
	}
	tr, err := r.s.st.GetHelpTranslation(ctx, store.GetHelpTranslationParams{WorkspaceID: ws, ArticleID: it.TargetID, Locale: it.Locale})
	if store.IsNotFound(err) {
		return linked(it.Version)
	}
	if err != nil {
		return err
	}
	if tr.Version != it.Version {
		return linked(it.Version)
	}
	var resolveErr error
	n := 0
	body := markdownDest.ReplaceAllStringFunc(tr.Body, func(m string) string {
		dest := markdownDest.FindStringSubmatch(m)[1]
		if !strings.HasPrefix(dest, "http://") && !strings.HasPrefix(dest, "https://") {
			return m
		}
		next, ok, err := r.resolveLink(ctx, dest, it.Locale)
		if err != nil {
			resolveErr = err
			return m
		}
		if !ok || next == dest {
			return m
		}
		n++
		return "](" + escapeMarkdownDest(next) + ")"
	})
	if resolveErr != nil {
		return resolveErr
	}
	if n == 0 {
		return linked(it.Version)
	}
	var saved store.HelpArticleTranslation
	written := false
	err = r.s.inTx(ctx, ws, func(q *store.Queries, _ *eventBatch) error {
		written = false
		cur, err := q.LockHelpTranslation(ctx, store.LockHelpTranslationParams{WorkspaceID: ws, ArticleID: it.TargetID, Locale: it.Locale})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if cur.Version != it.Version {
			return nil
		}
		saved, err = r.s.saveHelpDraft(ctx, q, r.p, cur, helpDraft{title: cur.Title, body: body, keywords: cur.Keywords, slug: cur.Slug})
		if err != nil {
			return err
		}
		written = true
		return r.s.addHelpRevision(ctx, q, r.p, saved, oas.HelpRevisionSave, saved.Title, saved.Body, saved.Keywords)
	})
	if err != nil {
		return err
	}
	if !written {
		return linked(it.Version)
	}
	r.counts.LinksRewritten += int32(n)
	r.s.helpArticleSignal(ctx, r.p, r.center, saved)
	return linked(saved.Version)
}

func escapeMarkdownDest(s string) string {
	return strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29").Replace(s)
}

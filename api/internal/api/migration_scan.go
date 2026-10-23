package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"sort"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/email/reply"
	"github.com/productdevbook/yuva/api/internal/migrate"
	"github.com/productdevbook/yuva/api/internal/migrate/imapread"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	scanPhaseList    = "list"
	scanPhaseHeaders = "headers"
	scanPhasePlan    = "plan"
	scanPhasePreview = "preview"

	scanHeaderPage      = 1000
	scanHeaderBytes     = 600
	planBatch           = 5000
	previewThreads      = 5
	previewMessages     = 20
	previewTextRunes    = 1000
	importMessagesPerS  = 20
	estimateMarginRatio = 1.1
	teamSuggestions     = 50
)

// sourceError marks a failure of the mailbox, as opposed to one of the database.
type sourceError struct{ err error }

func (e *sourceError) Error() string { return e.err.Error() }
func (e *sourceError) Unwrap() error { return e.err }

func fromSource(err error) error {
	if err == nil {
		return nil
	}
	return &sourceError{err}
}

// waitError asks the run to wait for the daily budget, or for what reason says.
type waitError struct {
	until  time.Time
	reason string
}

func (e *waitError) Error() string { return "waiting for the daily budget" }

type migrationEstimateDoc struct {
	DryRun          oas.MigrationEstimate         `json:"dry_run"`
	Preview         *oas.MigrationPreview         `json:"preview,omitempty"`
	TeamSuggestions []oas.MigrationTeamSuggestion `json:"team_suggestions,omitempty"`
}

func readEstimate(raw []byte) *migrationEstimateDoc {
	if len(raw) == 0 {
		return nil
	}
	var d migrationEstimateDoc
	if json.Unmarshal(raw, &d) != nil || d.DryRun.ScannedAt.IsZero() {
		return nil
	}
	return &d
}

func gmailPreset(p migrate.Preset) bool {
	return p.ID == migrate.PresetGmail || p.ID == migrate.PresetGoogleWorkspace
}

type listedFolder struct {
	Name        string
	Delim       string
	Kind        string
	Messages    int64
	UIDValidity *int64
	UIDNext     int64
}

// folderLabel is what a folder's conversations are labelled with under the options.
func folderLabel(ctx context.Context, q *store.Queries, ws uuid.UUID, opts oas.MigrationOptions, name, kind string) (string, *uuid.UUID, error) {
	for _, l := range opts.Labels {
		if l.Folder != name {
			continue
		}
		switch l.Action {
		case oas.MigrationLabelExisting:
			if l.LabelId == nil {
				return string(oas.FolderLabelNone), nil, nil
			}
			if _, err := q.GetLabel(ctx, store.GetLabelParams{WorkspaceID: ws, ID: *l.LabelId}); store.IsNotFound(err) {
				return string(oas.FolderLabelNone), nil, nil
			} else if err != nil {
				return "", nil, err
			}
			return string(oas.FolderLabelExisting), l.LabelId, nil
		case oas.MigrationLabelCreate:
			return string(oas.FolderLabelCreate), nil, nil
		}
		return string(oas.FolderLabelNone), nil, nil
	}
	if kind == migrate.KindUser || kind == migrate.KindLabel {
		return string(oas.FolderLabelCreate), nil, nil
	}
	return string(oas.FolderLabelNone), nil, nil
}

// recordFolders stores the folders a mailbox lists with what the options make of them. A folder
// whose UIDVALIDITY changed loses what was scanned of it; a folder that is gone goes.
func (s *Server) recordFolders(ctx context.Context, q *store.Queries, m store.Migration, listed []listedFolder) error {
	opts := migrationOptions(m.Options)
	preset := folderPreset(m)
	existing, err := q.ListMigrationFolders(ctx, store.ListMigrationFoldersParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID})
	if err != nil {
		return err
	}
	byName := map[string]store.MigrationFolder{}
	next := int32(1)
	for _, f := range existing {
		byName[f.Name] = f
		next = max(next, f.ID+1)
	}
	seen := map[string]bool{}
	for _, l := range listed {
		if seen[l.Name] || len([]rune(l.Name)) > 1000 || l.Name == "" {
			continue
		}
		seen[l.Name] = true
		selected := migrate.Readable(l.Kind)
		if selected {
			if opts.Folders == nil {
				selected = migrate.DefaultSelected(preset, l.Kind)
			} else {
				selected = slices.Contains(*opts.Folders, l.Name)
			}
		}
		action, labelID, err := folderLabel(ctx, q, m.WorkspaceID, opts, l.Name, l.Kind)
		if err != nil {
			return err
		}
		cur, ok := byName[l.Name]
		id := cur.ID
		uidValidity := l.UIDValidity
		if !ok {
			id = next
			next++
		} else if cur.Uidvalidity != nil && l.UIDValidity != nil && *cur.Uidvalidity != *l.UIDValidity {
			if err := q.DeleteMigrationFolderItems(ctx, store.DeleteMigrationFolderItemsParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, FolderID: &id}); err != nil {
				return err
			}
			if err := q.ResetMigrationFolder(ctx, store.ResetMigrationFolderParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, ID: id, Uidvalidity: l.UIDValidity}); err != nil {
				return err
			}
		} else if cur.Uidvalidity != nil {
			uidValidity = nil
		}
		if err := q.UpsertMigrationFolder(ctx, store.UpsertMigrationFolderParams{
			WorkspaceID: m.WorkspaceID, MigrationID: m.ID, ID: id, Name: l.Name, Kind: l.Kind, Selected: selected,
			LabelAction: action, LabelID: labelID, Uidvalidity: uidValidity, Uidnext: l.UIDNext, Total: int32(min(l.Messages, math.MaxInt32)),
			Delim: l.Delim,
		}); err != nil {
			return err
		}
	}
	for _, f := range existing {
		if seen[f.Name] {
			continue
		}
		if err := q.DeleteMigrationFolderItems(ctx, store.DeleteMigrationFolderItemsParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, FolderID: &f.ID}); err != nil {
			return err
		}
		if err := q.DeleteMigrationFolder(ctx, store.DeleteMigrationFolderParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, ID: f.ID}); err != nil {
			return err
		}
	}
	return nil
}

// folderUse is import for a selected folder; labels for a Gmail label or the Sent folder while
// All Mail is read, so mail from All Mail gets its labels and the Sent folder's senders are known;
// else skip.
func folderUse(f store.MigrationFolder, folders []store.MigrationFolder) oas.MigrationFolderUse {
	if f.Selected {
		return oas.FolderUseImport
	}
	if f.Kind != migrate.KindSent && (f.Kind != migrate.KindLabel || f.LabelAction == string(oas.FolderLabelNone)) {
		return oas.FolderUseSkip
	}
	for _, o := range folders {
		if o.Selected && o.Kind == migrate.KindAll {
			return oas.FolderUseLabels
		}
	}
	return oas.FolderUseSkip
}

// labelName is the name of the label a folder gives its conversations, "" for none.
func labelName(ctx context.Context, q *store.Queries, f store.MigrationFolder) (string, error) {
	switch f.LabelAction {
	case string(oas.FolderLabelCreate):
		return migrate.LabelName(f.Name, f.Delim), nil
	case string(oas.FolderLabelExisting):
		if f.LabelID == nil {
			return "", nil
		}
		l, err := q.GetLabel(ctx, store.GetLabelParams{WorkspaceID: f.WorkspaceID, ID: *f.LabelID})
		if store.IsNotFound(err) {
			return "", nil
		}
		return l.Name, err
	}
	return "", nil
}

type migrationTeam struct {
	addrs   map[string]bool
	members map[string]uuid.UUID
}

// teamOf lists the addresses whose mail is the team's: the mailbox, the channel, the addresses the
// options name and the members' sign-in addresses, with the member each one is imported as.
func (s *Server) teamOf(ctx context.Context, q *store.Queries, m store.Migration) (migrationTeam, error) {
	opts := migrationOptions(m.Options)
	t := migrationTeam{addrs: map[string]bool{strings.ToLower(m.SourceAddress): true}, members: map[string]uuid.UUID{}}
	if m.ChannelID != nil {
		e, err := q.GetEmailChannel(ctx, store.GetEmailChannelParams{WorkspaceID: m.WorkspaceID, ChannelID: *m.ChannelID})
		if err != nil && !store.IsNotFound(err) {
			return t, err
		}
		if err == nil {
			t.addrs[strings.ToLower(e.Address)] = true
			if e.FromAddress != nil {
				t.addrs[strings.ToLower(*e.FromAddress)] = true
			}
		}
	}
	for _, a := range opts.TeamAddresses {
		t.addrs[strings.ToLower(string(a))] = true
	}
	members, err := q.ListMembers(ctx, m.WorkspaceID)
	if err != nil {
		return t, err
	}
	for _, mem := range members {
		addr := strings.ToLower(mem.Email)
		t.addrs[addr] = true
		t.members[addr] = mem.ID
	}
	for _, p := range opts.People {
		addr := strings.ToLower(string(p.Address))
		t.addrs[addr] = true
		if p.MemberId != nil {
			t.members[addr] = *p.MemberId
		} else {
			delete(t.members, addr)
		}
	}
	return t, nil
}

func (r *migrationRun) scan(ctx context.Context) (MigrationSlice, error) {
	cp := readCheckpoint(r.m.Checkpoint)
	for r.timeLeft() {
		ok, err := r.renew(ctx)
		if err != nil || !ok {
			return MigrationSlice{Done: true}, err
		}
		phase := cp.Phase
		switch phase {
		case scanPhaseHeaders:
			var more bool
			switch {
			case helpdeskMigration(r.m):
				more, err = r.scanHelpdesk(ctx)
			case fileMigration(r.m):
				more, err = r.scanFile(ctx)
			default:
				more, err = r.scanHeaders(ctx)
			}
			if err == nil && !more {
				phase = scanPhasePlan
			}
		case scanPhasePlan:
			var more bool
			plan := r.plan
			if helpdeskMigration(r.m) {
				plan = r.planHelpdesk
			}
			if more, err = plan(ctx); err == nil && !more {
				phase = scanPhasePreview
			}
		case scanPhasePreview:
			preview := r.preview
			if helpdeskMigration(r.m) {
				preview = r.previewHelpdesk
			}
			if err = preview(ctx); err == nil {
				return MigrationSlice{Done: true}, r.scanned(ctx)
			}
		default:
			switch {
			case helpdeskMigration(r.m):
				err = r.listHelpdeskFolders(ctx)
			case fileMigration(r.m):
				err = r.listFileFolders(ctx)
			default:
				err = r.listFolders(ctx)
			}
			if err == nil {
				phase = scanPhaseHeaders
			}
		}
		var wait *waitError
		var src *sourceError
		switch {
		case errors.As(err, &wait):
			return r.waitFor(ctx, wait)
		case errors.As(err, &src):
			return r.trouble(ctx, src.err)
		case err != nil:
			return MigrationSlice{}, err
		}
		if err := r.clearWaiting(ctx); err != nil {
			return MigrationSlice{}, err
		}
		if phase != cp.Phase {
			cp.Phase, cp.Plan = phase, nil
			r.m.Checkpoint = mustJSON(cp)
			if err := r.s.st.SetMigrationCheckpoint(ctx, store.SetMigrationCheckpointParams{WorkspaceID: r.m.WorkspaceID, ID: r.m.ID, Checkpoint: r.m.Checkpoint}); err != nil {
				return MigrationSlice{}, err
			}
		}
	}
	return MigrationSlice{}, nil
}

func (r *migrationRun) waitFor(ctx context.Context, wait *waitError) (MigrationSlice, error) {
	if err := r.saveThrottle(ctx); err != nil {
		return MigrationSlice{}, err
	}
	until, reason := wait.until, wait.reason
	if reason == "" {
		reason = reasonDaily
	}
	m, err := r.s.st.SetMigrationRunState(ctx, store.SetMigrationRunStateParams{
		WorkspaceID: r.m.WorkspaceID, ID: r.m.ID, State: r.m.State, StateReason: reason, WaitingUntil: &until, Now: r.s.now(),
	})
	if err != nil {
		return MigrationSlice{}, err
	}
	r.m = m
	r.s.migrationProgress(ctx, m)
	return MigrationSlice{Until: until}, nil
}

// budget reserves the next read within a daily budget.
func (r *migrationRun) budget(next int64) error {
	if until := r.throttle.Wait(r.preset.Budget, r.s.now(), next); !until.IsZero() {
		return &waitError{until: until}
	}
	return nil
}

func (r *migrationRun) listFolders(ctx context.Context) error {
	rd, err := r.source(ctx)
	if err != nil {
		return fromSource(err)
	}
	folders, err := rd.Folders(ctx)
	if err != nil {
		return fromSource(err)
	}
	gmail := gmailPreset(r.preset)
	var listed []listedFolder
	for _, f := range folders {
		if !f.Selectable() {
			continue
		}
		st, err := rd.Status(ctx, f.Name)
		var se *imapread.ServerError
		if errors.As(err, &se) && se.Type == "NO" {
			continue
		}
		if err != nil {
			return fromSource(err)
		}
		uv := int64(st.UIDValidity)
		listed = append(listed, listedFolder{Name: f.Name, Delim: f.Delim, Kind: migrate.FolderKind(f, gmail), Messages: int64(st.Messages),
			UIDValidity: &uv, UIDNext: int64(st.UIDNext)})
	}
	return r.s.st.InTx(ctx, func(q *store.Queries) error {
		return r.s.recordFolders(ctx, q, r.m, listed)
	})
}

// scanHeaders reads the headers of the folders in use, a page at a time, after the last UID read.
func (r *migrationRun) scanHeaders(ctx context.Context) (bool, error) {
	s := r.s
	folders, err := s.st.ListMigrationFolders(ctx, store.ListMigrationFoldersParams{WorkspaceID: r.m.WorkspaceID, MigrationID: r.m.ID})
	if err != nil {
		return false, err
	}
	for _, f := range folders {
		if folderUse(f, folders) == oas.FolderUseSkip || (f.Uidnext > 0 && f.ScannedUid >= f.Uidnext-1) {
			continue
		}
		rd, err := r.source(ctx)
		if err != nil {
			return false, fromSource(err)
		}
		st, err := rd.Examine(ctx, f.Name)
		if err != nil {
			return false, fromSource(err)
		}
		uv := int64(st.UIDValidity)
		if f.Uidvalidity != nil && *f.Uidvalidity != uv {
			err := s.st.InTx(ctx, func(q *store.Queries) error {
				if err := q.DeleteMigrationFolderItems(ctx, store.DeleteMigrationFolderItemsParams{WorkspaceID: f.WorkspaceID, MigrationID: f.MigrationID, FolderID: &f.ID}); err != nil {
					return err
				}
				return q.ResetMigrationFolder(ctx, store.ResetMigrationFolderParams{WorkspaceID: f.WorkspaceID, MigrationID: f.MigrationID, ID: f.ID, Uidvalidity: &uv})
			})
			if err != nil {
				return false, err
			}
			f.ScannedUid = 0
		}
		all, err := rd.SearchUIDs(ctx, time.Time{})
		if err != nil {
			return false, fromSource(err)
		}
		var uids []uint32
		for _, u := range all {
			if int64(u) > f.ScannedUid {
				uids = append(uids, u)
			}
		}
		slices.Sort(uids)
		for len(uids) > 0 {
			if !r.timeLeft() {
				return true, nil
			}
			if ok, err := r.renew(ctx); err != nil || !ok {
				return true, err
			}
			page := uids[:min(scanHeaderPage, len(uids))]
			uids = uids[len(page):]
			if err := r.budget(int64(len(page)) * scanHeaderBytes); err != nil {
				return true, err
			}
			if err := r.scanPage(ctx, rd, f, uint32(uv), page); err != nil {
				return true, err
			}
		}
		last := max(f.ScannedUid, int64(st.UIDNext)-1)
		if err := s.st.SetMigrationFolderUIDValidity(ctx, store.SetMigrationFolderUIDValidityParams{
			WorkspaceID: f.WorkspaceID, MigrationID: f.MigrationID, ID: f.ID, Uidvalidity: &uv, ScannedUid: last,
		}); err != nil {
			return false, err
		}
	}
	return false, nil
}

// headerItems reads the headers of messages of the open folder; a message without a Message-ID
// is read whole, so it gets the key live ingest stores for it.
func (r *migrationRun) headerItems(ctx context.Context, rd *sourceReader, folderID int32, uidValidity uint32, uids []uint32) ([]migrate.Item, int64, error) {
	now := r.s.now()
	var read int64
	var items []migrate.Item
	err := rd.FetchHeaders(ctx, uids, migrate.HeaderFields, func(h imapread.Header) error {
		read += int64(len(h.Header)) + 200
		items = append(items, migrate.ScanHeader(h, folderID, uidValidity, now))
		return nil
	})
	if err != nil {
		return nil, read, fromSource(err)
	}
	var noID []uint32
	byUID := map[uint32]int{}
	for i, it := range items {
		if it.Meta.ID == "" {
			_, _, uid, _ := migrate.ParseSourceID(it.SourceID)
			noID, byUID[uid] = append(noID, uid), i
		}
	}
	err = rd.FetchRaw(ctx, noID, r.s.migrations.MaxMessageBytes, func(msg imapread.Message) error {
		if i, ok := byUID[msg.UID]; ok && msg.Raw != nil {
			items[i].DedupeKey = email.SynthesizedID(msg.Raw)
			read += int64(len(msg.Raw))
		}
		return nil
	})
	if err != nil {
		return nil, read, fromSource(err)
	}
	return items, read, nil
}

func (r *migrationRun) scanPage(ctx context.Context, rd *sourceReader, f store.MigrationFolder, uidValidity uint32, page []uint32) error {
	items, read, err := r.headerItems(ctx, rd, f.ID, uidValidity, page)
	if err != nil {
		return err
	}
	r.throttle.Read(r.s.now(), read)
	if err := r.saveThrottle(ctx); err != nil {
		return err
	}
	p := store.InsertMigrationItemsParams{WorkspaceID: f.WorkspaceID, MigrationID: f.MigrationID, State: migrate.StateSkipped, Reason: migrate.ReasonUnplanned}
	var size int64
	var oldest, newest *time.Time
	for _, it := range items {
		p.SourceIds = append(p.SourceIds, it.SourceID)
		p.FolderIds = append(p.FolderIds, it.FolderID)
		p.DedupeKeys = append(p.DedupeKeys, it.DedupeKey)
		p.ReceivedAts = append(p.ReceivedAts, it.Received)
		p.Sizes = append(p.Sizes, it.Size)
		p.Metas = append(p.Metas, mustJSON(it.Meta))
		size += it.Size
		at := it.Meta.At
		if oldest == nil || at.Before(*oldest) {
			oldest = &at
		}
		if newest == nil || at.After(*newest) {
			newest = &at
		}
	}
	return r.s.st.InTx(ctx, func(q *store.Queries) error {
		if len(items) > 0 {
			if _, err := q.InsertMigrationItems(ctx, p); err != nil {
				return err
			}
		}
		return q.SetMigrationFolderScanned(ctx, store.SetMigrationFolderScannedParams{
			WorkspaceID: f.WorkspaceID, MigrationID: f.MigrationID, ID: f.ID, ScannedUid: int64(slices.Max(page)), SizeBytes: size,
			OldestAt: oldest, NewestAt: newest,
		})
	})
}

func chunks[T any](s []T, n int) [][]T {
	var out [][]T
	for len(s) > n {
		out = append(out, s[:n])
		s = s[n:]
	}
	if len(s) > 0 {
		out = append(out, s)
	}
	return out
}

// planProgress is where the plan stands between slices: it joins threads in the database a page of
// messages at a time (nodes), then plans a batch of whole threads at a time (threads), adding up
// the dry run's totals as it goes.
type planProgress struct {
	Step   string         `json:"step"`
	After  string         `json:"after,omitempty"`
	Totals migrate.Totals `json:"totals"`
}

type planSetup struct {
	folders []store.MigrationFolder
	plan    map[int32]migrate.PlanFolder
	imports []int32
	labels  []int32
	sent    []int32
	team    migrationTeam
}

func (r *migrationRun) planSetup(ctx context.Context) (planSetup, error) {
	s, m := r.s, r.m
	q := s.st.Queries
	folders, err := q.ListMigrationFolders(ctx, store.ListMigrationFoldersParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID})
	if err != nil {
		return planSetup{}, err
	}
	ps := planSetup{folders: folders, plan: map[int32]migrate.PlanFolder{}}
	for _, f := range folders {
		use := folderUse(f, folders)
		if use == oas.FolderUseSkip {
			continue
		}
		if f.Kind == migrate.KindSent {
			ps.sent = append(ps.sent, f.ID)
		}
		if use == oas.FolderUseLabels {
			ps.labels = append(ps.labels, f.ID)
		}
		label, err := labelName(ctx, q, f)
		if err != nil {
			return planSetup{}, err
		}
		ps.plan[f.ID] = migrate.PlanFolder{Kind: f.Kind, Label: label, LabelsOnly: use == oas.FolderUseLabels}
	}
	ps.imports = migrate.PrimaryOrder(ps.plan)
	ps.team, err = s.teamOf(ctx, q, m)
	return ps, err
}

// plan threads the scanned messages and decides what each becomes, a part per call, and reports
// whether there is more to do; it rewrites the plan as a whole, which only happens before the copy
// starts.
func (r *migrationRun) plan(ctx context.Context) (bool, error) {
	cp := readCheckpoint(r.m.Checkpoint)
	ps, err := r.planSetup(ctx)
	if err != nil {
		return false, err
	}
	switch {
	case cp.Plan == nil:
		return true, r.planStart(ctx, cp, ps)
	case cp.Plan.Step == planStepNodes:
		return true, r.planNodes(ctx, cp, ps)
	}
	return r.planThreads(ctx, cp, ps)
}

const (
	planStepNodes   = "nodes"
	planStepThreads = "threads"
)

// savePlan stores the checkpoint in the transaction that did the work it records.
func (r *migrationRun) savePlan(ctx context.Context, work func(q *store.Queries) error, cp migrationCheckpoint) error {
	raw := mustJSON(cp)
	err := r.s.st.InTx(ctx, func(q *store.Queries) error {
		if err := work(q); err != nil {
			return err
		}
		return q.SetMigrationCheckpoint(ctx, store.SetMigrationCheckpointParams{WorkspaceID: r.m.WorkspaceID, ID: r.m.ID, Checkpoint: raw})
	})
	if err == nil {
		r.m.Checkpoint = raw
	}
	return err
}

func (r *migrationRun) planStart(ctx context.Context, cp migrationCheckpoint, ps planSetup) error {
	ws, id := r.m.WorkspaceID, r.m.ID
	cp.Plan = &planProgress{Step: planStepNodes}
	return r.savePlan(ctx, func(q *store.Queries) error {
		if err := q.ResetMigrationPlan(ctx, store.ResetMigrationPlanParams{
			WorkspaceID: ws, MigrationID: id, LabelFolderIds: ps.labels, ImportFolderIds: ps.imports,
		}); err != nil {
			return err
		}
		if err := q.DeleteMigrationThreads(ctx, store.DeleteMigrationThreadsParams{WorkspaceID: ws, MigrationID: id}); err != nil {
			return err
		}
		if err := q.DeleteMigrationPlanNodes(ctx, store.DeleteMigrationPlanNodesParams{WorkspaceID: ws, MigrationID: id}); err != nil {
			return err
		}
		if err := q.DeleteMigrationPlanNames(ctx, store.DeleteMigrationPlanNamesParams{WorkspaceID: ws, MigrationID: id}); err != nil {
			return err
		}
		return q.InsertMigrationPlanNames(ctx, store.InsertMigrationPlanNamesParams{WorkspaceID: ws, MigrationID: id, FolderIds: ps.imports})
	}, cp)
}

// planNode is the node a message id is joined by; an id too long for the index is joined by its
// hash, which no Message-ID the scan keeps can equal.
func planNode(id string) string {
	if len(id) <= 998 {
		return id
	}
	sum := sha256.Sum256([]byte(id))
	return "#" + hex.EncodeToString(sum[:])
}

// planNodes joins the threads of a page of messages: each message's id and the ids it refers to
// become one component, the smaller of two components joining the larger.
func (r *migrationRun) planNodes(ctx context.Context, cp migrationCheckpoint, ps planSetup) error {
	s, ws, id := r.s, r.m.WorkspaceID, r.m.ID
	rows, err := s.st.ListMigrationPlanPrimaries(ctx, store.ListMigrationPlanPrimariesParams{
		WorkspaceID: ws, MigrationID: id, FolderIds: ps.imports, After: cp.Plan.After, Lim: planBatch,
	})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		cp.Plan = &planProgress{Step: planStepThreads}
		return r.savePlan(ctx, func(*store.Queries) error { return nil }, cp)
	}
	type edge struct{ from, to string }
	var edges []edge
	nodes := map[string]bool{}
	for _, row := range rows {
		var refs []string
		_ = json.Unmarshal(row.Refs, &refs)
		if row.Thread != "" {
			refs = []string{migrate.ThreadNode(row.Thread)}
		}
		nodes[row.DedupeKey] = true
		for _, ref := range refs {
			n := planNode(ref)
			nodes[n] = true
			edges = append(edges, edge{row.DedupeKey, n})
		}
	}
	known, err := s.st.ListMigrationPlanNodes(ctx, store.ListMigrationPlanNodesParams{WorkspaceID: ws, MigrationID: id, Nodes: sortedKeys(nodes)})
	if err != nil {
		return err
	}
	rootOf := map[string]string{}
	roots := map[string]bool{}
	for _, k := range known {
		rootOf[k.Node] = k.Root
		roots[k.Root] = true
	}
	sizes := map[string]int32{}
	rootRows, err := s.st.ListMigrationPlanNodes(ctx, store.ListMigrationPlanNodesParams{WorkspaceID: ws, MigrationID: id, Nodes: sortedKeys(roots)})
	if err != nil {
		return err
	}
	for _, k := range rootRows {
		sizes[k.Node] = k.Size
	}
	canon := func(n string) string {
		if root, ok := rootOf[n]; ok {
			return root
		}
		return n
	}
	uf := planUnion{}
	for n := range nodes {
		uf.find(canon(n))
	}
	for _, e := range edges {
		uf.union(canon(e.from), canon(e.to))
	}
	type component struct {
		known []string
		fresh []string
	}
	comps := map[string]*component{}
	for _, c := range sortedKeys(uf.members()) {
		top := uf.find(c)
		if comps[top] == nil {
			comps[top] = &component{}
		}
		if roots[c] {
			comps[top].known = append(comps[top].known, c)
		} else {
			comps[top].fresh = append(comps[top].fresh, c)
		}
	}
	var oldRoots, newRoots, freshNodes, freshRoots, sizeNodes []string
	var freshSizes, sizeValues []int32
	for _, c := range comps {
		final := ""
		total := int32(len(c.fresh))
		for _, k := range c.known {
			total += sizes[k]
			if final == "" || sizes[k] > sizes[final] {
				final = k
			}
		}
		if final == "" {
			final = c.fresh[0]
		} else {
			sizeNodes, sizeValues = append(sizeNodes, final), append(sizeValues, total)
		}
		for _, k := range c.known {
			if k != final {
				oldRoots, newRoots = append(oldRoots, k), append(newRoots, final)
			}
		}
		for _, n := range c.fresh {
			size := int32(0)
			if n == final {
				size = total
			}
			freshNodes, freshRoots, freshSizes = append(freshNodes, n), append(freshRoots, final), append(freshSizes, size)
		}
	}
	cp.Plan.After = rows[len(rows)-1].DedupeKey
	return r.savePlan(ctx, func(q *store.Queries) error {
		if len(oldRoots) > 0 {
			if err := q.MoveMigrationPlanRoots(ctx, store.MoveMigrationPlanRootsParams{WorkspaceID: ws, MigrationID: id, OldRoots: oldRoots, NewRoots: newRoots}); err != nil {
				return err
			}
		}
		if len(freshNodes) > 0 {
			if err := q.InsertMigrationPlanNodes(ctx, store.InsertMigrationPlanNodesParams{
				WorkspaceID: ws, MigrationID: id, Nodes: freshNodes, Roots: freshRoots, Sizes: freshSizes,
			}); err != nil {
				return err
			}
		}
		if len(sizeNodes) > 0 {
			return q.SetMigrationPlanSizes(ctx, store.SetMigrationPlanSizesParams{WorkspaceID: ws, MigrationID: id, Nodes: sizeNodes, Sizes: sizeValues})
		}
		return nil
	}, cp)
}

type planUnion map[string]string

func (u planUnion) find(x string) string {
	root := x
	for {
		p, ok := u[root]
		if !ok {
			u[root] = root
			break
		}
		if p == root {
			break
		}
		root = p
	}
	for x != root {
		next := u[x]
		u[x] = root
		x = next
	}
	return root
}

func (u planUnion) union(a, b string) {
	if ra, rb := u.find(a), u.find(b); ra != rb {
		u[rb] = ra
	}
}

func (u planUnion) members() map[string]bool {
	out := make(map[string]bool, len(u))
	for k := range u {
		out[k] = true
	}
	return out
}

// planThreads plans the next batch of whole threads with the totals so far; when no thread is
// left it writes the dry run.
func (r *migrationRun) planThreads(ctx context.Context, cp migrationCheckpoint, ps planSetup) (bool, error) {
	s, m := r.s, r.m
	q := s.st.Queries
	roots, err := q.ListMigrationPlanRoots(ctx, store.ListMigrationPlanRootsParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, After: cp.Plan.After, Lim: planBatch})
	if err != nil {
		return false, err
	}
	if len(roots) == 0 {
		return false, r.planFinish(ctx, cp, ps)
	}
	var take []string
	var nodes int32
	for _, root := range roots {
		if len(take) > 0 && nodes+root.Size > planBatch {
			break
		}
		take, nodes = append(take, root.Node), nodes+root.Size
	}
	rows, err := q.ListMigrationPlanItems(ctx, store.ListMigrationPlanItemsParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, Roots: take})
	if err != nil {
		return false, err
	}
	items := make([]migrate.Item, 0, len(rows))
	addrs, others := map[string]bool{}, map[string]bool{}
	var keys []string
	for _, row := range rows {
		it := migrate.Item{SourceID: row.SourceID, DedupeKey: row.DedupeKey, Received: row.ReceivedAt, Size: row.SizeBytes}
		if row.FolderID != nil {
			it.FolderID = *row.FolderID
		}
		_ = json.Unmarshal(row.Meta, &it.Meta)
		items = append(items, it)
		keys = append(keys, row.DedupeKey)
		for _, a := range slices.Concat([]migrate.Address{it.Meta.From}, it.Meta.To, it.Meta.Cc) {
			if a.Email == "" {
				continue
			}
			addrs[a.Email] = true
			if !ps.team.addrs[a.Email] {
				others[a.Email] = true
			}
		}
	}
	names := map[string]string{}
	found, err := q.ListMigrationPlanNames(ctx, store.ListMigrationPlanNamesParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, Emails: sortedKeys(addrs)})
	if err != nil {
		return false, err
	}
	for _, n := range found {
		names[n.Email] = n.Name
	}
	blocked := map[string]bool{}
	for _, part := range chunks(sortedKeys(others), planBatch) {
		found, err := q.ListContactEmailsIn(ctx, store.ListContactEmailsInParams{WorkspaceID: m.WorkspaceID, Emails: part})
		if err != nil {
			return false, err
		}
		for _, f := range found {
			blocked[f.Email] = f.Blocked
		}
	}
	inYuva := map[string]bool{}
	if m.ChannelID != nil {
		for _, part := range chunks(keys, planBatch) {
			found, err := q.ListChannelHeaderIDsIn(ctx, store.ListChannelHeaderIDsInParams{WorkspaceID: m.WorkspaceID, ChannelID: m.ChannelID, Ids: part})
			if err != nil {
				return false, err
			}
			for _, id := range found {
				inYuva[id] = true
			}
		}
	}
	opts := migrationOptions(m.Options)
	plan := migrate.MakePlan(migrate.PlanInput{
		Items: items, Folders: ps.plan, Team: ps.team.addrs, Since: opts.Since, SkipAutomatic: opts.SkipAutomatic,
		CloseAfterDays: int(opts.CloseAfterDays), Now: s.now(), MaxMessageBytes: s.migrations.MaxMessageBytes, Blocked: blocked, InYuva: inYuva,
		Names: names, AttachmentOK: func(ct string, size int64) bool { return size <= s.attach.MaxBytes && s.typeAllowed(ct) },
	})
	cp.Plan.Totals.Add(plan.Totals)
	cp.Plan.After = take[len(take)-1]
	return true, r.savePlan(ctx, func(q *store.Queries) error {
		ids := make([]string, 0, len(plan.Items))
		for id := range plan.Items {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, part := range chunks(ids, planBatch) {
			p := store.ApplyMigrationItemsPlanParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID}
			for _, id := range part {
				ip := plan.Items[id]
				p.SourceIds, p.States, p.Reasons, p.ThreadKeys = append(p.SourceIds, id), append(p.States, ip.State), append(p.Reasons, ip.Reason), append(p.ThreadKeys, ip.ThreadKey)
			}
			if err := q.ApplyMigrationItemsPlan(ctx, p); err != nil {
				return err
			}
		}
		for _, part := range chunks(plan.Threads, planBatch) {
			p := store.InsertMigrationThreadsParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID}
			for _, t := range part {
				p.ThreadKeys, p.LastAts, p.Items, p.Plans = append(p.ThreadKeys, t.Key), append(p.LastAts, t.LastAt), append(p.Items, int32(len(t.SourceIDs))), append(p.Plans, mustJSON(t))
			}
			if err := q.InsertMigrationThreads(ctx, p); err != nil {
				return err
			}
		}
		return nil
	}, cp)
}

// planFinish writes the dry run and the team suggestions and drops the plan's working rows.
func (r *migrationRun) planFinish(ctx context.Context, cp migrationCheckpoint, ps planSetup) error {
	s, m := r.s, r.m
	q := s.st.Queries
	contacts, err := q.CountMigrationPlanContacts(ctx, store.CountMigrationPlanContactsParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, Team: sortedKeys(ps.team.addrs)})
	if err != nil {
		return err
	}
	est, err := s.dryRun(ctx, q, m, ps.folders, cp.Plan.Totals, contacts.Existing, contacts.Total-contacts.Existing)
	if err != nil {
		return err
	}
	senders, err := q.ListMigrationSentFrom(ctx, store.ListMigrationSentFromParams{
		WorkspaceID: m.WorkspaceID, MigrationID: m.ID, FolderIds: ps.sent, Mailbox: strings.ToLower(strings.TrimSpace(m.SourceAddress)), Lim: teamSuggestions,
	})
	if err != nil {
		return err
	}
	suggestions := []oas.MigrationTeamSuggestion{}
	for _, f := range senders {
		sg := oas.MigrationTeamSuggestion{Address: f.Email, Messages: f.Messages}
		if f.Name != "" {
			sg.Name = &f.Name
		}
		suggestions = append(suggestions, sg)
	}
	cp.Phase, cp.Plan = scanPhasePreview, nil
	return r.savePlan(ctx, func(q *store.Queries) error {
		if err := q.DeleteMigrationPlanNodes(ctx, store.DeleteMigrationPlanNodesParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID}); err != nil {
			return err
		}
		if err := q.DeleteMigrationPlanNames(ctx, store.DeleteMigrationPlanNamesParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID}); err != nil {
			return err
		}
		return q.SetMigrationEstimate(ctx, store.SetMigrationEstimateParams{WorkspaceID: m.WorkspaceID, ID: m.ID, Estimate: mustJSON(migrationEstimateDoc{DryRun: est, TeamSuggestions: suggestions})})
	}, cp)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (s *Server) dryRun(ctx context.Context, q *store.Queries, m store.Migration, folders []store.MigrationFolder, t migrate.Totals, contactsExisting, contactsNew int64) (oas.MigrationEstimate, error) {
	opts := migrationOptions(m.Options)
	est := oas.MigrationEstimate{
		ScannedAt: s.now(), Conversations: t.Conversations, Messages: t.In + t.Out + t.Messages, MessagesIn: t.In, MessagesOut: t.Out,
		Labels: []oas.MigrationEstimateLabel{}, Attachments: t.Attachments, AttachmentBytes: t.AttachmentBytes,
		AttachmentsSkipped: t.AttachmentsSkipped, Bytes: t.Bytes, StorageBytes: t.AttachmentBytes,
		Skipped: oas.MigrationSkipped{
			Internal: t.Skipped[migrate.ReasonInternal], Automatic: t.Skipped[migrate.ReasonAutomatic], OutsideRange: t.Skipped[migrate.ReasonOutsideRange],
			AlreadyInYuva: t.Skipped["already_in_yuva"], Blocked: t.Skipped[migrate.ReasonBlocked], TooLarge: t.Skipped[migrate.ReasonTooLarge],
			DuplicateCopies: t.Skipped["duplicate_copies"],
		},
	}
	if !t.Oldest.IsZero() {
		est.OldestAt, est.NewestAt = &t.Oldest, &t.Newest
	}
	if opts.KeepOriginals {
		est.StorageBytes += t.Bytes
	}
	est.ContactsExisting, est.ContactsNew = contactsExisting, contactsNew
	for _, f := range folders {
		switch {
		case f.Kind == migrate.KindSpam:
			est.Skipped.Spam += int64(f.Total)
		case f.Kind == migrate.KindTrash:
			est.Skipped.Trash += int64(f.Total)
		case f.Kind == migrate.KindDrafts:
			est.Skipped.Drafts += int64(f.Total)
		case folderUse(f, folders) == oas.FolderUseSkip:
			est.Skipped.OtherFolders += int64(f.Total)
		}
	}
	names := sortedKeysInt(t.Labels)
	for _, name := range names {
		l := oas.MigrationEstimateLabel{Name: name, Action: oas.EstimateLabelCreate, Conversations: t.Labels[name]}
		if row, err := q.GetLabelByName(ctx, store.GetLabelByNameParams{WorkspaceID: m.WorkspaceID, Name: name}); err == nil {
			l.Action, l.LabelId = oas.EstimateLabelExisting, &row.ID
		} else if !store.IsNotFound(err) {
			return est, err
		}
		est.Labels = append(est.Labels, l)
	}
	preset, _ := migrate.LookupPreset(m.Preset)
	secs := float64(est.Messages) / importMessagesPerS
	switch {
	case preset.Budget.BytesPerDay > 0:
		secs = max(secs, float64(t.Bytes)/float64(preset.Budget.BytesPerDay)*86400)
	case preset.Budget.BytesPerSecond > 0:
		secs = max(secs, float64(t.Bytes)/float64(preset.Budget.BytesPerSecond))
	case preset.Budget.RequestsPerMinute > 0 || preset.Budget.RequestsPerDay > 0:
		requests := float64(t.Conversations) * helpdeskRequestsPerTicket
		if preset.Budget.RequestsPerMinute > 0 {
			secs = max(secs, requests*60/float64(preset.Budget.RequestsPerMinute))
		}
		if preset.Budget.RequestsPerDay > 0 {
			secs = max(secs, requests*86400/float64(preset.Budget.RequestsPerDay))
		}
	}
	est.SecondsMin = int64(math.Ceil(secs))
	est.SecondsMax = int64(math.Ceil(secs * estimateMarginRatio))
	return est, nil
}

func sortedKeysInt(m map[string]int64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type fetchedItem struct {
	raw        []byte
	present    bool
	unreadable bool
}

// fetch reads whole messages, grouped by folder, and notes which ones the mailbox no longer has.
// It reports false when a folder's UIDVALIDITY changed.
func (r *migrationRun) fetch(ctx context.Context, folders map[int32]store.MigrationFolder, sourceIDs []string) (map[string]fetchedItem, int64, *store.MigrationFolder, error) {
	if fileMigration(r.m) {
		out, read, err := r.fetchFile(ctx, sourceIDs)
		return out, read, nil, err
	}
	rd, err := r.source(ctx)
	if err != nil {
		return nil, 0, nil, fromSource(err)
	}
	type ref struct {
		id  string
		uid uint32
	}
	byFolder := map[int32][]ref{}
	var order []int32
	for _, id := range sourceIDs {
		f, _, uid, ok := migrate.ParseSourceID(id)
		if !ok {
			continue
		}
		if _, seen := byFolder[f]; !seen {
			order = append(order, f)
		}
		byFolder[f] = append(byFolder[f], ref{id, uid})
	}
	out := map[string]fetchedItem{}
	var read int64
	for _, fid := range order {
		f, ok := folders[fid]
		if !ok {
			continue
		}
		st, err := rd.Open(ctx, f.Name)
		if err != nil {
			return nil, read, nil, fromSource(err)
		}
		if f.Uidvalidity == nil || int64(st.UIDValidity) != *f.Uidvalidity {
			return out, read, &f, nil
		}
		uids := make([]uint32, 0, len(byFolder[fid]))
		ids := map[uint32]string{}
		for _, rf := range byFolder[fid] {
			uids = append(uids, rf.uid)
			ids[rf.uid] = rf.id
			out[rf.id] = fetchedItem{}
		}
		err = rd.FetchRaw(ctx, uids, r.s.migrations.MaxMessageBytes, func(msg imapread.Message) error {
			if id, ok := ids[msg.UID]; ok {
				out[id] = fetchedItem{raw: msg.Raw, present: true}
				read += int64(len(msg.Raw))
			}
			return nil
		})
		if err != nil {
			return nil, read, nil, fromSource(err)
		}
	}
	return out, read, nil, nil
}

// preview fetches the newest threads in full and keeps them as they will look.
func (r *migrationRun) preview(ctx context.Context) error {
	s, m := r.s, r.m
	q := s.st.Queries
	threads, err := q.ListNewestMigrationThreads(ctx, store.ListNewestMigrationThreadsParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, Lim: previewThreads})
	if err != nil {
		return err
	}
	out := oas.MigrationPreview{Items: []oas.MigrationPreviewThread{}}
	if len(threads) > 0 {
		keys := make([]string, len(threads))
		for i, t := range threads {
			keys[i] = t.ThreadKey
		}
		items, err := q.ListMigrationThreadItems(ctx, store.ListMigrationThreadItemsParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, ThreadKeys: keys})
		if err != nil {
			return err
		}
		byThread := map[string][]store.ListMigrationThreadItemsRow{}
		var ids []string
		for _, it := range items {
			if len(byThread[it.ThreadKey]) == previewMessages {
				continue
			}
			byThread[it.ThreadKey] = append(byThread[it.ThreadKey], it)
			ids = append(ids, it.SourceID)
		}
		folderRows, err := q.ListMigrationFolders(ctx, store.ListMigrationFoldersParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID})
		if err != nil {
			return err
		}
		folders := map[int32]store.MigrationFolder{}
		for _, f := range folderRows {
			folders[f.ID] = f
		}
		fetched, read, changed, err := r.fetch(ctx, folders, ids)
		if err != nil {
			return err
		}
		if changed != nil {
			fetched = map[string]fetchedItem{}
		}
		r.throttle.Read(s.now(), read)
		if err := r.saveThrottle(ctx); err != nil {
			return err
		}
		team, err := s.teamOf(ctx, q, m)
		if err != nil {
			return err
		}
		for _, t := range threads {
			var plan migrate.ThreadPlan
			_ = json.Unmarshal(t.Plan, &plan)
			pt := oas.MigrationPreviewThread{
				Subject: plan.Subject, Contact: oas.ImportedAuthor{Name: plan.Contact.Name, Address: plan.Contact.Email},
				Status: oas.ConversationStatus(plan.Status), Labels: plan.Labels, MessagesTotal: int32(plan.In + plan.Out),
				Messages: []oas.MigrationPreviewMessage{},
			}
			if pt.Labels == nil {
				pt.Labels = []string{}
			}
			for _, it := range byThread[t.ThreadKey] {
				var meta migrate.Meta
				_ = json.Unmarshal(it.Meta, &meta)
				pm := oas.MigrationPreviewMessage{
					Direction: oas.Direction(migrate.Direction(team.addrs, meta.From.Email)), At: meta.At,
					From: oas.ImportedAuthor{Name: meta.From.Name, Address: meta.From.Email},
				}
				if id, ok := team.members[meta.From.Email]; ok && pm.Direction == oas.Out {
					pm.MemberId = &id
				}
				if f := fetched[it.SourceID]; f.raw != nil {
					if parsed, err := email.Parse(f.raw); err == nil {
						visible, _ := reply.Strip(parsed.Text)
						pm.Text = truncateRunes(strings.TrimSpace(visible), previewTextRunes)
						pm.Attachments = int32(len(parsed.Attachments))
					}
				}
				pt.Messages = append(pt.Messages, pm)
			}
			out.Items = append(out.Items, pt)
		}
	}
	cur, err := q.GetMigration(ctx, store.GetMigrationParams{WorkspaceID: m.WorkspaceID, ID: m.ID})
	if err != nil {
		return err
	}
	doc := readEstimate(cur.Estimate)
	if doc == nil {
		return errors.New("the plan's dry run is missing")
	}
	doc.Preview = &out
	return q.SetMigrationEstimate(ctx, store.SetMigrationEstimateParams{WorkspaceID: m.WorkspaceID, ID: m.ID, Estimate: mustJSON(doc)})
}

// scanned makes the migration ready with its dry run.
func (r *migrationRun) scanned(ctx context.Context) error {
	return r.s.inTx(ctx, r.m.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
		cur, err := q.LockMigration(ctx, store.LockMigrationParams{WorkspaceID: r.m.WorkspaceID, ID: r.m.ID})
		if err != nil || cur.State != migrationScanning {
			return err
		}
		if err := q.SetMigrationCheckpoint(ctx, store.SetMigrationCheckpointParams{WorkspaceID: cur.WorkspaceID, ID: cur.ID, Checkpoint: []byte("{}")}); err != nil {
			return err
		}
		m, err := q.SetMigrationRunState(ctx, store.SetMigrationRunStateParams{WorkspaceID: cur.WorkspaceID, ID: cur.ID, State: migrationReady, Now: r.s.now()})
		if err != nil {
			return err
		}
		r.m = m
		return r.s.migrationChanged(ctx, q, events, m, &cur.State)
	})
}

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/email/reply"
	"github.com/productdevbook/yuva/api/internal/migrate"
	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	reasonHelpdeskBudget      = "Keeping within the help desk's rate limit; continuing shortly."
	reasonHelpdeskDaily       = "The help desk's requests for today are used up; continuing when its day renews."
	helpdeskRequestsPerTicket = 2
	helpdeskReasonSpam        = "spam"
	helpdeskReasonTrash       = "trash"
	helpdeskReasonEmpty       = "empty"
	syncOverlap               = time.Minute
)

var errDeskBlocked = errors.New("the requester is blocked")

// deskErr turns the budget's wait into the run's and marks the help desk's failures as the source's.
func deskErr(err error) error {
	var w *helpdesk.WaitError
	if errors.As(err, &w) && w.Daily {
		return &waitError{until: w.Until, reason: reasonHelpdeskDaily}
	}
	if errors.As(err, &w) {
		return &waitError{until: w.Until, reason: reasonHelpdeskBudget}
	}
	return fromSource(err)
}

// desk signs in to the help desk once per slice.
func (r *migrationRun) desk(ctx context.Context) (helpdesk.Source, error) {
	if r.deskSrc != nil {
		return r.deskSrc, nil
	}
	if r.throttle.Requests == nil {
		r.throttle.Requests = &helpdesk.Budget{}
	}
	src, sec, err := r.s.openHelpdesk(ctx, r.m, r.throttle.Requests)
	if err != nil {
		return nil, err
	}
	r.deskSrc, r.secrets = src, sec.secrets()
	return src, nil
}

func (r *migrationRun) setSourceState(ctx context.Context, q *store.Queries, doc sourceStateDoc) error {
	raw := mustJSON(doc)
	if err := q.SetMigrationSourceState(ctx, store.SetMigrationSourceStateParams{WorkspaceID: r.m.WorkspaceID, ID: r.m.ID, SourceState: raw}); err != nil {
		return err
	}
	r.m.SourceState = raw
	return nil
}

// listHelpdeskFolders lists the brands or mailboxes again and starts the ticket listing over.
func (r *migrationRun) listHelpdeskFolders(ctx context.Context) error {
	src, err := r.desk(ctx)
	if err != nil {
		return deskErr(err)
	}
	folders, err := src.Folders(ctx)
	if err != nil {
		return deskErr(err)
	}
	if err := r.saveThrottle(ctx); err != nil {
		return err
	}
	return r.s.st.InTx(ctx, func(q *store.Queries) error {
		if err := r.s.recordHelpdeskFolders(ctx, q, r.m, folders); err != nil {
			return err
		}
		doc := readSourceState(r.m.SourceState)
		doc.Scan, doc.ScanDone = helpdesk.Cursor{}, false
		return r.setSourceState(ctx, q, doc)
	})
}

func folderRefs(rows []store.MigrationFolder) map[string]int32 {
	out := map[string]int32{}
	for _, f := range rows {
		if f.SourceRef != nil {
			out[*f.SourceRef] = f.ID
		}
	}
	return out
}

// syncStart is where reading changes begins once the scan read every ticket: an incremental
// desk's last cursor, or the tickets modified since the scan started, a minute early.
func syncStart(source string, last helpdesk.Cursor) helpdesk.Cursor {
	if d, _ := helpdesk.Lookup(source); d.Sync == helpdesk.SyncIncremental {
		if last.After != "" {
			return helpdesk.Cursor{After: last.After}
		}
		return helpdesk.Cursor{Start: last.StartedAt.Unix()}
	}
	return helpdesk.Cursor{ModifiedSince: last.StartedAt.Add(-syncOverlap)}
}

type deskFolderScan struct {
	adds           int32
	oldest, newest *time.Time
}

// scanHelpdesk lists the help desk's tickets a page at a time from where the last slice stopped,
// one item each.
func (r *migrationRun) scanHelpdesk(ctx context.Context) (bool, error) {
	doc := readSourceState(r.m.SourceState)
	if doc.ScanDone {
		return false, nil
	}
	src, err := r.desk(ctx)
	if err != nil {
		return false, deskErr(err)
	}
	rows, err := r.s.st.ListMigrationFolders(ctx, store.ListMigrationFoldersParams{WorkspaceID: r.m.WorkspaceID, MigrationID: r.m.ID})
	if err != nil {
		return false, err
	}
	refs := folderRefs(rows)
	for {
		if !r.timeLeft() {
			return true, nil
		}
		if ok, err := r.renew(ctx); err != nil || !ok {
			return true, err
		}
		if doc.Scan.StartedAt.IsZero() {
			doc.Scan.StartedAt = r.s.now()
		}
		page, err := src.Tickets(ctx, doc.Scan)
		if err != nil {
			if serr := r.saveThrottle(ctx); serr != nil {
				return true, serr
			}
			return true, deskErr(err)
		}
		p := store.UpsertHelpdeskItemsParams{WorkspaceID: r.m.WorkspaceID, MigrationID: r.m.ID}
		stats := map[int32]*deskFolderScan{}
		for _, t := range page.Tickets {
			folder := refs[t.Folder]
			p.SourceIds, p.FolderIds = append(p.SourceIds, r.m.Source+":"+t.ID), append(p.FolderIds, folder)
			p.ReceivedAts, p.Metas = append(p.ReceivedAts, t.UpdatedAt), append(p.Metas, mustJSON(t))
			if folder == 0 {
				continue
			}
			st := stats[folder]
			if st == nil {
				st = &deskFolderScan{}
				stats[folder] = st
			}
			st.adds++
			at := t.UpdatedAt
			if st.oldest == nil || at.Before(*st.oldest) {
				st.oldest = &at
			}
			if st.newest == nil || at.After(*st.newest) {
				st.newest = &at
			}
		}
		next := doc
		next.Scan = page.Next
		next.Scan.StartedAt = doc.Scan.StartedAt
		if page.End {
			next.ScanDone, next.Sync, next.SyncStarted = true, syncStart(r.m.Source, next.Scan), false
		}
		err = r.s.st.InTx(ctx, func(q *store.Queries) error {
			if len(p.SourceIds) > 0 {
				if err := q.UpsertHelpdeskItems(ctx, p); err != nil {
					return err
				}
			}
			var ids, adds []int32
			for id, st := range stats {
				ids, adds = append(ids, id), append(adds, st.adds)
				if err := q.SetMigrationFolderScanned(ctx, store.SetMigrationFolderScannedParams{
					WorkspaceID: r.m.WorkspaceID, MigrationID: r.m.ID, ID: id, OldestAt: st.oldest, NewestAt: st.newest,
				}); err != nil {
					return err
				}
			}
			if len(ids) > 0 {
				if err := q.AddMigrationFolderTotals(ctx, store.AddMigrationFolderTotalsParams{WorkspaceID: r.m.WorkspaceID, MigrationID: r.m.ID, Ids: ids, Adds: adds}); err != nil {
					return err
				}
			}
			return r.setSourceState(ctx, q, next)
		})
		if err != nil {
			return true, err
		}
		doc = next
		if err := r.saveThrottle(ctx); err != nil {
			return true, err
		}
		if page.End {
			return false, nil
		}
	}
}

// deskPlan decides what each ticket becomes under the options.
type deskPlan struct {
	ws       uuid.UUID
	selected map[int32]bool
	since    *time.Time
	labels   map[string]string
	mapping  map[string]oas.MigrationLabelMapping
}

func (r *migrationRun) deskPlanSetup(ctx context.Context, q *store.Queries) (*deskPlan, error) {
	rows, err := q.ListMigrationFolders(ctx, store.ListMigrationFoldersParams{WorkspaceID: r.m.WorkspaceID, MigrationID: r.m.ID})
	if err != nil {
		return nil, err
	}
	opts := migrationOptions(r.m.Options)
	d := &deskPlan{ws: r.m.WorkspaceID, selected: map[int32]bool{}, since: opts.Since, labels: map[string]string{}, mapping: map[string]oas.MigrationLabelMapping{}}
	for _, f := range rows {
		d.selected[f.ID] = f.Selected
	}
	for _, l := range opts.Labels {
		d.mapping[l.Folder] = l
	}
	return d, nil
}

// label is the label a tag becomes: of its own name unless the options map it to an existing
// label or to none.
func (d *deskPlan) label(ctx context.Context, q *store.Queries, tag string) (string, error) {
	if l, ok := d.labels[tag]; ok {
		return l, nil
	}
	out := truncateRunes(strings.TrimSpace(tag), 64)
	if m, ok := d.mapping[tag]; ok {
		switch m.Action {
		case oas.MigrationLabelNone:
			out = ""
		case oas.MigrationLabelExisting:
			out = ""
			if m.LabelId != nil {
				l, err := q.GetLabel(ctx, store.GetLabelParams{WorkspaceID: d.ws, ID: *m.LabelId})
				if err != nil && !store.IsNotFound(err) {
					return "", err
				}
				out = l.Name
			}
		}
	}
	d.labels[tag] = out
	return out, nil
}

func personAddress(p helpdesk.Person) migrate.Address {
	return migrate.Address{Name: truncateRunes(p.Name, 200), Email: p.Email}
}

func (d *deskPlan) decide(ctx context.Context, q *store.Queries, t helpdesk.Ticket, folder int32, blocked map[string]bool) (string, string, migrate.ThreadPlan, error) {
	switch {
	case folder != 0 && !d.selected[folder]:
		return migrate.StateSkipped, migrate.ReasonUnselected, migrate.ThreadPlan{}, nil
	case t.Status == helpdesk.StatusSpam:
		return migrate.StateSkipped, helpdeskReasonSpam, migrate.ThreadPlan{}, nil
	case t.Status == helpdesk.StatusDeleted:
		return migrate.StateSkipped, helpdeskReasonTrash, migrate.ThreadPlan{}, nil
	case d.since != nil && t.UpdatedAt.Before(*d.since):
		return migrate.StateSkipped, migrate.ReasonOutsideRange, migrate.ThreadPlan{}, nil
	case t.Requester.Email != "" && blocked[t.Requester.Email]:
		return migrate.StateSkipped, migrate.ReasonBlocked, migrate.ThreadPlan{}, nil
	}
	status := t.Status
	if status != helpdesk.StatusPending && status != helpdesk.StatusClosed {
		status = migrate.StatusOpen
	}
	plan := migrate.ThreadPlan{
		Contact: personAddress(t.Requester), Subject: migrate.BaseSubject(truncateRunes(t.Subject, 500)), Status: status,
		FirstAt: t.CreatedAt, LastAt: t.UpdatedAt, Messages: t.Messages,
	}
	if t.Assignee != nil {
		plan.Assignee = new(personAddress(*t.Assignee))
	}
	for _, tag := range t.Tags {
		l, err := d.label(ctx, q, tag)
		if err != nil {
			return "", "", plan, err
		}
		if l != "" && !slices.Contains(plan.Labels, l) {
			plan.Labels = append(plan.Labels, l)
		}
	}
	sort.Strings(plan.Labels)
	return migrate.StatePlanned, "", plan, nil
}

func blockedRequesters(ctx context.Context, q *store.Queries, ws uuid.UUID, tickets []helpdesk.Ticket) (map[string]bool, error) {
	seen := map[string]bool{}
	for _, t := range tickets {
		if t.Requester.Email != "" {
			seen[t.Requester.Email] = true
		}
	}
	out := map[string]bool{}
	if len(seen) == 0 {
		return out, nil
	}
	found, err := q.ListContactEmailsIn(ctx, store.ListContactEmailsInParams{WorkspaceID: ws, Emails: sortedKeys(seen)})
	if err != nil {
		return nil, err
	}
	for _, f := range found {
		out[f.Email] = f.Blocked
	}
	return out, nil
}

// planHelpdesk plans a batch of tickets per call: one conversation each, with the help desk's
// status, tags as labels and the assignee.
func (r *migrationRun) planHelpdesk(ctx context.Context) (bool, error) {
	cp := readCheckpoint(r.m.Checkpoint)
	ws, id := r.m.WorkspaceID, r.m.ID
	if cp.Plan == nil {
		cp.Plan = &planProgress{Step: planStepThreads}
		return true, r.savePlan(ctx, func(q *store.Queries) error {
			return q.DeleteMigrationThreads(ctx, store.DeleteMigrationThreadsParams{WorkspaceID: ws, MigrationID: id})
		}, cp)
	}
	q := r.s.st.Queries
	rows, err := q.ListHelpdeskItems(ctx, store.ListHelpdeskItemsParams{WorkspaceID: ws, MigrationID: id, After: cp.Plan.After, Lim: planBatch})
	if err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, r.planHelpdeskFinish(ctx, cp)
	}
	d, err := r.deskPlanSetup(ctx, q)
	if err != nil {
		return false, err
	}
	tickets := make([]helpdesk.Ticket, len(rows))
	for i, row := range rows {
		_ = json.Unmarshal(row.Meta, &tickets[i])
	}
	blocked, err := blockedRequesters(ctx, q, ws, tickets)
	if err != nil {
		return false, err
	}
	apply := store.ApplyMigrationItemsPlanParams{WorkspaceID: ws, MigrationID: id}
	threads := store.InsertMigrationThreadsParams{WorkspaceID: ws, MigrationID: id}
	totals := migrate.Totals{Labels: map[string]int64{}, Skipped: map[string]int64{}}
	for i, row := range rows {
		if row.State == migrate.StateImported {
			continue
		}
		folder := int32(0)
		if row.FolderID != nil {
			folder = *row.FolderID
		}
		t := tickets[i]
		state, reason, plan, err := d.decide(ctx, q, t, folder, blocked)
		if err != nil {
			return false, err
		}
		apply.SourceIds, apply.States, apply.Reasons, apply.ThreadKeys = append(apply.SourceIds, row.SourceID), append(apply.States, state),
			append(apply.Reasons, reason), append(apply.ThreadKeys, row.SourceID)
		if state != migrate.StatePlanned {
			if reason != migrate.ReasonUnselected {
				totals.Skipped[reason]++
			}
			continue
		}
		threads.ThreadKeys, threads.LastAts, threads.Items, threads.Plans = append(threads.ThreadKeys, row.SourceID), append(threads.LastAts, plan.LastAt),
			append(threads.Items, 1), append(threads.Plans, mustJSON(plan))
		totals.Conversations++
		totals.Messages += int64(plan.Messages)
		for _, l := range plan.Labels {
			totals.Labels[l]++
		}
		if totals.Oldest.IsZero() || plan.FirstAt.Before(totals.Oldest) {
			totals.Oldest = plan.FirstAt
		}
		if plan.LastAt.After(totals.Newest) {
			totals.Newest = plan.LastAt
		}
	}
	cp.Plan.Totals.Add(totals)
	cp.Plan.After = rows[len(rows)-1].SourceID
	return true, r.savePlan(ctx, func(q *store.Queries) error {
		if len(apply.SourceIds) > 0 {
			if err := q.ApplyMigrationItemsPlan(ctx, apply); err != nil {
				return err
			}
		}
		if len(threads.ThreadKeys) > 0 {
			return q.InsertMigrationThreads(ctx, threads)
		}
		return nil
	}, cp)
}

// planHelpdeskFinish writes the dry run; the team suggestions are the help desk's agents, by the
// conversations assigned to them.
func (r *migrationRun) planHelpdeskFinish(ctx context.Context, cp migrationCheckpoint) error {
	s, m := r.s, r.m
	q := s.st.Queries
	src, err := r.desk(ctx)
	if err != nil {
		return deskErr(err)
	}
	agents, err := src.Agents(ctx)
	if err != nil {
		return deskErr(err)
	}
	if err := r.saveThrottle(ctx); err != nil {
		return err
	}
	contacts, err := q.CountHelpdeskPlanContacts(ctx, store.CountHelpdeskPlanContactsParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID})
	if err != nil {
		return err
	}
	folders, err := q.ListMigrationFolders(ctx, store.ListMigrationFoldersParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID})
	if err != nil {
		return err
	}
	est, err := s.dryRun(ctx, q, m, folders, cp.Plan.Totals, contacts.Existing, contacts.Total-contacts.Existing)
	if err != nil {
		return err
	}
	est.Skipped.Spam += cp.Plan.Totals.Skipped[helpdeskReasonSpam]
	est.Skipped.Trash += cp.Plan.Totals.Skipped[helpdeskReasonTrash]
	assigned, err := q.ListHelpdeskAssignees(ctx, store.ListHelpdeskAssigneesParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID})
	if err != nil {
		return err
	}
	counts := map[string]int64{}
	names := map[string]string{}
	for _, a := range assigned {
		counts[a.Email], names[a.Email] = a.Conversations, a.Name
	}
	for _, a := range agents {
		if a.Email == "" {
			continue
		}
		if _, ok := counts[a.Email]; !ok {
			counts[a.Email] = 0
		}
		if a.Name != "" {
			names[a.Email] = a.Name
		}
	}
	addrs := make([]string, 0, len(counts))
	for a := range counts {
		addrs = append(addrs, a)
	}
	sort.Slice(addrs, func(i, j int) bool {
		if counts[addrs[i]] != counts[addrs[j]] {
			return counts[addrs[i]] > counts[addrs[j]]
		}
		return addrs[i] < addrs[j]
	})
	suggestions := []oas.MigrationTeamSuggestion{}
	for _, a := range addrs[:min(len(addrs), teamSuggestions)] {
		sg := oas.MigrationTeamSuggestion{Address: a, Messages: counts[a]}
		if names[a] != "" {
			sg.Name = new(names[a])
		}
		suggestions = append(suggestions, sg)
	}
	cp.Phase, cp.Plan = scanPhasePreview, nil
	return r.savePlan(ctx, func(q *store.Queries) error {
		return q.SetMigrationEstimate(ctx, store.SetMigrationEstimateParams{WorkspaceID: m.WorkspaceID, ID: m.ID, Estimate: mustJSON(migrationEstimateDoc{DryRun: est, TeamSuggestions: suggestions})})
	}, cp)
}

// deskText is a message's visible text as a preview shows it.
func deskText(msg helpdesk.Message) string {
	text := msg.Text
	if msg.Raw != nil {
		if parsed, err := email.Parse(msg.Raw); err == nil {
			text = parsed.Text
		}
	}
	visible, _ := reply.Strip(text)
	return truncateRunes(strings.TrimSpace(visible), previewTextRunes)
}

func (r *migrationRun) previewHelpdesk(ctx context.Context) error {
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
		tickets := map[string]helpdesk.Ticket{}
		for _, it := range items {
			var t helpdesk.Ticket
			_ = json.Unmarshal(it.Meta, &t)
			tickets[it.ThreadKey] = t
		}
		team, err := s.teamOf(ctx, q, m)
		if err != nil {
			return err
		}
		src, err := r.desk(ctx)
		if err != nil {
			return deskErr(err)
		}
		for _, th := range threads {
			t, ok := tickets[th.ThreadKey]
			if !ok {
				continue
			}
			msgs, err := src.Messages(ctx, t)
			if err != nil {
				_ = r.saveThrottle(ctx)
				return deskErr(err)
			}
			var plan migrate.ThreadPlan
			_ = json.Unmarshal(th.Plan, &plan)
			pt := oas.MigrationPreviewThread{
				Subject: plan.Subject, Contact: oas.ImportedAuthor{Name: plan.Contact.Name, Address: plan.Contact.Email},
				Status: oas.ConversationStatus(plan.Status), Labels: plan.Labels, MessagesTotal: int32(len(msgs)), Messages: []oas.MigrationPreviewMessage{},
			}
			if pt.Labels == nil {
				pt.Labels = []string{}
			}
			if plan.Assignee != nil {
				pt.Assignee = &oas.ImportedAuthor{Name: plan.Assignee.Name, Address: plan.Assignee.Email}
				if id, ok := team.members[plan.Assignee.Email]; ok {
					pt.AssigneeMemberId = &id
				}
			}
			for _, msg := range msgs[:min(len(msgs), previewMessages)] {
				dir := msg.Direction
				if msg.Note {
					dir = "out"
				}
				pm := oas.MigrationPreviewMessage{
					Direction: oas.Direction(dir), At: msg.At, From: oas.ImportedAuthor{Name: msg.Author.Name, Address: msg.Author.Email},
					Text: deskText(msg), Attachments: int32(len(msg.Attachments)),
				}
				if msg.Note {
					pm.Note = new(true)
				}
				if id, ok := team.members[msg.Author.Email]; ok && dir == "out" {
					pm.MemberId = &id
				}
				pt.Messages = append(pt.Messages, pm)
			}
			out.Items = append(out.Items, pt)
		}
		if err := r.saveThrottle(ctx); err != nil {
			return err
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

// folderInboxes is the inbox each of a help desk's brands or mailboxes goes to.
func folderInboxes(m store.Migration, rows []store.MigrationFolder) map[int32]uuid.UUID {
	byName := map[string]uuid.UUID{}
	if opts := migrationOptions(m.Options); opts.Inboxes != nil {
		for _, ib := range *opts.Inboxes {
			byName[ib.Folder] = ib.InboxId
		}
	}
	out := map[int32]uuid.UUID{}
	for _, f := range rows {
		if id, ok := byName[f.Name]; ok {
			out[f.ID] = id
		} else if m.InboxID != nil {
			out[f.ID] = *m.InboxID
		}
	}
	return out
}

type deskTarget struct {
	inbox   uuid.UUID
	channel *uuid.UUID
}

// deskTargets binds each brand or mailbox to its inbox's e-mail channel: the one at the folder's
// support address, else the inbox's oldest; the migration's own inbox keeps its channel.
func (r *migrationRun) deskTargets(ctx context.Context) (map[int32]deskTarget, deskTarget, error) {
	q, m := r.s.st.Queries, r.m
	def := deskTarget{inbox: *m.InboxID, channel: m.ChannelID}
	rows, err := q.ListMigrationFolders(ctx, store.ListMigrationFoldersParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID})
	if err != nil {
		return nil, def, err
	}
	out := map[int32]deskTarget{}
	for folder, inbox := range folderInboxes(m, rows) {
		if inbox == def.inbox {
			out[folder] = def
			continue
		}
		if _, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: m.WorkspaceID, ID: inbox}); store.IsNotFound(err) {
			out[folder] = def
			continue
		} else if err != nil {
			return nil, def, err
		}
		t := deskTarget{inbox: inbox}
		channels, err := q.ListChannels(ctx, store.ListChannelsParams{WorkspaceID: m.WorkspaceID, InboxID: inbox})
		if err != nil {
			return nil, def, err
		}
		var ids []uuid.UUID
		for _, c := range channels {
			if c.Kind == string(oas.ChannelKindEmail) {
				ids = append(ids, c.ID)
			}
		}
		if len(ids) > 0 {
			emails, err := q.ListEmailChannels(ctx, store.ListEmailChannelsParams{WorkspaceID: m.WorkspaceID, ChannelIds: ids})
			if err != nil {
				return nil, def, err
			}
			t.channel = &ids[0]
			for _, f := range rows {
				if f.ID != folder || f.Address == nil {
					continue
				}
				for _, e := range emails {
					if strings.EqualFold(e.Address, *f.Address) {
						t.channel = new(e.ChannelID)
					}
				}
			}
		}
		out[folder] = t
	}
	return out, def, nil
}

// copyHelpdesk imports planned tickets, newest first, each with all its messages and notes.
func (r *migrationRun) copyHelpdesk(ctx context.Context) (MigrationSlice, bool, error) {
	s, m := r.s, r.m
	if m.ChannelID == nil || m.InboxID == nil {
		return MigrationSlice{Done: true}, false, r.targetGone(ctx)
	}
	if wait, err := r.monthlyLimit(ctx); err != nil || !wait.IsZero() {
		return MigrationSlice{Until: wait}, false, err
	}
	team, err := s.teamOf(ctx, s.st.Queries, m)
	if err != nil {
		return MigrationSlice{}, false, err
	}
	targets, def, err := r.deskTargets(ctx)
	if err != nil {
		return MigrationSlice{}, false, err
	}
	stop := func(err error) (MigrationSlice, bool, error) {
		if serr := r.saveThrottle(ctx); serr != nil {
			return MigrationSlice{}, false, serr
		}
		var wait *waitError
		var src *sourceError
		switch {
		case errors.As(err, &wait):
			res, err := r.waitFor(ctx, wait)
			return res, false, err
		case errors.As(err, &src):
			res, err := r.trouble(ctx, src.err)
			return res, false, err
		}
		return MigrationSlice{}, false, err
	}
	for r.timeLeft() {
		ok, err := r.renew(ctx)
		if err != nil {
			return MigrationSlice{}, false, err
		}
		if !ok {
			_, _, err := s.refreshMigrationCounts(ctx, s.st.Queries, r.m)
			return MigrationSlice{Done: true}, false, err
		}
		batch, err := s.st.NextMigrationItems(ctx, store.NextMigrationItemsParams{
			WorkspaceID: m.WorkspaceID, MigrationID: m.ID, Threads: copyBatchThreads, Lim: copyBatchThreads,
		})
		if err != nil {
			return MigrationSlice{}, false, err
		}
		if len(batch) == 0 {
			return MigrationSlice{}, true, r.saveThrottle(ctx)
		}
		src, err := r.desk(ctx)
		if err != nil {
			return stop(deskErr(err))
		}
		for _, it := range batch {
			if !r.timeLeft() {
				break
			}
			tgt := def
			if it.FolderID != nil {
				if t, ok := targets[*it.FolderID]; ok {
					tgt = t
				}
			}
			if err := r.copyTicket(ctx, src, team, tgt, it); err != nil {
				return stop(err)
			}
		}
		if err := r.saveThrottle(ctx); err != nil {
			return MigrationSlice{}, false, err
		}
		if err := r.clearWaiting(ctx); err != nil {
			return MigrationSlice{}, false, err
		}
		if err := r.flush(ctx, false); err != nil {
			return MigrationSlice{}, false, err
		}
	}
	return MigrationSlice{}, false, r.flush(ctx, true)
}

// copyTicket reads a ticket's messages and imports those not imported yet, each in a transaction
// of its own with an id derived from the migration and its source, so a retry or a later change
// to the ticket adds only what is new.
func (r *migrationRun) copyTicket(ctx context.Context, src helpdesk.Source, team migrationTeam, tgt deskTarget, it store.NextMigrationItemsRow) error {
	s, m := r.s, r.m
	var t helpdesk.Ticket
	_ = json.Unmarshal(it.Meta, &t)
	msgs, err := src.Messages(ctx, t)
	var se *helpdesk.StatusError
	if errors.As(err, &se) && se.Status == http.StatusNotFound {
		s.addMigrationError(ctx, m, "gone", "the ticket is no longer in the help desk", "", &t.UpdatedAt, t.Subject)
		return s.finishMigrationItem(ctx, m, it.SourceID, it.ThreadKey, migrate.StateSkipped, migrate.ReasonGone)
	}
	if err != nil {
		return deskErr(err)
	}
	sort.SliceStable(msgs, func(a, b int) bool {
		if !msgs[a].At.Equal(msgs[b].At) {
			return msgs[a].At.Before(msgs[b].At)
		}
		return msgs[a].ID < msgs[b].ID
	})
	thread, err := s.st.LockMigrationThread(ctx, store.LockMigrationThreadParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, ThreadKey: it.ThreadKey})
	if err != nil {
		return err
	}
	requester := t.Requester
	if thread.ConversationID == nil {
		if requester, err = src.Customer(ctx, requester); err != nil {
			return deskErr(err)
		}
	}
	for _, msg := range msgs {
		msgID := migrate.NameID(m.ID, it.SourceID+":"+msg.ID)
		exists, err := s.st.ImportedMessageExists(ctx, store.ImportedMessageExistsParams{WorkspaceID: m.WorkspaceID, ID: msgID})
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		atts, err := r.deskAttachments(ctx, src, t, msg)
		if err != nil {
			return deskErr(err)
		}
		err = s.importDeskMessage(ctx, m, team, tgt, it, t, requester, msg, msgID, atts)
		if errors.Is(err, errDeskBlocked) {
			return s.finishMigrationItem(ctx, m, it.SourceID, it.ThreadKey, migrate.StateSkipped, migrate.ReasonBlocked)
		}
		if err != nil {
			return err
		}
	}
	thread, err = s.st.LockMigrationThread(ctx, store.LockMigrationThreadParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, ThreadKey: it.ThreadKey})
	if err != nil {
		return err
	}
	if thread.ConversationID == nil {
		return s.finishMigrationItem(ctx, m, it.SourceID, it.ThreadKey, migrate.StateSkipped, helpdeskReasonEmpty)
	}
	return s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.LockMigrationItem(ctx, store.LockMigrationItemParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, SourceID: it.SourceID})
		if store.IsNotFound(err) || (err == nil && cur.State != migrate.StatePlanned) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := q.SetMigrationItemDone(ctx, store.SetMigrationItemDoneParams{
			WorkspaceID: m.WorkspaceID, MigrationID: m.ID, SourceID: it.SourceID, State: migrate.StateImported,
		}); err != nil {
			return err
		}
		return q.AddMigrationThreadDone(ctx, store.AddMigrationThreadDoneParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, ThreadKey: it.ThreadKey})
	})
}

// deskAttachments fetches a message's files within the attachment limit; a file the help desk
// no longer has, or one too large, stays behind.
func (r *migrationRun) deskAttachments(ctx context.Context, src helpdesk.Source, t helpdesk.Ticket, msg helpdesk.Message) ([]email.Attachment, error) {
	if msg.Raw != nil {
		return nil, nil
	}
	var out []email.Attachment
	for _, a := range msg.Attachments {
		if a.Size > r.s.attach.MaxBytes {
			continue
		}
		data, err := src.Attachment(ctx, t, a)
		var se *helpdesk.StatusError
		var refused *helpdesk.RefusedError
		switch {
		case errors.Is(err, helpdesk.ErrTooLarge), errors.As(err, &refused),
			errors.As(err, &se) && (se.Status == http.StatusNotFound || se.Status == http.StatusGone || se.Status == http.StatusForbidden):
			continue
		case err != nil:
			return nil, err
		}
		out = append(out, email.Attachment{Filename: a.Name, ContentType: a.Type, Inline: a.Inline, Data: data})
	}
	return out, nil
}

func deskAttributes(source string, p helpdesk.Person) map[string]string {
	d, _ := helpdesk.Lookup(source)
	out := map[string]string{d.AttributePrefix + "_id": p.ID}
	if p.ExternalID != "" {
		out[d.AttributePrefix+"_external_id"] = p.ExternalID
	}
	if p.Phone != "" {
		out["phone"] = p.Phone
	}
	return out
}

// deskContact finds or creates the contact of a requester or customer: by address, else by an id
// derived from the migration and the help desk's own id, so a retry finds the same one.
func (s *Server) deskContact(ctx context.Context, q *store.Queries, m store.Migration, p helpdesk.Person) (uuid.UUID, error) {
	var id uuid.UUID
	if p.Email != "" {
		cid, blocked, err := s.importContact(ctx, q, m, personAddress(p))
		if err != nil {
			return cid, err
		}
		if blocked {
			return cid, errDeskBlocked
		}
		id = cid
	} else {
		id = migrate.NameID(m.ID, "contact:"+m.Source+":"+p.ID)
		c, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: m.WorkspaceID, ID: id})
		switch {
		case err == nil && c.Blocked:
			return id, errDeskBlocked
		case store.IsNotFound(err):
			if _, err := q.CreateImportedContact(ctx, store.CreateImportedContactParams{
				ID: id, WorkspaceID: m.WorkspaceID, Name: truncateRunes(p.Name, 200), Now: s.now(), MigrationID: &m.ID,
			}); err != nil {
				return id, err
			}
			if err := q.RefreshContactSearch(ctx, store.RefreshContactSearchParams{WorkspaceID: m.WorkspaceID, ID: id}); err != nil {
				return id, err
			}
		case err != nil:
			return id, err
		}
	}
	return id, q.MergeImportedContactAttributes(ctx, store.MergeImportedContactAttributesParams{
		WorkspaceID: m.WorkspaceID, ID: id, Attributes: mustJSON(deskAttributes(m.Source, p)),
	})
}

// importDeskMessage stores one message or note of a ticket, creating the ticket's conversation
// with its first. History is quiet; a customer's message written after the migration went live
// takes the live path, with its events, as mail found by catch-up sync does.
func (s *Server) importDeskMessage(ctx context.Context, m store.Migration, team migrationTeam, tgt deskTarget, it store.NextMigrationItemsRow,
	t helpdesk.Ticket, requester helpdesk.Person, msg helpdesk.Message, msgID uuid.UUID, atts []email.Attachment,
) error {
	ws := m.WorkspaceID
	now := s.now()
	at := msg.At
	if at.IsZero() || at.After(now) {
		at = now
	}
	live := m.LiveFrom != nil && at.After(*m.LiveFrom) && !msg.Note
	visible := truncateRunes(strings.TrimSpace(msg.Text), maxMessageBodyRunes)
	html := s.sanitizeHTML(msg.HTML, maxMessageHTMLBytes)
	var parsed *email.Message
	var content mailContent
	if msg.Raw != nil {
		if p, err := email.Parse(msg.Raw); err == nil {
			parsed, content = p, s.mailContent(p)
			visible, html, atts = content.visible, content.html, p.Attachments
		}
	}
	if visible == "" && html == nil && len(atts) == 0 {
		return nil
	}
	var stored []string
	var rawKey *string
	var rawSize *int64
	if parsed != nil && migrationOptions(m.Options).KeepOriginals {
		key := ws.String() + "/raw/" + msgID.String() + ".eml"
		if err := s.objects.Put(ctx, key, bytes.NewReader(msg.Raw), int64(len(msg.Raw)), "message/rfc822"); err != nil {
			return err
		}
		rawKey, rawSize, stored = &key, new(int64(len(msg.Raw))), append(stored, key)
	}
	files, _, err := s.keepAttachments(ctx, ws, atts, s.serverLimits(), func(i int) uuid.UUID { return migrate.NameID(msgID, "attachment:"+strconv.Itoa(i)) })
	for _, f := range files {
		stored = append(stored, f.key)
	}
	if err != nil {
		s.deleteObjects(ctx, stored)
		return err
	}
	kept := false
	err = s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
		cur, err := q.LockMigrationItem(ctx, store.LockMigrationItemParams{WorkspaceID: ws, MigrationID: m.ID, SourceID: it.SourceID})
		if store.IsNotFound(err) || (err == nil && cur.State != migrate.StatePlanned) {
			return nil
		}
		if err != nil {
			return err
		}
		if exists, err := q.ImportedMessageExists(ctx, store.ImportedMessageExistsParams{WorkspaceID: ws, ID: msgID}); err != nil || exists {
			return err
		}
		thread, err := q.LockMigrationThread(ctx, store.LockMigrationThreadParams{WorkspaceID: ws, MigrationID: m.ID, ThreadKey: it.ThreadKey})
		if err != nil {
			return err
		}
		var plan migrate.ThreadPlan
		_ = json.Unmarshal(thread.Plan, &plan)
		var conv store.Conversation
		newConv := thread.ConversationID == nil
		if newConv {
			contact, err := s.deskContact(ctx, q, m, requester)
			if err != nil {
				return err
			}
			var channel *uuid.UUID
			if requester.Email != "" {
				channel = tgt.channel
			}
			status := plan.Status
			if status == "" {
				status = migrate.StatusClosed
			}
			var closedAt *time.Time
			if status == migrate.StatusClosed {
				closedAt = new(plan.LastAt)
				if at.After(plan.LastAt) {
					closedAt = &at
				}
			}
			conv, err = q.ImportConversation(ctx, store.ImportConversationParams{
				ID: migrate.NameID(m.ID, "thread:"+it.ThreadKey), WorkspaceID: ws, InboxID: tgt.inbox, ContactID: contact, ChannelID: channel,
				Subject: truncateRunes(plan.Subject, 500), Status: status, At: &at, ClosedAt: closedAt, Now: now, MigrationID: &m.ID,
			})
			if err != nil {
				return err
			}
			if plan.Assignee != nil {
				if member, ok := team.members[plan.Assignee.Email]; ok {
					if err := q.SetImportedConversationAssignee(ctx, store.SetImportedConversationAssigneeParams{WorkspaceID: ws, ID: conv.ID, AssigneeID: &member}); err != nil {
						return err
					}
					conv.AssigneeID = &member
				}
			}
			if err := s.labelImported(ctx, q, ws, conv.ID, plan.Labels); err != nil {
				return err
			}
			if err := q.SetMigrationThreadConversation(ctx, store.SetMigrationThreadConversationParams{
				WorkspaceID: ws, MigrationID: m.ID, ThreadKey: it.ThreadKey, ConversationID: &conv.ID,
			}); err != nil {
				return err
			}
			if live {
				events.conversation(realtime.ConversationCreated, conv, conversationBody(conv, nil))
			}
		} else if conv, err = q.GetConversation(ctx, store.GetConversationParams{WorkspaceID: ws, ID: *thread.ConversationID}); err != nil {
			return err
		}
		member, mapped := team.members[msg.Author.Email]
		var author []byte
		if !mapped {
			author = mustJSON(oas.ImportedAuthor{Name: msg.Author.Name, Address: msg.Author.Email})
		}
		if msg.Note {
			p := store.ImportNoteParams{ID: msgID, WorkspaceID: ws, ConversationID: conv.ID, Body: visible, Html: html, CreatedAt: at, ImportedAt: &now, ImportedAuthor: author}
			if mapped {
				p.AuthorMemberID = &member
			}
			if err := q.ImportNote(ctx, p); err != nil {
				return err
			}
		} else {
			if parsed != nil && conv.ChannelID != nil {
				headerID := parsed.MessageID
				if headerID == "" || len(headerID) > 998 {
					headerID = email.SynthesizedID(msg.Raw)
				}
				if _, err := q.FindChannelEmailByHeader(ctx, store.FindChannelEmailByHeaderParams{WorkspaceID: ws, ChannelID: conv.ChannelID, HeaderMessageID: headerID}); err == nil {
					return nil
				} else if !store.IsNotFound(err) {
					return err
				}
			}
			dir := msg.Direction
			p := store.ImportMessageParams{ID: msgID, WorkspaceID: ws, ConversationID: conv.ID, Direction: &dir, Body: visible, Html: html, CreatedAt: at, ImportedAt: &now}
			if dir == string(oas.In) {
				contact := conv.ContactID
				if msg.Author.ID != requester.ID || (msg.Author.Email != "" && msg.Author.Email != requester.Email) {
					id, err := s.deskContact(ctx, q, m, msg.Author)
					if errors.Is(err, errDeskBlocked) {
						return nil
					}
					if err != nil {
						return err
					}
					contact = id
				}
				p.AuthorType, p.AuthorContactID = string(oas.AuthorTypeContact), &contact
				if live {
					p.ImportedAt = nil
				}
			} else {
				p.AuthorType, p.DeliveryState = string(oas.AuthorTypeMember), new(deliverySent)
				if mapped {
					p.AuthorMemberID = &member
				} else {
					p.ImportedAuthor = author
				}
			}
			if err := q.ImportMessage(ctx, p); err != nil {
				return err
			}
			if parsed != nil && conv.ChannelID != nil {
				headerID := parsed.MessageID
				if headerID == "" || len(headerID) > 998 {
					headerID = email.SynthesizedID(msg.Raw)
				}
				if err := q.ImportMessageEmail(ctx, store.ImportMessageEmailParams{
					WorkspaceID: ws, MessageID: msgID, ConversationID: conv.ID, ChannelID: conv.ChannelID, Direction: dir, HeaderMessageID: headerID,
					InReplyTo: content.inReplyTo, ReferencesIds: content.refs, FromAddress: parsed.From.Email, ToAddresses: normalizedAddressList(parsed.To),
					CcAddresses: normalizedAddressList(parsed.Cc), Subject: content.subject, FullText: parsed.Text, FullHtml: content.fullHTML,
					Quoted: content.quoted, Headers: content.headers, RawKey: rawKey, RawSize: rawSize, AuthenticationResults: nonEmpty(parsed.AuthResults),
					Auto: parsed.Auto, CreatedAt: at, ImportedAt: &now,
				}); err != nil {
					return err
				}
			}
		}
		var total int64
		if rawSize != nil {
			total = *rawSize
		}
		var created []store.Attachment
		for _, f := range files {
			a, err := q.CreateAttachment(ctx, store.CreateAttachmentParams{
				ID: f.id, WorkspaceID: ws, ConversationID: conv.ID, MessageID: msgID, StorageKey: f.key, Filename: f.filename,
				ContentType: f.contentType, SizeBytes: f.size, ContentID: f.contentID, Inline: f.inline, CreatedAt: at,
			})
			if err != nil {
				return err
			}
			created = append(created, a)
			total += f.size
		}
		if live {
			if err := q.TouchConversation(ctx, store.TouchConversationParams{WorkspaceID: ws, ID: conv.ID, Now: at, IsMessage: true}); err != nil {
				return err
			}
			if msg.Direction == string(oas.In) && !newConv {
				if conv, err = s.reopenConversation(ctx, q, events, ws, conv.ID, now); err != nil {
					return err
				}
			}
			row, err := q.GetMessage(ctx, store.GetMessageParams{WorkspaceID: ws, ID: msgID})
			if err != nil {
				return err
			}
			events.conversation(realtime.MessageCreated, conv, messageBody(messageRow(row), created))
		} else if err := q.ImportTouchConversation(ctx, store.ImportTouchConversationParams{WorkspaceID: ws, ID: conv.ID, At: at}); err != nil {
			return err
		}
		convs := int64(0)
		if newConv {
			convs = 1
		}
		if err := q.AddImportedUsage(ctx, store.AddImportedUsageParams{WorkspaceID: ws, Month: monthOf(now), Conversations: convs, Messages: 1, Bytes: total}); err != nil {
			return err
		}
		kept = true
		return nil
	})
	if err != nil || !kept {
		s.deleteObjects(ctx, stored)
	}
	return err
}

// syncHelpdesk reads what changed in the help desk since the last pass: a ticket that changed is
// planned again, and the copy adds the messages it gained.
func (r *migrationRun) syncHelpdesk(ctx context.Context) (syncPass, error) {
	var out syncPass
	doc := readSourceState(r.m.SourceState)
	src, err := r.desk(ctx)
	if err != nil {
		return out, deskErr(err)
	}
	rows, err := r.s.st.ListMigrationFolders(ctx, store.ListMigrationFoldersParams{WorkspaceID: r.m.WorkspaceID, MigrationID: r.m.ID})
	if err != nil {
		return out, err
	}
	refs := folderRefs(rows)
	for r.timeLeft() {
		if ok, err := r.renew(ctx); err != nil || !ok {
			return out, err
		}
		if !doc.SyncStarted {
			doc.Sync.StartedAt, doc.SyncStarted = r.s.now(), true
		}
		page, err := src.Tickets(ctx, doc.Sync)
		if err != nil {
			if serr := r.saveThrottle(ctx); serr != nil {
				return out, serr
			}
			return out, deskErr(err)
		}
		next := doc
		next.Sync = page.Next
		next.Sync.StartedAt = doc.Sync.StartedAt
		if page.End {
			if d, _ := deskOf(r.m); d.Sync == helpdesk.SyncIncremental {
				next.Sync = helpdesk.Cursor{After: page.Next.After}
				if next.Sync.After == "" {
					next.Sync = doc.Sync
				}
			} else {
				next.Sync = helpdesk.Cursor{ModifiedSince: doc.Sync.StartedAt.Add(-syncOverlap)}
			}
			next.SyncStarted = false
		}
		planned := false
		err = r.s.st.InTx(ctx, func(q *store.Queries) error {
			d, err := r.deskPlanSetup(ctx, q)
			if err != nil {
				return err
			}
			blocked, err := blockedRequesters(ctx, q, r.m.WorkspaceID, page.Tickets)
			if err != nil {
				return err
			}
			for _, t := range page.Tickets {
				got, err := r.syncTicket(ctx, q, d, blocked, refs[t.Folder], t)
				if err != nil {
					return err
				}
				planned = planned || got
			}
			return r.setSourceState(ctx, q, next)
		})
		if err != nil {
			return out, err
		}
		doc = next
		out.planned, out.stored = out.planned || planned, out.stored || planned
		if err := r.saveThrottle(ctx); err != nil {
			return out, err
		}
		if page.End {
			out.finished = true
			return out, nil
		}
	}
	return out, nil
}

func (r *migrationRun) syncTicket(ctx context.Context, q *store.Queries, d *deskPlan, blocked map[string]bool, folder int32, t helpdesk.Ticket) (bool, error) {
	ws, id := r.m.WorkspaceID, r.m.ID
	sourceID := r.m.Source + ":" + t.ID
	state, reason, plan, err := d.decide(ctx, q, t, folder, blocked)
	if err != nil {
		return false, err
	}
	meta := mustJSON(t)
	cur, err := q.GetHelpdeskItem(ctx, store.GetHelpdeskItemParams{WorkspaceID: ws, MigrationID: id, SourceID: sourceID})
	switch {
	case store.IsNotFound(err):
		if err := q.UpsertHelpdeskItems(ctx, store.UpsertHelpdeskItemsParams{
			WorkspaceID: ws, MigrationID: id, SourceIds: []string{sourceID}, FolderIds: []int32{folder}, ReceivedAts: []time.Time{t.UpdatedAt}, Metas: [][]byte{meta},
		}); err != nil {
			return false, err
		}
	case err != nil:
		return false, err
	}
	if state == migrate.StatePlanned {
		if err := q.ReplanHelpdeskItem(ctx, store.ReplanHelpdeskItemParams{
			WorkspaceID: ws, MigrationID: id, SourceID: sourceID, Meta: meta, ReceivedAt: t.UpdatedAt, FolderID: folder,
		}); err != nil {
			return false, err
		}
		return true, q.ReopenHelpdeskThread(ctx, store.ReopenHelpdeskThreadParams{WorkspaceID: ws, MigrationID: id, ThreadKey: sourceID, LastAt: plan.LastAt, Plan: mustJSON(plan)})
	}
	if err == nil && cur.State == migrate.StatePlanned {
		if err := q.SkipHelpdeskItem(ctx, store.SkipHelpdeskItemParams{
			WorkspaceID: ws, MigrationID: id, SourceID: sourceID, Meta: meta, ReceivedAt: t.UpdatedAt, FolderID: folder, Reason: reason,
		}); err != nil {
			return false, err
		}
		return false, q.AddMigrationThreadDone(ctx, store.AddMigrationThreadDoneParams{WorkspaceID: ws, MigrationID: id, ThreadKey: sourceID})
	}
	return false, nil
}

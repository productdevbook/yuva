package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/migrate"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	copyBatchThreads = 20
	copyBatchItems   = 50
	copyBatchBytes   = 16 << 20
)

// permanentError skips a message for good with a reason.
type permanentError struct{ reason, detail string }

func (e *permanentError) Error() string { return e.reason + ": " + e.detail }

func firstOfNextMonth(t time.Time) time.Time {
	y, mo, _ := t.UTC().Date()
	return time.Date(y, mo+1, 1, 0, 0, 0, 0, time.UTC)
}

func (r *migrationRun) copy(ctx context.Context) (MigrationSlice, error) {
	res, empty, err := r.copyPlanned(ctx)
	if err != nil || !empty {
		return res, err
	}
	if err := r.copied(ctx); err != nil {
		return MigrationSlice{}, err
	}
	if r.m.State == migrationCompleted {
		r.close()
		r.s.dropUploadLogged(ctx, r.m.WorkspaceID, r.m.ID)
		return MigrationSlice{Done: true}, nil
	}
	return MigrationSlice{Until: r.s.now()}, nil
}

func (r *migrationRun) targetGone(ctx context.Context) error {
	return r.needsAttention(ctx, "target_gone", "the inbox or e-mail channel was deleted",
		"The inbox or e-mail channel the history goes to was deleted; choose another and resume.")
}

// copyPlanned imports planned messages, newest threads first, until none is left (empty) or the
// slice's time is up; a result that is Done or waits ends the slice.
func (r *migrationRun) copyPlanned(ctx context.Context) (MigrationSlice, bool, error) {
	if helpdeskMigration(r.m) {
		return r.copyHelpdesk(ctx)
	}
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
	folders, err := r.folderMap(ctx)
	if err != nil {
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
			WorkspaceID: m.WorkspaceID, MigrationID: m.ID, Threads: copyBatchThreads, Lim: copyBatchItems,
		})
		if err != nil {
			return MigrationSlice{}, false, err
		}
		if len(batch) == 0 {
			return MigrationSlice{}, true, nil
		}
		var size int64
		for i, it := range batch {
			if i > 0 && size+it.SizeBytes > copyBatchBytes {
				batch = batch[:i]
				break
			}
			size += it.SizeBytes
		}
		if err := r.budget(size); err != nil {
			var wait *waitError
			if errors.As(err, &wait) {
				res, err := r.waitFor(ctx, wait)
				return res, false, err
			}
			return MigrationSlice{}, false, err
		}
		ids := make([]string, len(batch))
		for i, it := range batch {
			ids[i] = it.SourceID
		}
		started := time.Now()
		fetched, read, changed, err := r.fetch(ctx, folders, ids)
		var src *sourceError
		if errors.As(err, &src) {
			res, err := r.trouble(ctx, src.err)
			return res, false, err
		}
		if err != nil {
			return MigrationSlice{}, false, err
		}
		if changed != nil {
			if err := r.refolder(ctx, *changed, team); err != nil {
				if errors.As(err, &src) {
					res, err := r.trouble(ctx, src.err)
					return res, false, err
				}
				return MigrationSlice{}, false, err
			}
			if folders, err = r.folderMap(ctx); err != nil {
				return MigrationSlice{}, false, err
			}
			continue
		}
		r.throttle.Read(s.now(), read)
		if err := r.clearWaiting(ctx); err != nil {
			return MigrationSlice{}, false, err
		}
		if err := r.saveThrottle(ctx); err != nil {
			return MigrationSlice{}, false, err
		}
		for _, it := range batch {
			if err := r.copyItem(ctx, team, folders, it, fetched[it.SourceID]); err != nil {
				return MigrationSlice{}, false, err
			}
		}
		if err := r.flush(ctx, false); err != nil {
			return MigrationSlice{}, false, err
		}
		if d := migrate.Pace(r.preset.Budget, read, time.Since(started)); d > 0 {
			select {
			case <-ctx.Done():
				return MigrationSlice{}, false, ctx.Err()
			case <-time.After(min(d, time.Until(r.deadline))):
			}
		}
	}
	return MigrationSlice{}, false, r.flush(ctx, true)
}

func (r *migrationRun) folderMap(ctx context.Context) (map[int32]store.MigrationFolder, error) {
	rows, err := r.s.st.ListMigrationFolders(ctx, store.ListMigrationFoldersParams{WorkspaceID: r.m.WorkspaceID, MigrationID: r.m.ID})
	if err != nil {
		return nil, err
	}
	out := make(map[int32]store.MigrationFolder, len(rows))
	for _, f := range rows {
		out[f.ID] = f
	}
	return out, nil
}

// monthlyLimit waits for next month once the workspace imported YUVA_MIGRATION_MAX_BYTES.
func (r *migrationRun) monthlyLimit(ctx context.Context) (time.Time, error) {
	s := r.s
	if s.migrations.MaxBytes <= 0 {
		return time.Time{}, nil
	}
	used, err := s.st.ImportedBytesThisMonth(ctx, store.ImportedBytesThisMonthParams{WorkspaceID: r.m.WorkspaceID, Month: monthOf(s.now())})
	if err != nil || used < s.migrations.MaxBytes {
		return time.Time{}, err
	}
	until := firstOfNextMonth(s.now())
	if r.m.StateReason != reasonMonthly {
		s.addMigrationError(ctx, r.m, "hosted_limit", "the workspace reached the server's limit of imported bytes this month", "", nil, "")
	}
	m, err := s.st.SetMigrationRunState(ctx, store.SetMigrationRunStateParams{
		WorkspaceID: r.m.WorkspaceID, ID: r.m.ID, State: r.m.State, StateReason: reasonMonthly, WaitingUntil: &until, Now: s.now(),
	})
	if err == nil {
		r.m = m
		s.migrationProgress(ctx, m)
	}
	return until, err
}

// copied moves a migration whose every message is done on to syncing: mail the old mailbox
// receives from now on is new mail. A file migration has no mailbox to sync and completes.
func (r *migrationRun) copied(ctx context.Context) error {
	if err := r.flush(ctx, true); err != nil {
		return err
	}
	return r.s.inTx(ctx, r.m.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
		cur, err := q.LockMigration(ctx, store.LockMigrationParams{WorkspaceID: r.m.WorkspaceID, ID: r.m.ID})
		if err != nil || cur.State != migrationCopying {
			return err
		}
		n, err := q.CountPlannedMigrationItems(ctx, store.CountPlannedMigrationItemsParams{WorkspaceID: cur.WorkspaceID, MigrationID: cur.ID})
		if err != nil || n > 0 {
			return err
		}
		if fileMigration(cur) {
			m, err := r.s.completeMigration(ctx, q, events, nil, cur, "", "copied")
			if err == nil {
				r.m = m
			}
			return err
		}
		m, err := q.EnterMigrationSync(ctx, store.EnterMigrationSyncParams{WorkspaceID: cur.WorkspaceID, ID: cur.ID, Now: r.s.now()})
		if err != nil {
			return err
		}
		r.m = m
		return r.s.migrationChanged(ctx, q, events, m, &cur.State)
	})
}

func (s *Server) finishMigrationItem(ctx context.Context, m store.Migration, sourceID, threadKey, state, reason string) error {
	return s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.LockMigrationItem(ctx, store.LockMigrationItemParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, SourceID: sourceID})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil || cur.State != migrate.StatePlanned {
			return err
		}
		if err := q.SetMigrationItemDone(ctx, store.SetMigrationItemDoneParams{
			WorkspaceID: m.WorkspaceID, MigrationID: m.ID, SourceID: sourceID, State: state, Reason: reason,
		}); err != nil {
			return err
		}
		return q.AddMigrationThreadDone(ctx, store.AddMigrationThreadDoneParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, ThreadKey: threadKey})
	})
}

func (r *migrationRun) copyItem(ctx context.Context, team migrationTeam, folders map[int32]store.MigrationFolder, it store.NextMigrationItemsRow, f fetchedItem) error {
	s, m := r.s, r.m
	var meta migrate.Meta
	_ = json.Unmarshal(it.Meta, &meta)
	folder := ""
	if it.FolderID != nil {
		folder = folders[*it.FolderID].Name
	}
	skip := func(state, reason, code, detail string) error {
		if code != "" {
			s.addMigrationError(ctx, m, code, detail, folder, &meta.At, meta.Subject)
		}
		return s.finishMigrationItem(ctx, m, it.SourceID, it.ThreadKey, state, reason)
	}
	switch {
	case !f.present:
		return skip(migrate.StateSkipped, migrate.ReasonGone, "gone", "the message is no longer in the mailbox")
	case f.unreadable:
		return skip(migrate.StateSkipped, migrate.ReasonUnparseable, "unparseable", "the message cannot be read from the export file")
	case f.raw == nil:
		return skip(migrate.StateSkipped, migrate.ReasonTooLarge, "too_large", "the message is larger than the server's limit for one message")
	}
	err := s.importMigrationMessage(ctx, m, team, it, meta, f.raw)
	var perm *permanentError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &perm):
		return skip(migrate.StateSkipped, perm.reason, perm.reason, perm.detail)
	case ctx.Err() != nil:
		return ctx.Err()
	}
	s.log.WarnContext(ctx, "migration item", slog.String("migration_id", m.ID.String()), slog.Any("error", err))
	var state string
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		st, err := q.CountMigrationItemAttempt(ctx, store.CountMigrationItemAttemptParams{
			WorkspaceID: m.WorkspaceID, MigrationID: m.ID, SourceID: it.SourceID, MaxAttempts: migrationItemAttempts,
		})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		state = st
		if st != migrate.StateFailed {
			return nil
		}
		return q.AddMigrationThreadDone(ctx, store.AddMigrationThreadDoneParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, ThreadKey: it.ThreadKey})
	})
	if state == migrate.StateFailed {
		s.addMigrationError(ctx, m, "failed", "the message could not be stored after five attempts", folder, &meta.At, meta.Subject)
	}
	return err
}

func (s *Server) importContact(ctx context.Context, q *store.Queries, m store.Migration, a migrate.Address) (uuid.UUID, bool, error) {
	id, err := q.GetContactIDByEmail(ctx, store.GetContactIDByEmailParams{WorkspaceID: m.WorkspaceID, Email: a.Email})
	if err == nil {
		c, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: m.WorkspaceID, ID: id})
		if err != nil {
			return id, false, err
		}
		if c.Blocked {
			return id, true, nil
		}
		if c.Name == "" && a.Name != "" {
			err = q.FillContactName(ctx, store.FillContactNameParams{WorkspaceID: m.WorkspaceID, ID: id, Name: truncateRunes(a.Name, 200)})
		}
		return id, false, err
	}
	if !store.IsNotFound(err) {
		return id, false, err
	}
	id, err = q.CreateImportedContact(ctx, store.CreateImportedContactParams{
		ID: newID(), WorkspaceID: m.WorkspaceID, Name: truncateRunes(a.Name, 200), Now: s.now(), MigrationID: &m.ID,
	})
	if err != nil {
		return id, false, err
	}
	if err := q.AddContactEmail(ctx, store.AddContactEmailParams{WorkspaceID: m.WorkspaceID, ContactID: id, Email: a.Email, Position: 0}); err != nil {
		return id, false, err
	}
	return id, false, q.RefreshContactSearch(ctx, store.RefreshContactSearchParams{WorkspaceID: m.WorkspaceID, ID: id})
}

func (s *Server) importLabel(ctx context.Context, q *store.Queries, ws uuid.UUID, name string) (uuid.UUID, error) {
	if l, err := q.GetLabelByName(ctx, store.GetLabelByNameParams{WorkspaceID: ws, Name: name}); err == nil || !store.IsNotFound(err) {
		return l.ID, err
	}
	if err := q.CreateLabelIfMissing(ctx, store.CreateLabelIfMissingParams{ID: newID(), WorkspaceID: ws, Name: name, Color: defaultLabelColor, Now: s.now()}); err != nil {
		return uuid.Nil(), err
	}
	l, err := q.GetLabelByName(ctx, store.GetLabelByNameParams{WorkspaceID: ws, Name: name})
	return l.ID, err
}

// importMigrationMessage stores one message the way ingest stores live mail, with its original
// time and the imported marker, and writes no event: no webhook, notification or auto-reply
// follows. Its rows and objects take ids derived from the migration and the message's source, so
// a retry writes the same ones. A thread whose earlier messages the channel already has, from an
// export file or an earlier migration, continues in their conversation.
func (s *Server) importMigrationMessage(ctx context.Context, m store.Migration, team migrationTeam, it store.NextMigrationItemsRow, meta migrate.Meta, raw []byte) error {
	ws, channelID := m.WorkspaceID, *m.ChannelID
	parsed, err := email.Parse(raw)
	if err != nil {
		return &permanentError{migrate.ReasonUnparseable, "the message could not be read"}
	}
	from := parsed.From.Email
	name := parsed.From.Name
	if from == "" {
		from, name = meta.From.Email, meta.From.Name
	}
	if from == "" {
		return &permanentError{migrate.ReasonUnparseable, "the message names no sender"}
	}
	headerID := parsed.MessageID
	if headerID == "" || len(headerID) > 998 {
		headerID = email.SynthesizedID(raw)
	}
	at := meta.At
	if at.IsZero() {
		at = migrate.MessageTime(parsed.Date, it.ReceivedAt, s.now())
	}
	if st, err := s.st.GetMigrationItemState(ctx, store.GetMigrationItemStateParams{WorkspaceID: ws, MigrationID: m.ID, SourceID: it.SourceID}); err != nil || st != migrate.StatePlanned {
		if store.IsNotFound(err) {
			return nil
		}
		return err
	}
	msgID := migrate.NameID(m.ID, it.SourceID)
	content := s.mailContent(parsed)
	opts := migrationOptions(m.Options)
	var stored []string
	var rawKey *string
	var rawSize *int64
	if opts.KeepOriginals {
		key := ws.String() + "/raw/" + msgID.String() + ".eml"
		if err := s.objects.Put(ctx, key, bytes.NewReader(raw), int64(len(raw)), "message/rfc822"); err != nil {
			return err
		}
		rawKey, rawSize, stored = &key, new(int64(len(raw))), append(stored, key)
	}
	files, _, err := s.keepAttachments(ctx, ws, parsed.Attachments, s.serverLimits(), func(i int) uuid.UUID {
		return migrate.NameID(msgID, "attachment:"+strconv.Itoa(i))
	})
	for _, f := range files {
		stored = append(stored, f.key)
	}
	if err != nil {
		s.deleteObjects(ctx, stored)
		return err
	}
	keep := false
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.LockMigrationItem(ctx, store.LockMigrationItemParams{WorkspaceID: ws, MigrationID: m.ID, SourceID: it.SourceID})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if cur.State != migrate.StatePlanned {
			keep = cur.State == migrate.StateImported
			return nil
		}
		done := func(state, reason string, message *uuid.UUID) error {
			if err := q.SetMigrationItemDone(ctx, store.SetMigrationItemDoneParams{
				WorkspaceID: ws, MigrationID: m.ID, SourceID: it.SourceID, State: state, Reason: reason, MessageID: message,
			}); err != nil {
				return err
			}
			return q.AddMigrationThreadDone(ctx, store.AddMigrationThreadDoneParams{WorkspaceID: ws, MigrationID: m.ID, ThreadKey: it.ThreadKey})
		}
		if err := q.LockMigrationChannel(ctx, channelID); err != nil {
			return err
		}
		if _, err := q.FindChannelEmailByHeader(ctx, store.FindChannelEmailByHeaderParams{WorkspaceID: ws, ChannelID: &channelID, HeaderMessageID: headerID}); err == nil {
			return done(migrate.StateDuplicate, migrate.ReasonInYuva, nil)
		} else if !store.IsNotFound(err) {
			return err
		}
		thread, err := q.LockMigrationThread(ctx, store.LockMigrationThreadParams{WorkspaceID: ws, MigrationID: m.ID, ThreadKey: it.ThreadKey})
		if err != nil {
			return err
		}
		var plan migrate.ThreadPlan
		_ = json.Unmarshal(thread.Plan, &plan)
		dir := migrate.Direction(team.addrs, from)
		var authorContact *uuid.UUID
		if dir == "in" {
			id, blocked, err := s.importContact(ctx, q, m, migrate.Address{Name: name, Email: from})
			if err != nil {
				return err
			}
			if blocked {
				return done(migrate.StateSkipped, migrate.ReasonBlocked, nil)
			}
			authorContact = &id
		}
		convID, newConv := thread.ConversationID, false
		if convID == nil {
			existing, err := q.FindThreadConversationInChannel(ctx, store.FindThreadConversationInChannelParams{
				ChannelID: &channelID, WorkspaceID: ws, MigrationID: m.ID, ThreadKey: it.ThreadKey,
			})
			switch {
			case err == nil:
				if err := s.labelImported(ctx, q, ws, existing, plan.Labels); err != nil {
					return err
				}
				if err := q.SetMigrationThreadConversation(ctx, store.SetMigrationThreadConversationParams{
					WorkspaceID: ws, MigrationID: m.ID, ThreadKey: it.ThreadKey, ConversationID: &existing,
				}); err != nil {
					return err
				}
				convID = &existing
			case !store.IsNotFound(err):
				return err
			}
		}
		if convID == nil {
			contact, blocked, err := s.importContact(ctx, q, m, plan.Contact)
			if err != nil {
				return err
			}
			if blocked {
				return done(migrate.StateSkipped, migrate.ReasonBlocked, nil)
			}
			status := plan.Status
			if status == "" {
				status = migrate.StatusClosed
			}
			var closedAt *time.Time
			if status == migrate.StatusClosed {
				closedAt = new(plan.LastAt)
				if plan.LastAt.IsZero() {
					closedAt = &at
				}
			}
			conv, err := q.ImportConversation(ctx, store.ImportConversationParams{
				ID: migrate.NameID(m.ID, "thread:"+it.ThreadKey), WorkspaceID: ws, InboxID: *m.InboxID, ContactID: contact, ChannelID: &channelID,
				Subject: truncateRunes(plan.Subject, 500), Status: status, At: &at, ClosedAt: closedAt, Now: s.now(), MigrationID: &m.ID,
			})
			if err != nil {
				return err
			}
			if err := s.labelImported(ctx, q, ws, conv.ID, plan.Labels); err != nil {
				return err
			}
			if err := q.SetMigrationThreadConversation(ctx, store.SetMigrationThreadConversationParams{
				WorkspaceID: ws, MigrationID: m.ID, ThreadKey: it.ThreadKey, ConversationID: &conv.ID,
			}); err != nil {
				return err
			}
			convID, newConv = &conv.ID, true
		}
		now := s.now()
		p := store.ImportMessageParams{ID: msgID, WorkspaceID: ws, ConversationID: *convID, Direction: &dir, Body: content.visible, Html: content.html, CreatedAt: at, ImportedAt: &now}
		if dir == "in" {
			p.AuthorType, p.AuthorContactID = string(oas.AuthorTypeContact), authorContact
		} else {
			p.AuthorType, p.DeliveryState = string(oas.AuthorTypeMember), new(deliverySent)
			if member, ok := team.members[from]; ok {
				p.AuthorMemberID = &member
			} else {
				p.ImportedAuthor = mustJSON(oas.ImportedAuthor{Name: name, Address: from})
			}
		}
		if err := q.ImportMessage(ctx, p); err != nil {
			return err
		}
		if err := q.ImportMessageEmail(ctx, store.ImportMessageEmailParams{
			WorkspaceID: ws, MessageID: msgID, ConversationID: *convID, ChannelID: &channelID, Direction: dir, HeaderMessageID: headerID,
			InReplyTo: content.inReplyTo, ReferencesIds: content.refs, FromAddress: from, ToAddresses: normalizedAddressList(parsed.To),
			CcAddresses: normalizedAddressList(parsed.Cc), Subject: content.subject, FullText: parsed.Text, FullHtml: content.fullHTML,
			Quoted: content.quoted, Headers: content.headers, RawKey: rawKey, RawSize: rawSize, AuthenticationResults: nonEmpty(parsed.AuthResults),
			Auto: parsed.Auto, CreatedAt: at, ImportedAt: &now,
		}); err != nil {
			return err
		}
		var total int64
		if rawSize != nil {
			total = *rawSize
		}
		for _, f := range files {
			if _, err := q.CreateAttachment(ctx, store.CreateAttachmentParams{
				ID: f.id, WorkspaceID: ws, ConversationID: *convID, MessageID: msgID, StorageKey: f.key, Filename: f.filename,
				ContentType: f.contentType, SizeBytes: f.size, ContentID: f.contentID, Inline: f.inline, CreatedAt: at,
			}); err != nil {
				return err
			}
			total += f.size
		}
		if err := q.ImportTouchConversation(ctx, store.ImportTouchConversationParams{WorkspaceID: ws, ID: *convID, At: at}); err != nil {
			return err
		}
		convs := int64(0)
		if newConv {
			convs = 1
		}
		if err := q.AddImportedUsage(ctx, store.AddImportedUsageParams{WorkspaceID: ws, Month: monthOf(now), Conversations: convs, Messages: 1, Bytes: total}); err != nil {
			return err
		}
		keep = true
		return done(migrate.StateImported, "", &msgID)
	})
	if err != nil && store.IsUniqueViolation(err) && !strings.Contains(err.Error(), "contact_emails") {
		s.deleteObjects(ctx, stored)
		return s.finishMigrationItem(ctx, m, it.SourceID, it.ThreadKey, migrate.StateDuplicate, migrate.ReasonInYuva)
	}
	if err != nil {
		if st, serr := s.st.GetMigrationItemState(ctx, store.GetMigrationItemStateParams{WorkspaceID: ws, MigrationID: m.ID, SourceID: it.SourceID}); serr != nil || st != migrate.StateImported {
			s.deleteObjects(ctx, stored)
		}
		return err
	}
	if !keep {
		s.deleteObjects(ctx, stored)
	}
	return nil
}

func (s *Server) labelImported(ctx context.Context, q *store.Queries, ws, conversation uuid.UUID, labels []string) error {
	for _, l := range labels {
		labelID, err := s.importLabel(ctx, q, ws, l)
		if err != nil {
			return err
		}
		if err := q.AddConversationLabel(ctx, store.AddConversationLabelParams{WorkspaceID: ws, ConversationID: conversation, LabelID: labelID}); err != nil {
			return err
		}
	}
	return nil
}

func nonEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// refolder handles a folder whose UIDVALIDITY changed while copying: it reads the folder's
// headers again, moves planned messages to their new UIDs by their dedupe keys, skips those that
// are gone, and plans the messages it had not seen.
func (r *migrationRun) refolder(ctx context.Context, f store.MigrationFolder, team migrationTeam) error {
	s, m := r.s, r.m
	rd, err := r.source(ctx)
	if err != nil {
		return fromSource(err)
	}
	st, err := rd.Examine(ctx, f.Name)
	if err != nil {
		return fromSource(err)
	}
	uids, err := rd.SearchUIDs(ctx, time.Time{})
	if err != nil {
		return fromSource(err)
	}
	now := s.now()
	var items []migrate.Item
	for _, page := range chunks(uids, scanHeaderPage) {
		got, read, err := r.headerItems(ctx, rd, f.ID, st.UIDValidity, page)
		if err != nil {
			return err
		}
		r.throttle.Read(now, read)
		items = append(items, got...)
	}
	byKey := map[string]migrate.Item{}
	var keys []string
	for _, it := range items {
		if _, dup := byKey[it.DedupeKey]; !dup {
			byKey[it.DedupeKey] = it
			keys = append(keys, it.DedupeKey)
		}
	}
	uv := int64(st.UIDValidity)
	label, err := labelName(ctx, s.st.Queries, f)
	if err != nil {
		return err
	}
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		planned, err := q.ListFolderPlannedItems(ctx, store.ListFolderPlannedItemsParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, FolderID: &f.ID})
		if err != nil {
			return err
		}
		for _, p := range planned {
			if it, ok := byKey[p.DedupeKey]; ok {
				if err := q.RenameMigrationItem(ctx, store.RenameMigrationItemParams{
					WorkspaceID: m.WorkspaceID, MigrationID: m.ID, SourceID: p.SourceID, NewSourceID: it.SourceID, ReceivedAt: it.Received,
				}); err != nil {
					return err
				}
				delete(byKey, p.DedupeKey)
				continue
			}
			if err := q.SetMigrationItemDone(ctx, store.SetMigrationItemDoneParams{
				WorkspaceID: m.WorkspaceID, MigrationID: m.ID, SourceID: p.SourceID, State: migrate.StateSkipped, Reason: migrate.ReasonGone,
			}); err != nil {
				return err
			}
			if err := q.AddMigrationThreadDone(ctx, store.AddMigrationThreadDoneParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, ThreadKey: p.ThreadKey}); err != nil {
				return err
			}
		}
		known, err := q.ListMigrationItemsByDedupe(ctx, store.ListMigrationItemsByDedupeParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, Keys: keys})
		if err != nil {
			return err
		}
		for _, k := range known {
			delete(byKey, k.DedupeKey)
		}
		var fresh []migrate.Item
		for _, k := range keys {
			if it, ok := byKey[k]; ok {
				fresh = append(fresh, it)
			}
		}
		return s.planFresh(ctx, q, m, f, label, team, fresh, now)
	})
	if err != nil {
		return err
	}
	if err := s.st.SetMigrationFolderUIDValidity(ctx, store.SetMigrationFolderUIDValidityParams{
		WorkspaceID: m.WorkspaceID, MigrationID: m.ID, ID: f.ID, Uidvalidity: &uv, ScannedUid: int64(st.UIDNext) - 1,
	}); err != nil {
		return err
	}
	s.addMigrationError(ctx, m, "folder_changed", "the folder was recreated in the mailbox and read again", f.Name, nil, "")
	return nil
}

// planFresh plans messages a folder gained: each joins the thread its references name, the rest
// are threaded and planned as the scan plans them.
func (s *Server) planFresh(ctx context.Context, q *store.Queries, m store.Migration, f store.MigrationFolder, label string, team migrationTeam, fresh []migrate.Item, now time.Time) error {
	opts := migrationOptions(m.Options)
	var alone []migrate.Item
	for _, it := range fresh {
		thread := ""
		if len(it.Meta.Refs) > 0 {
			refs, err := q.ListMigrationItemsByDedupe(ctx, store.ListMigrationItemsByDedupeParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, Keys: it.Meta.Refs})
			if err != nil {
				return err
			}
			for _, ref := range refs {
				if _, err := q.LockMigrationThread(ctx, store.LockMigrationThreadParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, ThreadKey: ref.ThreadKey}); err == nil {
					thread = ref.ThreadKey
					break
				} else if !store.IsNotFound(err) {
					return err
				}
			}
		}
		if thread == "" {
			alone = append(alone, it)
			continue
		}
		if err := insertPlanned(ctx, q, m, []migrate.Item{it}, map[string]string{it.SourceID: thread}); err != nil {
			return err
		}
		if err := q.AddMigrationThreadItem(ctx, store.AddMigrationThreadItemParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, ThreadKey: thread, At: it.Meta.At}); err != nil {
			return err
		}
	}
	if len(alone) > 0 {
		plan := migrate.MakePlan(migrate.PlanInput{
			Items: alone, Folders: map[int32]migrate.PlanFolder{f.ID: {Kind: f.Kind, Label: label}}, Team: team.addrs, Since: opts.Since,
			SkipAutomatic: opts.SkipAutomatic, CloseAfterDays: int(opts.CloseAfterDays), Now: now, MaxMessageBytes: s.migrations.MaxMessageBytes,
		})
		threads := map[string]string{}
		var take []migrate.Item
		for _, it := range alone {
			if ip := plan.Items[it.SourceID]; ip.State == migrate.StatePlanned {
				threads[it.SourceID] = ip.ThreadKey
				take = append(take, it)
			}
		}
		if err := insertPlanned(ctx, q, m, take, threads); err != nil {
			return err
		}
		p := store.InsertMigrationThreadsParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID}
		for _, t := range plan.Threads {
			p.ThreadKeys, p.LastAts, p.Items, p.Plans = append(p.ThreadKeys, t.Key), append(p.LastAts, t.LastAt), append(p.Items, int32(len(t.SourceIDs))), append(p.Plans, mustJSON(t))
		}
		if len(p.ThreadKeys) > 0 {
			if err := q.InsertMigrationThreads(ctx, p); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertPlanned(ctx context.Context, q *store.Queries, m store.Migration, items []migrate.Item, threads map[string]string) error {
	if len(items) == 0 {
		return nil
	}
	p := store.InsertMigrationItemsParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, State: migrate.StatePlanned, InCopy: true}
	a := store.ApplyMigrationItemsPlanParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID}
	for _, it := range items {
		p.SourceIds, p.FolderIds, p.DedupeKeys = append(p.SourceIds, it.SourceID), append(p.FolderIds, it.FolderID), append(p.DedupeKeys, it.DedupeKey)
		p.ReceivedAts, p.Sizes, p.Metas = append(p.ReceivedAts, it.Received), append(p.Sizes, it.Size), append(p.Metas, mustJSON(it.Meta))
		a.SourceIds, a.States, a.Reasons, a.ThreadKeys = append(a.SourceIds, it.SourceID), append(a.States, migrate.StatePlanned), append(a.Reasons, ""), append(a.ThreadKeys, threads[it.SourceID])
	}
	if _, err := q.InsertMigrationItems(ctx, p); err != nil {
		return err
	}
	return q.ApplyMigrationItemsPlan(ctx, a)
}

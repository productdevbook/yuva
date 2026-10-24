package api

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strconv"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/migrate"
	"github.com/productdevbook/yuva/api/internal/migrate/imapread"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const syncPage = 50

// MigrationSyncArgs runs catch-up sync; the copy's own job carries on with it too.
type MigrationSyncArgs struct{ MigrationJobArgs }

func (MigrationSyncArgs) Kind() string { return "migration_sync" }

type migrationSyncWorker struct {
	river.WorkerDefaults[MigrationSyncArgs]
	s *Server
}

func (w *migrationSyncWorker) Timeout(*river.Job[MigrationSyncArgs]) time.Duration {
	return migrationJobTimeout
}

func (w *migrationSyncWorker) Work(ctx context.Context, job *river.Job[MigrationSyncArgs]) error {
	return w.s.workMigration(ctx, job.Args.MigrationJobArgs, job.ID)
}

// sync is one pass of catch-up sync: it stops the migration when its time is up or delivery is
// verified and quiet, imports messages planned since, then reads what the old mailbox received
// since the last pass. Mail received before live_from is history and imported quietly; mail
// after it takes the live path, without the greeting.
func (r *migrationRun) sync(ctx context.Context) (MigrationSlice, error) {
	s := r.s
	now := s.now()
	if r.m.LiveFrom != nil && !now.Before(r.m.LiveFrom.AddDate(0, 0, s.migrations.SyncMaxDays)) {
		return MigrationSlice{Done: true}, r.end(ctx, reasonSyncLimit, "sync_limit")
	}
	if q := quietUntil(r.m); q != nil && !now.Before(*q) {
		return MigrationSlice{Done: true}, r.end(ctx, reasonQuiet, "quiet_period")
	}
	res, empty, err := r.copyPlanned(ctx)
	if err != nil || !empty {
		return res, err
	}
	if r.m.ChannelID == nil || r.m.InboxID == nil {
		return MigrationSlice{Done: true}, r.targetGone(ctx)
	}
	var pass syncPass
	if helpdeskMigration(r.m) {
		pass, err = r.syncHelpdesk(ctx)
	} else {
		row, lerr := s.st.GetIngestChannel(ctx, store.GetIngestChannelParams{WorkspaceID: r.m.WorkspaceID, ChannelID: *r.m.ChannelID})
		if store.IsNotFound(lerr) {
			return MigrationSlice{Done: true}, r.targetGone(ctx)
		}
		if lerr != nil {
			return MigrationSlice{}, lerr
		}
		team, terr := s.teamOf(ctx, s.st.Queries, r.m)
		if terr != nil {
			return MigrationSlice{}, terr
		}
		pass, err = r.readNew(ctx, team, store.FindEmailChannelByAddressRow(row))
	}
	if pass.stored || (err == nil && pass.finished) {
		var quiet *time.Time
		if pass.stored {
			quiet = &now
		}
		if err := s.st.SetMigrationSynced(ctx, store.SetMigrationSyncedParams{WorkspaceID: r.m.WorkspaceID, ID: r.m.ID, Now: &now, QuietSince: quiet}); err != nil {
			return MigrationSlice{}, err
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
	if !pass.finished || pass.planned {
		return MigrationSlice{Until: s.now()}, r.flush(ctx, true)
	}
	if err := r.clearWaiting(ctx); err != nil {
		return MigrationSlice{}, err
	}
	return MigrationSlice{Until: now.Add(s.migrations.SyncEvery)}, nil
}

// end completes a syncing migration on its own and deletes its credentials.
func (r *migrationRun) end(ctx context.Context, reason, why string) error {
	return r.s.inTx(ctx, r.m.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
		cur, err := q.LockMigration(ctx, store.LockMigrationParams{WorkspaceID: r.m.WorkspaceID, ID: r.m.ID})
		if err != nil || cur.State != migrationSyncing {
			return err
		}
		m, err := r.s.completeMigration(ctx, q, events, nil, cur, reason, why)
		if err == nil {
			r.m = m
		}
		return err
	})
}

type syncPass struct {
	finished bool
	planned  bool
	stored   bool
}

// readNew reads the messages each imported folder gained since the last pass, a page at a time.
func (r *migrationRun) readNew(ctx context.Context, team migrationTeam, ch store.FindEmailChannelByAddressRow) (syncPass, error) {
	s, m := r.s, r.m
	var out syncPass
	folders, err := s.st.ListMigrationFolders(ctx, store.ListMigrationFoldersParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID})
	if err != nil {
		return out, err
	}
	rd, err := r.source(ctx)
	if err != nil {
		return out, fromSource(err)
	}
	for _, f := range folders {
		if folderUse(f, folders) != oas.FolderUseImport {
			continue
		}
		st, err := rd.Examine(ctx, f.Name)
		var se *imapread.ServerError
		if errors.As(err, &se) && se.Type == "NO" {
			continue
		}
		if err != nil {
			return out, fromSource(err)
		}
		if f.Uidvalidity != nil && int64(st.UIDValidity) != *f.Uidvalidity {
			if err := r.refolder(ctx, f, team); err != nil {
				return out, err
			}
			out.planned = true
			continue
		}
		seen := max(f.ScannedUid, f.SeenUid)
		if st.UIDNext > 0 && int64(st.UIDNext)-1 <= seen {
			continue
		}
		all, err := rd.SearchUIDs(ctx, time.Time{})
		if err != nil {
			return out, fromSource(err)
		}
		var uids []uint32
		for _, u := range all {
			if int64(u) > seen {
				uids = append(uids, u)
			}
		}
		slices.Sort(uids)
		for _, page := range chunks(uids, syncPage) {
			if !r.timeLeft() {
				return out, nil
			}
			if ok, err := r.renew(ctx); err != nil || !ok {
				return out, err
			}
			if err := r.budget(int64(len(page)) * scanHeaderBytes); err != nil {
				return out, err
			}
			stored, planned, err := r.syncPage(ctx, rd, team, ch, f, st.UIDValidity, page)
			if err != nil {
				return out, err
			}
			out.stored, out.planned = out.stored || stored, out.planned || planned
			if err := s.st.SetMigrationFolderSeen(ctx, store.SetMigrationFolderSeenParams{
				WorkspaceID: m.WorkspaceID, MigrationID: m.ID, ID: f.ID, SeenUid: int64(slices.Max(page)),
			}); err != nil {
				return out, err
			}
		}
	}
	out.finished = true
	return out, nil
}

// syncPage handles one page of new messages of the open folder: messages the migration already
// has are left, history is planned for the copy, new mail is stored as live mail.
func (r *migrationRun) syncPage(ctx context.Context, rd *sourceReader, team migrationTeam, ch store.FindEmailChannelByAddressRow, f store.MigrationFolder, uidValidity uint32, page []uint32) (bool, bool, error) {
	s, m := r.s, r.m
	items, read, err := r.headerItems(ctx, rd, f.ID, uidValidity, page)
	if err != nil {
		return false, false, err
	}
	r.throttle.Read(s.now(), read)
	if err := r.saveThrottle(ctx); err != nil {
		return false, false, err
	}
	keys := make([]string, len(items))
	for i, it := range items {
		keys[i] = it.DedupeKey
	}
	known, err := s.st.ListMigrationDedupeKeysIn(ctx, store.ListMigrationDedupeKeysInParams{WorkspaceID: m.WorkspaceID, MigrationID: m.ID, Keys: keys})
	if err != nil {
		return false, false, err
	}
	var history []migrate.Item
	live := map[uint32]migrate.Item{}
	var liveUIDs []uint32
	for _, it := range items {
		if slices.Contains(known, it.DedupeKey) {
			continue
		}
		if m.LiveFrom != nil && it.Received.Before(*m.LiveFrom) {
			history = append(history, it)
			continue
		}
		_, _, uid, _ := migrate.ParseSourceID(it.SourceID)
		live[uid] = it
		liveUIDs = append(liveUIDs, uid)
	}
	if len(history) > 0 {
		label, err := labelName(ctx, s.st.Queries, f)
		if err != nil {
			return false, false, err
		}
		if err := s.st.InTx(ctx, func(q *store.Queries) error {
			return s.planFresh(ctx, q, m, f, label, team, history, s.now())
		}); err != nil {
			return false, false, err
		}
	}
	stored := false
	var fetched []imapread.Message
	err = rd.FetchRaw(ctx, liveUIDs, s.migrations.MaxMessageBytes, func(msg imapread.Message) error {
		fetched = append(fetched, msg)
		return nil
	})
	if err != nil {
		return false, false, fromSource(err)
	}
	for _, msg := range fetched {
		it, ok := live[msg.UID]
		if !ok {
			continue
		}
		r.throttle.Read(s.now(), int64(len(msg.Raw)))
		got, err := r.syncMessage(ctx, team, ch, f, it, msg.Raw)
		if err != nil {
			return false, false, err
		}
		stored = stored || got
	}
	return stored, len(history) > 0, r.saveThrottle(ctx)
}

// syncMessage stores one message the old mailbox received after the migration went live. It
// reports true when it stored mail that had not reached Yuva directly.
func (r *migrationRun) syncMessage(ctx context.Context, team migrationTeam, ch store.FindEmailChannelByAddressRow, f store.MigrationFolder, it migrate.Item, raw []byte) (bool, error) {
	s, m := r.s, r.m
	if raw == nil {
		s.addMigrationError(ctx, m, "too_large", "the message is larger than the server's limit for one message", f.Name, &it.Meta.At, it.Meta.Subject)
		return false, nil
	}
	parsed, err := email.Parse(raw)
	if err != nil {
		s.addMigrationError(ctx, m, migrate.ReasonUnparseable, "the message could not be read", f.Name, &it.Meta.At, it.Meta.Subject)
		return false, nil
	}
	if parsed.MigrationProbe != "" {
		if mid, ok := s.probeMigration(m.WorkspaceID, parsed.MigrationProbe); ok {
			if mid == m.ID {
				return false, s.st.SetMigrationProbeSynced(ctx, store.SetMigrationProbeSyncedParams{WorkspaceID: m.WorkspaceID, ID: m.ID, Now: new(s.now())})
			}
			return false, nil
		}
	}
	from := parsed.From.Email
	if from == "" {
		from = it.Meta.From.Email
	}
	if from == "" {
		return false, nil
	}
	if migrate.Direction(team.addrs, from) == "out" {
		return false, s.syncOutgoing(ctx, m, team, ch, it, parsed, raw)
	}
	res, err := s.ingestToChannel(ctx, ch, ch.Address, "", false, raw, parsed, ingestOptions{noGreeting: true})
	if ie, ok := errorsAsIngest(err); ok {
		s.addMigrationError(ctx, m, string(ie.Code), ie.Reason, f.Name, &it.Meta.At, it.Meta.Subject)
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return res.Status == IngestStored, nil
}

// syncOutgoing stores mail a teammate still sent from the old mailbox as an outgoing message of
// the thread it answers, with a message.created event so the team sees it; it is never sent.
func (s *Server) syncOutgoing(ctx context.Context, m store.Migration, team migrationTeam, ch store.FindEmailChannelByAddressRow, it migrate.Item, parsed *email.Message, raw []byte) error {
	ws, channelID := m.WorkspaceID, ch.ChannelID
	var to *email.Address
	for _, a := range slices.Concat(parsed.To, parsed.Cc) {
		if a.Email != "" && !team.addrs[a.Email] {
			to = &a
			break
		}
	}
	if to == nil {
		return nil
	}
	headerID := parsed.MessageID
	if headerID == "" || len(headerID) > 998 {
		headerID = email.SynthesizedID(raw)
	}
	now := s.now()
	at := migrate.MessageTime(parsed.Date, it.Received, now)
	msgID := migrate.NameID(m.ID, "sync:"+it.SourceID)
	content := s.mailContent(parsed)
	var stored []string
	var rawKey *string
	var rawSize *int64
	if migrationOptions(m.Options).KeepOriginals {
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
	kept := false
	err = s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
		if err := q.LockMigrationChannel(ctx, channelID); err != nil {
			return err
		}
		if _, err := q.FindChannelEmailByHeader(ctx, store.FindChannelEmailByHeaderParams{WorkspaceID: ws, ChannelID: &channelID, HeaderMessageID: headerID}); err == nil {
			return nil
		} else if !store.IsNotFound(err) {
			return err
		}
		contact, err := s.contactForSender(ctx, q, ws, to.Email, to.Name, now)
		if errors.Is(err, errIngestBlocked) {
			return nil
		}
		if err != nil {
			return err
		}
		conv, err := threadOf(ctx, q, ch, parsed, contact.ID)
		if err != nil {
			return err
		}
		if conv != nil && conv.ContactID != contact.ID {
			conv = nil
		}
		if conv == nil {
			c, err := q.CreateConversation(ctx, store.CreateConversationParams{
				ID: newID(), WorkspaceID: ws, InboxID: ch.InboxID, ContactID: contact.ID, ChannelID: &channelID,
				Subject: content.subject, Priority: string(oas.Normal), Now: now,
			})
			if err != nil {
				return err
			}
			body, err := conversationBody(ctx, q, c, nil)
			if err != nil {
				return err
			}
			events.conversation(realtime.ConversationCreated, c, body)
			conv = &c
		}
		dir := string(oas.Out)
		p := store.ImportMessageParams{
			ID: msgID, WorkspaceID: ws, ConversationID: conv.ID, Direction: &dir, AuthorType: string(oas.AuthorTypeMember),
			Body: content.visible, Html: content.html, CreatedAt: at, DeliveryState: new(deliverySent), ImportedAt: &now,
		}
		if member, ok := team.members[parsed.From.Email]; ok {
			p.AuthorMemberID = &member
		} else {
			p.ImportedAuthor = mustJSON(oas.ImportedAuthor{Name: parsed.From.Name, Address: parsed.From.Email})
		}
		if err := q.ImportMessage(ctx, p); err != nil {
			return err
		}
		summary := store.ListMessageEmailsRow{
			MessageID: msgID, Direction: dir, HeaderMessageID: headerID, FromAddress: parsed.From.Email,
			ToAddresses: normalizedAddressList(parsed.To), CcAddresses: normalizedAddressList(parsed.Cc), Subject: content.subject,
			Quoted: content.quoted, HasRaw: rawKey != nil, Auto: parsed.Auto, Dmarc: email.DMARCUnknown,
		}
		if err := q.ImportMessageEmail(ctx, store.ImportMessageEmailParams{
			WorkspaceID: ws, MessageID: msgID, ConversationID: conv.ID, ChannelID: &channelID, Direction: dir, HeaderMessageID: headerID,
			InReplyTo: content.inReplyTo, ReferencesIds: content.refs, FromAddress: parsed.From.Email, ToAddresses: summary.ToAddresses,
			CcAddresses: summary.CcAddresses, Subject: content.subject, FullText: parsed.Text, FullHtml: content.fullHTML,
			Quoted: content.quoted, Headers: content.headers, RawKey: rawKey, RawSize: rawSize, AuthenticationResults: nonEmpty(parsed.AuthResults),
			Auto: parsed.Auto, CreatedAt: at, ImportedAt: &now,
		}); err != nil {
			return err
		}
		var atts []store.Attachment
		var total int64
		if rawSize != nil {
			total = *rawSize
		}
		for _, f := range files {
			a, err := q.CreateAttachment(ctx, store.CreateAttachmentParams{
				ID: f.id, WorkspaceID: ws, ConversationID: conv.ID, MessageID: msgID, StorageKey: f.key, Filename: f.filename,
				ContentType: f.contentType, SizeBytes: f.size, ContentID: f.contentID, Inline: f.inline, CreatedAt: at,
			})
			if err != nil {
				return err
			}
			atts = append(atts, a)
			total += f.size
		}
		if err := q.TouchConversation(ctx, store.TouchConversationParams{WorkspaceID: ws, ID: conv.ID, Now: now, IsMessage: true}); err != nil {
			return err
		}
		if err := q.AddImportedUsage(ctx, store.AddImportedUsageParams{WorkspaceID: ws, Month: monthOf(now), Messages: 1, Bytes: total}); err != nil {
			return err
		}
		msg, err := q.GetMessage(ctx, store.GetMessageParams{WorkspaceID: ws, ID: msgID})
		if err != nil {
			return err
		}
		kept = true
		events.conversation(realtime.MessageCreated, *conv, withEmail(messageBody(messageRow(msg), atts), &summary))
		return nil
	})
	if err != nil || !kept {
		s.deleteObjects(ctx, stored)
	}
	return err
}

package api

import (
	"context"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oapi-codegen/nullable"

	"github.com/productdevbook/yuva/api/internal/migrate"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

var (
	errNotMigrationManager = problem(http.StatusForbidden, "forbidden", "only owners, admins and API keys without an inbox limit move mailboxes into Yuva")
	migrationStates        = []any{"draft", "connected", "scanning", "ready", "copying", "paused", "needs_attention", "syncing", "completed", "cancelled", "removing", "removed"}
	migrationPresets       = func() []any {
		var out []any
		for _, p := range migrate.Presets {
			if p.Source != string(oas.MigrationSourceFile) {
				out = append(out, p.ID)
			}
		}
		return out
	}()
)

const readOnlyPromise = "Yuva only reads the mailbox: it opens folders read-only, never deletes, moves, flags or marks anything there, and the old mailbox stays as it is."

func mayManageMigrations(p principal) bool { return requireMigrationManager(p) == nil }

func migrationTool(ops ...string) mcpTool {
	return mcpTool{ops: ops, allow: mayManageMigrations, denied: errNotMigrationManager}
}

type migrationIn struct {
	MigrationID uuid.UUID `json:"migration_id"`
}

type mcpMigrationCounts struct {
	Total     int64 `json:"total" jsonschema:"messages read from the chosen folders; each ends imported, duplicate, skipped or failed"`
	Imported  int64 `json:"imported"`
	Duplicate int64 `json:"duplicate" jsonschema:"already in Yuva, or a second copy"`
	Skipped   int64 `json:"skipped" jsonschema:"automatic or internal mail, outside the date range, blocked, too large or gone"`
	Failed    int64 `json:"failed"`
	Bytes     int64 `json:"bytes" jsonschema:"bytes read from the mailbox"`
}

type mcpMigrationEstimate struct {
	Conversations    int64            `json:"conversations"`
	Messages         int64            `json:"messages"`
	MessagesIn       int64            `json:"messages_in"`
	MessagesOut      int64            `json:"messages_out"`
	ContactsNew      int64            `json:"contacts_new"`
	ContactsExisting int64            `json:"contacts_existing"`
	Labels           []string         `json:"labels" jsonschema:"labels the conversations get, from folders and Gmail labels"`
	Skipped          map[string]int64 `json:"skipped" jsonschema:"messages not imported, by reason"`
	StorageBytes     int64            `json:"storage_bytes"`
	SecondsMin       int64            `json:"seconds_min" jsonschema:"how long the copy takes at least"`
	SecondsMax       int64            `json:"seconds_max"`
	OldestAt         *time.Time       `json:"oldest_at,omitempty"`
	NewestAt         *time.Time       `json:"newest_at,omitempty"`
}

type mcpMigrationFolder struct {
	Name     string `json:"name"`
	Kind     string `json:"kind" jsonschema:"inbox, sent, archive, all, user, label, spam, trash, drafts or other"`
	Use      string `json:"use" jsonschema:"import, labels (only the labels of mail read from All Mail) or skip"`
	Messages int64  `json:"messages"`
	Imported int64  `json:"imported"`
	Skipped  int64  `json:"skipped"`
	Failed   int64  `json:"failed"`
}

type mcpTeamSuggestion struct {
	Address  string `json:"address"`
	Name     string `json:"name,omitempty"`
	Messages int64  `json:"messages" jsonschema:"messages it sent from the mailbox's Sent folders"`
}

type mcpForwardingConfirmation struct {
	Provider   string    `json:"provider"`
	Code       string    `json:"code" jsonschema:"the code to enter in the provider's forwarding settings"`
	Link       string    `json:"link,omitempty" jsonschema:"the provider's confirmation page, for the person to open"`
	ReceivedAt time.Time `json:"received_at"`
}

type mcpMigration struct {
	ID                     uuid.UUID                  `json:"id"`
	Source                 string                     `json:"source"`
	Provider               string                     `json:"provider" jsonschema:"the preset, such as gmail or yandex"`
	Address                string                     `json:"address" jsonschema:"the mailbox being moved"`
	InboxID                *uuid.UUID                 `json:"inbox_id,omitempty"`
	ChannelID              *uuid.UUID                 `json:"channel_id,omitempty"`
	State                  string                     `json:"state"`
	Reason                 string                     `json:"reason,omitempty" jsonschema:"why it needs attention, or what it waits for"`
	WaitingUntil           *time.Time                 `json:"waiting_until,omitempty"`
	Percent                int64                      `json:"percent" jsonschema:"messages done of those planned, 0 to 100"`
	Counts                 mcpMigrationCounts         `json:"counts"`
	Connected              bool                       `json:"connected" jsonschema:"the mailbox's credentials are stored; they are never shown"`
	Actions                []string                   `json:"actions" jsonschema:"what can be done now"`
	ConnectURL             string                     `json:"connect_url,omitempty" jsonschema:"the page where the person connects the mailbox (app password, sign-in or a help desk's API token); give it to them and never ask for a password or token; its workspace parameter opens it in this workspace even when another one is open in the panel"`
	Estimate               *mcpMigrationEstimate      `json:"estimate,omitempty" jsonschema:"the dry run, once scanned"`
	Folders                []mcpMigrationFolder       `json:"folders,omitempty"`
	TeamSuggestions        []mcpTeamSuggestion        `json:"team_suggestions,omitempty" jsonschema:"once scanned: the other addresses that sent mail from the mailbox's Sent folders, most first; ask the person which are the team's (aliases, colleagues) and pass those as team_addresses"`
	LiveFrom               *time.Time                 `json:"live_from,omitempty" jsonschema:"mail received after this is new mail with notifications"`
	ForwardingAddress      string                     `json:"forwarding_address,omitempty" jsonschema:"forward the old mailbox here"`
	ForwardingConfirmation *mcpForwardingConfirmation `json:"forwarding_confirmation,omitempty"`
	ProbeSentAt            *time.Time                 `json:"probe_sent_at,omitempty"`
	ProbeSyncedAt          *time.Time                 `json:"probe_synced_at,omitempty" jsonschema:"the test message was found only in the old mailbox: not forwarded yet"`
	VerifiedAt             *time.Time                 `json:"verified_at,omitempty" jsonschema:"a test message reached Yuva by the new route"`
	VerifiedRoute          string                     `json:"verified_route,omitempty" jsonschema:"forwarding or direct"`
	QuietUntil             *time.Time                 `json:"quiet_until,omitempty" jsonschema:"when sync finishes the migration on its own"`
	SyncUntil              *time.Time                 `json:"sync_until,omitempty"`
	ImportedConversations  int64                      `json:"imported_conversations,omitempty"`
	RemovableConversations int64                      `json:"removable_conversations,omitempty" jsonschema:"what remove_migration_history would delete; pass it as confirm_count"`
	CredentialsDeletedAt   *time.Time                 `json:"credentials_deleted_at,omitempty"`
	PanelURL               string                     `json:"panel_url,omitempty" jsonschema:"where a member follows the migration in Yuva; its workspace parameter opens it in this workspace even when another one is open in the panel"`
	ChannelCreated         bool                       `json:"channel_created,omitempty" jsonschema:"create_migration created the inbox's e-mail channel for the mailbox address"`
	ChannelSetup           string                     `json:"channel_setup,omitempty" jsonschema:"what the person sets in the panel at channel_settings_url; tell them"`
	ChannelSettingsURL     string                     `json:"channel_settings_url,omitempty" jsonschema:"the e-mail channel's sending settings in the panel"`
}

func (c *mcpCall) migrationOut(m oas.Migration) mcpMigration {
	out := mcpMigration{
		ID: m.Id, Source: string(m.Source), Provider: string(m.Preset), Address: m.SourceAddress, InboxID: m.InboxId, ChannelID: m.ChannelId,
		State: string(m.State), Reason: deref(m.StateReason), WaitingUntil: m.WaitingUntil, Connected: m.Credentials != nil,
		Counts: mcpMigrationCounts{
			Total: m.Counts.Total, Imported: m.Counts.Imported, Duplicate: m.Counts.Duplicate, Skipped: m.Counts.Skipped, Failed: m.Counts.Failed, Bytes: m.Counts.Bytes,
		},
		Actions: []string{}, LiveFrom: m.LiveFrom, ForwardingAddress: deref(m.ForwardingAddress), ProbeSentAt: m.ProbeSentAt,
		ProbeSyncedAt: m.ProbeSyncedAt, VerifiedAt: m.VerifiedAt, QuietUntil: m.QuietUntil, SyncUntil: m.SyncUntil,
		CredentialsDeletedAt: m.CredentialsDeletedAt,
	}
	if m.Counts.Total > 0 {
		out.Percent = done(m.Counts) * 100 / m.Counts.Total
	}
	for _, a := range m.Actions {
		out.Actions = append(out.Actions, string(a))
	}
	if m.VerifiedRoute != nil {
		out.VerifiedRoute = string(*m.VerifiedRoute)
	}
	if m.ImportedConversations != nil {
		out.ImportedConversations, out.RemovableConversations = *m.ImportedConversations, *m.RemovableConversations
	}
	if f := m.ForwardingConfirmation; f != nil {
		out.ForwardingConfirmation = &mcpForwardingConfirmation{Provider: string(f.Provider), Code: f.Code, Link: deref(f.Link), ReceivedAt: f.ReceivedAt}
	}
	if e := m.Estimate; e != nil {
		est := &mcpMigrationEstimate{
			Conversations: e.Conversations, Messages: e.Messages, MessagesIn: e.MessagesIn, MessagesOut: e.MessagesOut,
			ContactsNew: e.ContactsNew, ContactsExisting: e.ContactsExisting, Labels: []string{}, StorageBytes: e.StorageBytes,
			SecondsMin: e.SecondsMin, SecondsMax: e.SecondsMax, OldestAt: e.OldestAt, NewestAt: e.NewestAt,
			Skipped: map[string]int64{},
		}
		for _, l := range e.Labels {
			est.Labels = append(est.Labels, l.Name)
		}
		sk := e.Skipped
		for k, v := range map[string]int64{
			"spam": sk.Spam, "trash": sk.Trash, "drafts": sk.Drafts, "other_folders": sk.OtherFolders, "automatic": sk.Automatic,
			"internal": sk.Internal, "outside_range": sk.OutsideRange, "already_in_yuva": sk.AlreadyInYuva, "blocked": sk.Blocked,
			"too_large": sk.TooLarge, "duplicate_copies": sk.DuplicateCopies,
		} {
			if v > 0 {
				est.Skipped[k] = v
			}
		}
		out.Estimate = est
	}
	if m.TeamSuggestions != nil {
		for _, t := range *m.TeamSuggestions {
			out.TeamSuggestions = append(out.TeamSuggestions, mcpTeamSuggestion{Address: t.Address, Name: deref(t.Name), Messages: t.Messages})
		}
	}
	if !c.s.noPanel {
		out.PanelURL = c.panelURL("/migrations/" + m.Id.String())
		switch m.State {
		case oas.MigrationDraft, oas.MigrationConnected, oas.MigrationNeedsAttention:
			out.ConnectURL = c.panelURL("/migrations/" + m.Id.String() + "/connect")
		}
	}
	return out
}

func (c *mcpCall) getMigration(ctx context.Context, id uuid.UUID) (mcpMigration, error) {
	res, err := c.s.GetMigration(ctx, oas.GetMigrationRequestObject{MigrationId: id})
	if err != nil {
		return mcpMigration{}, err
	}
	out := c.migrationOut(oas.Migration(res.(oas.GetMigration200JSONResponse)))
	fres, err := c.s.ListMigrationFolders(ctx, oas.ListMigrationFoldersRequestObject{MigrationId: id})
	if err != nil {
		return mcpMigration{}, err
	}
	for _, f := range fres.(oas.ListMigrationFolders200JSONResponse).Items {
		out.Folders = append(out.Folders, mcpMigrationFolder{
			Name: f.Name, Kind: string(f.Kind), Use: string(f.Use), Messages: f.Messages, Imported: f.Imported, Skipped: f.Skipped, Failed: f.Failed,
		})
	}
	return out, nil
}

type detectSourceIn struct {
	Address string `json:"address" jsonschema:"the mailbox to move, such as support@example.com"`
}

type detectSourceOut struct {
	Address   string   `json:"address"`
	Provider  string   `json:"provider" jsonschema:"the preset to create the migration with"`
	Name      string   `json:"name" jsonschema:"the provider's name"`
	Available bool     `json:"available" jsonschema:"false when this server cannot connect to the provider yet"`
	Auth      []string `json:"auth" jsonschema:"how the mailbox connects: password (an app password), microsoft (the person signs in with Microsoft at connect_url) oauth (a help desk's sign-in) or token (a help desk's API token, entered at connect_url)"`
	Guide     string   `json:"guide" jsonschema:"the provider's guide"`
	Note      string   `json:"note,omitempty"`
	FoundBy   string   `json:"found_by"`
	ReadOnly  string   `json:"read_only" jsonschema:"the promise to quote to the person"`
}

type listMigrationsIn struct {
	States []string `json:"states,omitempty"`
	Cursor string   `json:"cursor,omitempty"`
	Limit  int32    `json:"limit,omitempty" jsonschema:"1 to 100, default 25"`
}

type migrationOptionsIn struct {
	Folders          []string   `json:"folders,omitempty" jsonschema:"folder names to read, from get_migration; the provider's default when empty"`
	Since            *time.Time `json:"since,omitempty" jsonschema:"only threads whose last message is newer"`
	SkipAutomatic    *bool      `json:"skip_automatic,omitempty" jsonschema:"skip threads of only automatic mail; true by default"`
	CloseAfterDays   *int32     `json:"close_after_days,omitempty" jsonschema:"threads older than this are imported closed; 14 by default"`
	KeepOriginals    *bool      `json:"keep_originals,omitempty" jsonschema:"keep each message's original source; true by default"`
	TeamAddresses    []string   `json:"team_addresses,omitempty" jsonschema:"other addresses whose mail counts as sent by the team, such as aliases"`
	QuietPeriodHours *int32     `json:"quiet_period_hours,omitempty" jsonschema:"how long sync runs after delivery is verified; 24 by default"`
}

func (o *migrationOptionsIn) body() *oas.MigrationOptionsInput {
	if o == nil {
		return nil
	}
	out := &oas.MigrationOptionsInput{
		SkipAutomatic: o.SkipAutomatic, CloseAfterDays: o.CloseAfterDays, KeepOriginals: o.KeepOriginals, QuietPeriodHours: o.QuietPeriodHours,
	}
	if o.Folders != nil {
		out.Folders = nullable.NewNullableWithValue(o.Folders)
	}
	if o.Since != nil {
		out.Since = nullable.NewNullableWithValue(*o.Since)
	}
	if o.TeamAddresses != nil {
		addrs := make([]oas.Email, len(o.TeamAddresses))
		for i, a := range o.TeamAddresses {
			addrs[i] = oas.Email(a)
		}
		out.TeamAddresses = &addrs
	}
	return out
}

type createMigrationIn struct {
	Address   string              `json:"address" jsonschema:"the mailbox to move"`
	Provider  string              `json:"provider,omitempty" jsonschema:"the preset from detect_mail_source, or zendesk or helpscout for a help desk (address is then its support address); detected when empty"`
	InboxID   *uuid.UUID          `json:"inbox_id,omitempty" jsonschema:"the inbox the history goes to"`
	ChannelID *uuid.UUID          `json:"channel_id,omitempty" jsonschema:"the inbox's e-mail channel that receives mail at the address; with inbox_id and no channel_id, that channel is found or created"`
	Options   *migrationOptionsIn `json:"options,omitempty"`
}

// migrationChannel finds the inbox's e-mail channel for the mailbox address, or creates it the way
// the wizard's Map step does: the mailbox address and no SMTP account, which only the panel takes.
func (c *mcpCall) migrationChannel(ctx context.Context, inboxID uuid.UUID, address string) (uuid.UUID, bool, error) {
	q := c.s.st.Queries
	inbox, err := visibleInbox(ctx, q, c.p, inboxID)
	if err != nil {
		return uuid.UUID{}, false, err
	}
	rows, err := q.ListChannels(ctx, store.ListChannelsParams{WorkspaceID: c.p.workspaceID, InboxID: inboxID})
	if err != nil {
		return uuid.UUID{}, false, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	emails, err := emailChannelsByID(ctx, q, c.p.workspaceID, ids)
	if err != nil {
		return uuid.UUID{}, false, err
	}
	for _, id := range ids {
		if e, ok := emails[id]; ok && strings.EqualFold(e.Address, address) {
			return id, false, nil
		}
	}
	if err := keyMayCall(c.p, "CreateChannel"); err != nil {
		return uuid.UUID{}, false, mcpScopeError(err)
	}
	res, err := c.s.CreateChannel(ctx, oas.CreateChannelRequestObject{InboxId: inboxID, Body: &oas.ChannelCreate{
		Kind: oas.ChannelKindEmail, Name: inbox.Name, Email: &oas.EmailChannelInput{Address: oas.Email(address)},
	}})
	if err != nil {
		return uuid.UUID{}, false, err
	}
	return res.(oas.CreateChannel201JSONResponse).Id, true, nil
}

type updateMigrationIn struct {
	MigrationID uuid.UUID           `json:"migration_id"`
	InboxID     *uuid.UUID          `json:"inbox_id,omitempty"`
	ChannelID   *uuid.UUID          `json:"channel_id,omitempty"`
	Options     *migrationOptionsIn `json:"options,omitempty"`
}

type mcpTestStep struct {
	Step   string `json:"step"`
	OK     bool   `json:"ok"`
	Code   string `json:"code,omitempty"`
	Detail string `json:"detail,omitempty" jsonschema:"the server's answer, with any secret removed"`
}

type testConnectionOut struct {
	OK        bool          `json:"ok"`
	Steps     []mcpTestStep `json:"steps"`
	Folders   int32         `json:"folders,omitempty"`
	Messages  int64         `json:"messages,omitempty"`
	Migration mcpMigration  `json:"migration"`
}

type mcpPreviewMessage struct {
	Direction       string               `json:"direction" jsonschema:"in from the contact, out from the team"`
	At              time.Time            `json:"at"`
	MemberID        *uuid.UUID           `json:"member_id,omitempty" jsonschema:"the member the message is imported as"`
	Attachments     int32                `json:"attachments"`
	CustomerContent mcpPreviewMsgContent `json:"customer_content"`
}

type mcpPreviewMsgContent struct {
	FromName    string `json:"from_name,omitempty"`
	FromAddress string `json:"from_address"`
	Text        string `json:"text"`
}

type mcpPreviewThread struct {
	Status          string                  `json:"status" jsonschema:"what the conversation is imported as"`
	Labels          []string                `json:"labels"`
	MessagesTotal   int32                   `json:"messages_total"`
	Messages        []mcpPreviewMessage     `json:"messages"`
	CustomerContent mcpPreviewThreadContent `json:"customer_content"`
}

type mcpPreviewThreadContent struct {
	Subject        string `json:"subject"`
	ContactName    string `json:"contact_name,omitempty"`
	ContactAddress string `json:"contact_address"`
}

type mcpMigrationError struct {
	ID              int64             `json:"id"`
	Code            string            `json:"code"`
	Detail          string            `json:"detail"`
	Folder          string            `json:"folder,omitempty"`
	ItemDate        *time.Time        `json:"item_date,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	CustomerContent mcpErrorItemTitle `json:"customer_content"`
}

type mcpErrorItemTitle struct {
	Subject string `json:"subject,omitempty"`
}

type listMigrationErrorsIn struct {
	MigrationID uuid.UUID `json:"migration_id"`
	Cursor      string    `json:"cursor,omitempty"`
	Limit       int32     `json:"limit,omitempty" jsonschema:"1 to 100, default 25"`
}

type finishMigrationIn struct {
	MigrationID uuid.UUID `json:"migration_id"`
	Force       bool      `json:"force,omitempty" jsonschema:"finish although no test message reached Yuva; only when the person insists"`
}

type removeHistoryIn struct {
	MigrationID  uuid.UUID `json:"migration_id"`
	ConfirmCount int64     `json:"confirm_count" jsonschema:"removable_conversations from get_migration, which the person confirmed"`
}

func (c *mcpCall) step(ctx context.Context, res any, err error) (mcpMigration, error) {
	if err != nil {
		return mcpMigration{}, err
	}
	var m oas.Migration
	switch r := res.(type) {
	case oas.ScanMigration200JSONResponse:
		m = oas.Migration(r)
	case oas.StartMigration200JSONResponse:
		m = oas.Migration(r)
	case oas.PauseMigration200JSONResponse:
		m = oas.Migration(r)
	case oas.ResumeMigration200JSONResponse:
		m = oas.Migration(r)
	case oas.CancelMigration200JSONResponse:
		m = oas.Migration(r)
	case oas.FinishMigration200JSONResponse:
		m = oas.Migration(r)
	case oas.RetryMigrationItems200JSONResponse:
		m = oas.Migration(r)
	case oas.SendMigrationProbe202JSONResponse:
		m = oas.Migration(r)
	case oas.RemoveMigrationHistory202JSONResponse:
		m = oas.Migration(r)
	case oas.UpdateMigration200JSONResponse:
		m = oas.Migration(r)
	case oas.CreateMigration201JSONResponse:
		m = oas.Migration(r)
	}
	return c.migrationOut(m), nil
}

var mcpMigrationTools = []mcpTool{
	defineTool("detect_mail_source", "Detect a mailbox's provider",
		"Tells which provider hosts a mailbox (from the address, the domain's MX and its autoconfig), how it connects on this server and the provider's guide. Owners, admins and keys only. Maps to POST /v1/migrations/detect.",
		readOnly, nil, migrationTool("DetectMigrationSource"),
		func(ctx context.Context, c *mcpCall, in detectSourceIn) (detectSourceOut, error) {
			res, err := c.s.DetectMigrationSource(ctx, oas.DetectMigrationSourceRequestObject{Body: &oas.DetectMigrationSourceJSONRequestBody{Address: oas.Email(in.Address)}})
			if err != nil {
				return detectSourceOut{}, err
			}
			d := res.(oas.DetectMigrationSource200JSONResponse)
			out := detectSourceOut{
				Address: d.Address, Provider: string(d.Preset), Name: d.Name, Available: d.Available, Auth: []string{}, Guide: d.Guide,
				FoundBy: string(d.FoundBy), ReadOnly: readOnlyPromise,
			}
			for _, a := range d.Auth {
				out.Auth = append(out.Auth, string(a))
			}
			if d.Note != nil {
				out.Note = string(*d.Note)
			}
			return out, nil
		}),
	defineTool("list_migrations", "List migrations",
		"Lists the workspace's migrations of mailboxes into Yuva, newest first, with their state and progress. Owners, admins and keys only. Maps to GET /v1/migrations.",
		readOnly, map[string][]any{"states": migrationStates}, migrationTool("ListMigrations"),
		func(ctx context.Context, c *mcpCall, in listMigrationsIn) (mcpPage[mcpMigration], error) {
			params := oas.ListMigrationsParams{Cursor: optString[string](in.Cursor), Limit: optLimit(in.Limit)}
			if len(in.States) > 0 {
				states := make([]oas.MigrationState, len(in.States))
				for i, st := range in.States {
					states[i] = oas.MigrationState(st)
				}
				params.State = &states
			}
			res, err := c.s.ListMigrations(ctx, oas.ListMigrationsRequestObject{Params: params})
			if err != nil {
				return mcpPage[mcpMigration]{}, err
			}
			page := res.(oas.ListMigrations200JSONResponse)
			out := mcpPage[mcpMigration]{Items: []mcpMigration{}}
			for _, m := range page.Items {
				out.Items = append(out.Items, c.migrationOut(m))
			}
			if page.NextCursor != nil {
				out.NextCursor = *page.NextCursor
			}
			return out, nil
		}),
	defineTool("get_migration", "Get a migration",
		"Returns a migration: state and reason, progress, the dry run, folders, what can be done now, the connect_url while the mailbox is not connected, and during the switch the forwarding address, a forwarding confirmation code and whether a test message arrived. Owners, admins and keys only. Maps to GET /v1/migrations/{id} and its folders.",
		readOnly, nil, migrationTool("GetMigration", "ListMigrationFolders"),
		func(ctx context.Context, c *mcpCall, in migrationIn) (mcpMigration, error) {
			return c.getMigration(ctx, in.MigrationID)
		}),
	defineTool("create_migration", "Start moving a mailbox",
		"Creates a migration of a mailbox into an inbox and returns connect_url, the page where the person enters the app password or signs in (Outlook.com and Microsoft 365: provider microsoft, where the person signs in with Microsoft; nothing is needed from you). Never ask for a password: give the person connect_url. Nothing is read until they connect. With inbox_id and no channel_id it uses the inbox's e-mail channel for the mailbox address and creates that channel when the inbox has none (channel_created, which needs inboxes:manage); then tell the person channel_setup. Owners, admins and keys only. Maps to POST /v1/migrations, and to POST /v1/inboxes/{id}/channels with kind email when it creates the channel.",
		additive, map[string][]any{"provider": migrationPresets}, migrationTool("CreateMigration", "DetectMigrationSource"),
		func(ctx context.Context, c *mcpCall, in createMigrationIn) (mcpMigration, error) {
			preset := in.Provider
			if preset == "" {
				res, err := c.s.DetectMigrationSource(ctx, oas.DetectMigrationSourceRequestObject{Body: &oas.DetectMigrationSourceJSONRequestBody{Address: oas.Email(in.Address)}})
				if err != nil {
					return mcpMigration{}, err
				}
				preset = string(res.(oas.DetectMigrationSource200JSONResponse).Preset)
			}
			channelID, created := in.ChannelID, false
			if channelID == nil && in.InboxID != nil {
				if !oas.MigrationPresetId(preset).Valid() {
					return mcpMigration{}, errValidation("preset is not a known preset")
				}
				if err := c.s.checkMigrationPreset(presetSource(preset), preset); err != nil {
					return mcpMigration{}, err
				}
				address, err := normalizeEmail(oas.Email(in.Address))
				if err != nil {
					return mcpMigration{}, errValidation("source_address is not a valid address")
				}
				id, made, err := c.migrationChannel(ctx, *in.InboxID, address)
				if err != nil {
					return mcpMigration{}, err
				}
				channelID, created = &id, made
			}
			res, err := c.s.CreateMigration(ctx, oas.CreateMigrationRequestObject{Body: &oas.MigrationCreate{
				Source: new(oas.MigrationSource(presetSource(preset))), Preset: oas.MigrationPresetId(preset), SourceAddress: oas.Email(in.Address),
				InboxId: in.InboxID, ChannelId: channelID, Options: in.Options.body(),
			}})
			out, err := c.step(ctx, res, err)
			if err != nil || !created {
				return out, err
			}
			out.ChannelCreated = true
			out.ChannelSetup = "Yuva created the inbox's e-mail channel for " + out.Address + " without an SMTP account. Replies from Yuva are sent only once the person sets the mailbox's SMTP server (host, port, username and password) in the panel at channel_settings_url; Yuva never takes that password through an assistant."
			if c.s.migrations.ProbeSMTP == nil || c.s.migrations.ProbeFrom == "" {
				out.ChannelSetup += " This server sends the switch's test message through that SMTP account too, so set it before send_migration_probe."
			}
			if !c.s.noPanel {
				out.ChannelSettingsURL = c.panelURL("/settings/inboxes/" + in.InboxID.String() + "/channels/" + channelID.String())
			}
			return out, nil
		}),
	defineTool("update_migration", "Change a migration",
		"Changes the inbox, the channel and the options (folders, since, automatic mail, closing, team addresses, quiet period) while the migration is a draft, connected or ready; a change to what is read needs a new scan_migration. Owners, admins and keys only. Maps to PATCH /v1/migrations/{id}.",
		idempotent, nil, migrationTool("UpdateMigration"),
		func(ctx context.Context, c *mcpCall, in updateMigrationIn) (mcpMigration, error) {
			body := oas.MigrationUpdate{Options: in.Options.body()}
			if in.InboxID != nil {
				body.InboxId = nullable.NewNullableWithValue(*in.InboxID)
			}
			if in.ChannelID != nil {
				body.ChannelId = nullable.NewNullableWithValue(*in.ChannelID)
			}
			res, err := c.s.UpdateMigration(ctx, oas.UpdateMigrationRequestObject{MigrationId: in.MigrationID, Body: &body})
			return c.step(ctx, res, err)
		}),
	defineTool("test_migration_connection", "Test a mailbox's connection",
		"Connects to the mailbox with the credentials the person entered at connect_url and lists its folders, step by step: name, connection, encryption, sign-in, folders. Reads the mailbox only. At most 10 tests per migration an hour. Owners, admins and keys only. Maps to POST /v1/migrations/{id}/test.",
		readOnly, nil, migrationTool("TestMigrationConnection"),
		func(ctx context.Context, c *mcpCall, in migrationIn) (testConnectionOut, error) {
			res, err := c.s.TestMigrationConnection(ctx, oas.TestMigrationConnectionRequestObject{MigrationId: in.MigrationID})
			if err != nil {
				return testConnectionOut{}, err
			}
			r := res.(oas.TestMigrationConnection200JSONResponse)
			out := testConnectionOut{OK: r.Ok, Steps: []mcpTestStep{}, Migration: c.migrationOut(r.Migration)}
			for _, st := range r.Steps {
				s := mcpTestStep{Step: string(st.Step), OK: st.Ok, Detail: deref(st.Detail)}
				if st.Code != nil {
					s.Code = string(*st.Code)
				}
				out.Steps = append(out.Steps, s)
			}
			if r.Folders != nil {
				out.Folders = *r.Folders
			}
			if r.Messages != nil {
				out.Messages = *r.Messages
			}
			return out, nil
		}),
	defineTool("scan_migration", "Scan a mailbox for a dry run",
		"Reads the message headers of the chosen folders and makes the dry run: what would be imported, skipped and why, contacts, labels, storage and time. Nothing is imported. Poll get_migration until ready. Owners, admins and keys only. Maps to POST /v1/migrations/{id}/scan.",
		idempotent, nil, migrationTool("ScanMigration"),
		func(ctx context.Context, c *mcpCall, in migrationIn) (mcpMigration, error) {
			res, err := c.s.ScanMigration(ctx, oas.ScanMigrationRequestObject{MigrationId: in.MigrationID})
			return c.step(ctx, res, err)
		}),
	defineTool("preview_migration", "Preview imported conversations",
		"Returns the five newest threads as they will be imported: subject, contact, status, labels and messages. Owners, admins and keys only. Maps to GET /v1/migrations/{id}/preview.",
		readOnly, nil, migrationTool("GetMigrationPreview"),
		func(ctx context.Context, c *mcpCall, in migrationIn) (mcpPage[mcpPreviewThread], error) {
			res, err := c.s.GetMigrationPreview(ctx, oas.GetMigrationPreviewRequestObject{MigrationId: in.MigrationID})
			if err != nil {
				return mcpPage[mcpPreviewThread]{}, err
			}
			out := mcpPage[mcpPreviewThread]{Items: []mcpPreviewThread{}}
			for _, t := range res.(oas.GetMigrationPreview200JSONResponse).Items {
				pt := mcpPreviewThread{
					Status: string(t.Status), Labels: t.Labels, MessagesTotal: t.MessagesTotal, Messages: []mcpPreviewMessage{},
					CustomerContent: mcpPreviewThreadContent{Subject: t.Subject, ContactName: t.Contact.Name, ContactAddress: t.Contact.Address},
				}
				for _, m := range t.Messages {
					pt.Messages = append(pt.Messages, mcpPreviewMessage{
						Direction: string(m.Direction), At: m.At, MemberID: m.MemberId, Attachments: m.Attachments,
						CustomerContent: mcpPreviewMsgContent{FromName: m.From.Name, FromAddress: m.From.Address, Text: m.Text},
					})
				}
				out.Items = append(out.Items, pt)
			}
			return out, nil
		}),
	defineTool("start_migration", "Start copying history",
		"Starts copying the history the dry run showed, newest threads first, quietly: no notification, webhook or auto-reply. Call it only after the person confirmed the estimate. Owners, admins and keys only. Maps to POST /v1/migrations/{id}/start.",
		idempotent, nil, migrationTool("StartMigration"),
		func(ctx context.Context, c *mcpCall, in migrationIn) (mcpMigration, error) {
			res, err := c.s.StartMigration(ctx, oas.StartMigrationRequestObject{MigrationId: in.MigrationID})
			return c.step(ctx, res, err)
		}),
	defineTool("pause_migration", "Pause copying",
		"Pauses a copying migration after the batch in progress. Owners, admins and keys only. Maps to POST /v1/migrations/{id}/pause.",
		idempotent, nil, migrationTool("PauseMigration"),
		func(ctx context.Context, c *mcpCall, in migrationIn) (mcpMigration, error) {
			res, err := c.s.PauseMigration(ctx, oas.PauseMigrationRequestObject{MigrationId: in.MigrationID})
			return c.step(ctx, res, err)
		}),
	defineTool("resume_migration", "Resume copying",
		"Resumes a paused migration, or one that needed attention once the person reconnected the mailbox at connect_url. Owners, admins and keys only. Maps to POST /v1/migrations/{id}/resume.",
		idempotent, nil, migrationTool("ResumeMigration"),
		func(ctx context.Context, c *mcpCall, in migrationIn) (mcpMigration, error) {
			res, err := c.s.ResumeMigration(ctx, oas.ResumeMigrationRequestObject{MigrationId: in.MigrationID})
			return c.step(ctx, res, err)
		}),
	defineTool("cancel_migration", "Cancel a migration",
		"Stops a migration for good: what was imported stays, and the mailbox's credentials are deleted. Owners, admins and keys only. Maps to POST /v1/migrations/{id}/cancel.",
		idempotent, nil, migrationTool("CancelMigration"),
		func(ctx context.Context, c *mcpCall, in migrationIn) (mcpMigration, error) {
			res, err := c.s.CancelMigration(ctx, oas.CancelMigrationRequestObject{MigrationId: in.MigrationID})
			return c.step(ctx, res, err)
		}),
	defineTool("finish_migration", "Finish a migration",
		"Stops catch-up sync and completes the migration; its credentials are deleted. Refused until a test message reached Yuva (verified_at) unless force is true. Owners, admins and keys only. Maps to POST /v1/migrations/{id}/finish.",
		idempotent, nil, migrationTool("FinishMigration"),
		func(ctx context.Context, c *mcpCall, in finishMigrationIn) (mcpMigration, error) {
			body := oas.MigrationFinish{}
			if in.Force {
				body.Force = &in.Force
			}
			res, err := c.s.FinishMigration(ctx, oas.FinishMigrationRequestObject{MigrationId: in.MigrationID, Body: &body})
			return c.step(ctx, res, err)
		}),
	defineTool("retry_migration_items", "Retry failed messages",
		"Puts the messages that failed back in line. Owners, admins and keys only. Maps to POST /v1/migrations/{id}/retry.",
		idempotent, nil, migrationTool("RetryMigrationItems"),
		func(ctx context.Context, c *mcpCall, in migrationIn) (mcpMigration, error) {
			res, err := c.s.RetryMigrationItems(ctx, oas.RetryMigrationItemsRequestObject{MigrationId: in.MigrationID})
			return c.step(ctx, res, err)
		}),
	defineTool("list_migration_errors", "List what went wrong",
		"Lists skipped or failed messages and problems with the mailbox, newest first, with a code and a short text. Owners, admins and keys only. Maps to GET /v1/migrations/{id}/errors.",
		readOnly, nil, migrationTool("ListMigrationErrors"),
		func(ctx context.Context, c *mcpCall, in listMigrationErrorsIn) (mcpPage[mcpMigrationError], error) {
			res, err := c.s.ListMigrationErrors(ctx, oas.ListMigrationErrorsRequestObject{MigrationId: in.MigrationID, Params: oas.ListMigrationErrorsParams{
				Cursor: optString[string](in.Cursor), Limit: optLimit(in.Limit),
			}})
			if err != nil {
				return mcpPage[mcpMigrationError]{}, err
			}
			page := res.(oas.ListMigrationErrors200JSONResponse)
			out := mcpPage[mcpMigrationError]{Items: []mcpMigrationError{}}
			for _, e := range page.Items {
				out.Items = append(out.Items, mcpMigrationError{
					ID: e.Id, Code: e.Code, Detail: e.Detail, Folder: e.Folder, ItemDate: e.ItemDate, CreatedAt: e.CreatedAt,
					CustomerContent: mcpErrorItemTitle{Subject: e.Subject},
				})
			}
			if page.NextCursor != nil {
				out.NextCursor = *page.NextCursor
			}
			return out, nil
		}),
	defineTool("send_migration_probe", "Send a test message",
		"While the migration syncs, mails the old mailbox one short test message; when it reaches Yuva by the new forwarding or routing, get_migration shows verified_at. At most one a minute. Owners, admins and keys only. Maps to POST /v1/migrations/{id}/probe.",
		delivering, nil, migrationTool("SendMigrationProbe"),
		func(ctx context.Context, c *mcpCall, in migrationIn) (mcpMigration, error) {
			res, err := c.s.SendMigrationProbe(ctx, oas.SendMigrationProbeRequestObject{MigrationId: in.MigrationID})
			return c.step(ctx, res, err)
		}),
	defineTool("remove_migration_history", "Remove imported history",
		"Deletes the conversations a completed or cancelled migration imported that nobody wrote in since, and the contacts it created that are left without one; conversations someone answered stay. confirm_count must be the removable_conversations get_migration showed and the person confirmed. Cannot be undone. Owners, admins and keys only. Maps to POST /v1/migrations/{id}/remove-history.",
		destructive, nil, migrationTool("RemoveMigrationHistory"),
		func(ctx context.Context, c *mcpCall, in removeHistoryIn) (mcpMigration, error) {
			res, err := c.s.RemoveMigrationHistory(ctx, oas.RemoveMigrationHistoryRequestObject{MigrationId: in.MigrationID, Body: &oas.MigrationRemoveHistory{ConfirmCount: &in.ConfirmCount}})
			return c.step(ctx, res, err)
		}),
}

func addMigrationPrompt(srv *mcp.Server, c *mcpCall) {
	if !toolsListed(c.p, "detect_mail_source", "create_migration", "get_migration", "update_migration", "scan_migration", "preview_migration", "start_migration", "finish_migration") {
		return
	}
	srv.AddPrompt(&mcp.Prompt{
		Name: "move_to_yuva", Title: "Move a mailbox to Yuva",
		Description: "Move a team mailbox's history into Yuva and switch its mail over, without asking for a password: the person connects the mailbox on a Yuva page.",
		Arguments: []*mcp.PromptArgument{
			{Name: "address", Description: "The mailbox to move, such as support@example.com.", Required: true},
			{Name: "inbox", Description: "The inbox it goes to, by name or id; found or created when empty."},
		},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return c.moveToYuvaPrompt(req.Params.Arguments), nil
	})
}

func (c *mcpCall) moveToYuvaPrompt(args map[string]string) *mcp.GetPromptResult {
	address := strings.TrimSpace(args["address"])
	inbox := "the inbox whose name fits the mailbox, or the only inbox if there is one; ask me only when several could fit"
	if v := strings.TrimSpace(args["inbox"]); v != "" {
		inbox = "the inbox " + v
	}
	createInbox := "If none fits, ask me before creating one."
	if toolsListed(c.p, "create_inbox") {
		createInbox = "If none fits, create one with create_inbox after I agree."
	}
	return promptResult("Move "+address+" to Yuva",
		"Move the mailbox "+address+" into Yuva: its history first, then its new mail.",
		"Never ask me for a password, an app password or a token, and never accept one if I paste it: Yuva's connect page takes it. No tool takes a credential.",
		"1. Call detect_mail_source with address "+address+". Tell me which provider it is and how it connects (usually an app password), and quote its read_only promise. If available is false, explain the note and stop.\n"+
			"2. Inbox: call list_inboxes and pick "+inbox+". "+createInbox+"\n"+
			"3. Call create_migration with address "+address+", the provider and the inbox_id, without channel_id: it uses the inbox's e-mail channel for "+address+" and creates that channel when there is none. If channel_setup is in the answer, tell me what it says and give me channel_settings_url. Then give me connect_url and ask me to open it, follow the provider's guide there and connect the mailbox; wait until I say I did.\n"+
			"4. Call get_migration until connected is true and state is connected (call test_migration_connection once if it stays draft, and explain any failed step). Suggest in plain words which folders to read and a date range, apply what I choose with update_migration, then call scan_migration.\n"+
			"5. Call get_migration until state is ready. The first time it is ready with team_suggestions, show those addresses with their counts and ask me which are the team's (aliases or colleagues): mail from them is imported as sent by the team, mail from the others as from customers. If I name any, call update_migration with team_addresses set to them and every team address set before, call scan_migration again and wait for ready. Summarise the estimate: conversations, messages in and out, new and existing contacts, labels, what is skipped and why, storage and time. Call preview_migration and show two or three threads briefly. Ask me to confirm. Only after I confirm, call start_migration.\n"+
			"6. When I ask, report progress from get_migration (percent, counts, waiting_until). If state is needs_attention, explain reason and how to fix it; when the mailbox must be reconnected, give me connect_url again, then call resume_migration.\n"+
			"7. When state is syncing, the history is in and new mail is copied every two minutes. Explain how to switch the mail for this provider: forward the mailbox to forwarding_address (keeping a copy), route it there on the admin side, or move the MX. If forwarding_confirmation appears, give me its code and link for the provider's settings. Then call send_migration_probe and check get_migration: verified_at means mail reaches Yuva; probe_synced_at without verified_at after ten minutes means mail still reaches only the old mailbox.\n"+
			"8. Once verified, the migration finishes on its own at quiet_until; when I want to finish earlier, call finish_migration. Finishing deletes the mailbox's credentials: tell me to remove the app password at the provider too (for Outlook.com and Microsoft 365, to remove the app from the Microsoft account or My Apps), and that the old mailbox is unchanged and can stay as an archive.",
		"Use remove_migration_history only when I ask to undo an import, with confirm_count equal to removable_conversations after I confirmed that number.")
}

// presetSource is the source a preset reads: a help desk's own, else IMAP.
func presetSource(preset string) string {
	if p, ok := migrate.LookupPreset(preset); ok && migrate.Helpdesk(p.Source) {
		return p.Source
	}
	return string(oas.MigrationSourceImap)
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oapi-codegen/nullable"

	"github.com/productdevbook/yuva/api/internal/mail/design"
	"github.com/productdevbook/yuva/api/internal/oas"
)

var (
	errNotBroadcastManager = problem(http.StatusForbidden, "forbidden", "only owners and admins work on designs and broadcasts")
	templateLocales        = []any{"en", "tr", "de"}
	broadcastStates        = []any{"draft", "scheduled", "preparing", "sending", "active", "paused", "sent", "cancelled", "failed"}
	broadcastKinds         = []any{"once", "triggered"}
	triggerEvents          = []any{"contact_identified"}
	triggerBases           = []any{"service", "marketing"}
	delayUnits             = []any{"minutes", "hours", "days"}
	enrollmentStates       = []any{"waiting", "queued", "skipped", "cancelled"}
	conditionOps           = []any{"eq", "neq", "contains", "in", "exists", "not_exists", "gt", "lt"}
)

type mcpExtraContent interface {
	extraContent() []mcp.Content
}

func mayManageBroadcasts(p principal) bool { return requireManagerOrKey(p) == nil }

func maySchedule(p principal) error {
	if !mayManageBroadcasts(p) {
		return errNotBroadcastManager
	}
	if p.scoped() && !p.broadcastsByKeys {
		return errBroadcastsByKeys
	}
	return nil
}

type mcpEmailTemplate struct {
	Slug        string   `json:"slug" jsonschema:"pass as template to create_design"`
	Category    string   `json:"category"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Locales     []string `json:"locales" jsonschema:"the languages the template's text exists in"`
	PreviewURL  string   `json:"preview_url" jsonschema:"a picture of the template"`
}

type listTemplatesIn struct {
	Locale string `json:"locale,omitempty" jsonschema:"the language of names and descriptions; en when empty"`
}

type mcpDesign struct {
	ID           uuid.UUID           `json:"id"`
	Name         string              `json:"name"`
	InboxID      *uuid.UUID          `json:"inbox_id,omitempty" jsonschema:"the inbox the design belongs to; absent for a design every inbox may use"`
	Version      int32               `json:"version" jsonschema:"pass as base_version to update_design"`
	TemplateSlug string              `json:"template_slug,omitempty"`
	Document     map[string]any      `json:"document" jsonschema:"the design: schema 1, a theme and blocks; written by the workspace's members"`
	Warnings     []mcpDesignIssue    `json:"warnings" jsonschema:"what to fix before sending, such as missing alt text"`
	Locales      []string            `json:"locales,omitempty" jsonschema:"the design's language versions, the base language first; absent for a design in one language"`
	Fallback     string              `json:"fallback_locale,omitempty" jsonschema:"the version contacts without a matching language get"`
	MissingTexts map[string][]string `json:"missing_texts,omitempty" jsonschema:"per version, the ids of blocks that still show the base language's text"`
	UpdatedAt    time.Time           `json:"updated_at"`
	PanelURL     string              `json:"panel_url,omitempty" jsonschema:"where a member opens it in Yuva; its workspace parameter opens it in this workspace even when another one is open in the panel"`
}

type mcpDesignIssue struct {
	Code    string `json:"code"`
	BlockID string `json:"block_id,omitempty"`
	Field   string `json:"field,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

func designIssuesOut(is []oas.EmailDesignIssue) []mcpDesignIssue {
	out := []mcpDesignIssue{}
	for _, i := range is {
		out = append(out, mcpDesignIssue{Code: i.Code, BlockID: deref(i.BlockId), Field: deref(i.Field), Detail: deref(i.Detail)})
	}
	return out
}

func documentMap(d oas.EmailDesignDocument) (map[string]any, error) {
	b, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	return out, json.Unmarshal(b, &out)
}

func documentIn(m map[string]any) (oas.EmailDesignDocument, error) {
	var out oas.EmailDesignDocument
	b, err := json.Marshal(m)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, errValidation("document is not a design document: " + err.Error())
	}
	return out, nil
}

func (c *mcpCall) designOut(d oas.EmailDesign) (mcpDesign, error) {
	doc, err := documentMap(d.Document)
	if err != nil {
		return mcpDesign{}, err
	}
	out := mcpDesign{
		ID: d.Id, Name: d.Name, InboxID: d.InboxId, Version: d.Version, TemplateSlug: deref(d.TemplateSlug), Document: doc,
		Warnings: designIssuesOut(d.Warnings), UpdatedAt: d.UpdatedAt,
	}
	if pd, ok := parsedDocument(d.Document); ok && pd.Versioned() {
		out.Locales, out.Fallback = pd.Versions(), pd.Fallback()
		for _, tag := range out.Locales[1:] {
			if m := pd.Missing(tag); len(m) > 0 {
				if out.MissingTexts == nil {
					out.MissingTexts = map[string][]string{}
				}
				out.MissingTexts[tag] = m
			}
		}
	}
	if !c.s.noPanel {
		out.PanelURL = c.panelURL("/broadcasts/designs/" + d.Id.String())
	}
	return out, nil
}

func parsedDocument(d oas.EmailDesignDocument) (design.Document, bool) {
	raw, err := json.Marshal(d)
	if err != nil {
		return design.Document{}, false
	}
	pd, _, err := design.Parse(raw)
	return pd, err == nil
}

type createDesignIn struct {
	Name       string         `json:"name" jsonschema:"shown to members, such as October newsletter"`
	InboxID    *uuid.UUID     `json:"inbox_id,omitempty" jsonschema:"the inbox it belongs to; the design is shared by every inbox when absent"`
	Template   string         `json:"template,omitempty" jsonschema:"a template slug from list_email_templates to start from"`
	Locale     string         `json:"locale,omitempty" jsonschema:"the design's base language: the template's language when starting from a template, else the language of document's texts"`
	AddLocales []string       `json:"add_locales,omitempty" jsonschema:"with template: the template's other languages to add as versions, such as [\"tr\", \"de\"]; one design with versions serves contacts in several languages"`
	Document   map[string]any `json:"document,omitempty" jsonschema:"a whole design document instead of a template; a blank design when neither is given"`
}

type updateDesignIn struct {
	DesignID    uuid.UUID         `json:"design_id"`
	BaseVersion int32             `json:"base_version" jsonschema:"the version create_design or update_design returned last"`
	Document    map[string]any    `json:"document,omitempty" jsonschema:"the whole changed document of the base language: start from the document returned last, keep block ids, the theme and the footer, change texts, links and images, add or remove blocks. Text blocks hold a small HTML subset (p, a, strong, em, ul, ol, li, br); there is no raw HTML block and no remote image. Merge fields need a fallback, such as {{ contact.first_name | \"there\" }}. Its language versions are kept when it has no translations; unchanged when absent"`
	Name        string            `json:"name,omitempty" jsonschema:"a new name; unchanged when empty"`
	Locale      string            `json:"locale,omitempty" jsonschema:"another language to write a version of, such as es or pt-BR: texts, preheader and direction then change that version (added when new) and document is left out. Absent or the base language: document changes the base. A design without a base language takes it as its base language"`
	Texts       map[string]string `json:"texts,omitempty" jsonschema:"with locale: by block id, the block's text in that language (a heading's text, a text or footer block's HTML, a button's label, an image's alt); an empty text removes the entry"`
	Preheader   *string           `json:"preheader,omitempty" jsonschema:"with locale: that version's preheader; empty removes it"`
	Direction   string            `json:"direction,omitempty" jsonschema:"with locale: that version's writing direction"`
	Remove      bool              `json:"remove_locale,omitempty" jsonschema:"with locale: delete that version"`
	Fallback    string            `json:"fallback_locale,omitempty" jsonschema:"the version contacts without a matching language get; the base language by default"`
}

var errNoBaseLocale = errValidation("the design has no base language yet; call update_design with locale alone to set it, then add versions")

func (in updateDesignIn) document(cur oas.EmailDesign) (oas.EmailDesignDocument, error) {
	base := ""
	if cur.Document.Locale != nil {
		base = string(*cur.Document.Locale)
	}
	tag := ""
	if in.Locale != "" {
		t, ok := design.CanonicalTag(in.Locale)
		if !ok {
			return oas.EmailDesignDocument{}, errValidation("locale must be a language tag such as en or es-MX")
		}
		tag = t
	}
	versionEdit := len(in.Texts) > 0 || in.Preheader != nil || in.Direction != "" || in.Remove
	if tag == "" || tag == base || base == "" {
		if versionEdit {
			if base == "" && tag != "" {
				return oas.EmailDesignDocument{}, errNoBaseLocale
			}
			return oas.EmailDesignDocument{}, errValidation("texts, preheader, direction and remove_locale need locale naming a version other than the base language")
		}
		doc := cur.Document
		if in.Document != nil {
			d, err := documentIn(in.Document)
			if err != nil {
				return d, err
			}
			doc = d
		}
		if base == "" && tag != "" {
			doc.Locale = new(oas.LanguageTag(tag))
		}
		if in.Fallback != "" {
			if doc.Translations == nil && cur.Document.Translations != nil {
				doc.Translations = cur.Document.Translations
			}
			doc.FallbackLocale = new(oas.LanguageTag(in.Fallback))
		}
		return doc, nil
	}
	if in.Document != nil {
		return oas.EmailDesignDocument{}, errValidation("document changes the base language; leave it out when locale names another version")
	}
	doc := cur.Document
	tr := map[string]oas.EmailDesignTranslation{}
	if doc.Translations != nil {
		for k, v := range *doc.Translations {
			tr[k] = v
		}
	}
	if in.Remove {
		delete(tr, tag)
		if doc.FallbackLocale != nil && string(*doc.FallbackLocale) == tag {
			doc.FallbackLocale = nil
		}
	} else {
		e := tr[tag]
		blocks := map[string]string{}
		for k, v := range e.Blocks {
			blocks[k] = v
		}
		for k, v := range in.Texts {
			if v == "" {
				delete(blocks, k)
			} else {
				blocks[k] = v
			}
		}
		e.Blocks = blocks
		if in.Preheader != nil {
			e.Preheader = optString[string](*in.Preheader)
		}
		if in.Direction != "" {
			e.Direction = new(oas.EmailDesignTranslationDirection(in.Direction))
		}
		tr[tag] = e
	}
	doc.Translations = &tr
	if len(tr) == 0 {
		doc.FallbackLocale = nil
	}
	if in.Fallback != "" {
		doc.FallbackLocale = new(oas.LanguageTag(in.Fallback))
	}
	return doc, nil
}

type previewDesignIn struct {
	DesignID  uuid.UUID  `json:"design_id"`
	ListID    *uuid.UUID `json:"list_id,omitempty" jsonschema:"render the footer and unsubscribe links of this list"`
	InboxID   *uuid.UUID `json:"inbox_id,omitempty" jsonschema:"render the footer of this inbox; the list's inbox when list_id is given"`
	Subject   string     `json:"subject,omitempty"`
	Preheader string     `json:"preheader,omitempty"`
	Dark      bool       `json:"dark,omitempty" jsonschema:"true renders as a mail app in dark mode shows it"`
	Locale    string     `json:"locale,omitempty" jsonschema:"render the version a contact with this language gets"`
}

type previewDesignOut struct {
	DesignID  uuid.UUID        `json:"design_id"`
	Locale    string           `json:"locale,omitempty" jsonschema:"the version rendered; absent for a design in one language"`
	Text      string           `json:"text" jsonschema:"the plain-text part of the mail, with merge fields filled for a sample contact"`
	SizeBytes int64            `json:"size_bytes" jsonschema:"the HTML's size; Gmail cuts mail off above 102 KB"`
	Warnings  []mcpDesignIssue `json:"warnings"`
	HTMLURI   string           `json:"html_uri" jsonschema:"the HTML comes as an embedded text/html resource with this URI"`
	html      string
}

func (o previewDesignOut) extraContent() []mcp.Content {
	return []mcp.Content{&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: o.HTMLURI, MIMEType: "text/html", Text: o.html}}}
}

type mcpBroadcastCondition struct {
	Attribute string   `json:"attribute" jsonschema:"a contact attribute"`
	Op        string   `json:"op"`
	Value     string   `json:"value,omitempty"`
	Values    []string `json:"values,omitempty" jsonschema:"for op in"`
}

type mcpBroadcastCounts struct {
	Recipients          int64            `json:"recipients"`
	Queued              int64            `json:"queued"`
	Sent                int64            `json:"sent"`
	Failed              int64            `json:"failed"`
	Bounced             int64            `json:"bounced"`
	Complained          int64            `json:"complained"`
	Unsubscribed        int64            `json:"unsubscribed"`
	Replied             int64            `json:"replied"`
	AutoReplied         int64            `json:"auto_replied"`
	Clicked             int64            `json:"clicked" jsonschema:"unique clicks, only with click tracking"`
	SkippedSuppressed   int64            `json:"skipped_suppressed"`
	SkippedOptedOut     int64            `json:"skipped_opted_out"`
	SkippedUnsubscribed int64            `json:"skipped_unsubscribed"`
	SkippedCancelled    int64            `json:"skipped_cancelled"`
	Enrolled            *int64           `json:"enrolled,omitempty" jsonschema:"triggered e-mails: contacts the trigger enrolled, each once"`
	Waiting             *int64           `json:"waiting,omitempty" jsonschema:"triggered e-mails: contacts whose mail is not due yet"`
	SkippedTriggered    map[string]int64 `json:"skipped_triggered,omitempty" jsonschema:"triggered e-mails: contacts skipped at their due time, by reason"`
}

func broadcastCountsOut(c oas.BroadcastCounts) mcpBroadcastCounts {
	out := mcpBroadcastCounts{
		Recipients: c.Recipients, Queued: c.Queued, Sent: c.Sent, Failed: c.Failed, Bounced: c.Bounced, Complained: c.Complained,
		Unsubscribed: c.Unsubscribed, Replied: c.Replied, AutoReplied: c.AutoReplied, Clicked: c.Clicked,
		SkippedSuppressed: c.Skipped.Suppressed, SkippedOptedOut: c.Skipped.OptedOut, SkippedUnsubscribed: c.Skipped.Unsubscribed,
		SkippedCancelled: c.Skipped.Cancelled, Enrolled: c.Enrolled, Waiting: c.Waiting,
	}
	if c.Enrolled != nil {
		k := c.Skipped
		out.SkippedTriggered = map[string]int64{}
		for name, n := range map[string]*int64{
			"no_address": k.NoAddress, "blocked": k.Blocked, "not_subscribed": k.NotSubscribed, "not_matching": k.NotMatching,
			"late": k.Late, "removed": k.Removed, "stopped": k.Stopped,
		} {
			if n != nil {
				out.SkippedTriggered[name] = *n
			}
		}
	}
	return out
}

type mcpBroadcastTranslation struct {
	Locale    string `json:"locale" jsonschema:"a version of the design, such as es or pt-BR"`
	Subject   string `json:"subject,omitempty" jsonschema:"the subject in that language; merge fields need a fallback"`
	Preheader string `json:"preheader,omitempty" jsonschema:"the preheader in that language; wins over the design version's own"`
}

func broadcastTranslationsIn(ts []mcpBroadcastTranslation) *map[string]oas.BroadcastTranslation {
	out := map[string]oas.BroadcastTranslation{}
	for _, t := range ts {
		out[t.Locale] = oas.BroadcastTranslation{Subject: optString[string](t.Subject), Preheader: optString[string](t.Preheader)}
	}
	return &out
}

type mcpBroadcast struct {
	ID               uuid.UUID                 `json:"id"`
	InboxID          uuid.UUID                 `json:"inbox_id"`
	ChannelID        *uuid.UUID                `json:"channel_id,omitempty" jsonschema:"the e-mail channel it goes out through"`
	Name             string                    `json:"name"`
	Subject          string                    `json:"subject"`
	Preheader        string                    `json:"preheader"`
	Translations     []mcpBroadcastTranslation `json:"translations" jsonschema:"subject and preheader per language version of the design; versions without one use subject and preheader"`
	DesignID         *uuid.UUID                `json:"design_id,omitempty"`
	ListIDs          []uuid.UUID               `json:"list_ids"`
	Conditions       []mcpBroadcastCondition   `json:"conditions" jsonschema:"contact attribute conditions every recipient meets"`
	Locales          []string                  `json:"locales" jsonschema:"only contacts with these languages; everyone when empty"`
	ClickTracking    bool                      `json:"click_tracking"`
	Kind             string                    `json:"kind" jsonschema:"once, or triggered: sent to each contact once, a set time after the trigger"`
	Trigger          *oas.BroadcastTrigger     `json:"trigger,omitempty" jsonschema:"a triggered e-mail's event, delay in minutes, consent basis and back-fill days"`
	ServiceConfirmed *time.Time                `json:"service_confirmed_at,omitempty" jsonschema:"when starting a service triggered e-mail was confirmed as being about using the product"`
	State            string                    `json:"state"`
	StateDetail      string                    `json:"state_detail,omitempty" jsonschema:"why it paused or failed, such as the mail server's answer"`
	PauseReason      string                    `json:"pause_reason,omitempty"`
	ScheduledAt      *time.Time                `json:"scheduled_at,omitempty"`
	StartedAt        *time.Time                `json:"started_at,omitempty"`
	FinishedAt       *time.Time                `json:"finished_at,omitempty"`
	LastTestAt       *time.Time                `json:"last_test_at,omitempty"`
	Pace             *oas.BroadcastPace        `json:"pace,omitempty" jsonschema:"its own pace: at most per_hour mails in any 60 minutes, per_day in any 24 hours (with a send window: in each day's window), per_month in any 30 days"`
	PaceNextAt       *time.Time                `json:"pace_next_at,omitempty" jsonschema:"while sending with at least a minute between mails: the earliest time the next mail may go out"`
	Timezone         string                    `json:"timezone" jsonschema:"the IANA time zone of send_window and send_at_local"`
	SendWindow       *oas.BroadcastSendWindow  `json:"send_window,omitempty" jsonschema:"mails go out only between start and end each day, in timezone; end before start crosses midnight"`
	WindowOpens      *time.Time                `json:"window_opens_at,omitempty" jsonschema:"while waiting for the send window: when it opens next"`
	ScheduledAtLocal string                    `json:"scheduled_at_local,omitempty" jsonschema:"scheduled_at as a wall-clock time YYYY-MM-DDTHH:MM in timezone, the form send_at_local takes"`
	ScheduledOffset  string                    `json:"scheduled_at_offset,omitempty" jsonschema:"timezone's offset at scheduled_at, such as GMT+3"`
	Counts           mcpBroadcastCounts        `json:"counts"`
	CreatedAt        time.Time                 `json:"created_at"`
	UpdatedAt        time.Time                 `json:"updated_at"`
	PanelURL         string                    `json:"panel_url,omitempty" jsonschema:"where a member reviews it in Yuva; its workspace parameter opens it in this workspace even when another one is open in the panel"`
}

func (c *mcpCall) broadcastOut(b oas.Broadcast) mcpBroadcast {
	out := mcpBroadcast{
		ID: b.Id, InboxID: b.InboxId, ChannelID: b.ChannelId, Name: b.Name, Subject: b.Subject, Preheader: b.Preheader,
		DesignID: b.DesignId, ListIDs: b.Audience.ListIds, Conditions: []mcpBroadcastCondition{}, Locales: []string{},
		ClickTracking: b.ClickTracking, State: string(b.State), StateDetail: deref(b.StateDetail), ScheduledAt: b.ScheduledAt,
		StartedAt: b.StartedAt, FinishedAt: b.FinishedAt, LastTestAt: b.LastTestAt, Pace: b.Pace, PaceNextAt: b.PaceNextAt,
		Counts: broadcastCountsOut(b.Counts), CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
		Timezone: b.Timezone, SendWindow: b.SendWindow, WindowOpens: b.WindowOpensAt, Kind: string(b.Kind), Trigger: b.Trigger,
	}
	if b.ServiceConfirmed != nil {
		out.ServiceConfirmed = &b.ServiceConfirmed.At
	}
	if b.ScheduledAt != nil {
		if loc, err := zone(b.Timezone); err == nil {
			at := b.ScheduledAt.In(loc)
			out.ScheduledAtLocal, out.ScheduledOffset = at.Format("2006-01-02T15:04"), gmtOffset(at)
		}
	}
	if out.ListIDs == nil {
		out.ListIDs = []uuid.UUID{}
	}
	out.Translations = []mcpBroadcastTranslation{}
	for _, tag := range slices.Sorted(maps.Keys(b.Translations)) {
		t := b.Translations[tag]
		out.Translations = append(out.Translations, mcpBroadcastTranslation{Locale: tag, Subject: deref(t.Subject), Preheader: deref(t.Preheader)})
	}
	for _, cd := range b.Audience.Conditions {
		mc := mcpBroadcastCondition{Attribute: cd.Attribute, Op: string(cd.Op), Value: deref(cd.Value)}
		if cd.Values != nil {
			mc.Values = *cd.Values
		}
		out.Conditions = append(out.Conditions, mc)
	}
	for _, l := range b.Audience.Locales {
		out.Locales = append(out.Locales, string(l))
	}
	if b.PauseReason != nil {
		out.PauseReason = string(*b.PauseReason)
	}
	if !c.s.noPanel {
		out.PanelURL = c.panelURL("/broadcasts/" + b.Id.String())
	}
	return out
}

type listBroadcastsIn struct {
	InboxID *uuid.UUID `json:"inbox_id,omitempty"`
	Kind    string     `json:"kind,omitempty" jsonschema:"only one-off broadcasts (once) or only triggered e-mails (triggered)"`
	States  []string   `json:"states,omitempty" jsonschema:"only broadcasts in these states"`
	Cursor  string     `json:"cursor,omitempty"`
	Limit   int32      `json:"limit,omitempty" jsonschema:"1 to 100, default 25"`
}

type broadcastIn struct {
	BroadcastID uuid.UUID `json:"broadcast_id"`
}

type mcpReviewItem struct {
	Key    string `json:"key"`
	Status string `json:"status" jsonschema:"ok, warning or blocking; a blocking item stops scheduling"`
	Detail string `json:"detail,omitempty"`
}

type mcpReview struct {
	Ready      bool            `json:"ready" jsonschema:"true when nothing blocks scheduling"`
	Recipients int64           `json:"recipients" jsonschema:"who would get it now (for a triggered e-mail: the contacts the back-fill would enroll, 0 without back-fill); schedule_broadcast needs this number as confirm_recipients"`
	HTMLBytes  int32           `json:"html_bytes,omitempty"`
	Days       int32           `json:"days,omitempty" jsonschema:"how many days sending takes under the workspace's daily limit"`
	Duration   int64           `json:"duration_seconds,omitempty" jsonschema:"about how many seconds sending takes from its start, at the slowest of the pace, the channel's rate and the daily limit"`
	Items      []mcpReviewItem `json:"items"`
}

type mcpListCount struct {
	ListID     uuid.UUID `json:"list_id"`
	Recipients int64     `json:"recipients"`
}

type mcpLocaleCount struct {
	Locale     string `json:"locale"`
	Recipients int64  `json:"recipients"`
	Fallback   int64  `json:"fallback" jsonschema:"of them, contacts whose language matched no version (or who have none) and get this one as the fallback"`
}

type mcpAudience struct {
	ByList             []mcpListCount   `json:"by_list"`
	ByLocale           []mcpLocaleCount `json:"by_locale,omitempty" jsonschema:"with a design with language versions: recipients per version, largest first"`
	Excluded           map[string]int64 `json:"excluded" jsonschema:"people on the lists left out, by reason"`
	MissingMergeFields map[string]int64 `json:"missing_merge_fields" jsonschema:"merge fields without fallback that would be empty, with how many recipients"`
}

type getBroadcastOut struct {
	Broadcast mcpBroadcast `json:"broadcast"`
	Review    *mcpReview   `json:"review,omitempty" jsonschema:"the checks scheduling runs, for a draft"`
	Audience  *mcpAudience `json:"audience,omitempty" jsonschema:"who the draft would reach"`
}

func (c *mcpCall) getBroadcast(ctx context.Context, id uuid.UUID) (getBroadcastOut, error) {
	res, err := c.s.GetBroadcast(ctx, oas.GetBroadcastRequestObject{BroadcastId: id})
	if err != nil {
		return getBroadcastOut{}, err
	}
	b := oas.Broadcast(res.(oas.GetBroadcast200JSONResponse))
	out := getBroadcastOut{Broadcast: c.broadcastOut(b)}
	if b.State != oas.BroadcastStateDraft {
		return out, nil
	}
	rres, err := c.s.ReviewBroadcast(ctx, oas.ReviewBroadcastRequestObject{BroadcastId: id})
	if err != nil {
		return out, err
	}
	r := oas.BroadcastReview(rres.(oas.ReviewBroadcast200JSONResponse))
	rv := &mcpReview{Ready: r.Ready, Recipients: r.Recipients, Items: []mcpReviewItem{}}
	if r.HtmlBytes != nil {
		rv.HTMLBytes = *r.HtmlBytes
	}
	if r.Days != nil {
		rv.Days = *r.Days
	}
	if r.DurationSeconds != nil {
		rv.Duration = *r.DurationSeconds
	}
	for _, it := range r.Items {
		rv.Items = append(rv.Items, mcpReviewItem{Key: string(it.Key), Status: string(it.Status), Detail: deref(it.Detail)})
	}
	out.Review = rv
	if b.Kind == oas.KindTriggered {
		return out, nil
	}
	ares, err := c.s.CountBroadcastAudience(ctx, oas.CountBroadcastAudienceRequestObject{BroadcastId: id})
	if err != nil {
		return out, err
	}
	a := oas.BroadcastAudienceCount(ares.(oas.CountBroadcastAudience200JSONResponse))
	ex := a.Excluded
	au := &mcpAudience{ByList: []mcpListCount{}, MissingMergeFields: map[string]int64{}, Excluded: map[string]int64{
		"blocked": ex.Blocked, "duplicate": ex.Duplicate, "filtered": ex.Filtered, "opted_out": ex.OptedOut, "pending": ex.Pending,
		"suppressed": ex.Suppressed, "unsubscribed": ex.Unsubscribed,
	}}
	for _, l := range a.ByList {
		au.ByList = append(au.ByList, mcpListCount{ListID: l.ListId, Recipients: l.Recipients})
	}
	for k, v := range a.MissingMergeFields {
		au.MissingMergeFields[k] = v
	}
	if a.ByLocale != nil {
		for _, l := range *a.ByLocale {
			au.ByLocale = append(au.ByLocale, mcpLocaleCount{Locale: string(l.Locale), Recipients: l.Recipients, Fallback: l.Fallback})
		}
	}
	out.Audience = au
	return out, nil
}

type createBroadcastIn struct {
	InboxID       uuid.UUID                 `json:"inbox_id"`
	ChannelID     *uuid.UUID                `json:"channel_id,omitempty" jsonschema:"the inbox's e-mail channel to send through; list_mailing_lists shows each list's"`
	Name          string                    `json:"name" jsonschema:"shown to members"`
	Subject       string                    `json:"subject" jsonschema:"the mail's subject; merge fields need a fallback"`
	Preheader     string                    `json:"preheader,omitempty" jsonschema:"the line inboxes show after the subject"`
	DesignID      *uuid.UUID                `json:"design_id,omitempty"`
	ListIDs       []uuid.UUID               `json:"list_ids" jsonschema:"1 to 10 lists of the inbox"`
	Locales       []string                  `json:"locales,omitempty" jsonschema:"only contacts with these languages, BCP 47 tags"`
	Conditions    []mcpBroadcastCondition   `json:"conditions,omitempty" jsonschema:"contact attribute conditions every recipient must meet"`
	ClickTracking bool                      `json:"click_tracking,omitempty" jsonschema:"count unique clicks on links; links then point at this server"`
	PacePerHour   *int32                    `json:"pace_per_hour,omitempty" jsonschema:"at most this many mails in any 60 minutes, 1 to 10,000,000; optional"`
	PacePerDay    *int32                    `json:"pace_per_day,omitempty" jsonschema:"at most this many mails in any 24 hours, 1 to 10,000,000; optional"`
	PacePerMonth  *int32                    `json:"pace_per_month,omitempty" jsonschema:"at most this many mails in any 30 days, 1 to 10,000,000; optional"`
	Timezone      string                    `json:"timezone,omitempty" jsonschema:"an IANA time zone such as Europe/Istanbul; the inbox's when absent"`
	WindowStart   string                    `json:"send_window_start,omitempty" jsonschema:"HH:MM, 24-hour, in the broadcast's time zone; mails go out only between send_window_start and send_window_end each day; end before start crosses midnight"`
	WindowEnd     string                    `json:"send_window_end,omitempty" jsonschema:"HH:MM, 24-hour (24:00 for the end of the day), in the broadcast's time zone; at least 30 minutes after send_window_start"`
	Translations  []mcpBroadcastTranslation `json:"translations,omitempty" jsonschema:"subject and preheader for the design's other language versions"`
}

var errWindowHalf = problem(http.StatusBadRequest, "invalid_request", "give send_window_start and send_window_end together")

func windowIn(start, end string) (*oas.BroadcastSendWindow, *apiError) {
	if start == "" && end == "" {
		return nil, nil
	}
	if start == "" || end == "" {
		return nil, errWindowHalf
	}
	return &oas.BroadcastSendWindow{Start: start, End: end}, nil
}

func audienceIn(listIDs []uuid.UUID, locales []string, conditions []mcpBroadcastCondition) oas.BroadcastAudienceInput {
	aud := oas.BroadcastAudienceInput{ListIds: listIDs}
	if aud.ListIds == nil {
		aud.ListIds = []uuid.UUID{}
	}
	if len(locales) > 0 {
		tags := make([]oas.LanguageTag, len(locales))
		for i, l := range locales {
			tags[i] = oas.LanguageTag(l)
		}
		aud.Locales = &tags
	}
	if len(conditions) > 0 {
		conds := make([]oas.BroadcastCondition, len(conditions))
		for i, cd := range conditions {
			conds[i] = oas.BroadcastCondition{Attribute: cd.Attribute, Op: oas.BroadcastConditionOp(cd.Op), Value: optString[string](cd.Value)}
			if cd.Values != nil {
				conds[i].Values = &cd.Values
			}
		}
		aud.Conditions = &conds
	}
	return aud
}

type updateBroadcastIn struct {
	BroadcastID   uuid.UUID                 `json:"broadcast_id"`
	Name          *string                   `json:"name,omitempty" jsonschema:"shown to members"`
	Subject       *string                   `json:"subject,omitempty" jsonschema:"the mail's subject; merge fields need a fallback"`
	Preheader     *string                   `json:"preheader,omitempty" jsonschema:"the line inboxes show after the subject; empty removes it"`
	ChannelID     *uuid.UUID                `json:"channel_id,omitempty" jsonschema:"the inbox's e-mail channel to send through, which sets the sender"`
	DesignID      *uuid.UUID                `json:"design_id,omitempty"`
	ListIDs       []uuid.UUID               `json:"list_ids,omitempty" jsonschema:"1 to 10 lists of the inbox; replaces the whole audience together with locales and conditions"`
	Locales       []string                  `json:"locales,omitempty" jsonschema:"with list_ids: only contacts with these languages, BCP 47 tags"`
	Conditions    []mcpBroadcastCondition   `json:"conditions,omitempty" jsonschema:"with list_ids: contact attribute conditions every recipient must meet"`
	ClickTracking *bool                     `json:"click_tracking,omitempty" jsonschema:"count unique clicks on links; links then point at this server"`
	UTM           *bool                     `json:"utm,omitempty" jsonschema:"add utm_source, utm_medium and utm_campaign to links"`
	UTMCampaign   string                    `json:"utm_campaign,omitempty" jsonschema:"lowercase letters, digits, -, _ and ."`
	PacePerHour   *int32                    `json:"pace_per_hour,omitempty" jsonschema:"at most this many mails in any 60 minutes, 1 to 10,000,000"`
	PacePerDay    *int32                    `json:"pace_per_day,omitempty" jsonschema:"at most this many mails in any 24 hours, 1 to 10,000,000"`
	PacePerMonth  *int32                    `json:"pace_per_month,omitempty" jsonschema:"at most this many mails in any 30 days, 1 to 10,000,000"`
	RemovePace    bool                      `json:"remove_pace,omitempty" jsonschema:"send as fast as the channel and the daily limit allow"`
	Timezone      string                    `json:"timezone,omitempty" jsonschema:"an IANA time zone such as Europe/Istanbul; never moves scheduled_at"`
	WindowStart   string                    `json:"send_window_start,omitempty" jsonschema:"HH:MM, 24-hour, in the broadcast's time zone; mails go out only between send_window_start and send_window_end each day; end before start crosses midnight"`
	WindowEnd     string                    `json:"send_window_end,omitempty" jsonschema:"HH:MM, 24-hour (24:00 for the end of the day), in the broadcast's time zone; at least 30 minutes after send_window_start"`
	RemoveWindow  bool                      `json:"remove_send_window,omitempty" jsonschema:"send at any hour of the day again"`
	Translations  []mcpBroadcastTranslation `json:"translations,omitempty" jsonschema:"subject and preheader for the design's other language versions; replaces all of them, an empty list removes them"`
	Delay         *int32                    `json:"delay,omitempty" jsonschema:"triggered e-mails: the new delay in delay_unit; moves the due time of everyone waiting"`
	DelayUnit     string                    `json:"delay_unit,omitempty" jsonschema:"minutes, hours or days, with delay"`
	BackfillDays  *int32                    `json:"backfill_days,omitempty" jsonschema:"triggered e-mails before they start: back-fill days, 0 to 30"`
}

var errNothingToUpdate = problem(http.StatusBadRequest, "invalid_request", "name at least one field to change")

var errAudienceWithoutLists = problem(http.StatusBadRequest, "invalid_request", "locales and conditions replace the audience together with list_ids; pass list_ids too")

func (in updateBroadcastIn) body() (oas.BroadcastUpdate, *apiError) {
	body := oas.BroadcastUpdate{
		Name: in.Name, Subject: in.Subject, Preheader: in.Preheader, ClickTracking: in.ClickTracking, Utm: in.UTM,
		UtmCampaign: optString[string](in.UTMCampaign),
	}
	if in.ChannelID != nil {
		body.ChannelId = nullable.NewNullableWithValue(*in.ChannelID)
	}
	if in.DesignID != nil {
		body.DesignId = nullable.NewNullableWithValue(*in.DesignID)
	}
	if in.Translations != nil {
		body.Translations = broadcastTranslationsIn(in.Translations)
	}
	if in.ListIDs != nil {
		aud := audienceIn(in.ListIDs, in.Locales, in.Conditions)
		body.Audience = &aud
	} else if len(in.Locales) > 0 || len(in.Conditions) > 0 {
		return body, errAudienceWithoutLists
	}
	if in.PacePerHour != nil || in.PacePerDay != nil || in.PacePerMonth != nil {
		body.Pace = &oas.BroadcastPace{PerHour: in.PacePerHour, PerDay: in.PacePerDay, PerMonth: in.PacePerMonth}
	} else if in.RemovePace {
		body.Pace = &oas.BroadcastPace{}
	}
	if in.Timezone != "" {
		body.Timezone = &in.Timezone
	}
	w, perr := windowIn(in.WindowStart, in.WindowEnd)
	switch {
	case perr != nil:
		return body, perr
	case w != nil:
		body.SendWindow = nullable.NewNullableWithValue(*w)
	case in.RemoveWindow:
		body.SendWindow = nullable.NewNullNullable[oas.BroadcastSendWindow]()
	}
	if in.Delay != nil || in.BackfillDays != nil {
		body.Trigger = &oas.BroadcastTriggerInput{BackfillDays: in.BackfillDays}
		if in.Delay != nil {
			m, err := delayMinutes(*in.Delay, in.DelayUnit)
			if err != nil {
				return body, err.(*apiError)
			}
			body.Trigger.DelayMinutes = &m
		}
	}
	if reflect.DeepEqual(body, oas.BroadcastUpdate{}) {
		return body, errNothingToUpdate
	}
	return body, nil
}

type sendTestIn struct {
	BroadcastID uuid.UUID   `json:"broadcast_id"`
	MemberIDs   []uuid.UUID `json:"member_ids,omitempty" jsonschema:"with an API key only: 1 to 5 members to send it to; a signed-in assistant sends it to you"`
	Locale      string      `json:"locale,omitempty" jsonschema:"send the version a contact with this language gets"`
}

type sendTestOut struct {
	SentTo []string `json:"sent_to" jsonschema:"the members' addresses the test goes to"`
	Locale string   `json:"locale,omitempty" jsonschema:"the version sent; absent for a design in one language"`
}

type scheduleBroadcastIn struct {
	BroadcastID       uuid.UUID  `json:"broadcast_id"`
	SendAt            *time.Time `json:"send_at,omitempty" jsonschema:"when sending starts, an instant with its offset, at most 90 days ahead; a time in the past sends now. Give send_at or send_at_local, or neither to send now"`
	SendAtLocal       string     `json:"send_at_local,omitempty" jsonschema:"a wall-clock time YYYY-MM-DDTHH:MM in the broadcast's time zone, as get_broadcast shows it; use this when the person says a time such as 9:00 Istanbul time"`
	ConfirmRecipients int64      `json:"confirm_recipients" jsonschema:"the recipient count get_broadcast showed and the person you work for confirmed; for a triggered e-mail the back-fill count (0 without back-fill)"`
	BackfillDays      *int32     `json:"backfill_days,omitempty" jsonschema:"triggered e-mails: also enroll contacts whose trigger happened in this many days before the start, 0 to 30; get_broadcast's review counts them"`
	ConfirmService    bool       `json:"confirm_service,omitempty" jsonschema:"triggered e-mails with basis service: must be true, after the person you work for confirmed the mail is about using the product (getting started, the account, a feedback request) and not advertising"`
}

type createTriggeredIn struct {
	InboxID       uuid.UUID                 `json:"inbox_id"`
	ChannelID     *uuid.UUID                `json:"channel_id,omitempty" jsonschema:"the inbox's e-mail channel to send through"`
	Name          string                    `json:"name" jsonschema:"shown to members"`
	Subject       string                    `json:"subject" jsonschema:"the mail's subject; merge fields need a fallback"`
	Preheader     string                    `json:"preheader,omitempty" jsonschema:"the line inboxes show after the subject"`
	DesignID      *uuid.UUID                `json:"design_id,omitempty"`
	Translations  []mcpBroadcastTranslation `json:"translations,omitempty" jsonschema:"subject and preheader for the design's other language versions"`
	Event         string                    `json:"event" jsonschema:"contact_identified: the contact first signs in to the product (gets its first external id in the inbox)"`
	Delay         int32                     `json:"delay" jsonschema:"how long after the trigger the mail is due, in delay_unit; at most 365 days"`
	DelayUnit     string                    `json:"delay_unit" jsonschema:"minutes, hours or days"`
	Basis         string                    `json:"basis" jsonschema:"service: mail about using the product, to everyone who matches, no lists; marketing: only to subscribers of list_ids. Advertising and offers are always marketing"`
	ListIDs       []uuid.UUID               `json:"list_ids,omitempty" jsonschema:"marketing only: 1 to 10 lists of the inbox"`
	Locales       []string                  `json:"locales,omitempty" jsonschema:"only contacts with these languages, BCP 47 tags"`
	Conditions    []mcpBroadcastCondition   `json:"conditions,omitempty" jsonschema:"contact attribute conditions a contact must meet at its due time"`
	ClickTracking bool                      `json:"click_tracking,omitempty" jsonschema:"count unique clicks on links; links then point at this server"`
	Timezone      string                    `json:"timezone,omitempty" jsonschema:"an IANA time zone for the sending hours; the inbox's when absent"`
	WindowStart   string                    `json:"send_window_start,omitempty" jsonschema:"HH:MM, 24-hour, in the time zone; mails go out only between send_window_start and send_window_end"`
	WindowEnd     string                    `json:"send_window_end,omitempty" jsonschema:"HH:MM, 24-hour (24:00 for the end of the day), at least 30 minutes after send_window_start"`
}

func delayMinutes(n int32, unit string) (int32, error) {
	per := map[string]int64{"minutes": 1, "hours": 60, "days": 1440}[unit]
	if per == 0 {
		return 0, errValidation("delay_unit must be minutes, hours or days")
	}
	m := int64(n) * per
	if n < 0 || m > maxDelayMinutes {
		return 0, errValidation("the delay must be 0 minutes to 365 days")
	}
	return int32(m), nil
}

type listWaitingIn struct {
	BroadcastID uuid.UUID `json:"broadcast_id"`
	State       string    `json:"state,omitempty" jsonschema:"waiting (default: the contacts whose mail is not due yet, due first), queued, skipped or cancelled"`
	Cursor      string    `json:"cursor,omitempty"`
	Limit       int32     `json:"limit,omitempty" jsonschema:"1 to 100, default 25"`
}

type mcpEnrollmentContent struct {
	ContactName  string `json:"contact_name"`
	ContactEmail string `json:"contact_email,omitempty"`
}

type mcpEnrollment struct {
	ContactID       uuid.UUID            `json:"contact_id"`
	State           string               `json:"state"`
	TriggeredAt     time.Time            `json:"triggered_at"`
	SendsAt         *time.Time           `json:"sends_at,omitempty" jsonschema:"while waiting: when the mail is due, moved into the sending hours"`
	SkipReason      string               `json:"skip_reason,omitempty"`
	Source          string               `json:"source" jsonschema:"trigger, or backfill when the start enrolled it"`
	DeliveryID      *uuid.UUID           `json:"delivery_id,omitempty"`
	CustomerContent mcpEnrollmentContent `json:"customer_content"`
}

type removeWaitingIn struct {
	BroadcastID uuid.UUID `json:"broadcast_id"`
	ContactID   uuid.UUID `json:"contact_id"`
}

type removeWaitingOut struct {
	Removed bool `json:"removed"`
}

type mcpReportPoint struct {
	At           time.Time `json:"at"`
	Sent         int64     `json:"sent"`
	Bounced      int64     `json:"bounced"`
	Complained   int64     `json:"complained"`
	Unsubscribed int64     `json:"unsubscribed"`
	Replied      int64     `json:"replied"`
	Clicked      int64     `json:"clicked"`
}

type mcpReportLink struct {
	URL    string `json:"url"`
	Clicks int64  `json:"clicks"`
}

type mcpReplyContent struct {
	ContactName string `json:"contact_name"`
}

type mcpReportReply struct {
	ConversationID  uuid.UUID       `json:"conversation_id" jsonschema:"read the reply with get_conversation"`
	ContactID       uuid.UUID       `json:"contact_id"`
	At              time.Time       `json:"at"`
	CustomerContent mcpReplyContent `json:"customer_content"`
}

type broadcastReportIn struct {
	BroadcastID uuid.UUID `json:"broadcast_id"`
	Locale      string    `json:"locale,omitempty" jsonschema:"only the recipients of this language version"`
}

type mcpLocaleReport struct {
	Locale string             `json:"locale"`
	Counts mcpBroadcastCounts `json:"counts"`
	Rates  map[string]float64 `json:"rates"`
}

func ratesOut(r oas.BroadcastRates) map[string]float64 {
	return map[string]float64{
		"bounced": r.Bounced, "complained": r.Complained, "unsubscribed": r.Unsubscribed, "replied": r.Replied, "clicked": r.Clicked,
	}
}

type mcpReport struct {
	BroadcastID uuid.UUID          `json:"broadcast_id"`
	Locale      string             `json:"locale,omitempty" jsonschema:"the version this report is narrowed to"`
	Locales     []mcpLocaleReport  `json:"locales,omitempty" jsonschema:"for a design with language versions: counts and rates per version"`
	Counts      mcpBroadcastCounts `json:"counts"`
	Rates       map[string]float64 `json:"rates" jsonschema:"shares of sent mail, 0 to 1"`
	Timeline    []mcpReportPoint   `json:"timeline" jsonschema:"hourly for the first 72 hours, then daily"`
	Links       []mcpReportLink    `json:"links,omitempty" jsonschema:"clicks per link, with click tracking"`
	Replies     []mcpReportReply   `json:"replies" jsonschema:"conversations opened by replies"`
}

func scheduleProblem(err error) error {
	var e *apiError
	if !errors.As(err, &e) {
		return err
	}
	switch e.Code {
	case "audience_changed":
		if n, ok := e.Extra["recipients"].(int64); ok {
			return problem(e.Status, e.Code, "the audience is now "+strconv.FormatInt(n, 10)+" recipients, not the number given; show the new number to the person you work for and schedule only after they confirm it")
		}
	case "review_failed":
		if items, ok := e.Extra["items"].([]oas.BroadcastReviewItem); ok {
			var parts []string
			for _, it := range items {
				parts = append(parts, string(it.Key)+": "+deref(it.Detail))
			}
			return problem(e.Status, e.Code, "the review blocks scheduling: "+strings.Join(parts, "; "))
		}
	}
	return err
}

var mcpBroadcastTools = []mcpTool{
	defineTool("list_email_templates", "List e-mail templates",
		"Lists the starter designs (newsletter, product update, announcement, welcome, event, digest) with the languages each exists in. Start a design from one with create_design. Owners, admins and keys only. Maps to GET /v1/email-templates.",
		readOnly, map[string][]any{"locale": templateLocales},
		mcpTool{ops: []string{"ListEmailTemplates"}, allow: mayManageBroadcasts, denied: errNotBroadcastManager},
		func(ctx context.Context, c *mcpCall, in listTemplatesIn) (mcpPage[mcpEmailTemplate], error) {
			res, err := c.s.ListEmailTemplates(ctx, oas.ListEmailTemplatesRequestObject{Params: oas.ListEmailTemplatesParams{Locale: optString[oas.TemplateLocale](in.Locale)}})
			if err != nil {
				return mcpPage[mcpEmailTemplate]{}, err
			}
			out := mcpPage[mcpEmailTemplate]{Items: []mcpEmailTemplate{}}
			for _, t := range res.(oas.ListEmailTemplates200JSONResponse).Items {
				locales := make([]string, len(t.Locales))
				for i, l := range t.Locales {
					locales[i] = string(l)
				}
				out.Items = append(out.Items, mcpEmailTemplate{Slug: t.Slug, Category: t.Category, Name: t.Name, Description: t.Description, Locales: locales, PreviewURL: t.PreviewUrl})
			}
			return out, nil
		}),
	defineTool("create_design", "Create an e-mail design",
		"Creates an e-mail design from a template in a language, from a whole document, or blank, and returns it with its document and version. Owners, admins and keys only. Maps to POST /v1/email-designs.",
		additive, map[string][]any{"locale": templateLocales},
		mcpTool{ops: []string{"CreateEmailDesign"}, allow: mayManageBroadcasts, denied: errNotBroadcastManager},
		func(ctx context.Context, c *mcpCall, in createDesignIn) (mcpDesign, error) {
			body := oas.EmailDesignCreate{Name: in.Name, InboxId: in.InboxID}
			if in.Template != "" {
				body.Template = &oas.EmailDesignTemplateRef{Slug: in.Template, Locale: optString[oas.EmailTemplateLocale](in.Locale)}
				if len(in.AddLocales) > 0 {
					ls := make([]oas.EmailTemplateLocale, len(in.AddLocales))
					for i, l := range in.AddLocales {
						ls[i] = oas.EmailTemplateLocale(l)
					}
					body.Template.Locales = &ls
				}
			} else if len(in.AddLocales) > 0 {
				return mcpDesign{}, errValidation("add_locales takes a template's languages; pass template too, or add versions with update_design")
			}
			if in.Document != nil {
				doc, err := documentIn(in.Document)
				if err != nil {
					return mcpDesign{}, err
				}
				if in.Locale != "" {
					doc.Locale = new(oas.LanguageTag(in.Locale))
				}
				body.Document = &doc
			}
			res, err := c.s.CreateEmailDesign(ctx, oas.CreateEmailDesignRequestObject{Body: &body})
			if err != nil {
				return mcpDesign{}, err
			}
			return c.designOut(oas.EmailDesign(res.(oas.CreateEmailDesign201JSONResponse)))
		}),
	defineTool("update_design", "Change an e-mail design",
		"Saves a new version of a design, based on the version you changed: the whole changed document of the base language, or with locale the texts of one language version (layout, images and links are shared by all versions). If someone saved it since, nothing is saved and the answer says so; ask the person you work for before overwriting their change. Owners, admins and keys only. Maps to PUT /v1/email-designs/{id}.",
		idempotent, map[string][]any{"direction": {"ltr", "rtl", "auto"}},
		mcpTool{ops: []string{"GetEmailDesign", "UpdateEmailDesign"}, allow: mayManageBroadcasts, denied: errNotBroadcastManager},
		func(ctx context.Context, c *mcpCall, in updateDesignIn) (mcpDesign, error) {
			res, err := c.s.GetEmailDesign(ctx, oas.GetEmailDesignRequestObject{DesignId: in.DesignID})
			if err != nil {
				return mcpDesign{}, err
			}
			cur := oas.EmailDesign(res.(oas.GetEmailDesign200JSONResponse))
			doc, err := in.document(cur)
			if err != nil {
				return mcpDesign{}, err
			}
			name := cur.Name
			if strings.TrimSpace(in.Name) != "" {
				name = in.Name
			}
			ures, err := c.s.UpdateEmailDesign(ctx, oas.UpdateEmailDesignRequestObject{DesignId: in.DesignID, Body: &oas.EmailDesignUpdate{
				BaseVersion: in.BaseVersion, Document: doc, Name: name, InboxId: cur.InboxId,
			}})
			if err != nil {
				return mcpDesign{}, err
			}
			return c.designOut(oas.EmailDesign(ures.(oas.UpdateEmailDesign200JSONResponse)))
		}),
	defineTool("preview_design", "Preview an e-mail design",
		"Renders a design as the mail goes out: the text part, the HTML size and warnings, and the HTML as an embedded text/html resource. Merge fields show a sample contact. Owners, admins and keys only. Maps to POST /v1/email-designs/render with the design's document.",
		readOnly, nil,
		mcpTool{ops: []string{"GetEmailDesign", "RenderEmailDesign"}, allow: mayManageBroadcasts, denied: errNotBroadcastManager},
		func(ctx context.Context, c *mcpCall, in previewDesignIn) (previewDesignOut, error) {
			res, err := c.s.GetEmailDesign(ctx, oas.GetEmailDesignRequestObject{DesignId: in.DesignID})
			if err != nil {
				return previewDesignOut{}, err
			}
			d := oas.EmailDesign(res.(oas.GetEmailDesign200JSONResponse))
			body := oas.EmailDesignRenderRequest{Document: d.Document, ListId: in.ListID, InboxId: in.InboxID, Dark: &in.Dark}
			if in.InboxID == nil && in.ListID == nil {
				body.InboxId = d.InboxId
			}
			if in.Subject != "" {
				body.Subject = &in.Subject
			}
			if in.Preheader != "" {
				body.Preheader = &in.Preheader
			}
			if in.Locale != "" {
				body.Locale = new(oas.LanguageTag(in.Locale))
			}
			rres, err := c.s.RenderEmailDesign(ctx, oas.RenderEmailDesignRequestObject{Body: &body})
			if err != nil {
				return previewDesignOut{}, err
			}
			r := oas.EmailDesignRender(rres.(oas.RenderEmailDesign200JSONResponse))
			return previewDesignOut{
				DesignID: d.Id, Locale: deref((*string)(r.Locale)), Text: r.Text, SizeBytes: r.SizeBytes, Warnings: designIssuesOut(r.Warnings),
				HTMLURI: mcpURIPrefix + "design/" + d.Id.String() + "/preview.html", html: r.Html,
			}, nil
		}),
	defineTool("list_broadcasts", "List broadcasts",
		"Lists the broadcasts and triggered e-mails of the inboxes you can see, newest first, with their state and counts; kind narrows to one of the two. Maps to GET /v1/broadcasts.",
		readOnly, map[string][]any{"states": broadcastStates, "kind": broadcastKinds}, mcpTool{ops: []string{"ListBroadcasts"}},
		func(ctx context.Context, c *mcpCall, in listBroadcastsIn) (mcpPage[mcpBroadcast], error) {
			params := oas.ListBroadcastsParams{InboxId: in.InboxID, Cursor: optString[string](in.Cursor), Limit: optLimit(in.Limit), Kind: optString[oas.BroadcastKind](in.Kind)}
			if len(in.States) > 0 {
				states := make([]oas.BroadcastState, len(in.States))
				for i, st := range in.States {
					states[i] = oas.BroadcastState(st)
				}
				params.State = &states
			}
			res, err := c.s.ListBroadcasts(ctx, oas.ListBroadcastsRequestObject{Params: params})
			if err != nil {
				return mcpPage[mcpBroadcast]{}, err
			}
			page := res.(oas.ListBroadcasts200JSONResponse)
			out := mcpPage[mcpBroadcast]{Items: []mcpBroadcast{}}
			for _, b := range page.Items {
				out.Items = append(out.Items, c.broadcastOut(b))
			}
			if page.NextCursor != nil {
				out.NextCursor = *page.NextCursor
			}
			return out, nil
		}),
	defineTool("get_broadcast", "Get a broadcast",
		"Returns a broadcast with its state and counts and, for a draft, the review scheduling runs (what blocks it) and the audience: how many people would get it now, by list, and who is left out. Maps to GET /v1/broadcasts/{id}, its review and POST /v1/broadcasts/{id}/audience.",
		readOnly, nil, mcpTool{ops: []string{"GetBroadcast", "ReviewBroadcast", "CountBroadcastAudience"}},
		func(ctx context.Context, c *mcpCall, in broadcastIn) (getBroadcastOut, error) {
			return c.getBroadcast(ctx, in.BroadcastID)
		}),
	defineTool("create_broadcast_draft", "Create a draft broadcast",
		"Creates a draft broadcast for an inbox: the e-mail channel, the lists, the design, subject and preheader, an optional pace (per hour, day, 30 days) that spreads the mails evenly, and optional sending hours in the broadcast's time zone (the inbox's unless timezone is given). Nothing is sent; check it with get_broadcast. Owners, admins and keys only. Maps to POST /v1/broadcasts.",
		additive, map[string][]any{"conditions.op": conditionOps},
		mcpTool{ops: []string{"CreateBroadcast"}, allow: mayManageBroadcasts, denied: errNotBroadcastManager},
		func(ctx context.Context, c *mcpCall, in createBroadcastIn) (mcpBroadcast, error) {
			aud := audienceIn(in.ListIDs, in.Locales, in.Conditions)
			body := oas.BroadcastCreate{
				InboxId: in.InboxID, ChannelId: in.ChannelID, Name: in.Name, Subject: &in.Subject, DesignId: in.DesignID,
				Audience: &aud, ClickTracking: &in.ClickTracking,
			}
			if in.Preheader != "" {
				body.Preheader = &in.Preheader
			}
			if in.PacePerHour != nil || in.PacePerDay != nil || in.PacePerMonth != nil {
				body.Pace = &oas.BroadcastPace{PerHour: in.PacePerHour, PerDay: in.PacePerDay, PerMonth: in.PacePerMonth}
			}
			if in.Timezone != "" {
				body.Timezone = &in.Timezone
			}
			w, perr := windowIn(in.WindowStart, in.WindowEnd)
			if perr != nil {
				return mcpBroadcast{}, perr
			}
			body.SendWindow = w
			if len(in.Translations) > 0 {
				body.Translations = broadcastTranslationsIn(in.Translations)
			}
			res, err := c.s.CreateBroadcast(ctx, oas.CreateBroadcastRequestObject{Body: &body})
			if err != nil {
				return mcpBroadcast{}, err
			}
			return c.broadcastOut(oas.Broadcast(res.(oas.CreateBroadcast201JSONResponse))), nil
		}),
	defineTool("update_broadcast", "Change a draft broadcast",
		"Changes the fields you pass on a draft: name, subject, preheader, the e-mail channel (the sender), the design, the audience (list_ids with optional locales and conditions, replacing the whole audience), click tracking, UTM tags, the pace (the pace fields replace the whole pace; remove_pace drops it), the time zone and the sending hours (remove_send_window drops them), and a triggered e-mail's delay and back-fill days. While a broadcast is scheduled or sending only the pace, time zone and sending hours can change; a started triggered e-mail also takes a new delay, and while paused its design, subjects and conditions (with the same list_ids). Returns the broadcast with the review again, so you see what still blocks it. Nothing is sent. Owners, admins and keys only. Maps to PATCH /v1/broadcasts/{id}.",
		idempotent, map[string][]any{"conditions.op": conditionOps, "delay_unit": delayUnits},
		mcpTool{ops: []string{"UpdateBroadcast", "GetBroadcast", "ReviewBroadcast", "CountBroadcastAudience"}, allow: mayManageBroadcasts, denied: errNotBroadcastManager},
		func(ctx context.Context, c *mcpCall, in updateBroadcastIn) (getBroadcastOut, error) {
			body, perr := in.body()
			if perr != nil {
				return getBroadcastOut{}, perr
			}
			if _, err := c.s.UpdateBroadcast(ctx, oas.UpdateBroadcastRequestObject{BroadcastId: in.BroadcastID, Body: &body}); err != nil {
				return getBroadcastOut{}, err
			}
			return c.getBroadcast(ctx, in.BroadcastID)
		}),
	defineTool("send_test_broadcast", "Send a test of a broadcast",
		"Sends a test of a draft to you (with an API key, to the members named), never to subscribers; at most 20 tests an hour per workspace. Owners, admins and keys only. Maps to POST /v1/broadcasts/{id}/test.",
		delivering, nil,
		mcpTool{ops: []string{"SendBroadcastTest"}, allow: mayManageBroadcasts, denied: errNotBroadcastManager},
		func(ctx context.Context, c *mcpCall, in sendTestIn) (sendTestOut, error) {
			body := oas.BroadcastTest{}
			if in.Locale != "" {
				body.Locale = new(oas.LanguageTag(in.Locale))
			}
			if len(in.MemberIDs) > 0 {
				if !c.p.isKey() {
					return sendTestOut{}, errValidation("a test from a connected assistant goes to you; leave member_ids out")
				}
				body.MemberIds = &in.MemberIDs
			}
			res, err := c.s.SendBroadcastTest(ctx, oas.SendBroadcastTestRequestObject{BroadcastId: in.BroadcastID, Body: &body})
			if err != nil {
				return sendTestOut{}, err
			}
			tr := res.(oas.SendBroadcastTest202JSONResponse)
			out := sendTestOut{SentTo: []string{}, Locale: deref((*string)(tr.Locale))}
			for _, e := range tr.SentTo {
				out.SentTo = append(out.SentTo, string(e))
			}
			return out, nil
		}),
	defineTool("schedule_broadcast", "Schedule a broadcast or start a triggered e-mail",
		"Schedules a draft broadcast to go out to its audience at send_at, or at send_at_local in the broadcast's time zone (now when neither is given); with sending hours that are closed then, sending waits for them. For a triggered e-mail it starts it then instead: from then on each contact the trigger enrolls gets it once, after the delay; backfill_days also enrolls those whose trigger happened in that many days before the start, and a service one needs confirm_service. confirm_recipients must be the recipient count that get_broadcast showed and the person you work for confirmed (for a triggered e-mail the back-fill count); if it changed since, nothing is scheduled. The review must pass. Listed only while the workspace lets API keys and assistants schedule broadcasts. Maps to POST /v1/broadcasts/{id}/schedule.",
		delivering, nil,
		mcpTool{ops: []string{"ScheduleBroadcast"}, gate: maySchedule},
		func(ctx context.Context, c *mcpCall, in scheduleBroadcastIn) (mcpBroadcast, error) {
			body := &oas.BroadcastSchedule{SendAt: in.SendAt, SendAtLocal: optString[string](in.SendAtLocal), ConfirmRecipients: &in.ConfirmRecipients, BackfillDays: in.BackfillDays}
			if in.ConfirmService {
				body.ConfirmService = &in.ConfirmService
			}
			res, err := c.s.ScheduleBroadcast(ctx, oas.ScheduleBroadcastRequestObject{BroadcastId: in.BroadcastID, Body: body})
			if err != nil {
				return mcpBroadcast{}, scheduleProblem(err)
			}
			return c.broadcastOut(oas.Broadcast(res.(oas.ScheduleBroadcast200JSONResponse))), nil
		}),
	defineTool("create_triggered_email", "Create a draft triggered e-mail",
		"Creates a draft triggered e-mail for an inbox: one design sent to each contact once, a set time after the trigger (contact_identified: the contact first signs in to the product). basis service is for mail about using the product, to everyone who matches, with no lists; marketing goes only to subscribers of list_ids and is the only basis for advertising and offers. Nothing is sent and nobody is enrolled until schedule_broadcast starts it; check it with get_broadcast. Owners, admins and keys only. Maps to POST /v1/broadcasts with kind triggered.",
		additive, map[string][]any{"conditions.op": conditionOps, "event": triggerEvents, "basis": triggerBases, "delay_unit": delayUnits},
		mcpTool{ops: []string{"CreateBroadcast"}, allow: mayManageBroadcasts, denied: errNotBroadcastManager},
		func(ctx context.Context, c *mcpCall, in createTriggeredIn) (mcpBroadcast, error) {
			delay, err := delayMinutes(in.Delay, in.DelayUnit)
			if err != nil {
				return mcpBroadcast{}, err
			}
			aud := audienceIn(in.ListIDs, in.Locales, in.Conditions)
			kind, event, basis := oas.KindTriggered, oas.BroadcastTriggerEvent(in.Event), oas.BroadcastBasis(in.Basis)
			body := oas.BroadcastCreate{
				InboxId: in.InboxID, ChannelId: in.ChannelID, Name: in.Name, Subject: &in.Subject, DesignId: in.DesignID,
				Audience: &aud, ClickTracking: &in.ClickTracking, Kind: &kind,
				Trigger: &oas.BroadcastTriggerInput{Event: &event, DelayMinutes: &delay, Basis: &basis},
			}
			if in.Preheader != "" {
				body.Preheader = &in.Preheader
			}
			if in.Timezone != "" {
				body.Timezone = &in.Timezone
			}
			w, perr := windowIn(in.WindowStart, in.WindowEnd)
			if perr != nil {
				return mcpBroadcast{}, perr
			}
			body.SendWindow = w
			if len(in.Translations) > 0 {
				body.Translations = broadcastTranslationsIn(in.Translations)
			}
			res, err := c.s.CreateBroadcast(ctx, oas.CreateBroadcastRequestObject{Body: &body})
			if err != nil {
				return mcpBroadcast{}, err
			}
			return c.broadcastOut(oas.Broadcast(res.(oas.CreateBroadcast201JSONResponse))), nil
		}),
	defineTool("list_triggered_waiting", "List a triggered e-mail's contacts",
		"Lists the contacts a triggered e-mail enrolled: by default those still waiting, the one due first first, with when their mail goes out; with state also those mailed (queued), skipped with the reason, or dropped when it was stopped (cancelled). Maps to GET /v1/broadcasts/{id}/enrollments.",
		readOnly, map[string][]any{"state": enrollmentStates}, mcpTool{ops: []string{"ListBroadcastEnrollments"}},
		func(ctx context.Context, c *mcpCall, in listWaitingIn) (mcpPage[mcpEnrollment], error) {
			state := in.State
			if state == "" {
				state = enrollWaiting
			}
			params := oas.ListBroadcastEnrollmentsParams{State: new(oas.TriggeredEnrollmentState(state)), Cursor: optString[string](in.Cursor), Limit: optLimit(in.Limit)}
			res, err := c.s.ListBroadcastEnrollments(ctx, oas.ListBroadcastEnrollmentsRequestObject{BroadcastId: in.BroadcastID, Params: params})
			if err != nil {
				return mcpPage[mcpEnrollment]{}, err
			}
			page := res.(oas.ListBroadcastEnrollments200JSONResponse)
			out := mcpPage[mcpEnrollment]{Items: []mcpEnrollment{}}
			for _, e := range page.Items {
				m := mcpEnrollment{
					ContactID: e.Contact.Id, State: string(e.State), TriggeredAt: e.TriggeredAt, SendsAt: e.SendsAt, Source: string(e.Source),
					DeliveryID: e.DeliveryId, CustomerContent: mcpEnrollmentContent{ContactName: e.Contact.Name, ContactEmail: deref((*string)(e.Contact.Email))},
				}
				if e.SkipReason != nil {
					m.SkipReason = string(*e.SkipReason)
				}
				out.Items = append(out.Items, m)
			}
			if page.NextCursor != nil {
				out.NextCursor = *page.NextCursor
			}
			return out, nil
		}),
	defineTool("remove_triggered_contact", "Take a contact off a triggered e-mail",
		"A waiting contact gets no mail from this triggered e-mail, now or later: it is skipped (removed) and never enrolled again. Only when the person you work for asks. Owners, admins and keys only. Maps to DELETE /v1/broadcasts/{id}/enrollments/{contact_id}.",
		destructive, nil, mcpTool{ops: []string{"RemoveBroadcastEnrollment"}, allow: mayManageBroadcasts, denied: errNotBroadcastManager},
		func(ctx context.Context, c *mcpCall, in removeWaitingIn) (removeWaitingOut, error) {
			if _, err := c.s.RemoveBroadcastEnrollment(ctx, oas.RemoveBroadcastEnrollmentRequestObject{BroadcastId: in.BroadcastID, ContactId: in.ContactID}); err != nil {
				return removeWaitingOut{}, err
			}
			return removeWaitingOut{Removed: true}, nil
		}),
	defineTool("get_broadcast_report", "Get a broadcast's report",
		"Reports a broadcast: counts, rates of sent mail, a timeline, clicks per link with click tracking, and the conversations replies opened; for a design with language versions also counts and rates per version, and with locale the report of one version. Maps to GET /v1/broadcasts/{id}/report.",
		readOnly, nil, mcpTool{ops: []string{"GetBroadcastReport"}},
		func(ctx context.Context, c *mcpCall, in broadcastReportIn) (mcpReport, error) {
			params := oas.GetBroadcastReportParams{}
			if in.Locale != "" {
				params.Locale = new(oas.LanguageTag(in.Locale))
			}
			res, err := c.s.GetBroadcastReport(ctx, oas.GetBroadcastReportRequestObject{BroadcastId: in.BroadcastID, Params: params})
			if err != nil {
				return mcpReport{}, err
			}
			r := oas.BroadcastReport(res.(oas.GetBroadcastReport200JSONResponse))
			out := mcpReport{
				BroadcastID: r.BroadcastId, Locale: deref((*string)(r.Locale)), Counts: broadcastCountsOut(r.Counts), Timeline: []mcpReportPoint{},
				Replies: []mcpReportReply{}, Rates: ratesOut(r.Rates),
			}
			if r.Locales != nil {
				for _, l := range *r.Locales {
					out.Locales = append(out.Locales, mcpLocaleReport{Locale: string(l.Locale), Counts: broadcastCountsOut(l.Counts), Rates: ratesOut(l.Rates)})
				}
			}
			for _, pt := range r.Timeline {
				out.Timeline = append(out.Timeline, mcpReportPoint{At: pt.At, Sent: pt.Sent, Bounced: pt.Bounced, Complained: pt.Complained,
					Unsubscribed: pt.Unsubscribed, Replied: pt.Replied, Clicked: pt.Clicked})
			}
			if r.Links != nil {
				for _, l := range *r.Links {
					out.Links = append(out.Links, mcpReportLink{URL: l.Url, Clicks: l.Clicks})
				}
			}
			for _, rp := range r.Replies {
				out.Replies = append(out.Replies, mcpReportReply{
					ConversationID: rp.ConversationId, ContactID: rp.Contact.Id, At: rp.At, CustomerContent: mcpReplyContent{ContactName: rp.Contact.Name},
				})
			}
			return out, nil
		}),
}

package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	maxCustomFields       = 50
	maxFieldOptions       = 100
	maxFieldTextRunes     = 1000
	fieldClearBatch       = 500
	fieldClearBudget      = 20 * time.Second
	fieldFilterPrefix     = "field."
	fieldObjectConv       = string(oas.CustomFieldObjectConversation)
	fieldObjectContact    = string(oas.CustomFieldObjectContact)
	fieldTypeSelect       = string(oas.CustomFieldTypeSelect)
	fieldTypeNumber       = string(oas.CustomFieldTypeNumber)
	fieldTypeCheckbox     = string(oas.CustomFieldTypeCheckbox)
	fieldTypeDate         = string(oas.CustomFieldTypeDate)
	fieldTypeText         = string(oas.CustomFieldTypeText)
	customFieldClearKind  = "custom_field_clear"
	customFieldClearTries = 10
)

var (
	fieldKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

	errFieldGone     = problem(http.StatusNotFound, "not_found", "no such custom field")
	errFieldTaken    = problem(http.StatusConflict, "field_taken", "another field of this object has this key")
	errFieldLimit    = problem(http.StatusForbidden, "field_limit", "an object holds at most 50 custom fields")
	errFieldClearing = problem(http.StatusConflict, "field_clearing", "values of a deleted field with this key are still being cleared; try again shortly")
)

func errFieldInvalid(key, detail string) *apiError {
	e := problem(http.StatusBadRequest, "field_invalid", detail)
	e.Field = key
	return e
}

func errFieldRequired(f store.CustomField) *apiError {
	e := problem(http.StatusConflict, "field_required", "the conversation cannot be closed without "+f.Name+" ("+f.Key+")")
	e.Field = f.Key
	return e
}

func fieldOptions(f store.CustomField) []string {
	out := []string{}
	_ = json.Unmarshal(f.Options, &out)
	return out
}

func customFieldBody(f store.CustomField) oas.CustomField {
	return oas.CustomField{
		Id: f.ID, Object: oas.CustomFieldObject(f.Object), Key: f.Key, Name: f.Name, Type: oas.CustomFieldType(f.Type),
		Options: fieldOptions(f), RequiredOnClose: f.RequiredOnClose, Position: f.Position, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
	}
}

// fieldValue checks v against the field's type and returns the value to store.
func fieldValue(f store.CustomField, v any) (any, bool) {
	switch f.Type {
	case fieldTypeText:
		s, ok := v.(string)
		s = strings.TrimSpace(s)
		return s, ok && s != "" && utf8.RuneCountInString(s) <= maxFieldTextRunes
	case fieldTypeNumber:
		n, ok := v.(float64)
		return n, ok && !math.IsNaN(n) && !math.IsInf(n, 0)
	case fieldTypeDate:
		s, ok := v.(string)
		if !ok || len(s) != 10 {
			return nil, false
		}
		_, err := time.Parse(time.DateOnly, s)
		return s, err == nil
	case fieldTypeSelect:
		s, ok := v.(string)
		return s, ok && slices.Contains(fieldOptions(f), s)
	case fieldTypeCheckbox:
		b, ok := v.(bool)
		return b, ok
	}
	return nil, false
}

// fieldFromText reads a value written as text, as in a query string or a CSV cell.
func fieldFromText(f store.CustomField, s string) (any, bool) {
	s = strings.TrimSpace(s)
	switch f.Type {
	case fieldTypeNumber:
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, false
		}
		return fieldValue(f, n)
	case fieldTypeCheckbox:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return nil, false
		}
		return b, true
	}
	return fieldValue(f, s)
}

func fieldValueDetail(f store.CustomField) string {
	switch f.Type {
	case fieldTypeText:
		return f.Key + " must be text of 1 to 1000 characters"
	case fieldTypeNumber:
		return f.Key + " must be a number"
	case fieldTypeDate:
		return f.Key + " must be a date, YYYY-MM-DD"
	case fieldTypeSelect:
		return f.Key + " must be one of its options"
	}
	return f.Key + " must be true or false"
}

func fieldsByKey(defs []store.CustomField) map[string]store.CustomField {
	out := make(map[string]store.CustomField, len(defs))
	for _, d := range defs {
		out[d.Key] = d
	}
	return out
}

func decodeFields(b []byte) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out
}

// mergeConversationFields applies an update to the stored values: null clears a key, every other
// key must be a conversation field and its value fit the type.
func mergeConversationFields(cur []byte, defs []store.CustomField, upd map[string]any) ([]byte, error) {
	if len(upd) > maxCustomFields {
		return nil, errValidation("fields names at most 50 keys")
	}
	byKey := fieldsByKey(defs)
	out := decodeFields(cur)
	for _, k := range slices.Sorted(maps.Keys(upd)) {
		f, ok := byKey[k]
		if !ok {
			return nil, errFieldInvalid(k, k+" is not a conversation custom field")
		}
		if upd[k] == nil {
			delete(out, k)
			continue
		}
		v, ok := fieldValue(f, upd[k])
		if !ok {
			return nil, errFieldInvalid(k, fieldValueDetail(f))
		}
		out[k] = v
	}
	return mustJSON(out), nil
}

// missingRequired is the first field required on close that has no value.
func missingRequired(defs []store.CustomField, values []byte) (store.CustomField, bool) {
	have := decodeFields(values)
	for _, d := range defs {
		if d.RequiredOnClose && have[d.Key] == nil {
			return d, true
		}
	}
	return store.CustomField{}, false
}

func sameFields(a, b []byte) bool {
	return maps.EqualFunc(decodeFields(a), decodeFields(b), func(x, y any) bool { return string(mustJSON(x)) == string(mustJSON(y)) })
}

func conversationFieldDefs(ctx context.Context, q *store.Queries, workspaceID uuid.UUID) ([]store.CustomField, error) {
	return q.ListCustomFields(ctx, store.ListCustomFieldsParams{WorkspaceID: workspaceID, Object: new(fieldObjectConv)})
}

// applyFieldChange sets next.Fields from upd and refuses a close without a required field; the
// shared lock keeps a definition from being deleted while values for it are written.
func applyFieldChange(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, cur store.Conversation, next *store.Conversation, upd *oas.CustomFieldValuesUpdate) error {
	closed := string(oas.ConversationStatusClosed)
	closing := next.Status == closed && !next.Spam && (cur.Status != closed || cur.Spam)
	if upd == nil && !closing {
		return nil
	}
	if upd != nil {
		if err := q.ShareLockCustomFields(ctx, workspaceID); err != nil {
			return err
		}
	}
	defs, err := conversationFieldDefs(ctx, q, workspaceID)
	if err != nil {
		return err
	}
	if upd != nil {
		if next.Fields, err = mergeConversationFields(cur.Fields, defs, *upd); err != nil {
			return err
		}
	}
	if closing {
		if f, missing := missingRequired(defs, next.Fields); missing {
			return errFieldRequired(f)
		}
	}
	return nil
}

// fieldFilter turns field.<key>=<value> pairs into the jsonb the list matches with @>.
func fieldFilter(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, pairs map[string][]string) ([]byte, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	defs, err := filterFieldDefs(ctx, q, workspaceID)
	if err != nil {
		return nil, err
	}
	byKey := fieldsByKey(defs)
	out := map[string]any{}
	for _, k := range slices.Sorted(maps.Keys(pairs)) {
		f, ok := byKey[k]
		if !ok {
			return nil, errFieldInvalid(k, k+" is not a conversation custom field")
		}
		if len(pairs[k]) != 1 {
			return nil, errFieldInvalid(k, "field."+k+" may be given once")
		}
		v, ok := fieldFromText(f, pairs[k][0])
		if !ok {
			return nil, errFieldInvalid(k, fieldValueDetail(f))
		}
		out[k] = v
	}
	return mustJSON(out), nil
}

type fieldFilterKey struct{}

// withFieldFilter hands field filters to ListConversations from callers that have no query
// string, such as MCP.
func withFieldFilter(ctx context.Context, pairs map[string]string) context.Context {
	m := map[string][]string{}
	for k, v := range pairs {
		m[k] = []string{v}
	}
	return context.WithValue(ctx, fieldFilterKey{}, m)
}

func fieldFilterPairs(ctx context.Context) map[string][]string {
	if m, ok := ctx.Value(fieldFilterKey{}).(map[string][]string); ok {
		return m
	}
	r := requestFrom(ctx)
	if r == nil {
		return nil
	}
	out := map[string][]string{}
	for k, v := range r.URL.Query() {
		if key, ok := strings.CutPrefix(k, fieldFilterPrefix); ok {
			out[key] = v
		}
	}
	return out
}

func contactFieldDefs(ctx context.Context, q *store.Queries, workspaceID uuid.UUID) ([]store.CustomField, error) {
	return q.ListCustomFields(ctx, store.ListCustomFieldsParams{WorkspaceID: workspaceID, Object: new(fieldObjectContact)})
}

// checkContactFields refuses attributes whose key is a contact field and whose value does not fit
// it; other keys stay free-form and null leaves a field unset.
func checkContactFields(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, attrs map[string]any) error {
	if len(attrs) == 0 {
		return nil
	}
	defs, err := contactFieldDefs(ctx, q, workspaceID)
	if err != nil {
		return err
	}
	for _, d := range defs {
		v, present := attrs[d.Key]
		if !present || v == nil {
			continue
		}
		if _, ok := fieldValue(d, v); !ok {
			return errFieldInvalid(d.Key, fieldValueDetail(d))
		}
	}
	return nil
}

// keepValidContactFields drops attributes that a contact field refuses, for writers that must not
// be refused (identity tokens, imports); fromText reads text cells into the field's type.
func keepValidContactFields(defs []store.CustomField, attrs map[string]any, fromText bool) (map[string]any, []string) {
	var dropped []string
	for _, d := range defs {
		v, present := attrs[d.Key]
		if !present || v == nil {
			continue
		}
		var got any
		var ok bool
		if s, isText := v.(string); fromText && isText {
			got, ok = fieldFromText(d, s)
		} else {
			got, ok = fieldValue(d, v)
		}
		if !ok {
			delete(attrs, d.Key)
			dropped = append(dropped, d.Key)
			continue
		}
		attrs[d.Key] = got
	}
	return attrs, dropped
}

func validFieldOptions(typ string, opts *[]string) ([]string, error) {
	if typ != fieldTypeSelect {
		if opts != nil && len(*opts) > 0 {
			return nil, errValidation("options are only for select fields")
		}
		return []string{}, nil
	}
	if opts == nil || len(*opts) == 0 || len(*opts) > maxFieldOptions {
		return nil, errValidation("a select field needs 1 to 100 options")
	}
	out := make([]string, 0, len(*opts))
	for _, o := range *opts {
		o = strings.TrimSpace(o)
		if o == "" || utf8.RuneCountInString(o) > 100 {
			return nil, errValidation("each option must be 1 to 100 characters")
		}
		if slices.Contains(out, o) {
			return nil, errValidation("options must be distinct")
		}
		out = append(out, o)
	}
	return out, nil
}

func fieldPosition(p *int32) (int32, error) {
	if p == nil {
		return 0, nil
	}
	if *p < 0 || *p > 10000 {
		return 0, errValidation("position must be 0 to 10000")
	}
	return *p, nil
}

func readsFields(p principal) error {
	if p.scoped() && requireScope(p, oas.ConversationsRead) != nil && requireScope(p, oas.ContactsRead) != nil {
		return errInsufficientScope(oas.ConversationsRead)
	}
	return nil
}

func (s *Server) ListCustomFields(ctx context.Context, req oas.ListCustomFieldsRequestObject) (oas.ListCustomFieldsResponseObject, error) {
	p := principalFrom(ctx)
	if err := readsFields(p); err != nil {
		return nil, err
	}
	arg := store.ListCustomFieldsParams{WorkspaceID: p.workspaceID}
	if o := req.Params.Object; o != nil {
		if !o.Valid() {
			return nil, errValidation("object must be conversation or contact")
		}
		arg.Object = new(string(*o))
	}
	rows, err := s.st.ListCustomFields(ctx, arg)
	if err != nil {
		return nil, err
	}
	out := oas.ListCustomFields200JSONResponse{Items: make([]oas.CustomField, len(rows))}
	for i, r := range rows {
		out.Items[i] = customFieldBody(r)
	}
	return out, nil
}

func (s *Server) GetCustomField(ctx context.Context, req oas.GetCustomFieldRequestObject) (oas.GetCustomFieldResponseObject, error) {
	p := principalFrom(ctx)
	if err := readsFields(p); err != nil {
		return nil, err
	}
	f, err := s.st.GetCustomField(ctx, store.GetCustomFieldParams{WorkspaceID: p.workspaceID, ID: req.CustomFieldId})
	if store.IsNotFound(err) {
		return nil, errFieldGone
	}
	if err != nil {
		return nil, err
	}
	return oas.GetCustomField200JSONResponse(customFieldBody(f)), nil
}

func (s *Server) CreateCustomField(ctx context.Context, req oas.CreateCustomFieldRequestObject) (oas.CreateCustomFieldResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrFullKey(p); err != nil {
		return nil, err
	}
	b := req.Body
	if !b.Object.Valid() {
		return nil, errValidation("object must be conversation or contact")
	}
	if !b.Type.Valid() {
		return nil, errValidation("type must be text, number, date, select or checkbox")
	}
	if !fieldKeyPattern.MatchString(b.Key) {
		return nil, errValidation("key must start with a-z and hold up to 40 of a-z, 0-9 and _")
	}
	name, err := trimmed(b.Name, 1, 100, "name")
	if err != nil {
		return nil, err
	}
	opts, err := validFieldOptions(string(b.Type), b.Options)
	if err != nil {
		return nil, err
	}
	pos, err := fieldPosition(b.Position)
	if err != nil {
		return nil, err
	}
	required := b.RequiredOnClose != nil && *b.RequiredOnClose
	if required && b.Object != oas.CustomFieldObjectConversation {
		return nil, errValidation("required_on_close is only for conversation fields")
	}
	var out store.CustomField
	var clearing bool
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if err := q.LockCustomFields(ctx, p.workspaceID); err != nil {
			return err
		}
		n, err := q.CountCustomFields(ctx, store.CountCustomFieldsParams{WorkspaceID: p.workspaceID, Object: string(b.Object)})
		if err != nil {
			return err
		}
		if n >= maxCustomFields {
			return errFieldLimit
		}
		taken, err := q.CustomFieldKeyTaken(ctx, store.CustomFieldKeyTakenParams{WorkspaceID: p.workspaceID, Object: string(b.Object), Key: b.Key})
		if err != nil {
			return err
		}
		if taken {
			return errFieldTaken
		}
		if b.Object == oas.CustomFieldObjectConversation {
			held, err := q.ConversationsHoldField(ctx, store.ConversationsHoldFieldParams{WorkspaceID: p.workspaceID, Key: b.Key})
			if err != nil {
				return err
			}
			if held {
				clearing = true
				events.job(CustomFieldClearArgs{WorkspaceID: p.workspaceID, Key: b.Key, Run: newID()}, nil)
				return nil
			}
		}
		out, err = q.CreateCustomField(ctx, store.CreateCustomFieldParams{
			ID: newID(), WorkspaceID: p.workspaceID, Object: string(b.Object), Key: b.Key, Name: name, Type: string(b.Type),
			Options: mustJSON(opts), RequiredOnClose: required, Position: pos, Now: s.now(),
		})
		return err
	})
	if store.IsUniqueViolation(err) {
		return nil, errFieldTaken
	}
	if err != nil {
		return nil, err
	}
	if clearing {
		return nil, errFieldClearing
	}
	return oas.CreateCustomField201JSONResponse(customFieldBody(out)), nil
}

func (s *Server) UpdateCustomField(ctx context.Context, req oas.UpdateCustomFieldRequestObject) (oas.UpdateCustomFieldResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrFullKey(p); err != nil {
		return nil, err
	}
	b := req.Body
	var out store.CustomField
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if err := q.LockCustomFields(ctx, p.workspaceID); err != nil {
			return err
		}
		cur, err := q.GetCustomField(ctx, store.GetCustomFieldParams{WorkspaceID: p.workspaceID, ID: req.CustomFieldId})
		if store.IsNotFound(err) {
			return errFieldGone
		}
		if err != nil {
			return err
		}
		arg := store.UpdateCustomFieldParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, Name: cur.Name, Options: cur.Options,
			RequiredOnClose: cur.RequiredOnClose, Position: cur.Position, Now: s.now(),
		}
		if b.Name != nil {
			if arg.Name, err = trimmed(*b.Name, 1, 100, "name"); err != nil {
				return err
			}
		}
		var removed []string
		if b.Options != nil {
			opts, err := validFieldOptions(cur.Type, b.Options)
			if err != nil {
				return err
			}
			for _, o := range fieldOptions(cur) {
				if !slices.Contains(opts, o) {
					removed = append(removed, o)
				}
			}
			arg.Options = mustJSON(opts)
		}
		if b.RequiredOnClose != nil {
			if *b.RequiredOnClose && cur.Object != fieldObjectConv {
				return errValidation("required_on_close is only for conversation fields")
			}
			arg.RequiredOnClose = *b.RequiredOnClose
		}
		if b.Position != nil {
			if arg.Position, err = fieldPosition(b.Position); err != nil {
				return err
			}
		}
		if out, err = q.UpdateCustomField(ctx, arg); err != nil {
			return err
		}
		if cur.Object == fieldObjectConv {
			for _, o := range removed {
				events.job(CustomFieldClearArgs{WorkspaceID: p.workspaceID, FieldID: &cur.ID, Key: cur.Key, Option: new(o), Run: newID()}, nil)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateCustomField200JSONResponse(customFieldBody(out)), nil
}

func (s *Server) DeleteCustomField(ctx context.Context, req oas.DeleteCustomFieldRequestObject) (oas.DeleteCustomFieldResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrFullKey(p); err != nil {
		return nil, err
	}
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if err := q.LockCustomFields(ctx, p.workspaceID); err != nil {
			return err
		}
		f, err := q.DeleteCustomField(ctx, store.DeleteCustomFieldParams{WorkspaceID: p.workspaceID, ID: req.CustomFieldId})
		if store.IsNotFound(err) {
			return errFieldGone
		}
		if err != nil {
			return err
		}
		if f.Object == fieldObjectConv {
			events.job(CustomFieldClearArgs{WorkspaceID: p.workspaceID, FieldID: &f.ID, Key: f.Key, Run: newID()}, nil)
			return q.StripFieldFromSavedViews(ctx, store.StripFieldFromSavedViewsParams{WorkspaceID: p.workspaceID, Key: f.Key})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteCustomField204Response{}, nil
}

// CustomFieldClearArgs clears the conversation values of a deleted field, or of a select option
// the field no longer offers (Option set). Run makes every deletion its own job: a unique job would
// be skipped while an earlier one for the same key is kept as completed. A create refused with
// field_clearing queues one without FieldID, so values a discarded job left are cleared too.
type CustomFieldClearArgs struct {
	WorkspaceID uuid.UUID  `json:"workspace_id"`
	FieldID     *uuid.UUID `json:"field_id,omitempty"`
	Key         string     `json:"key"`
	Option      *string    `json:"option,omitempty"`
	Run         uuid.UUID  `json:"run"`
}

func (CustomFieldClearArgs) Kind() string { return customFieldClearKind }

func (CustomFieldClearArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: customFieldClearTries}
}

type customFieldClearWorker struct {
	river.WorkerDefaults[CustomFieldClearArgs]
	s *Server
}

func (w *customFieldClearWorker) Timeout(*river.Job[CustomFieldClearArgs]) time.Duration {
	return fieldClearBudget + time.Minute
}

func (w *customFieldClearWorker) Work(ctx context.Context, job *river.Job[CustomFieldClearArgs]) error {
	done, err := w.s.ClearCustomFieldValues(ctx, job.Args, fieldClearBudget)
	if err != nil {
		return err
	}
	if !done {
		return river.JobSnooze(0)
	}
	return nil
}

// ClearCustomFieldValues clears batches until none is left or the budget is spent, and reports
// whether it is done. Each cleared conversation sends conversation.updated.
func (s *Server) ClearCustomFieldValues(ctx context.Context, a CustomFieldClearArgs, budget time.Duration) (bool, error) {
	start := time.Now()
	total := 0
	after := uuid.Nil()
	for {
		var n int
		err := s.inTx(ctx, a.WorkspaceID, func(q *store.Queries, events *eventBatch) error {
			if err := q.ShareLockCustomFields(ctx, a.WorkspaceID); err != nil {
				return err
			}
			rows, err := q.ClearConversationField(ctx, store.ClearConversationFieldParams{
				WorkspaceID: a.WorkspaceID, Key: a.Key, Option: a.Option, After: after, Lim: fieldClearBatch, Now: s.now(),
			})
			if err != nil {
				return err
			}
			n = len(rows)
			for _, r := range rows {
				if r.ID.Compare(after) > 0 {
					after = r.ID
				}
			}
			return conversationsUpdated(ctx, q, events, a.WorkspaceID, rows)
		})
		if err != nil {
			return false, err
		}
		total += n
		if n < fieldClearBatch {
			s.log.InfoContext(ctx, "custom field values cleared", slog.String("workspace_id", a.WorkspaceID.String()), slog.Int("conversations", total))
			return true, nil
		}
		if time.Since(start) > budget {
			return false, nil
		}
	}
}

// conversationsUpdated sends conversation.updated for rows changed outside changeConversation.
func conversationsUpdated(ctx context.Context, q *store.Queries, events *eventBatch, workspaceID uuid.UUID, rows []store.Conversation) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	labels, err := conversationLabels(ctx, q, workspaceID, ids)
	if err != nil {
		return err
	}
	bodies := make([]oas.Conversation, len(rows))
	refs := make([]*oas.Conversation, len(rows))
	for i, r := range rows {
		bodies[i] = conversationBody(r, labels[r.ID])
		refs[i] = &bodies[i]
	}
	if err := fillImportSources(ctx, q, workspaceID, refs...); err != nil {
		return err
	}
	if err := fillIdeas(ctx, q, workspaceID, refs...); err != nil {
		return err
	}
	for i, r := range rows {
		events.conversation(realtime.ConversationUpdated, r, bodies[i])
	}
	return nil
}

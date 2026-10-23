package api

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const feedbackFallbackCategory = "other"

var errUnknownFeedbackCategory = errValidation("category must be one of the inbox's feedback categories")

var (
	categoryKeyPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	categoryIconPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

func inboxCategories(in store.Inbox) []oas.InboxFeedbackCategory {
	var out []oas.InboxFeedbackCategory
	_ = json.Unmarshal(in.FeedbackCategories, &out)
	if out == nil {
		out = []oas.InboxFeedbackCategory{}
	}
	return out
}

func categoryKeys(list []oas.InboxFeedbackCategory) []string {
	keys := make([]string, len(list))
	for i, c := range list {
		keys[i] = c.Key
	}
	return keys
}

func validCategories(list []oas.InboxFeedbackCategory) error {
	if len(list) < 1 || len(list) > 20 {
		return errValidation("feedback_categories must have 1 to 20 entries")
	}
	seen := map[string]bool{}
	for i := range list {
		c := &list[i]
		if !categoryKeyPattern.MatchString(c.Key) {
			return errValidation("feedback_categories keys are lowercase letters, digits and underscores, starting with a letter, up to 32 characters")
		}
		if seen[c.Key] {
			return errValidation("feedback_categories keys must be unique: " + c.Key)
		}
		seen[c.Key] = true
		if len(c.Names) < 1 || len(c.Names) > 40 {
			return errValidation("feedback_categories " + c.Key + " needs 1 to 40 names")
		}
		names := make(map[string]string, len(c.Names))
		for loc, name := range c.Names {
			if len(loc) > 35 || !languagePattern.MatchString(loc) {
				return errValidation("feedback_categories names are keyed by language tags such as en or tr")
			}
			t, err := trimmed(name, 1, 60, "feedback_categories name")
			if err != nil {
				return err
			}
			names[strings.ToLower(loc)] = t
		}
		c.Names = names
		if len(c.Icon) > 40 || !categoryIconPattern.MatchString(c.Icon) {
			return errValidation("feedback_categories icon is a lowercase icon name such as bug or message-circle")
		}
	}
	if !seen[feedbackFallbackCategory] {
		return errValidation("feedback_categories must keep the category other")
	}
	return nil
}

func categoryName(c oas.InboxFeedbackCategory, accept, def string) string {
	for _, tag := range acceptedLanguages(accept) {
		if n, ok := c.Names[tag]; ok {
			return n
		}
		if primary, _, ok := strings.Cut(tag, "-"); ok {
			if n, ok := c.Names[primary]; ok {
				return n
			}
		}
	}
	for _, tag := range []string{def, "en"} {
		if n, ok := c.Names[tag]; ok {
			return n
		}
	}
	keys := make([]string, 0, len(c.Names))
	for k := range c.Names {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if len(keys) > 0 {
		return c.Names[keys[0]]
	}
	return c.Key
}

func acceptedLanguages(header string) []string {
	type weighted struct {
		tag string
		q   float64
	}
	var list []weighted
	for _, part := range strings.Split(header, ",") {
		tag, params, _ := strings.Cut(part, ";")
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" || tag == "*" {
			continue
		}
		w := 1.0
		if _, v, ok := strings.Cut(strings.ReplaceAll(params, " ", ""), "q="); ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				w = f
			}
		}
		if w > 0 {
			list = append(list, weighted{tag, w})
		}
	}
	sort.SliceStable(list, func(a, b int) bool { return list[a].q > list[b].q })
	out := make([]string, len(list))
	for i, x := range list {
		out[i] = x.tag
	}
	return out
}

func clientCategories(list []oas.InboxFeedbackCategory, accept, def string) ([]oas.FeedbackCategory, []oas.ClientFeedbackCategory) {
	keys := []oas.FeedbackCategory{}
	items := make([]oas.ClientFeedbackCategory, len(list))
	for i, c := range list {
		if slices.Contains(legacyFeedbackCategories, c.Key) {
			keys = append(keys, c.Key)
		}
		items[i] = oas.ClientFeedbackCategory{Key: c.Key, Name: categoryName(c, accept, def), Icon: c.Icon}
	}
	return keys, items
}

func (s *Server) setFeedbackCategories(ctx context.Context, q *store.Queries, in store.Inbox, list []oas.InboxFeedbackCategory) (store.Inbox, error) {
	if err := validCategories(list); err != nil {
		return in, err
	}
	in, err := q.SetInboxFeedbackCategories(ctx, store.SetInboxFeedbackCategoriesParams{
		WorkspaceID: in.WorkspaceID, ID: in.ID, FeedbackCategories: mustJSON(list), Now: s.now(),
	})
	if err != nil {
		return in, err
	}
	_, err = q.MoveFeedbackToOther(ctx, store.MoveFeedbackToOtherParams{WorkspaceID: in.WorkspaceID, InboxID: in.ID, Keys: categoryKeys(list)})
	return in, err
}

var legacyFeedbackCategories = []string{"bug", "idea", "praise", "other"}

func legacyCategory(key string) string {
	if slices.Contains(legacyFeedbackCategories, key) {
		return key
	}
	return feedbackFallbackCategory
}

// clientFeedback is the feedback as /client/v1 shows it: shipped SDKs decode category as one of the
// four built-in keys, so a custom key reads as other there and the real one is in category_key.
func clientFeedback(ctx context.Context, in store.Inbox, f *oas.Feedback) *oas.Feedback {
	if f == nil {
		return nil
	}
	out := *f
	var accept string
	if r := requestFrom(ctx); r != nil {
		accept = r.Header.Get("Accept-Language")
	}
	key := f.Category
	out.CategoryKey, out.Category = &key, legacyCategory(key)
	for _, c := range inboxCategories(in) {
		if c.Key == key {
			name := categoryName(c, accept, in.DefaultLocale)
			out.CategoryName = &name
			break
		}
	}
	return &out
}

func (s *Server) requireInboxCategory(ctx context.Context, q *store.Queries, ws, inboxID uuid.UUID, key string) error {
	raw, err := q.ShareInboxFeedbackCategories(ctx, store.ShareInboxFeedbackCategoriesParams{WorkspaceID: ws, ID: inboxID})
	if store.IsNotFound(err) {
		return errInboxGone
	}
	if err != nil {
		return err
	}
	var list []oas.InboxFeedbackCategory
	_ = json.Unmarshal(raw, &list)
	if !slices.Contains(categoryKeys(list), key) {
		return errUnknownFeedbackCategory
	}
	return nil
}

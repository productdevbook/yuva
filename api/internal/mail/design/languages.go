package design

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/language"
)

const MaxTranslations = 20

// Translation is one version of a document in another language: block texts by block id, its
// preheader and optionally its writing direction. Everything else is the base document's.
type Translation struct {
	Preheader string            `json:"preheader,omitempty"`
	Direction string            `json:"direction,omitempty"`
	Blocks    map[string]string `json:"blocks"`
}

var tagPattern = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// CanonicalTag returns a BCP 47 tag in canonical case (es-MX, zh-Hant), without replacing
// deprecated subtags; false when it is not a tag.
func CanonicalTag(s string) (string, bool) {
	if len(s) > 35 || !tagPattern.MatchString(s) {
		return "", false
	}
	t, err := language.Raw.Parse(s)
	if err != nil {
		return "", false
	}
	return t.String(), true
}

// FooterLang is the language of the required footer for a version: en, tr or de, and false when
// the server has no footer words in the version's language.
func FooterLang(tag string) (string, bool) {
	primary, _, _ := strings.Cut(strings.ToLower(tag), "-")
	if _, ok := footerTexts[primary]; ok {
		return primary, true
	}
	return "en", false
}

func parseLanguages(top map[string]json.RawMessage, d *Document, issues []Issue) []Issue {
	tag := func(field string) string {
		raw, ok := top[field]
		if !ok {
			return ""
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			issues = append(issues, Issue{Field: field, Code: "invalid"})
			return ""
		}
		c, ok := CanonicalTag(s)
		if !ok {
			issues = append(issues, Issue{Field: field, Code: "invalid_locale", Detail: "a language tag such as en or es-MX"})
		}
		return c
	}
	d.Locale = tag("locale")
	d.FallbackLocale = tag("fallback_locale")
	raw, ok := top["translations"]
	if !ok {
		return issues
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil || entries == nil {
		return append(issues, Issue{Field: "translations", Code: "invalid", Detail: "an object by language tag"})
	}
	if len(entries) > MaxTranslations {
		return append(issues, Issue{Field: "translations", Code: "too_many", Detail: fmt.Sprintf("at most %d languages", MaxTranslations)})
	}
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	d.Translations = map[string]Translation{}
	for _, k := range keys {
		field := "translations." + k
		c, ok := CanonicalTag(k)
		if !ok {
			issues = append(issues, Issue{Field: field, Code: "invalid_locale", Detail: "a language tag such as en or es-MX"})
			continue
		}
		if _, dup := d.Translations[c]; dup {
			issues = append(issues, Issue{Field: field, Code: "invalid_locale", Detail: "the same language as translations." + c})
			continue
		}
		var t Translation
		if err := strict(entries[k], &t); err != nil {
			issues = append(issues, Issue{Field: field, Code: "invalid", Detail: err.Error()})
			continue
		}
		if t.Blocks == nil {
			issues = append(issues, Issue{Field: field + ".blocks", Code: "required"})
			t.Blocks = map[string]string{}
		}
		d.Translations[c] = t
	}
	return issues
}

func (c *checker) languages(d Document) {
	if d.Locale == "" && (len(d.Translations) > 0 || d.FallbackLocale != "") {
		c.add("", "locale", "locale_required", "the language of the document's own texts")
	}
	if len(d.Translations) > MaxTranslations {
		c.add("", "translations", "too_many", fmt.Sprintf("at most %d languages", MaxTranslations))
	}
	if d.FallbackLocale != "" && d.Locale != "" && !slices.Contains(d.Versions(), d.FallbackLocale) {
		c.add("", "fallback_locale", "invalid_locale", "locale or a key of translations")
	}
	blocks := map[string]Block{}
	d.Walk(func(b Block) { blocks[b.ID] = b })
	for _, tag := range d.translationTags() {
		t := d.Translations[tag]
		prefix := "translations." + tag
		if strings.EqualFold(tag, d.Locale) {
			c.add("", prefix, "invalid_locale", "the same language as locale")
		}
		if utf8.RuneCountInString(t.Preheader) > 300 {
			c.add("", prefix+".preheader", "too_long", "at most 300 characters")
		}
		c.plain("", prefix+".preheader", t.Preheader)
		if t.Direction != "" && !slices.Contains([]string{"ltr", "rtl", "auto"}, t.Direction) {
			c.add("", prefix+".direction", "invalid", "")
		}
		if len(t.Blocks) > MaxBlocks {
			c.add("", prefix+".blocks", "too_many", "")
		}
		ids := make([]string, 0, len(t.Blocks))
		for id := range t.Blocks {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		for _, id := range ids {
			s := t.Blocks[id]
			field := prefix + ".blocks." + id
			b, ok := blocks[id]
			if !ok || !hasText(b.Type) {
				c.add(id, field, "translation_block", "no heading, text, image, button or footer block has this id")
				continue
			}
			switch b.Type {
			case "heading":
				if s == "" {
					c.add(id, field, "required", "")
				} else {
					c.rich(id, field, s, true, 1000)
				}
			case "text":
				c.rich(id, field, s, false, 20000)
			case "image":
				if utf8.RuneCountInString(s) > 300 {
					c.add(id, field, "too_long", "at most 300 characters")
				}
			case "button":
				switch n := utf8.RuneCountInString(s); {
				case n == 0:
					c.add(id, field, "required", "")
				case n > 80:
					c.add(id, field, "too_long", "at most 80 characters")
				}
				c.plain(id, field, s)
			case "footer":
				c.rich(id, field, s, false, 2000)
			}
		}
	}
}

func hasText(typ string) bool {
	return slices.Contains([]string{"heading", "text", "image", "button", "footer"}, typ)
}

func baseText(b Block) string {
	switch b.Type {
	case "heading":
		return b.Text
	case "text", "footer":
		return b.HTML
	case "image":
		if b.Decorative {
			return ""
		}
		return b.Alt
	case "button":
		return b.Label
	}
	return ""
}

// Versioned is true when the document has translations; without them it is a single-language
// design and its mail carries no version.
func (d Document) Versioned() bool { return len(d.Translations) > 0 }

func (d Document) translationTags() []string {
	out := make([]string, 0, len(d.Translations))
	for k := range d.Translations {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Versions lists the base language, then the translations' sorted; none for a document without
// translations.
func (d Document) Versions() []string {
	if !d.Versioned() || d.Locale == "" {
		return nil
	}
	return append([]string{d.Locale}, d.translationTags()...)
}

// Fallback is the version for contacts without a language or a matching version.
func (d Document) Fallback() string {
	if d.FallbackLocale != "" {
		return d.FallbackLocale
	}
	return d.Locale
}

// Pick is the version a contact with this language gets and whether their language matched one;
// "" for a document without translations.
func (d Document) Pick(contact string) (string, bool) {
	if !d.Versioned() {
		return "", false
	}
	return Resolve(contact, d.Versions(), d.Fallback())
}

// Resolve matches a contact's language against versions: the tag exactly, then shorter tags by
// RFC 4647 lookup (es-MX, es), then the first version of the same language in another region,
// else fallback. Matched is false when fallback was used.
func Resolve(contact string, versions []string, fallback string) (string, bool) {
	want := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(contact), "_", "-"))
	if c, ok := CanonicalTag(want); ok {
		want = strings.ToLower(c)
	}
	if want == "" {
		return fallback, false
	}
	find := func(tag string) (string, bool) {
		for _, v := range versions {
			if strings.ToLower(v) == tag {
				return v, true
			}
		}
		return "", false
	}
	parts := strings.Split(want, "-")
	for n := len(parts); n >= 1; n-- {
		if n < len(parts) && len(parts[n-1]) == 1 {
			continue
		}
		if v, ok := find(strings.Join(parts[:n], "-")); ok {
			return v, true
		}
	}
	for _, v := range versions {
		if p, _, _ := strings.Cut(strings.ToLower(v), "-"); p == parts[0] {
			return v, true
		}
	}
	return fallback, false
}

// Localized is the document as a version reads: its texts, preheader and direction in place of
// the base's, without translations. The base, or a tag that is not a version, is the base.
func (d Document) Localized(tag string) Document {
	var out Document
	if err := json.Unmarshal(Marshal(d), &out); err != nil {
		panic(err)
	}
	out.Translations = nil
	out.FallbackLocale = ""
	t, ok := d.Translations[tag]
	if !ok {
		return out
	}
	out.Locale = tag
	if t.Preheader != "" {
		out.Preheader = t.Preheader
	}
	if t.Direction != "" {
		out.Theme.Direction = t.Direction
	}
	var walk func(bs []Block)
	walk = func(bs []Block) {
		for i := range bs {
			b := &bs[i]
			if s, ok := t.Blocks[b.ID]; ok {
				switch b.Type {
				case "heading":
					b.Text = s
				case "text", "footer":
					b.HTML = s
				case "image":
					b.Alt = s
				case "button":
					b.Label = s
				}
			}
			walk(b.Blocks)
			for j := range b.Columns {
				walk(b.Columns[j].Blocks)
			}
		}
	}
	walk(out.Blocks)
	return out
}

// Missing lists the blocks with a base text that have none in a translation, in document order.
func (d Document) Missing(tag string) []string {
	t, ok := d.Translations[tag]
	if !ok {
		return nil
	}
	var out []string
	d.Walk(func(b Block) {
		if baseText(b) == "" {
			return
		}
		if _, ok := t.Blocks[b.ID]; !ok {
			out = append(out, b.ID)
		}
	})
	return out
}

// VersionIssues are the warnings of a version beyond its render: translation_missing per block
// without a text and footer_language when its required footer is in English.
func (d Document) VersionIssues(tag string) []Issue {
	var out []Issue
	for _, id := range d.Missing(tag) {
		out = append(out, Issue{BlockID: id, Field: "translations." + tag + ".blocks." + id, Code: "translation_missing"})
	}
	if _, ok := FooterLang(tag); !ok && d.Versioned() {
		out = append(out, Issue{Code: "footer_language", Detail: tag})
	}
	return out
}

// Prune drops translation entries of blocks the document no longer has.
func (d *Document) Prune() {
	ids := map[string]bool{}
	d.Walk(func(b Block) {
		if hasText(b.Type) {
			ids[b.ID] = true
		}
	})
	for tag, t := range d.Translations {
		for id := range t.Blocks {
			if !ids[id] {
				delete(t.Blocks, id)
			}
		}
		d.Translations[tag] = t
	}
}

// TranslationOf takes the texts of other, a document with the same block ids in another
// language, as a translation of d.
func (d Document) TranslationOf(other Document) Translation {
	texts := map[string]string{}
	other.Walk(func(b Block) {
		if s := baseText(b); s != "" {
			texts[b.ID] = s
		}
	})
	t := Translation{Preheader: other.Preheader, Blocks: map[string]string{}}
	d.Walk(func(b Block) {
		if s, ok := texts[b.ID]; ok && hasText(b.Type) {
			t.Blocks[b.ID] = s
		}
	})
	return t
}

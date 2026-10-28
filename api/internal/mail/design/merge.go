package design

import (
	"net/url"
	"regexp"
	"strings"
)

const maxFallback = 100

var (
	fieldName     = regexp.MustCompile(`^(contact\.(name|first_name|email|attributes\.[A-Za-z0-9_.-]{1,64})|inbox\.name|list\.name|unsubscribe_url|preferences_url)$`)
	mergeFallback = regexp.MustCompile(`^"([^"]*)"$`)
)

// Field is one {{ name }} or {{ name | "fallback" }} in a text.
type Field struct {
	Name        string
	Fallback    string
	HasFallback bool
}

// segment is literal text or a merge field.
type segment struct {
	text  string
	field *Field
}

// parseMerge splits s into text and merge fields. A "{{" without a closing "}}" is text; a closed
// one that is not a known field is returned in bad.
func parseMerge(s string) (segs []segment, bad []string) {
	for {
		i := strings.Index(s, "{{")
		if i < 0 {
			break
		}
		j := strings.Index(s[i+2:], "}}")
		if j < 0 {
			break
		}
		inner := s[i+2 : i+2+j]
		if i > 0 {
			segs = append(segs, segment{text: s[:i]})
		}
		if f, ok := parseField(inner); ok {
			segs = append(segs, segment{field: &f})
		} else {
			bad = append(bad, strings.TrimSpace(inner))
			segs = append(segs, segment{text: s[i : i+4+j]})
		}
		s = s[i+4+j:]
	}
	if s != "" {
		segs = append(segs, segment{text: s})
	}
	return segs, bad
}

func parseField(inner string) (Field, bool) {
	name, fb, hasFB := strings.Cut(inner, "|")
	f := Field{Name: strings.TrimSpace(name)}
	if !fieldName.MatchString(f.Name) {
		return f, false
	}
	if hasFB {
		m := mergeFallback.FindStringSubmatch(strings.TrimSpace(fb))
		if m == nil || len([]rune(m[1])) > maxFallback {
			return f, false
		}
		f.Fallback, f.HasFallback = m[1], true
	}
	return f, true
}

// Fields lists the merge fields in s.
func Fields(s string) []Field {
	segs, _ := parseMerge(s)
	var out []Field
	for _, sg := range segs {
		if sg.field != nil {
			out = append(out, *sg.field)
		}
	}
	return out
}

// Values are what merge fields are replaced with for one recipient.
type Values struct {
	ContactName    string
	ContactEmail   string
	Attributes     map[string]string
	InboxName      string
	ListName       string
	UnsubscribeURL string
	PreferencesURL string
}

func firstName(name string) string {
	f, _, _ := strings.Cut(strings.TrimSpace(name), " ")
	return f
}

func (v Values) value(f Field) string {
	var s string
	switch {
	case f.Name == "contact.name":
		s = v.ContactName
	case f.Name == "contact.first_name":
		s = firstName(v.ContactName)
	case f.Name == "contact.email":
		s = v.ContactEmail
	case strings.HasPrefix(f.Name, "contact.attributes."):
		s = v.Attributes[strings.TrimPrefix(f.Name, "contact.attributes.")]
	case f.Name == "inbox.name":
		s = v.InboxName
	case f.Name == "list.name":
		s = v.ListName
	case f.Name == "unsubscribe_url":
		s = v.UnsubscribeURL
	case f.Name == "preferences_url":
		s = v.PreferencesURL
	}
	if strings.TrimSpace(s) == "" {
		return f.Fallback
	}
	return s
}

// Text replaces the merge fields of s with plain values.
func (v Values) Text(s string) string {
	segs, _ := parseMerge(s)
	var b strings.Builder
	for _, sg := range segs {
		if sg.field != nil {
			b.WriteString(v.value(*sg.field))
		} else {
			b.WriteString(sg.text)
		}
	}
	return b.String()
}

// URL replaces the merge fields of a URL: a URL that is only {{ unsubscribe_url }} or
// {{ preferences_url }} becomes that link, others are query-escaped.
func (v Values) URL(s string) string {
	segs, _ := parseMerge(s)
	if len(segs) == 1 && segs[0].field != nil && isLinkField(segs[0].field.Name) {
		return v.value(*segs[0].field)
	}
	var b strings.Builder
	for _, sg := range segs {
		if sg.field != nil {
			b.WriteString(url.QueryEscape(v.value(*sg.field)))
		} else {
			b.WriteString(sg.text)
		}
	}
	return b.String()
}

func isLinkField(name string) bool { return name == "unsubscribe_url" || name == "preferences_url" }

// Mail sent in a conversation has no list and no unsubscribe or preference page.
func ConversationFields(name string) bool { return name != "list.name" && !isLinkField(name) }

func (d Document) FieldIssues(v Values, allowed func(name string) bool) []Issue {
	var out []Issue
	seen := map[string]bool{}
	check := func(id string, fields []Field) {
		for _, f := range fields {
			code := ""
			switch {
			case !allowed(f.Name):
				code = "merge_field_unavailable"
			case !f.HasFallback && strings.TrimSpace(v.value(f)) == "":
				code = "merge_field_empty"
			default:
				continue
			}
			if k := id + "\x00" + code + "\x00" + f.Name; !seen[k] {
				seen[k] = true
				out = append(out, Issue{BlockID: id, Code: code, Detail: f.Name})
			}
		}
	}
	check("", Fields(d.Preheader))
	d.Walk(func(b Block) {
		switch b.Type {
		case "heading":
			f, _ := richFields(b.Text)
			check(b.ID, f)
		case "text", "footer":
			f, _ := richFields(b.HTML)
			check(b.ID, f)
		case "button":
			check(b.ID, append(Fields(b.Label), Fields(b.URL)...))
		case "image":
			check(b.ID, Fields(b.Link))
		}
	})
	return out
}

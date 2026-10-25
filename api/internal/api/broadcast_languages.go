package api

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/productdevbook/yuva/api/internal/mail/design"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

type bcTranslation struct {
	Subject   string `json:"subject,omitempty"`
	Preheader string `json:"preheader,omitempty"`
}

func parseTranslations(raw []byte) map[string]bcTranslation {
	out := map[string]bcTranslation{}
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = map[string]bcTranslation{}
	}
	return out
}

func translationsJSON(m map[string]bcTranslation) []byte {
	if m == nil {
		m = map[string]bcTranslation{}
	}
	return mustJSON(m)
}

func checkTranslations(in map[string]oas.BroadcastTranslation) (map[string]bcTranslation, error) {
	out := map[string]bcTranslation{}
	if len(in) > design.MaxTranslations {
		return out, errValidation(fmt.Sprintf("translations names at most %d languages", design.MaxTranslations))
	}
	tags := map[string]string{}
	for _, k := range slices.Sorted(maps.Keys(in)) {
		tag, err := languageTag(k, "each key of translations")
		if err != nil {
			return out, err
		}
		if _, dup := tags[tag]; dup {
			return out, errValidation("translations names " + tag + " twice")
		}
		tags[tag] = k
	}
	for tag, k := range tags {
		t := in[k]
		var err error
		var v bcTranslation
		if t.Subject != nil {
			if v.Subject, err = trimmed(*t.Subject, 0, 300, "translations."+tag+".subject"); err != nil {
				return out, err
			}
		}
		if t.Preheader != nil {
			if v.Preheader, err = trimmed(*t.Preheader, 0, 300, "translations."+tag+".preheader"); err != nil {
				return out, err
			}
		}
		if v != (bcTranslation{}) {
			out[tag] = v
		}
	}
	return out, nil
}

func translationsBody(raw []byte) map[string]oas.BroadcastTranslation {
	out := map[string]oas.BroadcastTranslation{}
	for tag, t := range parseTranslations(raw) {
		var v oas.BroadcastTranslation
		if t.Subject != "" {
			v.Subject = new(t.Subject)
		}
		if t.Preheader != "" {
			v.Preheader = new(t.Preheader)
		}
		out[tag] = v
	}
	return out
}

func broadcastSubject(b store.Broadcast, tag string) string {
	if t := parseTranslations(b.Translations)[tag]; t.Subject != "" {
		return t.Subject
	}
	return b.Subject
}

func broadcastPreheader(b store.Broadcast, doc design.Document, tag string) string {
	if t := parseTranslations(b.Translations)[tag]; t.Preheader != "" {
		return t.Preheader
	}
	if t, ok := doc.Translations[tag]; ok && t.Preheader != "" {
		return t.Preheader
	}
	if b.Preheader != "" {
		return b.Preheader
	}
	return doc.Preheader
}

func frozenVersions(b store.Broadcast) []string {
	if b.Document == nil {
		return nil
	}
	d, issues, err := design.Parse(b.Document)
	if err != nil || len(issues) > 0 {
		return nil
	}
	return d.Versions()
}

func (l *reviewList) translations(b store.Broadcast, doc *design.Document) {
	trans := parseTranslations(b.Translations)
	var versions []string
	if doc != nil {
		versions = doc.Versions()
	}
	var parts []string
	if len(versions) > 1 {
		var noSubject, english []string
		var missing []string
		for _, tag := range versions[1:] {
			if trans[tag].Subject == "" {
				noSubject = append(noSubject, tag)
			}
			if n := len(doc.Missing(tag)); n > 0 {
				missing = append(missing, fmt.Sprintf("%s %d", tag, n))
			}
		}
		for _, tag := range versions {
			if _, ok := design.FooterLang(tag); !ok {
				english = append(english, tag)
			}
		}
		if len(noSubject) > 0 {
			parts = append(parts, "no subject of their own, they get the "+versions[0]+" subject: "+strings.Join(noSubject, ", "))
		}
		if len(missing) > 0 {
			parts = append(parts, "texts not translated, the "+versions[0]+" text shows: "+strings.Join(missing, ", "))
		}
		if len(english) > 0 {
			parts = append(parts, "the required footer is in English: "+strings.Join(english, ", "))
		}
	}
	var unused []string
	for tag := range trans {
		if !slices.Contains(versions, tag) {
			unused = append(unused, tag)
		}
	}
	slices.Sort(unused)
	if len(unused) > 0 {
		parts = append(parts, "subjects for languages the design does not have are not used: "+strings.Join(unused, ", "))
	}
	if len(parts) == 0 {
		l.add(oas.BroadcastReviewKeyTranslations, oas.ReviewOk, "")
		return
	}
	l.add(oas.BroadcastReviewKeyTranslations, oas.ReviewWarning, strings.Join(parts, "; "))
}

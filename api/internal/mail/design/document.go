// Package design reads, checks and renders e-mail design documents (EmailDesignDocument in the
// contract) to table HTML with inline styles and a plain-text part. It does no I/O: images and
// links come from the caller.
package design

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"unicode/utf8"
	"uuid"
)

// Schema is the newest document format this server knows.
const Schema = 1

const (
	MaxJSONBytes     = 300 << 10
	MaxDocumentBytes = 1 << 20
	MaxBlocks        = 200
)

var (
	ErrTooLarge          = errors.New("design: document over 300 KiB (1 MiB with translations) or 200 blocks")
	ErrUnsupportedSchema = errors.New("design: document schema is newer than this server")
)

// upgrades[n] turns a schema n document into schema n+1.
var upgrades = map[int]func(map[string]json.RawMessage) error{}

type Colors struct {
	Page   string `json:"page,omitempty"`
	Card   string `json:"card,omitempty"`
	Text   string `json:"text,omitempty"`
	Muted  string `json:"muted,omitempty"`
	Accent string `json:"accent,omitempty"`
	Link   string `json:"link,omitempty"`
	Border string `json:"border,omitempty"`
}

type Theme struct {
	Width       int     `json:"width"`
	Direction   string  `json:"direction,omitempty"`
	Font        string  `json:"font"`
	HeadingFont string  `json:"heading_font,omitempty"`
	Radius      int     `json:"radius,omitempty"`
	Colors      Colors  `json:"colors"`
	Dark        *Colors `json:"dark,omitempty"`
}

type Column struct {
	Width  int     `json:"width"`
	Blocks []Block `json:"blocks"`
}

type SocialItem struct {
	Network string `json:"network"`
	URL     string `json:"url"`
}

// Block holds the fields of every block type; Type says which apply.
type Block struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Padding    []int  `json:"padding,omitempty"`
	Background string `json:"background,omitempty"`
	Align      string `json:"align,omitempty"`
	Hide       string `json:"hide,omitempty"`

	Blocks       []Block  `json:"blocks,omitempty"`
	BorderRadius *int     `json:"border_radius,omitempty"`
	Columns      []Column `json:"columns,omitempty"`
	Stack        *bool    `json:"stack,omitempty"`
	Gap          *int     `json:"gap,omitempty"`

	Level int    `json:"level,omitempty"`
	Text  string `json:"text,omitempty"`
	Color string `json:"color,omitempty"`
	HTML  string `json:"html,omitempty"`
	Size  int    `json:"size,omitempty"`

	ImageID    string `json:"image_id,omitempty"`
	Asset      string `json:"asset,omitempty"`
	Alt        string `json:"alt,omitempty"`
	Decorative bool   `json:"decorative,omitempty"`
	Width      int    `json:"width,omitempty"`
	Link       string `json:"link,omitempty"`
	Plate      bool   `json:"plate,omitempty"`

	Label     string `json:"label,omitempty"`
	URL       string `json:"url,omitempty"`
	TextColor string `json:"text_color,omitempty"`
	FullWidth bool   `json:"full_width,omitempty"`

	Thickness int `json:"thickness,omitempty"`
	Height    int `json:"height,omitempty"`

	Items []SocialItem `json:"items,omitempty"`
	Style string       `json:"style,omitempty"`
}

type Document struct {
	Schema         int                    `json:"schema"`
	Theme          Theme                  `json:"theme"`
	Preheader      string                 `json:"preheader,omitempty"`
	Locale         string                 `json:"locale,omitempty"`
	FallbackLocale string                 `json:"fallback_locale,omitempty"`
	Translations   map[string]Translation `json:"translations,omitempty"`
	Blocks         []Block                `json:"blocks"`
}

// Issue is a validation error or a render warning.
type Issue struct {
	BlockID string `json:"block_id,omitempty"`
	Field   string `json:"field,omitempty"`
	Code    string `json:"code"`
	Detail  string `json:"detail,omitempty"`
}

var common = []string{"id", "type", "padding", "background", "align", "hide"}

var blockFields = map[string][]string{
	"section": {"blocks", "border_radius"},
	"columns": {"columns", "stack", "gap"},
	"heading": {"level", "text", "color"},
	"text":    {"html", "size", "color"},
	"image":   {"image_id", "asset", "alt", "decorative", "width", "link", "plate"},
	"button":  {"label", "url", "color", "text_color", "full_width"},
	"divider": {"color", "thickness", "width"},
	"spacer":  {"height"},
	"social":  {"items", "style", "color"},
	"footer":  {"html"},
}

// Parse reads a document, upgrading an older schema, and checks its shape: unknown fields and
// wrong types are issues, a newer schema is ErrUnsupportedSchema, size limits ErrTooLarge.
// Content rules are Validate's.
func Parse(raw []byte) (Document, []Issue, error) {
	var d Document
	if len(raw) > MaxDocumentBytes {
		return d, nil, ErrTooLarge
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return d, []Issue{{Code: "invalid", Detail: "the document is not a JSON object"}}, nil
	}
	if len(raw)-len(top["translations"]) > MaxJSONBytes {
		return d, nil, ErrTooLarge
	}
	var schema int
	if err := json.Unmarshal(top["schema"], &schema); err != nil || schema < 1 {
		return d, []Issue{{Field: "schema", Code: "required"}}, nil
	}
	if schema > Schema {
		return d, nil, ErrUnsupportedSchema
	}
	for ; schema < Schema; schema++ {
		if err := upgrades[schema](top); err != nil {
			return d, nil, err
		}
	}
	top["schema"] = json.RawMessage(strconv.Itoa(Schema))
	var issues []Issue
	for k := range top {
		if !slices.Contains([]string{"schema", "theme", "preheader", "locale", "fallback_locale", "translations", "blocks"}, k) {
			issues = append(issues, Issue{Field: k, Code: "invalid", Detail: "unknown field"})
		}
	}
	if t, ok := top["theme"]; ok {
		if err := strict(t, &d.Theme); err != nil {
			issues = append(issues, Issue{Field: "theme", Code: "invalid", Detail: err.Error()})
		}
	} else {
		issues = append(issues, Issue{Field: "theme", Code: "required"})
	}
	if p, ok := top["preheader"]; ok {
		if err := json.Unmarshal(p, &d.Preheader); err != nil {
			issues = append(issues, Issue{Field: "preheader", Code: "invalid"})
		}
	}
	issues = parseLanguages(top, &d, issues)
	d.Schema = Schema
	var blocks []json.RawMessage
	if err := json.Unmarshal(top["blocks"], &blocks); err != nil || top["blocks"] == nil {
		issues = append(issues, Issue{Field: "blocks", Code: "required"})
		return d, issues, nil
	}
	count := 0
	d.Blocks, issues = parseBlocks(blocks, issues, &count)
	if count > MaxBlocks {
		return d, nil, ErrTooLarge
	}
	return d, issues, nil
}

func strict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func parseBlocks(raws []json.RawMessage, issues []Issue, count *int) ([]Block, []Issue) {
	out := make([]Block, 0, len(raws))
	for i, raw := range raws {
		*count++
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			issues = append(issues, Issue{Field: "blocks." + strconv.Itoa(i), Code: "invalid", Detail: "not an object"})
			continue
		}
		var head struct{ ID, Type string }
		_ = json.Unmarshal(raw, &head)
		allowed, ok := blockFields[head.Type]
		if !ok {
			issues = append(issues, Issue{BlockID: head.ID, Field: "type", Code: "invalid", Detail: fmt.Sprintf("unknown block type %q", head.Type)})
			continue
		}
		bad := false
		for k := range fields {
			if !slices.Contains(common, k) && !slices.Contains(allowed, k) {
				issues = append(issues, Issue{BlockID: head.ID, Field: k, Code: "invalid", Detail: "not a field of " + head.Type + " blocks"})
				bad = true
			}
		}
		flat := maps.Clone(fields)
		delete(flat, "blocks")
		delete(flat, "columns")
		var b Block
		if err := json.Unmarshal(mustJSON(flat), &b); err != nil {
			issues = append(issues, Issue{BlockID: head.ID, Code: "invalid", Detail: err.Error()})
			continue
		}
		if bad {
			continue
		}
		switch head.Type {
		case "section":
			var inner []json.RawMessage
			if err := json.Unmarshal(fields["blocks"], &inner); err != nil {
				issues = append(issues, Issue{BlockID: b.ID, Field: "blocks", Code: "required"})
			}
			b.Blocks, issues = parseBlocks(inner, issues, count)
			if b.Blocks == nil {
				b.Blocks = []Block{}
			}
		case "columns":
			var cols []struct {
				Width  int               `json:"width"`
				Blocks []json.RawMessage `json:"blocks"`
			}
			if err := json.Unmarshal(fields["columns"], &cols); err != nil {
				issues = append(issues, Issue{BlockID: b.ID, Field: "columns", Code: "required"})
			}
			for _, c := range cols {
				col := Column{Width: c.Width}
				col.Blocks, issues = parseBlocks(c.Blocks, issues, count)
				if col.Blocks == nil {
					col.Blocks = []Block{}
				}
				b.Columns = append(b.Columns, col)
			}
		}
		out = append(out, b)
	}
	return out, issues
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// Marshal writes the stored form of a document.
func Marshal(d Document) []byte {
	b, err := json.Marshal(d)
	if err != nil {
		panic(err)
	}
	return b
}

var (
	hexColor  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	blockID   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
	assetName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*\.(png|jpg|gif)$`)
	fonts     = []string{"system", "humanist", "geometric", "serif", "mono"}
	networks  = []string{"website", "x", "bluesky", "mastodon", "linkedin", "github", "youtube", "instagram", "facebook", "tiktok", "discord", "threads"}
)

// Options says what a document may hold besides the usual.
type Options struct {
	// Assets allows image blocks to name a template's files (gallery templates only).
	Assets bool
}

type checker struct {
	opt    Options
	issues []Issue
	ids    map[string]bool
}

func (c *checker) add(block, field, code, detail string) {
	c.issues = append(c.issues, Issue{BlockID: block, Field: field, Code: code, Detail: detail})
}

// Validate checks a parsed document's content: ranges, colours, rich text, links, merge fields,
// nesting and ids.
func Validate(d Document, opt Options) []Issue {
	c := &checker{opt: opt, ids: map[string]bool{}}
	t := d.Theme
	if t.Width < 480 || t.Width > 720 {
		c.add("", "theme.width", "invalid", "480 to 720")
	}
	if t.Direction != "" && !slices.Contains([]string{"ltr", "rtl", "auto"}, t.Direction) {
		c.add("", "theme.direction", "invalid", "")
	}
	if !slices.Contains(fonts, t.Font) {
		c.add("", "theme.font", "invalid", "")
	}
	if t.HeadingFont != "" && !slices.Contains(fonts, t.HeadingFont) {
		c.add("", "theme.heading_font", "invalid", "")
	}
	if t.Radius < 0 || t.Radius > 24 {
		c.add("", "theme.radius", "invalid", "0 to 24")
	}
	c.colors("theme.colors", t.Colors, true)
	if t.Dark != nil {
		c.colors("theme.dark", *t.Dark, false)
	}
	if utf8.RuneCountInString(d.Preheader) > 300 {
		c.add("", "preheader", "too_long", "at most 300 characters")
	}
	c.plain("", "preheader", d.Preheader)
	if len(d.Blocks) > MaxBlocks {
		c.add("", "blocks", "too_many", "")
	}
	for _, b := range d.Blocks {
		c.block(b, 0)
	}
	c.languages(d)
	return c.issues
}

func (c *checker) colors(prefix string, cs Colors, required bool) {
	for _, f := range []struct{ name, v string }{
		{"page", cs.Page}, {"card", cs.Card}, {"text", cs.Text}, {"muted", cs.Muted},
		{"accent", cs.Accent}, {"link", cs.Link}, {"border", cs.Border},
	} {
		switch {
		case f.v == "" && required:
			c.add("", prefix+"."+f.name, "required", "")
		case f.v != "" && !hexColor.MatchString(f.v):
			c.add("", prefix+"."+f.name, "invalid", "a #rrggbb colour")
		}
	}
}

func (c *checker) color(id, field, v string) {
	if v != "" && !hexColor.MatchString(v) {
		c.add(id, field, "invalid", "a #rrggbb colour")
	}
}

func (c *checker) between(id, field string, v, lo, hi int) {
	if v < lo || v > hi {
		c.add(id, field, "invalid", fmt.Sprintf("%d to %d", lo, hi))
	}
}

// plain checks the merge fields of plain text.
func (c *checker) plain(id, field, s string) {
	_, bad := parseMerge(s)
	for _, b := range bad {
		c.add(id, field, "unknown_merge_field", b)
	}
}

func (c *checker) url(id, field, s string, required bool) {
	if s == "" {
		if required {
			c.add(id, field, "required", "")
		}
		return
	}
	if code := checkURL(s); code != "" {
		c.add(id, field, code, "")
	}
}

func (c *checker) rich(id, field, s string, inline bool, max int) {
	if utf8.RuneCountInString(s) > max {
		c.add(id, field, "too_long", fmt.Sprintf("at most %d characters", max))
		return
	}
	ok, badURL := checkRich(s, inline)
	if !ok {
		c.add(id, field, "invalid_rich_text", "only p, br, strong, em, u, s, a href, ul, ol, li and code, without other attributes")
		return
	}
	if badURL != "" {
		c.add(id, field, checkURL(badURL), badURL)
	}
	_, bad := richFields(s)
	for _, b := range bad {
		c.add(id, field, "unknown_merge_field", b)
	}
}

// depth: 0 top level, 1 inside a section, 2 inside columns.
func (c *checker) block(b Block, depth int) {
	id := b.ID
	if !blockID.MatchString(id) {
		c.add(id, "id", "invalid", "1 to 32 letters, digits, - and _")
	} else if c.ids[id] {
		c.add(id, "id", "duplicate_id", "")
	}
	c.ids[id] = true
	if b.Padding != nil {
		if len(b.Padding) != 4 {
			c.add(id, "padding", "invalid", "four numbers")
		}
		for _, p := range b.Padding {
			if p < 0 || p > 64 {
				c.add(id, "padding", "invalid", "0 to 64")
				break
			}
		}
	}
	c.color(id, "background", b.Background)
	if b.Align != "" && !slices.Contains([]string{"left", "center", "right"}, b.Align) {
		c.add(id, "align", "invalid", "")
	}
	if b.Hide != "" && !slices.Contains([]string{"none", "mobile", "desktop"}, b.Hide) {
		c.add(id, "hide", "invalid", "")
	}
	switch b.Type {
	case "section":
		if depth > 0 {
			c.add(id, "type", "nesting", "sections do not nest")
		}
		if b.BorderRadius != nil {
			c.between(id, "border_radius", *b.BorderRadius, 0, 24)
		}
		if len(b.Blocks) > 100 {
			c.add(id, "blocks", "too_many", "at most 100")
		}
		for _, in := range b.Blocks {
			c.block(in, 1)
		}
	case "columns":
		if depth > 1 {
			c.add(id, "type", "nesting", "columns do not nest")
		}
		if len(b.Columns) < 2 || len(b.Columns) > 3 {
			c.add(id, "columns", "invalid", "2 or 3 columns")
		}
		if b.Gap != nil {
			c.between(id, "gap", *b.Gap, 0, 32)
		}
		sum := 0
		for i, col := range b.Columns {
			c.between(id, "columns."+strconv.Itoa(i)+".width", col.Width, 10, 90)
			sum += col.Width
			if len(col.Blocks) > 50 {
				c.add(id, "columns."+strconv.Itoa(i)+".blocks", "too_many", "at most 50")
			}
			for _, in := range col.Blocks {
				c.block(in, 2)
			}
		}
		if sum != 100 && len(b.Columns) >= 2 {
			c.add(id, "columns", "column_widths", "the widths add up to 100")
		}
	case "heading":
		if !slices.Contains([]int{1, 2, 3}, b.Level) {
			c.add(id, "level", "invalid", "1, 2 or 3")
		}
		if b.Text == "" {
			c.add(id, "text", "required", "")
		} else {
			c.rich(id, "text", b.Text, true, 1000)
		}
		c.color(id, "color", b.Color)
	case "text":
		c.rich(id, "html", b.HTML, false, 20000)
		if b.Size != 0 {
			c.between(id, "size", b.Size, 12, 22)
		}
		c.color(id, "color", b.Color)
	case "image":
		switch {
		case b.Asset != "" && !c.opt.Assets:
			c.add(id, "asset", "asset_not_allowed", "designs use image_id")
		case b.Asset != "" && !assetName.MatchString(b.Asset):
			c.add(id, "asset", "invalid", "")
		case b.Asset != "" && b.ImageID != "":
			c.add(id, "asset", "invalid", "image_id or asset, not both")
		}
		if b.ImageID != "" {
			if _, err := uuid.Parse(b.ImageID); err != nil {
				c.add(id, "image_id", "invalid", "")
			}
		}
		if utf8.RuneCountInString(b.Alt) > 300 {
			c.add(id, "alt", "too_long", "at most 300 characters")
		}
		if b.Width != 0 {
			c.between(id, "width", b.Width, 16, 720)
		}
		c.url(id, "link", b.Link, false)
	case "button":
		switch n := utf8.RuneCountInString(b.Label); {
		case n == 0:
			c.add(id, "label", "required", "")
		case n > 80:
			c.add(id, "label", "too_long", "at most 80 characters")
		}
		c.plain(id, "label", b.Label)
		c.url(id, "url", b.URL, true)
		c.color(id, "color", b.Color)
		c.color(id, "text_color", b.TextColor)
	case "divider":
		c.color(id, "color", b.Color)
		if b.Thickness != 0 {
			c.between(id, "thickness", b.Thickness, 1, 4)
		}
		if b.Width != 0 {
			c.between(id, "width", b.Width, 10, 100)
		}
	case "spacer":
		c.between(id, "height", b.Height, 4, 96)
	case "social":
		if len(b.Items) < 1 || len(b.Items) > 10 {
			c.add(id, "items", "invalid", "1 to 10 links")
		}
		for i, it := range b.Items {
			if !slices.Contains(networks, it.Network) {
				c.add(id, "items."+strconv.Itoa(i)+".network", "invalid", "")
			}
			c.url(id, "items."+strconv.Itoa(i)+".url", it.URL, true)
		}
		if b.Style != "" && b.Style != "icons" && b.Style != "text" {
			c.add(id, "style", "invalid", "")
		}
		if b.Color != "" && !slices.Contains([]string{"dark", "light", "brand"}, b.Color) {
			c.add(id, "color", "invalid", "")
		}
	case "footer":
		c.rich(id, "html", b.HTML, false, 2000)
	}
}

// Walk calls f for every block, nested ones included, in document order.
func (d Document) Walk(f func(b Block)) {
	var walk func(bs []Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			f(b)
			walk(b.Blocks)
			for _, col := range b.Columns {
				walk(col.Blocks)
			}
		}
	}
	walk(d.Blocks)
}

// ImageIDs lists the uploaded images a document uses, once each.
func (d Document) ImageIDs() []string {
	var out []string
	d.Walk(func(b Block) {
		if b.Type == "image" && b.ImageID != "" && !slices.Contains(out, b.ImageID) {
			out = append(out, b.ImageID)
		}
	})
	return out
}

// MapImages rewrites image blocks; templates use it to swap assets for uploaded images.
func (d *Document) MapImages(f func(b *Block)) {
	var walk func(bs []Block)
	walk = func(bs []Block) {
		for i := range bs {
			if bs[i].Type == "image" {
				f(&bs[i])
			}
			walk(bs[i].Blocks)
			for j := range bs[i].Columns {
				walk(bs[i].Columns[j].Blocks)
			}
		}
	}
	walk(d.Blocks)
}

// MergeFields lists the merge fields of the preheader, headings, rich text, button labels and
// URLs and image links, in document order, then those of each translation.
func (d Document) MergeFields() []Field {
	out := d.baseMergeFields()
	for _, tag := range d.translationTags() {
		out = append(out, d.Localized(tag).baseMergeFields()...)
	}
	return out
}

func (d Document) baseMergeFields() []Field {
	out := Fields(d.Preheader)
	d.Walk(func(b Block) {
		switch b.Type {
		case "heading":
			f, _ := richFields(b.Text)
			out = append(out, f...)
		case "text", "footer":
			f, _ := richFields(b.HTML)
			out = append(out, f...)
		case "button":
			out = append(append(out, Fields(b.Label)...), Fields(b.URL)...)
		case "image":
			out = append(out, Fields(b.Link)...)
		}
	})
	return out
}

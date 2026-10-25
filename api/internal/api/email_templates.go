package api

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/gallery"
	"github.com/productdevbook/yuva/api/internal/mail/design"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const emailTemplatesPathPrefix = "/email-templates/"

var (
	errTemplateGone = problem(http.StatusNotFound, "not_found", "no such template; it may need a newer Yuva")
	previewFile     = regexp.MustCompile(`^(en|tr|de)\.(html|webp)$`)
)

func templateLocale(l *oas.EmailTemplateLocale) string {
	if l == nil {
		return "en"
	}
	return string(*l)
}

func (s *Server) templatePreviewURL(slug, locale, ext string) string {
	return s.auth.PublicURL + emailTemplatesPathPrefix + slug + "/" + locale + "." + ext + "?v=" + s.assetVersion()
}

func (s *Server) templateBody(t gallery.Template, locale string) oas.EmailTemplate {
	shown := t.DocumentLocale(locale)
	locales := make([]oas.EmailTemplateLocale, len(t.Locales))
	for i, l := range t.Locales {
		locales[i] = oas.EmailTemplateLocale(l)
	}
	out := oas.EmailTemplate{
		Slug: t.Slug, Category: t.Category, Name: gallery.Text(t.Name, locale), Description: gallery.Text(t.Description, locale),
		Locales: locales, PreviewUrl: s.templatePreviewURL(t.Slug, shown, "webp"), PreviewHtmlUrl: s.templatePreviewURL(t.Slug, shown, "html"),
	}
	_ = out.Added.UnmarshalText([]byte(t.Added))
	return out
}

func (s *Server) ListEmailTemplates(ctx context.Context, req oas.ListEmailTemplatesRequestObject) (oas.ListEmailTemplatesResponseObject, error) {
	if err := requireManagerOrKey(principalFrom(ctx)); err != nil {
		return nil, err
	}
	ts, err := gallery.List()
	if err != nil {
		return nil, err
	}
	locale := templateLocale(req.Params.Locale)
	items := make([]oas.EmailTemplate, len(ts))
	for i, t := range ts {
		items[i] = s.templateBody(t, locale)
	}
	return oas.ListEmailTemplates200JSONResponse{Items: items}, nil
}

func (s *Server) GetEmailTemplate(ctx context.Context, req oas.GetEmailTemplateRequestObject) (oas.GetEmailTemplateResponseObject, error) {
	if err := requireManagerOrKey(principalFrom(ctx)); err != nil {
		return nil, err
	}
	t, ok := gallery.Find(req.Slug)
	if !ok {
		return nil, errTemplateGone
	}
	locale := templateLocale(req.Params.Locale)
	d, used, err := t.Document(gallery.Embedded(), locale)
	if err != nil {
		return nil, err
	}
	doc, err := documentBody(d)
	if err != nil {
		return nil, err
	}
	imgs := []oas.EmailTemplateImage{}
	seen := map[string]bool{}
	d.Walk(func(b design.Block) {
		if b.Type != "image" || b.Asset == "" || seen[b.Asset] {
			return
		}
		seen[b.Asset] = true
		raw, err := gallery.Asset(gallery.Embedded(), t.Slug, b.Asset)
		if err != nil {
			return
		}
		w, h, err := gallery.AssetSize(raw)
		if err != nil {
			return
		}
		imgs = append(imgs, oas.EmailTemplateImage{
			Asset: b.Asset, Url: s.auth.PublicURL + emailTemplatesPathPrefix + t.Slug + "/assets/" + b.Asset + "?v=" + s.assetVersion(),
			Width: int32(w), Height: int32(h),
		})
	})
	body := s.templateBody(t, locale)
	return oas.GetEmailTemplate200JSONResponse{
		Slug: body.Slug, Category: body.Category, Name: body.Name, Description: body.Description, Locales: body.Locales,
		PreviewUrl: body.PreviewUrl, PreviewHtmlUrl: body.PreviewHtmlUrl, Added: body.Added,
		Locale: oas.EmailTemplateLocale(used), Document: doc, Images: imgs,
	}, nil
}

// documentFromTemplate is a template's design with its files stored as the workspace's images.
func (s *Server) documentFromTemplate(ctx context.Context, q *store.Queries, p principal, slug string, locale *oas.EmailTemplateLocale, others []oas.EmailTemplateLocale) (design.Document, error) {
	t, ok := gallery.Find(slug)
	if !ok {
		return design.Document{}, errTemplateGone
	}
	d, base, err := t.Document(gallery.Embedded(), templateLocale(locale))
	if err != nil {
		return d, err
	}
	if len(others) > 2 {
		return d, errValidation("template.locales names at most 2 languages")
	}
	for _, l := range others {
		tag := string(l)
		if tag == base || d.Translations[tag].Blocks != nil {
			return d, errValidation("template.locales names other languages than template.locale, once each")
		}
		if !slices.Contains(t.Locales, tag) {
			return d, errValidation("the template " + t.Slug + " has no " + tag + " version")
		}
		other, _, err := t.Document(gallery.Embedded(), tag)
		if err != nil {
			return d, err
		}
		if d.Translations == nil {
			d.Locale, d.Translations = base, map[string]design.Translation{}
		}
		d.Translations[tag] = d.TranslationOf(other)
	}
	ids := map[string]uuid.UUID{}
	var failed error
	d.MapImages(func(b *design.Block) {
		if b.Asset == "" || failed != nil {
			return
		}
		id, ok := ids[b.Asset]
		if !ok {
			raw, err := gallery.Asset(gallery.Embedded(), t.Slug, b.Asset)
			if err != nil {
				failed = err
				return
			}
			img, _, err := s.storeEmailImage(ctx, q, p, raw, b.Asset)
			if err != nil {
				failed = err
				return
			}
			id = img.ID
			ids[b.Asset] = id
		}
		b.ImageID, b.Asset = id.String(), ""
	})
	if failed != nil {
		return d, failed
	}
	if issues := design.Validate(d, design.Options{}); len(issues) > 0 {
		return d, issuesProblem(issues)
	}
	return d, nil
}

// serveTemplatePreview answers /email-templates/{slug}/{locale}.html|webp, the gallery's
// previews, for anyone.
func (s *Server) serveTemplatePreview(w http.ResponseWriter, r *http.Request) {
	slug, file := r.PathValue("slug"), r.PathValue("file")
	t, ok := gallery.Find(slug)
	if !ok || !previewFile.MatchString(file) {
		writeProblem(w, errTemplateGone)
		return
	}
	locale, ext, _ := strings.Cut(file, ".")
	b, err := fs.ReadFile(gallery.Embedded(), t.Slug+"/preview."+t.DocumentLocale(locale)+"."+ext)
	if errors.Is(err, fs.ErrNotExist) {
		writeProblem(w, errTemplateGone)
		return
	}
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	ct := "image/webp"
	if ext == "html" {
		ct = "text/html; charset=utf-8"
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'")
	}
	servePublicImage(w, r, ct, `"`+s.assetVersion()+"-"+slug+"-"+file+`"`, b)
}

func (s *Server) serveTemplateAsset(w http.ResponseWriter, r *http.Request) {
	slug, file := r.PathValue("slug"), r.PathValue("file")
	b, err := gallery.Asset(gallery.Embedded(), slug, file)
	if err != nil {
		writeProblem(w, errNotFound)
		return
	}
	ct := "image/png"
	switch {
	case strings.HasSuffix(file, ".jpg"):
		ct = "image/jpeg"
	case strings.HasSuffix(file, ".gif"):
		ct = "image/gif"
	case !strings.HasSuffix(file, ".png"):
		writeProblem(w, errNotFound)
		return
	}
	servePublicImage(w, r, ct, `"`+s.assetVersion()+"-"+slug+"-"+file+`"`, b)
}

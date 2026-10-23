package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/mail/design"
	"github.com/productdevbook/yuva/api/internal/mail/images"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/storage"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	emailImagePathPrefix = "/i/"
	emailImageCache      = "public, max-age=31536000, immutable"
	unusedImagesKept     = 7 * 24 * time.Hour
)

var (
	errEmailImageGone  = problem(http.StatusNotFound, "not_found", "no such e-mail image")
	errImageType       = problem(http.StatusUnsupportedMediaType, "image_type", "upload a PNG, JPEG, GIF or WebP image; SVG and other types are refused")
	errImageTooLarge   = problem(http.StatusUnprocessableEntity, "image_too_large", "the image is over 40 megapixels, 8,000 pixels on a side, 4,000 pixels tall at 1,200 wide, or an animated GIF over its limits")
	errGIFTooWide      = problem(http.StatusUnprocessableEntity, "gif_too_wide", "GIFs are kept as they are and must be at most 1,200 pixels wide")
	errImageFileTooBig = problem(http.StatusRequestEntityTooLarge, "file_too_large", "an image upload is at most 10 MiB")
	errImageQuota      = problem(http.StatusConflict, "image_quota", "the workspace's e-mail images would exceed the server's limit")
	errImageSent       = problem(http.StatusConflict, "image_sent", "a sent broadcast used this image; delete it with force=true and mail already delivered shows a broken image")
	limitImageUpload   = limit{30, time.Minute}
	imageToken         = regexp.MustCompile(`^[a-z2-7]{26}$`)
	iconFile           = regexp.MustCompile(`^[a-z]+-(dark|light|brand)\.png$`)
	tokenEncoding      = base32.StdEncoding.WithPadding(base32.NoPadding)
)

func newImageToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return strings.ToLower(tokenEncoding.EncodeToString(b))
}

func (s *Server) emailImageBody(r store.EmailImage) oas.EmailImage {
	return oas.EmailImage{
		Id: r.ID, Url: s.imageURL(r.Token), Width: r.Width, Height: r.Height, SizeBytes: r.SizeBytes,
		ContentType: oas.EmailImageContentType(r.ContentType), Filename: r.Filename, SentAt: r.SentAt, CreatedAt: r.CreatedAt,
	}
}

func imageError(err error) error {
	switch {
	case errors.Is(err, images.ErrType):
		return errImageType
	case errors.Is(err, images.ErrTooLarge):
		return errImageTooLarge
	case errors.Is(err, images.ErrGIFWide):
		return errGIFTooWide
	}
	return err
}

// storeEmailImage normalises data and keeps it as the workspace's image, or returns the image it
// already has with the same bytes (created false).
func (s *Server) storeEmailImage(ctx context.Context, q *store.Queries, p principal, data []byte, filename string) (store.EmailImage, bool, error) {
	n, err := images.Normalize(data)
	if err != nil {
		return store.EmailImage{}, false, imageError(err)
	}
	existing, err := q.GetEmailImageBySHA256(ctx, store.GetEmailImageBySHA256Params{WorkspaceID: p.workspaceID, Sha256: n.SHA256[:]})
	if err == nil {
		return existing, false, nil
	}
	if !store.IsNotFound(err) {
		return store.EmailImage{}, false, err
	}
	if s.imagesMax > 0 {
		if err := q.LockEmailImageQuota(ctx, p.workspaceID); err != nil {
			return store.EmailImage{}, false, err
		}
		used, err := q.SumEmailImageBytes(ctx, p.workspaceID)
		if err != nil {
			return store.EmailImage{}, false, err
		}
		if used+int64(len(n.Data)) > s.imagesMax {
			return store.EmailImage{}, false, errImageQuota
		}
	}
	id := newID()
	key := "email-images/" + p.workspaceID.String() + "/" + id.String()
	if err := s.objects.Put(ctx, key, bytes.NewReader(n.Data), int64(len(n.Data)), n.ContentType); err != nil {
		return store.EmailImage{}, false, err
	}
	row, err := q.CreateEmailImage(ctx, store.CreateEmailImageParams{
		ID: id, WorkspaceID: p.workspaceID, Token: newImageToken(), StorageKey: key, ContentType: n.ContentType,
		Width: int32(n.Width), Height: int32(n.Height), SizeBytes: int64(len(n.Data)), Sha256: n.SHA256[:],
		Filename: cleanFilename(filename), MemberID: p.memberRef(), Now: s.now(),
	})
	if store.IsNotFound(err) {
		s.deleteObjects(ctx, []string{key})
		row, err = q.GetEmailImageBySHA256(ctx, store.GetEmailImageBySHA256Params{WorkspaceID: p.workspaceID, Sha256: n.SHA256[:]})
		return row, false, err
	}
	if err != nil {
		s.deleteObjects(ctx, []string{key})
		return store.EmailImage{}, false, err
	}
	if err := s.addUsage(ctx, q, p.workspaceID, 0, 0, row.SizeBytes); err != nil {
		return row, true, err
	}
	return row, true, nil
}

func (s *Server) UploadEmailImage(ctx context.Context, req oas.UploadEmailImageRequestObject) (oas.UploadEmailImageResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	if err := s.rateLimit(ctx, rateCheck{"email-image-upload:" + p.workspaceID.String(), limitImageUpload}); err != nil {
		return nil, err
	}
	data, filename, err := readImageUpload(req.Body)
	if err != nil {
		return nil, err
	}
	var row store.EmailImage
	var created bool
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		var err error
		row, created, err = s.storeEmailImage(ctx, q, p, data, filename)
		return err
	})
	if err != nil {
		return nil, err
	}
	if created {
		return oas.UploadEmailImage201JSONResponse(s.emailImageBody(row)), nil
	}
	return oas.UploadEmailImage200JSONResponse(s.emailImageBody(row)), nil
}

func readImageUpload(body *multipart.Reader) ([]byte, string, error) {
	var data []byte
	var filename string
	for {
		part, err := body.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", uploadError(err)
		}
		if part.FormName() != "file" {
			return nil, "", errValidation("unknown form field " + part.FormName())
		}
		if data != nil {
			return nil, "", errValidation("send one file")
		}
		data, err = io.ReadAll(io.LimitReader(part, images.MaxUploadBytes+1))
		if err != nil {
			return nil, "", uploadError(err)
		}
		if len(data) > images.MaxUploadBytes {
			return nil, "", errImageFileTooBig
		}
		filename = part.FileName()
	}
	if len(data) == 0 {
		return nil, "", errValidation("file is required")
	}
	return data, filename, nil
}

func (s *Server) ListEmailImages(ctx context.Context, req oas.ListEmailImagesRequestObject) (oas.ListEmailImagesResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListEmailImages(ctx, store.ListEmailImagesParams{WorkspaceID: p.workspaceID, CursorAt: at, CursorID: id, Lim: lim + 1})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(rows[lim-1].CreatedAt, rows[lim-1].ID)
		next = &c
	}
	items := make([]oas.EmailImage, len(rows))
	for i, r := range rows {
		items[i] = s.emailImageBody(r)
	}
	return oas.ListEmailImages200JSONResponse{Items: items, NextCursor: next}, nil
}

func (s *Server) DeleteEmailImage(ctx context.Context, req oas.DeleteEmailImageRequestObject) (oas.DeleteEmailImageResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	var key string
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		img, err := q.LockEmailImage(ctx, store.LockEmailImageParams{WorkspaceID: p.workspaceID, ID: req.ImageId})
		if store.IsNotFound(err) {
			return errEmailImageGone
		}
		if err != nil {
			return err
		}
		users, err := q.ListDesignsUsingImage(ctx, store.ListDesignsUsingImageParams{WorkspaceID: p.workspaceID, ImageID: img.ID.String()})
		if err != nil {
			return err
		}
		articles, err := q.ListHelpArticlesUsingImage(ctx, store.ListHelpArticlesUsingImageParams{WorkspaceID: p.workspaceID, ImageID: img.ID})
		if err != nil {
			return err
		}
		entries, err := q.ListChangelogEntriesUsingImage(ctx, store.ListChangelogEntriesUsingImageParams{WorkspaceID: p.workspaceID, ImageID: &img.ID})
		if err != nil {
			return err
		}
		if len(users) > 0 || len(articles) > 0 || len(entries) > 0 {
			designs := make([]map[string]any, len(users))
			for i, u := range users {
				designs[i] = map[string]any{"id": u.ID, "name": u.Name}
			}
			usedBy := map[string]any{"designs": designs}
			if len(articles) > 0 {
				help := make([]map[string]any, len(articles))
				for i, a := range articles {
					help[i] = map[string]any{"id": a.ID, "center_id": a.CenterID, "number": a.Number, "title": a.Title}
				}
				usedBy["help_articles"] = help
			}
			if len(entries) > 0 {
				news := make([]map[string]any, len(entries))
				for i, n := range entries {
					news[i] = map[string]any{"id": n.ID, "changelog_id": n.ChangelogID, "number": n.Number, "title": n.Title}
				}
				usedBy["changelog_entries"] = news
			}
			e := problem(http.StatusConflict, "image_in_use", "designs, help articles or What's new entries use this image; take it out of them first")
			e.Extra = map[string]any{"used_by": usedBy}
			return e
		}
		if img.SentAt != nil && !boolOr(req.Params.Force, false) {
			return errImageSent
		}
		if _, err := q.DeleteEmailImage(ctx, store.DeleteEmailImageParams{WorkspaceID: p.workspaceID, ID: img.ID}); err != nil {
			return err
		}
		key = img.StorageKey
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.deleteObjects(ctx, []string{key})
	return oas.DeleteEmailImage204Response{}, nil
}

// publicImageHeaders are the same for every public e-mail image: cached for a year, never
// sniffed, embeddable anywhere, and nothing else.
func publicImageHeaders(h http.Header, contentType, etag string, size int) {
	h.Set("Content-Type", contentType)
	h.Set("Content-Length", strconv.Itoa(size))
	h.Set("Cache-Control", emailImageCache)
	h.Set("ETag", etag)
	h.Set("X-Content-Type-Options", "nosniff")
	if h.Get("Content-Security-Policy") == "" {
		h.Set("Content-Security-Policy", "default-src 'none'")
	}
	h.Set("Cross-Origin-Resource-Policy", "cross-origin")
	h.Set("Referrer-Policy", "no-referrer")
}

func etagMatches(r *http.Request, etag string) bool {
	for _, v := range strings.Split(r.Header.Get("If-None-Match"), ",") {
		if v = strings.TrimSpace(v); v == etag || v == "*" || strings.TrimPrefix(v, "W/") == etag {
			return true
		}
	}
	return false
}

func servePublicImage(w http.ResponseWriter, r *http.Request, contentType, etag string, body []byte) {
	if etagMatches(r, etag) {
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", emailImageCache)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	publicImageHeaders(w.Header(), contentType, etag, len(body))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

// serveEmailImage answers /i/{token} for anyone, recording nothing about the request.
func (s *Server) serveEmailImage(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if !imageToken.MatchString(token) {
		writeProblem(w, errNotFound)
		return
	}
	img, err := s.st.GetPublicEmailImage(r.Context(), token)
	if store.IsNotFound(err) {
		writeProblem(w, errNotFound)
		return
	}
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	etag := `"` + hex.EncodeToString(img.Sha256) + `"`
	if etagMatches(r, etag) {
		servePublicImage(w, r, img.ContentType, etag, nil)
		return
	}
	f, err := s.objects.Open(r.Context(), img.StorageKey)
	if errors.Is(err, storage.ErrNotFound) {
		writeProblem(w, errNotFound)
		return
	}
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, img.SizeBytes+1))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	servePublicImage(w, r, img.ContentType, etag, body)
}

func (s *Server) serveSocialIcon(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	body, ok := design.IconPNG(file)
	if !iconFile.MatchString(file) || !ok {
		writeProblem(w, errNotFound)
		return
	}
	servePublicImage(w, r, "image/png", `"`+s.assetVersion()+"-"+file+`"`, body)
}

// assetVersion names the build, so built-in files can be cached as immutable.
func (s *Server) assetVersion() string {
	if s.version == "" {
		return "dev"
	}
	return s.version
}

// An upload no design, help article, kept revision or sent broadcast uses is deleted after a week.
func (s *Server) CleanUnusedImages(ctx context.Context) error {
	ids, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	for _, ws := range ids {
		if err := s.cleanUnusedImages(ctx, ws); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) cleanUnusedImages(ctx context.Context, ws uuid.UUID) error {
	for {
		keys, err := s.st.DeleteUnusedEmailImages(ctx, store.DeleteUnusedEmailImagesParams{WorkspaceID: ws, Before: s.now().Add(-unusedImagesKept), Lim: retentionBatch})
		if err != nil {
			return err
		}
		s.deleteObjects(ctx, keys)
		if len(keys) < retentionBatch {
			return nil
		}
	}
}

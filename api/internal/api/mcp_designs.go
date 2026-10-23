package api

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/mail/images"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
	"github.com/productdevbook/yuva/api/internal/webhook"
)

const imageFetchTimeout = 10 * time.Second

var (
	errImageSource      = errValidation("give the image as url or as data_base64, one of them")
	errImageURL         = errValidation("url must be an https URL of the image")
	errImageURLRefused  = problem(http.StatusUnprocessableEntity, "url_refused", "the URL's host is a private, local or reserved address; give a public https URL")
	errOwnerGrantOnly   = problem(http.StatusForbidden, "forbidden", "only the workspace's owner asks for broadcasts to be allowed, through their own connection; an API key has no person behind it")
	errConfirmName      = problem(http.StatusConflict, "confirm_name_mismatch", "confirm_name is not the broadcast's name; show the person the name from get_broadcast and cancel only after they confirm it")
	errImageFetchFailed = func(detail string) *apiError {
		return problem(http.StatusUnprocessableEntity, "url_fetch_failed", "the image could not be fetched: "+detail)
	}
)

type requestApprovalIn struct {
	Website            string `json:"website" jsonschema:"the workspace's website, an http(s) URL, as the person gave it"`
	SenderDescription  string `json:"sender_description" jsonschema:"who sends and what the broadcasts are about, in the person's own words"`
	ConsentDescription string `json:"consent_description" jsonschema:"how the subscribers agreed to get them, in the person's own words"`
}

type requestApprovalOut struct {
	Status   string     `json:"status" jsonschema:"approval_requested once the operator has the request"`
	StatusAt *time.Time `json:"status_at,omitempty"`
}

// mayRequestApproval lists request_broadcast_approval to an owner's OAuth connection only.
func mayRequestApproval(p principal) error {
	if !p.isGrant() || p.role != roleOwner {
		return errOwnerGrantOnly
	}
	return nil
}

type mcpEmailImage struct {
	ID          uuid.UUID  `json:"id" jsonschema:"pass as image_id in an image block of update_design"`
	URL         string     `json:"url" jsonschema:"the public address mail clients load it from"`
	Width       int32      `json:"width"`
	Height      int32      `json:"height"`
	SizeBytes   int64      `json:"size_bytes"`
	ContentType string     `json:"content_type"`
	Filename    string     `json:"filename"`
	SentAt      *time.Time `json:"sent_at,omitempty" jsonschema:"when a sent broadcast first used it"`
	CreatedAt   time.Time  `json:"created_at"`
}

func emailImageOut(i oas.EmailImage) mcpEmailImage {
	return mcpEmailImage{
		ID: i.Id, URL: i.Url, Width: i.Width, Height: i.Height, SizeBytes: i.SizeBytes, ContentType: string(i.ContentType),
		Filename: i.Filename, SentAt: i.SentAt, CreatedAt: i.CreatedAt,
	}
}

type pageIn struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int32  `json:"limit,omitempty" jsonschema:"1 to 100, default 25"`
}

type uploadImageIn struct {
	URL        string `json:"url,omitempty" jsonschema:"a public https URL of a PNG, JPEG, GIF or WebP image, such as the sender's logo on their site; redirects are not followed"`
	DataBase64 string `json:"data_base64,omitempty" jsonschema:"the image's bytes in base64 instead of url; up to about 5 MB of image fits in one request"`
	Filename   string `json:"filename,omitempty" jsonschema:"shown to members; taken from the URL when empty"`
}

type uploadImageOut struct {
	Image   mcpEmailImage `json:"image"`
	Created bool          `json:"created" jsonschema:"false when the workspace already had the same image"`
}

// fetchImage reads an image from a public https URL through the webhook client, which refuses
// private and reserved addresses and follows no redirects.
func (c *mcpCall) fetchImage(ctx context.Context, raw string) ([]byte, string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, "", errImageURL
	}
	if webhook.CheckHost(u.Hostname(), false) != nil {
		return nil, "", errImageURLRefused
	}
	ctx, cancel := context.WithTimeout(ctx, imageFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", errImageURL
	}
	req.Header.Set("Accept", "image/png, image/jpeg, image/gif, image/webp")
	req.Header.Set("User-Agent", "Yuva/"+c.s.assetVersion())
	res, err := webhook.NewClient(false, c.s.webhooks.Resolver).Do(req)
	if errors.Is(err, webhook.ErrRefusedAddress) {
		return nil, "", errImageURLRefused
	}
	if err != nil {
		return nil, "", errImageFetchFailed("the server did not answer in time or could not be reached")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", errImageFetchFailed("the server answered " + strconv.Itoa(res.StatusCode) + "; give the image's final URL")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, images.MaxUploadBytes+1))
	if err != nil {
		return nil, "", errImageFetchFailed("the download broke off")
	}
	if len(data) > images.MaxUploadBytes {
		return nil, "", errImageFileTooBig
	}
	return data, path.Base(u.Path), nil
}

func (c *mcpCall) uploadImage(ctx context.Context, in uploadImageIn) (uploadImageOut, error) {
	if (in.URL == "") == (in.DataBase64 == "") {
		return uploadImageOut{}, errImageSource
	}
	if err := requireManagerOrKey(c.p); err != nil {
		return uploadImageOut{}, err
	}
	if err := c.s.rateLimit(ctx, rateCheck{"email-image-upload:" + c.p.workspaceID.String(), limitImageUpload}); err != nil {
		return uploadImageOut{}, err
	}
	var data []byte
	filename := in.Filename
	if in.URL != "" {
		d, name, err := c.fetchImage(ctx, in.URL)
		if err != nil {
			return uploadImageOut{}, err
		}
		data = d
		if filename == "" {
			filename = name
		}
	} else {
		if base64.StdEncoding.DecodedLen(len(in.DataBase64)) > images.MaxUploadBytes+3 {
			return uploadImageOut{}, errImageFileTooBig
		}
		d, err := base64.StdEncoding.DecodeString(strings.TrimSpace(in.DataBase64))
		if err != nil {
			return uploadImageOut{}, errValidation("data_base64 is not base64")
		}
		if len(d) > images.MaxUploadBytes {
			return uploadImageOut{}, errImageFileTooBig
		}
		data = d
	}
	if len(data) == 0 {
		return uploadImageOut{}, errValidation("the image is empty")
	}
	if filename == "" || filename == "/" || filename == "." {
		filename = "image"
	}
	var row store.EmailImage
	var created bool
	err := c.s.st.InTx(ctx, func(q *store.Queries) error {
		var err error
		row, created, err = c.s.storeEmailImage(ctx, q, c.p, data, filename)
		return err
	})
	if err != nil {
		return uploadImageOut{}, err
	}
	return uploadImageOut{Image: emailImageOut(c.s.emailImageBody(row)), Created: created}, nil
}

type listDesignsIn struct {
	InboxID  *uuid.UUID `json:"inbox_id,omitempty" jsonschema:"only this inbox's designs"`
	Query    string     `json:"query,omitempty" jsonschema:"part of the name"`
	Archived bool       `json:"archived,omitempty" jsonschema:"true lists archived designs instead"`
	Cursor   string     `json:"cursor,omitempty"`
	Limit    int32      `json:"limit,omitempty" jsonschema:"1 to 100, default 25"`
}

type mcpDesignSummary struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	InboxID      *uuid.UUID `json:"inbox_id,omitempty" jsonschema:"absent for a design every inbox may use"`
	Version      int32      `json:"version"`
	TemplateSlug string     `json:"template_slug,omitempty"`
	ArchivedAt   *time.Time `json:"archived_at,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at"`
	PanelURL     string     `json:"panel_url,omitempty"`
}

type designIn struct {
	DesignID uuid.UUID `json:"design_id"`
}

type duplicateDesignIn struct {
	DesignID uuid.UUID `json:"design_id"`
	Name     string    `json:"name,omitempty" jsonschema:"the copy's name; the original's name with (copy) when empty"`
}

type cancelBroadcastIn struct {
	BroadcastID uuid.UUID `json:"broadcast_id"`
	ConfirmName string    `json:"confirm_name" jsonschema:"the broadcast's name as get_broadcast shows it, which the person you work for confirmed"`
}

func (c *mcpCall) getDesign(ctx context.Context, id uuid.UUID) (mcpDesign, error) {
	res, err := c.s.GetEmailDesign(ctx, oas.GetEmailDesignRequestObject{DesignId: id})
	if err != nil {
		return mcpDesign{}, err
	}
	return c.designOut(oas.EmailDesign(res.(oas.GetEmailDesign200JSONResponse)))
}

var mcpDesignTools = []mcpTool{
	defineTool("list_email_images", "List e-mail images",
		"Lists the workspace's uploaded e-mail images, newest first, with the id an image block of update_design takes. Owners, admins and keys only. Maps to GET /v1/email-images.",
		readOnly, nil, mcpTool{ops: []string{"ListEmailImages"}, allow: mayManageBroadcasts, denied: errNotBroadcastManager},
		func(ctx context.Context, c *mcpCall, in pageIn) (mcpPage[mcpEmailImage], error) {
			res, err := c.s.ListEmailImages(ctx, oas.ListEmailImagesRequestObject{Params: oas.ListEmailImagesParams{Cursor: optString[string](in.Cursor), Limit: optLimit(in.Limit)}})
			if err != nil {
				return mcpPage[mcpEmailImage]{}, err
			}
			page := res.(oas.ListEmailImages200JSONResponse)
			out := mcpPage[mcpEmailImage]{Items: []mcpEmailImage{}}
			for _, i := range page.Items {
				out.Items = append(out.Items, emailImageOut(i))
			}
			if page.NextCursor != nil {
				out.NextCursor = *page.NextCursor
			}
			return out, nil
		}),
	defineTool("upload_email_image", "Upload an e-mail image",
		"Adds a PNG, JPEG, GIF or WebP image for designs, such as the sender's logo: from a public https URL (fetched by Yuva within 10 seconds; private and local addresses are refused) or from bytes in data_base64. At most 10 MiB; wide images are scaled to 1,200 pixels. Returns the image id to put in an image block with update_design; the same image twice gives the same id. Owners, admins and keys only. Maps to POST /v1/email-images.",
		additive, nil, mcpTool{ops: []string{"UploadEmailImage"}, allow: mayManageBroadcasts, denied: errNotBroadcastManager},
		func(ctx context.Context, c *mcpCall, in uploadImageIn) (uploadImageOut, error) {
			return c.uploadImage(ctx, in)
		}),
	defineTool("list_designs", "List e-mail designs",
		"Lists the workspace's e-mail designs, most recently changed first. Read one with get_design. Owners, admins and keys only. Maps to GET /v1/email-designs.",
		readOnly, nil, mcpTool{ops: []string{"ListEmailDesigns"}, allow: mayManageBroadcasts, denied: errNotBroadcastManager},
		func(ctx context.Context, c *mcpCall, in listDesignsIn) (mcpPage[mcpDesignSummary], error) {
			params := oas.ListEmailDesignsParams{InboxId: in.InboxID, Q: optString[string](in.Query), Cursor: optString[string](in.Cursor), Limit: optLimit(in.Limit)}
			if in.Archived {
				params.Archived = &in.Archived
			}
			res, err := c.s.ListEmailDesigns(ctx, oas.ListEmailDesignsRequestObject{Params: params})
			if err != nil {
				return mcpPage[mcpDesignSummary]{}, err
			}
			page := res.(oas.ListEmailDesigns200JSONResponse)
			out := mcpPage[mcpDesignSummary]{Items: []mcpDesignSummary{}}
			for _, d := range page.Items {
				s := mcpDesignSummary{ID: d.Id, Name: d.Name, InboxID: d.InboxId, Version: d.Version, TemplateSlug: deref(d.TemplateSlug), ArchivedAt: d.ArchivedAt, UpdatedAt: d.UpdatedAt}
				if !c.s.noPanel {
					s.PanelURL = c.panelURL("/broadcasts/designs/" + d.Id.String())
				}
				out.Items = append(out.Items, s)
			}
			if page.NextCursor != nil {
				out.NextCursor = *page.NextCursor
			}
			return out, nil
		}),
	defineTool("get_design", "Get an e-mail design",
		"Returns a design with its document, version and warnings; change it with update_design. Owners, admins and keys only. Maps to GET /v1/email-designs/{id}.",
		readOnly, nil, mcpTool{ops: []string{"GetEmailDesign"}, allow: mayManageBroadcasts, denied: errNotBroadcastManager},
		func(ctx context.Context, c *mcpCall, in designIn) (mcpDesign, error) {
			return c.getDesign(ctx, in.DesignID)
		}),
	defineTool("duplicate_design", "Duplicate an e-mail design",
		"Copies a design into a new one, such as last month's newsletter as the start of this month's. Owners, admins and keys only. Maps to POST /v1/email-designs/{id}/duplicate.",
		additive, nil, mcpTool{ops: []string{"DuplicateEmailDesign"}, allow: mayManageBroadcasts, denied: errNotBroadcastManager},
		func(ctx context.Context, c *mcpCall, in duplicateDesignIn) (mcpDesign, error) {
			body := oas.EmailDesignDuplicate{Name: optString[string](strings.TrimSpace(in.Name))}
			res, err := c.s.DuplicateEmailDesign(ctx, oas.DuplicateEmailDesignRequestObject{DesignId: in.DesignID, Body: &body})
			if err != nil {
				return mcpDesign{}, err
			}
			return c.designOut(oas.EmailDesign(res.(oas.DuplicateEmailDesign201JSONResponse)))
		}),
	defineTool("pause_broadcast", "Pause a broadcast",
		"Pauses a broadcast that is preparing or sending; nothing more goes out until resume_broadcast. Listed only while the workspace lets API keys and assistants schedule broadcasts. Maps to POST /v1/broadcasts/{id}/pause.",
		idempotent, nil, mcpTool{ops: []string{"PauseBroadcast"}, gate: maySchedule},
		func(ctx context.Context, c *mcpCall, in broadcastIn) (mcpBroadcast, error) {
			res, err := c.s.PauseBroadcast(ctx, oas.PauseBroadcastRequestObject{BroadcastId: in.BroadcastID})
			if err != nil {
				return mcpBroadcast{}, err
			}
			return c.broadcastOut(oas.Broadcast(res.(oas.PauseBroadcast200JSONResponse))), nil
		}),
	defineTool("resume_broadcast", "Resume a broadcast",
		"Resumes a paused broadcast, so it goes on sending to the rest of its audience. Only when the person you work for asks. Listed only while the workspace lets API keys and assistants schedule broadcasts. Maps to POST /v1/broadcasts/{id}/resume.",
		delivering, nil, mcpTool{ops: []string{"ResumeBroadcast"}, gate: maySchedule},
		func(ctx context.Context, c *mcpCall, in broadcastIn) (mcpBroadcast, error) {
			res, err := c.s.ResumeBroadcast(ctx, oas.ResumeBroadcastRequestObject{BroadcastId: in.BroadcastID})
			if err != nil {
				return mcpBroadcast{}, err
			}
			return c.broadcastOut(oas.Broadcast(res.(oas.ResumeBroadcast200JSONResponse))), nil
		}),
	defineTool("unschedule_broadcast", "Unschedule a broadcast",
		"Turns a scheduled broadcast back into a draft before it starts; nothing is sent. Listed only while the workspace lets API keys and assistants schedule broadcasts. Maps to POST /v1/broadcasts/{id}/unschedule.",
		idempotent, nil, mcpTool{ops: []string{"UnscheduleBroadcast"}, gate: maySchedule},
		func(ctx context.Context, c *mcpCall, in broadcastIn) (mcpBroadcast, error) {
			res, err := c.s.UnscheduleBroadcast(ctx, oas.UnscheduleBroadcastRequestObject{BroadcastId: in.BroadcastID})
			if err != nil {
				return mcpBroadcast{}, err
			}
			return c.broadcastOut(oas.Broadcast(res.(oas.UnscheduleBroadcast200JSONResponse))), nil
		}),
	defineTool("cancel_broadcast", "Cancel a broadcast",
		"Cancels a scheduled, preparing, sending or paused broadcast for good: mail not yet sent never goes out, and it cannot be resumed. confirm_name must be the broadcast's name, which the person you work for confirmed. Listed only while the workspace lets API keys and assistants schedule broadcasts. Maps to POST /v1/broadcasts/{id}/cancel.",
		destructive, nil, mcpTool{ops: []string{"GetBroadcast", "CancelBroadcast"}, gate: maySchedule},
		func(ctx context.Context, c *mcpCall, in cancelBroadcastIn) (mcpBroadcast, error) {
			res, err := c.s.GetBroadcast(ctx, oas.GetBroadcastRequestObject{BroadcastId: in.BroadcastID})
			if err != nil {
				return mcpBroadcast{}, err
			}
			if res.(oas.GetBroadcast200JSONResponse).Name != in.ConfirmName {
				return mcpBroadcast{}, errConfirmName
			}
			cres, err := c.s.CancelBroadcast(ctx, oas.CancelBroadcastRequestObject{BroadcastId: in.BroadcastID})
			if err != nil {
				return mcpBroadcast{}, err
			}
			return c.broadcastOut(oas.Broadcast(cres.(oas.CancelBroadcast200JSONResponse))), nil
		}),
	defineTool("request_broadcast_approval", "Ask to allow broadcasts",
		"Asks the server's operator to allow this workspace to send broadcasts, when the workspace's broadcasts status is approval_required (409 invalid_state otherwise); it becomes approval_requested until the operator answers. website, sender_description and consent_description come from the person you work for, word for word: ask them, never write or guess any of them. Listed only to the workspace's owner. Maps to POST /v1/workspace/broadcast-approval.",
		delivering, nil, mcpTool{ops: []string{"RequestBroadcastApproval"}, gate: mayRequestApproval},
		func(ctx context.Context, c *mcpCall, in requestApprovalIn) (requestApprovalOut, error) {
			res, err := c.s.RequestBroadcastApproval(ctx, oas.RequestBroadcastApprovalRequestObject{Body: &oas.BroadcastApprovalRequest{
				Website: in.Website, SenderDescription: in.SenderDescription, ConsentDescription: in.ConsentDescription,
			}})
			if err != nil {
				return requestApprovalOut{}, err
			}
			b := oas.Workspace(res.(oas.RequestBroadcastApproval202JSONResponse)).Broadcasts
			return requestApprovalOut{Status: string(b.Status), StatusAt: b.StatusAt}, nil
		}),
}

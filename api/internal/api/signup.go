package api

import (
	"context"
	"net/http"
	"time"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	defaultSignupWorkspacesPerPerson = 3
	defaultSignupWorkspacesPerIP     = 5
	signupIPWindow                   = 24 * time.Hour
)

var (
	errSignupClosed   = problem(http.StatusForbidden, "signup_closed", "this server is invite only; ask its operator for a workspace")
	errWorkspaceLimit = problem(http.StatusForbidden, "workspace_limit", "you have created as many workspaces as this server allows")
	errSignupIPLimit  = problem(http.StatusTooManyRequests, "rate_limited", "too many workspaces created from this address today; try again later")
)

func (s *Server) CreateWorkspace(ctx context.Context, req oas.CreateWorkspaceRequestObject) (oas.CreateWorkspaceResponseObject, error) {
	if err := principalFrom(ctx).ssoScope(nil); err != nil {
		return nil, err
	}
	cfg := s.auth.Signup
	if !cfg.Open {
		return nil, errSignupClosed
	}
	name, ok := workspaceName(req.Body.Name)
	if !ok {
		return nil, errValidation("name must be 1 to 200 characters")
	}
	perPerson, perIP := int64(cfg.WorkspacesPerPerson), int64(cfg.WorkspacesPerIP)
	if perPerson == 0 {
		perPerson = defaultSignupWorkspacesPerPerson
	}
	if perIP == 0 {
		perIP = defaultSignupWorkspacesPerIP
	}
	p := principalFrom(ctx)
	ip := rateIP(s.clientIP(requestFrom(ctx)))
	now := s.now()
	var ws store.Workspace
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		if err := q.LockBootstrap(ctx); err != nil {
			return err
		}
		n, err := q.CountWorkspacesCreatedBy(ctx, &p.personID)
		if err != nil {
			return err
		}
		if n >= perPerson {
			return errWorkspaceLimit
		}
		if n, err = q.CountWorkspacesCreatedFromIP(ctx, store.CountWorkspacesCreatedFromIPParams{Ip: &ip, Since: now.Add(-signupIPWindow)}); err != nil {
			return err
		}
		if n >= perIP {
			return errSignupIPLimit
		}
		if ws, _, err = createOwnedWorkspace(ctx, q, store.CreateWorkspaceParams{ID: newID(), Name: name, CreatedBy: &p.personID, CreatedIp: &ip}, p.personID); err != nil {
			return err
		}
		if s.broadcastMode() == broadcastsApproval {
			if ws, err = q.SetWorkspaceBroadcastsStatus(ctx, store.SetWorkspaceBroadcastsStatusParams{ID: ws.ID, Status: wsApprovalRequired, Now: now}); err != nil {
				return err
			}
		}
		if len(s.operator) == 0 {
			return nil
		}
		person, err := q.GetPerson(ctx, p.personID)
		if err != nil {
			return err
		}
		return s.noteOperator(ctx, q, noticeWorkspace, person.Email, false, &name)
	})
	if err != nil {
		return nil, err
	}
	body, err := s.workspaceBody(ctx, ws)
	if err != nil {
		return nil, err
	}
	return oas.CreateWorkspace201JSONResponse(body), nil
}

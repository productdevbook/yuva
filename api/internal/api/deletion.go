package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	purgeConversationBatch = 200
	purgeContactBatch      = 1000
	purgeBudget            = 30 * time.Second
)

var (
	errWorkspaceConfirmation = problem(http.StatusBadRequest, "confirmation_mismatch", "type the workspace's name exactly to delete it")
	errAccountConfirmation   = problem(http.StatusBadRequest, "confirmation_mismatch", "type your e-mail address to delete your account")
)

type WorkspaceDeleteArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
}

func (WorkspaceDeleteArgs) Kind() string { return "workspace_delete" }

func (WorkspaceDeleteArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

type workspaceDeleteWorker struct {
	river.WorkerDefaults[WorkspaceDeleteArgs]
	s *Server
}

func (w *workspaceDeleteWorker) Timeout(*river.Job[WorkspaceDeleteArgs]) time.Duration {
	return purgeBudget + 90*time.Second
}

func (w *workspaceDeleteWorker) Work(ctx context.Context, job *river.Job[WorkspaceDeleteArgs]) error {
	p, err := w.s.PurgeWorkspace(ctx, job.Args.WorkspaceID, purgeBudget)
	if err != nil {
		return err
	}
	_ = river.RecordOutput(ctx, p)
	w.s.log.InfoContext(ctx, "workspace deletion", slog.String("workspace_id", job.Args.WorkspaceID.String()),
		slog.Int64("conversations_deleted", p.Conversations), slog.Int64("conversations_left", p.ConversationsLeft),
		slog.Int64("contacts_deleted", p.Contacts), slog.Int("files_deleted", p.Files), slog.Bool("done", p.Done))
	if !p.Done {
		return river.JobSnooze(0)
	}
	return nil
}

func (s *Server) DeleteWorkspace(ctx context.Context, req oas.DeleteWorkspaceRequestObject) (oas.DeleteWorkspaceResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireOwner(p); err != nil {
		return nil, err
	}
	w, err := s.st.GetWorkspace(ctx, p.workspaceID)
	if err != nil {
		return nil, err
	}
	if req.Body.Name != w.Name {
		return nil, errWorkspaceConfirmation
	}
	if err := s.closeWorkspace(ctx, p.workspaceID); err != nil {
		return nil, err
	}
	return oas.DeleteWorkspace202Response{}, nil
}

// OperatorDeleteWorkspace closes a workspace for the command line and queues the deletion of its data.
func (s *Server) OperatorDeleteWorkspace(ctx context.Context, workspaceID uuid.UUID) error {
	return s.closeWorkspace(ctx, workspaceID)
}

// closeWorkspace marks the workspace deleted, which every authentication path checks, removes
// what would still reach outside (webhook endpoints, push subscriptions of members left without a
// workspace) and queues the job that deletes the rest.
func (s *Server) closeWorkspace(ctx context.Context, workspaceID uuid.UUID) error {
	now := s.now()
	return s.st.InTxRaw(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.LockWorkspace(ctx, workspaceID); err != nil {
			return err
		}
		if _, err := q.MarkWorkspaceDeleted(ctx, store.MarkWorkspaceDeletedParams{ID: workspaceID, Now: &now}); err != nil {
			return err
		}
		if _, err := q.DeleteWorkspaceWebhookEndpoints(ctx, workspaceID); err != nil {
			return err
		}
		if _, err := q.DeleteLonelyMembersPushSubscriptions(ctx, workspaceID); err != nil {
			return err
		}
		_, err := s.jobs.InsertTx(ctx, tx, WorkspaceDeleteArgs{WorkspaceID: workspaceID}, nil)
		return err
	})
}

// queueWorkspaceDeletions queues the deletion again for workspaces whose job was lost.
func (s *Server) queueWorkspaceDeletions(ctx context.Context) error {
	ids, err := s.st.ListDeletedWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.jobs.Insert(ctx, WorkspaceDeleteArgs{WorkspaceID: id}, nil); err != nil {
			return err
		}
	}
	return nil
}

// WorkspacePurge is the progress of one run of a workspace's deletion.
type WorkspacePurge struct {
	Conversations     int64 `json:"conversations_deleted"`
	ConversationsLeft int64 `json:"conversations_left"`
	Contacts          int64 `json:"contacts_deleted"`
	Files             int   `json:"files_deleted"`
	Done              bool  `json:"done"`
}

// PurgeWorkspace deletes a workspace marked deleted, in batches, for at most about budget: the
// conversations with their stored files, then the contacts, then the workspace row, which takes
// everything else with it. Done reports that nothing is left. A workspace not marked deleted is
// never touched.
func (s *Server) PurgeWorkspace(ctx context.Context, workspaceID uuid.UUID, budget time.Duration) (WorkspacePurge, error) {
	var out WorkspacePurge
	if _, err := s.st.GetDeletedWorkspace(ctx, workspaceID); store.IsNotFound(err) {
		out.Done = true
		return out, nil
	} else if err != nil {
		return out, err
	}
	deadline := time.Now().Add(budget)
	for {
		n, files, err := s.deleteConversationBatch(ctx, workspaceID, func(q *store.Queries) ([]uuid.UUID, error) {
			return q.ListWorkspaceConversationIDs(ctx, store.ListWorkspaceConversationIDsParams{WorkspaceID: workspaceID, MaxRows: purgeConversationBatch})
		})
		if err != nil {
			return out, err
		}
		out.Conversations += int64(n)
		out.Files += files
		if n < purgeConversationBatch {
			break
		}
		if time.Now().After(deadline) {
			left, err := s.st.CountWorkspaceConversations(ctx, workspaceID)
			out.ConversationsLeft = left
			return out, err
		}
	}
	for {
		n, err := s.st.DeleteWorkspaceContacts(ctx, store.DeleteWorkspaceContactsParams{WorkspaceID: workspaceID, MaxRows: purgeContactBatch})
		if err != nil {
			return out, err
		}
		out.Contacts += n
		if n < purgeContactBatch {
			break
		}
		if time.Now().After(deadline) {
			return out, nil
		}
	}
	if _, err := s.st.PurgeWorkspace(ctx, workspaceID); err != nil {
		return out, err
	}
	out.Done = true
	return out, nil
}

// deleteConversationBatch deletes the conversations list returns, with their messages and
// attachments, and then their stored files. It returns how many conversations and files went.
func (s *Server) deleteConversationBatch(ctx context.Context, workspaceID uuid.UUID, list func(q *store.Queries) ([]uuid.UUID, error)) (int, int, error) {
	var keys []string
	var n int
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		ids, err := list(q)
		if err != nil || len(ids) == 0 {
			return err
		}
		n = len(ids)
		if keys, err = q.ListConversationsStorageKeys(ctx, store.ListConversationsStorageKeysParams{WorkspaceID: workspaceID, Ids: ids}); err != nil {
			return err
		}
		raw, err := q.ListConversationsRawKeys(ctx, store.ListConversationsRawKeysParams{WorkspaceID: workspaceID, Ids: ids})
		if err != nil {
			return err
		}
		keys = append(keys, raw...)
		_, err = q.DeleteConversations(ctx, store.DeleteConversationsParams{WorkspaceID: workspaceID, Ids: ids})
		return err
	})
	if err != nil {
		return 0, 0, err
	}
	s.deleteObjects(ctx, keys)
	return n, len(keys), nil
}

func (s *Server) DeleteMe(ctx context.Context, req oas.DeleteMeRequestObject) (oas.DeleteMeResponseObject, error) {
	p := principalFrom(ctx)
	person, err := s.st.GetPerson(ctx, p.personID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(req.Body.Email), person.Email) {
		return nil, errAccountConfirmation
	}
	if err := s.deletePerson(ctx, person); err != nil {
		return nil, err
	}
	cookie := s.sessionCookie("", -1)
	return oas.DeleteMe204Response{Headers: oas.DeleteMe204ResponseHeaders{SetCookie: &cookie}}, nil
}

// OperatorDeletePerson deletes a person's account for the command line, with the same rules as
// DELETE /v1/me.
func (s *Server) OperatorDeletePerson(ctx context.Context, email string) (store.Person, error) {
	addr, err := normalizeEmail(oas.Email(email))
	if err != nil {
		return store.Person{}, err
	}
	person, err := s.st.GetPersonByEmail(ctx, addr)
	if store.IsNotFound(err) {
		return store.Person{}, fmt.Errorf("no person with the address %s", addr)
	}
	if err != nil {
		return store.Person{}, err
	}
	return person, s.deletePerson(ctx, person)
}

// deletePerson removes a person who is not the last owner of a workspace. Their memberships,
// sessions, passkeys and push subscriptions go with the row; messages they wrote lose their
// author.
func (s *Server) deletePerson(ctx context.Context, person store.Person) error {
	var workspaces []uuid.UUID
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		owned, err := q.ListOwnedWorkspaces(ctx, person.ID)
		if err != nil {
			return err
		}
		var sole []string
		for _, w := range owned {
			if _, err := q.LockWorkspace(ctx, w.ID); err != nil {
				return err
			}
			n, err := q.CountOwners(ctx, w.ID)
			if err != nil {
				return err
			}
			if n <= 1 {
				sole = append(sole, w.Name)
			}
		}
		if len(sole) > 0 {
			return problem(http.StatusConflict, "last_owner", fmt.Sprintf(
				"you are the only owner of %s; make someone else an owner or delete the workspace first", strings.Join(sole, ", ")))
		}
		if workspaces, err = q.ListPersonWorkspaceIDs(ctx, person.ID); err != nil {
			return err
		}
		if err := q.DeleteLoginCodesByEmail(ctx, person.Email); err != nil {
			return err
		}
		_, err = q.DeletePerson(ctx, person.ID)
		return err
	})
	if err != nil {
		return err
	}
	for _, ws := range workspaces {
		s.presenceHint(ctx, ws)
	}
	return nil
}

// workspaceLive tells jobs queued before a workspace was deleted to do nothing.
func (s *Server) workspaceLive(ctx context.Context, workspaceID uuid.UUID) (bool, error) {
	return s.st.IsWorkspaceLive(ctx, workspaceID)
}

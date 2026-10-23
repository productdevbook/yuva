package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/store"
)

type sealedColumn struct {
	store.SealedColumn
	context func(ws, id uuid.UUID) []byte
}

func sealedIn(table, column, idColumn string, context func(ws, id uuid.UUID) []byte) sealedColumn {
	return sealedColumn{store.SealedColumn{Table: table, Column: column, WorkspaceColumn: "workspace_id", IDColumn: idColumn}, context}
}

func perWorkspace(context func(ws uuid.UUID) []byte) func(ws, id uuid.UUID) []byte {
	return func(ws, _ uuid.UUID) []byte { return context(ws) }
}

// sealedColumns is every column that holds a value sealed under the master key, with the context
// it was sealed with.
var sealedColumns = []sealedColumn{
	sealedIn("inboxes", "identity_secret", "id", identitySecretContext),
	sealedIn("email_channels", "smtp_password", "channel_id", smtpSecretContext),
	sealedIn("email_channels", "broadcast_smtp_password", "channel_id", broadcastSMTPSecretContext),
	sealedIn("email_channels", "dkim_private_key", "channel_id", dkimSecretContext),
	sealedIn("webhook_endpoints", "secret", "id", webhookSecretContext),
	sealedIn("webhook_endpoints", "previous_secret", "id", webhookSecretContext),
	sealedIn("status_monitors", "http_headers", "id", headerSecretContext),
	sealedIn("migration_credentials", "sealed", "migration_id", migrationSecretContext),
	sealedIn("migration_sign_ins", "sealed", "migration_id", migrationSecretContext),
	sealedIn("analytics_properties", "visitor_key", "id", analyticsSecretContext),
	sealedIn("help_imports", "sealed", "id", helpImportSecretContext),
	sealedIn("slack_installations", "bot_token", "workspace_id", perWorkspace(slackTokenContext)),
	sealedIn("sso_settings", "oidc_client_secret", "id", ssoSecretContext),
	sealedIn("issue_integrations", "token", "id", issueTokenContext),
	sealedIn("issue_integrations", "webhook_secret", "id", issueSecretContext),
	sealedIn("app_push_settings", "apns_key", "channel_id", apnsSecretContext),
	sealedIn("app_push_settings", "fcm_credentials", "channel_id", fcmSecretContext),
	sealedIn("workspace_keys", "optout_key", "workspace_id", perWorkspace(optOutKeyContext)),
}

const optOutKeyInfo = "yuva email optout v1 "

var errOptOutKey = errors.New("the opt-out key does not open with the master key")

func optOutKeyContext(ws uuid.UUID) []byte { return append([]byte("optout-key:"), ws[:]...) }

type optOutKeyEntry struct {
	key  []byte
	kept bool
}

// optOutKey is the key a workspace's opt-out hashes are made with. The hashes keep no address and
// cannot be made again under a new master key, so the key is kept, sealed, the first time it is
// needed for a new opt-out (keep) and survives rotations; until then it is the one derived from the
// oldest master key in use, which made the hashes recorded before keys were kept. Only a key read
// back as kept counts as kept in the cache: one kept in the caller's transaction may roll back.
func (s *Server) optOutKey(ctx context.Context, q *store.Queries, ws uuid.UUID, keep bool) ([]byte, error) {
	if e, ok := s.optKeys.Load(ws); ok && (e.(optOutKeyEntry).kept || !keep) {
		return e.(optOutKeyEntry).key, nil
	}
	sealed, err := q.GetOptOutKey(ctx, ws)
	kept := err == nil
	if store.IsNotFound(err) {
		key := s.secrets.DeriveFirst(optOutKeyInfo + ws.String())
		if !keep {
			s.optKeys.Store(ws, optOutKeyEntry{key: key})
			return key, nil
		}
		err = q.KeepOptOutKey(ctx, store.KeepOptOutKeyParams{WorkspaceID: ws, OptoutKey: s.secrets.Seal(key, optOutKeyContext(ws)), Now: s.now()})
		if err == nil {
			sealed, err = q.GetOptOutKey(ctx, ws)
		}
	}
	if err != nil {
		return nil, err
	}
	key, err := s.secrets.Open(sealed, optOutKeyContext(ws))
	if err != nil {
		return nil, fmt.Errorf("%w of workspace %s: %w", errOptOutKey, ws, err)
	}
	s.optKeys.Store(ws, optOutKeyEntry{key: key, kept: kept})
	return key, nil
}

// anyDerived reports whether match holds for the key for info under the current or the previous
// master key: values made before a rotation keep checking out until the previous key is removed.
func (s *Server) anyDerived(info string, match func(key []byte) bool) bool {
	return slices.ContainsFunc(s.secrets.Derived(info), match)
}

func optOutHash(key []byte, email string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(strings.ToLower(email)))
	return mac.Sum(nil)
}

// SealedReport counts one column's values: under the current master key, re-sealed from the
// previous one (or, for a check, still under it), and opening with neither.
type SealedReport struct {
	Column     string
	Current    int
	Previous   int
	Unreadable []string
}

type MasterKeyReport struct {
	Columns []SealedReport
	// UnkeptOptOutKeys counts workspaces with opt-outs whose key is not kept yet.
	UnkeptOptOutKeys int
}

func (r MasterKeyReport) Clean() bool {
	for _, c := range r.Columns {
		if c.Previous > 0 || len(c.Unreadable) > 0 {
			return false
		}
	}
	return r.UnkeptOptOutKeys == 0
}

const masterKeyWorkspacePage = 500

func (s *Server) eachWorkspace(ctx context.Context, fn func(ws uuid.UUID) error) error {
	after := uuid.Nil()
	for {
		ids, err := s.st.ListWorkspaceIDsAfter(ctx, store.ListWorkspaceIDsAfterParams{After: after, Lim: masterKeyWorkspacePage})
		if err != nil {
			return err
		}
		for _, ws := range ids {
			if err := fn(ws); err != nil {
				return err
			}
		}
		if len(ids) < masterKeyWorkspacePage {
			return nil
		}
		after = ids[len(ids)-1]
	}
}

// RotateMasterKey re-seals, workspace by workspace in batches, every value that opens only with the
// previous master key, after keeping every workspace's opt-out key. A value the server changed
// meanwhile is left as it is: it is already under the current key. Running it again picks up what is
// left. progress gets each column's report when the column is done.
func (s *Server) RotateMasterKey(ctx context.Context, batch int, progress func(SealedReport)) (MasterKeyReport, error) {
	if !s.secrets.HasPrevious() {
		return MasterKeyReport{}, errors.New("YUVA_MASTER_KEY_PREVIOUS is not set: there is no previous master key to re-seal from")
	}
	err := s.eachWorkspace(ctx, func(ws uuid.UUID) error {
		_, err := s.st.GetOptOutKey(ctx, ws)
		if store.IsNotFound(err) {
			key := s.secrets.DeriveFirst(optOutKeyInfo + ws.String())
			err = s.st.KeepOptOutKey(ctx, store.KeepOptOutKeyParams{WorkspaceID: ws, OptoutKey: s.secrets.Seal(key, optOutKeyContext(ws)), Now: s.now()})
		}
		return err
	})
	if err != nil {
		return MasterKeyReport{}, err
	}
	return s.scanSealed(ctx, batch, true, progress)
}

// CheckMasterKey reports, without writing, every value that does not open with the current master
// key alone and every workspace whose opt-out key is not kept yet.
func (s *Server) CheckMasterKey(ctx context.Context, batch int, progress func(SealedReport)) (MasterKeyReport, error) {
	return s.scanSealed(ctx, batch, false, progress)
}

func (s *Server) scanSealed(ctx context.Context, batch int, write bool, progress func(SealedReport)) (MasterKeyReport, error) {
	if batch <= 0 {
		batch = 500
	}
	var out MasterKeyReport
	for _, c := range sealedColumns {
		rep := SealedReport{Column: c.Table + "." + c.Column}
		err := s.eachWorkspace(ctx, func(ws uuid.UUID) error {
			after := uuid.Nil()
			for {
				page, err := s.st.SealedPage(ctx, c.SealedColumn, ws, after, batch)
				if err != nil {
					return err
				}
				for _, v := range page {
					next, stale, err := s.secrets.Reseal(v.Sealed, c.context(ws, v.ID))
					switch {
					case err != nil:
						rep.Unreadable = append(rep.Unreadable, "workspace "+ws.String()+" row "+v.ID.String())
					case !stale:
						rep.Current++
					case !write:
						rep.Previous++
					default:
						replaced, err := s.st.ReplaceSealed(ctx, c.SealedColumn, ws, v.ID, v.Sealed, next)
						if err != nil {
							return err
						}
						if replaced {
							rep.Previous++
						} else {
							rep.Current++
						}
					}
				}
				if len(page) < batch {
					return nil
				}
				after = page[len(page)-1].ID
			}
		})
		if err != nil {
			return out, fmt.Errorf("%s: %w", rep.Column, err)
		}
		out.Columns = append(out.Columns, rep)
		if progress != nil {
			progress(rep)
		}
	}
	err := s.eachWorkspace(ctx, func(ws uuid.UUID) error {
		unkept, err := s.st.HasOptOutsWithoutKey(ctx, ws)
		if unkept {
			out.UnkeptOptOutKeys++
		}
		return err
	})
	return out, err
}

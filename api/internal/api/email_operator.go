package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/store"
)

// OperatorChannel is an e-mail channel as the operator's command line names it.
type OperatorChannel struct {
	WorkspaceID, ChannelID uuid.UUID
	Name, Address          string
	Verified               bool
}

var (
	ErrProvenElsewhere = errors.New("another workspace proved this address; it proves it again only through its own channel")
	ErrSharedCatchAll  = errors.New("a catch-all on a shared mail domain (the forwarding domain or a public mail provider) cannot be proven")
)

// OperatorEmailChannels finds e-mail channels by id or address, in workspace when it is set, or
// every unproven channel of workspace with allUnproven.
func (s *Server) OperatorEmailChannels(ctx context.Context, workspace *uuid.UUID, ref string, allUnproven bool) ([]OperatorChannel, error) {
	arg := store.OperatorEmailChannelsParams{WorkspaceID: workspace, AllUnproven: allUnproven}
	if ref != "" {
		if id, err := uuid.Parse(ref); err == nil {
			arg.ChannelID = &id
		} else {
			addr := strings.ToLower(strings.TrimSpace(ref))
			arg.Address = &addr
		}
	}
	rows, err := s.st.OperatorEmailChannels(ctx, arg)
	if err != nil {
		return nil, err
	}
	out := make([]OperatorChannel, len(rows))
	for i, r := range rows {
		out[i] = OperatorChannel{WorkspaceID: r.WorkspaceID, ChannelID: r.ChannelID, Name: r.Name, Address: r.Address, Verified: r.Verified}
	}
	return out, nil
}

// OperatorProveCheck says why the operator cannot vouch for the channel's address, or nil.
func (s *Server) OperatorProveCheck(ctx context.Context, c OperatorChannel) error {
	return s.operatorProveCheck(ctx, s.st.Queries, c.WorkspaceID, c.Address)
}

func (s *Server) operatorProveCheck(ctx context.Context, q *store.Queries, ws uuid.UUID, address string) error {
	if isCatchAll(address) && s.sharedMailDomain(email.Domain(address)) {
		return ErrSharedCatchAll
	}
	taken, err := q.EmailAddressProvenElsewhere(ctx, store.EmailAddressProvenElsewhereParams{Address: address, WorkspaceID: ws})
	if err != nil {
		return err
	}
	if taken {
		return ErrProvenElsewhere
	}
	return nil
}

// OperatorProveChannel records the operator's word that the channel's address belongs to its
// workspace, under the same rules as a proof: never a shared catch-all, never an address another
// workspace proved.
func (s *Server) OperatorProveChannel(ctx context.Context, c OperatorChannel) error {
	now := s.now()
	p := principal{workspaceID: c.WorkspaceID}
	var losses []addressLoss
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.LockEmailChannel(ctx, store.LockEmailChannelParams{WorkspaceID: c.WorkspaceID, ChannelID: c.ChannelID})
		if store.IsNotFound(err) {
			return fmt.Errorf("channel %s is gone", c.ChannelID)
		}
		if err != nil {
			return err
		}
		if cur.VerifiedAt != nil {
			return nil
		}
		if err := q.LockEmailAddress(ctx, cur.Address); err != nil {
			return err
		}
		if err := s.operatorProveCheck(ctx, q, cur.WorkspaceID, cur.Address); err != nil {
			return err
		}
		_, err = s.proveAddress(ctx, q, &p, cur, verifiedByOperator, now, &losses)
		return err
	})
	if err != nil {
		return err
	}
	s.notifyAddressLost(ctx, losses)
	return nil
}

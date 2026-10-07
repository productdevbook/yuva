package realtime

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/productdevbook/yuva/api/internal/store"
)

func Listen(ctx context.Context, dsn string, q *store.Queries, hub *Hub, log *slog.Logger) {
	const maxBackoff = 30 * time.Second
	backoff := 500 * time.Millisecond
	for ctx.Err() == nil {
		listening, err := listenOnce(ctx, dsn, q, hub)
		if ctx.Err() != nil {
			return
		}
		if listening {
			backoff = 500 * time.Millisecond
		}
		log.Warn("realtime listener stopped; reconnecting", slog.Any("error", err), slog.Duration("backoff", backoff))
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

func listenOnce(ctx context.Context, dsn string, q *store.Queries, hub *Hub) (bool, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return false, err
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, "LISTEN "+Channel); err != nil {
		return false, err
	}
	// Connections that subscribed while nothing was listening may have missed events; they resume.
	hub.StopAll(ReasonRestart)
	hub.listening.Store(true)
	defer hub.listening.Store(false)
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return true, err
		}
		workspaceID, id, err := parsePayload(n.Payload)
		if err != nil {
			continue
		}
		row, err := q.GetEvent(ctx, store.GetEventParams{WorkspaceID: workspaceID, ID: id})
		if store.IsNotFound(err) {
			continue
		}
		if err != nil {
			return true, fmt.Errorf("load event %d: %w", id, err)
		}
		hub.Publish(FromRow(row))
	}
}

func parsePayload(p string) (uuid.UUID, int64, error) {
	ws, idPart, ok := strings.Cut(p, ":")
	if !ok {
		return uuid.UUID{}, 0, fmt.Errorf("bad payload %q", p)
	}
	workspaceID, err := uuid.Parse(ws)
	if err != nil {
		return uuid.UUID{}, 0, err
	}
	id, err := strconv.ParseInt(idPart, 10, 64)
	return workspaceID, id, err
}

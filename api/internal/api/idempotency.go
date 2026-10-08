package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	idempotencyHeader   = "Idempotency-Key"
	replayedHeader      = "Idempotent-Replayed"
	idempotencyMemBody  = 1 << 20
	idempotencyMaxReply = 1 << 20

	callerAPIKey  = "api_key"
	callerMember  = "member"
	callerContact = "contact"
)

var (
	errIdempotencyKey    = errValidation("Idempotency-Key must be 1 to 255 printable ASCII characters")
	errIdempotencyReused = problem(http.StatusConflict, "idempotency_key_reused", "this Idempotency-Key was used for a different request")
	errIdempotencyInUse  = problem(http.StatusConflict, "idempotency_key_in_use", "a request with this Idempotency-Key is still running")
	replayHeaders        = []string{"Content-Type", "Location"}
)

const idempotencyStateCtxKey ctxKey = iota + 200

type idempotencyClaim struct {
	workspaceID uuid.UUID
	callerType  string
	callerID    uuid.UUID
}

type idempotencyState struct {
	key   string
	body  *hashedBody
	claim *idempotencyClaim
}

// hashedBody hashes the request body as the handler reads it; finish reads the rest so the hash
// is complete before the handler runs, and serves those bytes afterwards.
type hashedBody struct {
	src  io.ReadCloser
	h    hash.Hash
	rest io.Reader
	file *os.File
}

func (b *hashedBody) Read(p []byte) (int, error) {
	if b.rest != nil {
		return b.rest.Read(p)
	}
	n, err := b.src.Read(p)
	b.h.Write(p[:n])
	return n, err
}

func (b *hashedBody) Close() error {
	if b.file != nil {
		b.file.Close()
		os.Remove(b.file.Name())
	}
	return b.src.Close()
}

func (b *hashedBody) finish() ([]byte, error) {
	if b.rest != nil {
		return nil, errors.New("body already finished")
	}
	var buf bytes.Buffer
	_, err := io.CopyN(io.MultiWriter(&buf, b.h), b.src, idempotencyMemBody)
	switch {
	case errors.Is(err, io.EOF):
		b.rest = &buf
		return b.h.Sum(nil), nil
	case err != nil:
		b.rest = io.MultiReader(&buf, errorReader{err})
		return nil, err
	}
	f, err := os.CreateTemp("", "yuva-idempotency-*")
	if err != nil {
		b.rest = io.MultiReader(&buf, errorReader{err})
		return nil, err
	}
	b.file = f
	if _, err = io.Copy(io.MultiWriter(f, b.h), b.src); err == nil {
		_, err = f.Seek(0, io.SeekStart)
	}
	if err != nil {
		b.rest = io.MultiReader(&buf, errorReader{err})
		return nil, err
	}
	b.rest = io.MultiReader(&buf, f)
	return b.h.Sum(nil), nil
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

type replyRecorder struct {
	http.ResponseWriter
	status   int
	body     bytes.Buffer
	overflow bool
}

func (r *replyRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *replyRecorder) Write(p []byte) (int, error) {
	if r.body.Len()+len(p) > idempotencyMaxReply {
		r.overflow = true
	} else {
		r.body.Write(p)
	}
	return r.ResponseWriter.Write(p)
}

func (r *replyRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func validIdempotencyKey(k string) bool {
	if len(k) < 1 || len(k) > 255 {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] < 0x20 || k[i] > 0x7e {
			return false
		}
	}
	return true
}

// idempotency records the answer to a POST with an Idempotency-Key once authenticate has claimed
// the key for its caller.
func (s *Server) idempotency(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, present := r.Header[idempotencyHeader]
		if r.Method != http.MethodPost || !present ||
			!(strings.HasPrefix(r.URL.Path, "/v1/") || strings.HasPrefix(r.URL.Path, "/client/v1/")) {
			next.ServeHTTP(w, r)
			return
		}
		body := &hashedBody{src: r.Body, h: sha256.New()}
		r.Body = body
		st := &idempotencyState{key: strings.Join(key, ","), body: body}
		rec := &replyRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if st.claim == nil {
				return
			}
			ctx := context.WithoutCancel(r.Context())
			v := recover()
			if v != nil || rec.status >= 500 || rec.overflow {
				s.releaseIdempotency(ctx, st)
			} else {
				s.completeIdempotency(ctx, st, rec)
			}
			if v != nil {
				panic(v)
			}
		}()
		next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), idempotencyStateCtxKey, st)))
	})
}

func (s *Server) releaseIdempotency(ctx context.Context, st *idempotencyState) {
	c := st.claim
	if err := s.st.ReleaseIdempotencyKey(ctx, store.ReleaseIdempotencyKeyParams{
		WorkspaceID: c.workspaceID, CallerType: c.callerType, CallerID: c.callerID, Key: st.key,
	}); err != nil {
		s.log.ErrorContext(ctx, "release idempotency key", slog.Any("error", err))
	}
}

func (s *Server) completeIdempotency(ctx context.Context, st *idempotencyState, rec *replyRecorder) {
	c := st.claim
	headers := map[string]string{}
	for _, h := range replayHeaders {
		if v := rec.Header().Get(h); v != "" {
			headers[h] = v
		}
	}
	if err := s.st.CompleteIdempotencyKey(ctx, store.CompleteIdempotencyKeyParams{
		WorkspaceID: c.workspaceID, CallerType: c.callerType, CallerID: c.callerID, Key: st.key,
		Status: new(int32(rec.status)), Headers: mustJSON(headers), Body: rec.body.Bytes(),
	}); err != nil {
		s.log.ErrorContext(ctx, "store idempotent response", slog.Any("error", err))
	}
}

// claimIdempotency runs once the caller is known: it claims the request's Idempotency-Key, or
// answers with the stored response and reports true.
func (s *Server) claimIdempotency(ctx context.Context, w http.ResponseWriter, r *http.Request, workspaceID uuid.UUID, callerType string, callerID uuid.UUID) (bool, error) {
	st, _ := ctx.Value(idempotencyStateCtxKey).(*idempotencyState)
	if st == nil || st.claim != nil {
		return false, nil
	}
	if !validIdempotencyKey(st.key) {
		return false, errIdempotencyKey
	}
	sum, err := st.body.finish()
	if err != nil {
		return false, nil
	}
	now := s.now()
	n, err := s.st.ClaimIdempotencyKey(ctx, store.ClaimIdempotencyKeyParams{
		WorkspaceID: workspaceID, CallerType: callerType, CallerID: callerID, Key: st.key,
		Method: r.Method, Path: r.URL.Path, BodySha256: sum, Now: now, StaleBefore: now.Add(-jobs.IdempotencyRetention),
	})
	if err != nil {
		return false, err
	}
	claim := &idempotencyClaim{workspaceID: workspaceID, callerType: callerType, callerID: callerID}
	if n == 1 {
		st.claim = claim
		return false, nil
	}
	prev, err := s.st.GetIdempotencyKey(ctx, store.GetIdempotencyKeyParams{
		WorkspaceID: workspaceID, CallerType: callerType, CallerID: callerID, Key: st.key,
	})
	if err != nil {
		return false, err
	}
	if prev.Method != r.Method || prev.Path != r.URL.Path || !bytes.Equal(prev.BodySha256, sum) {
		return false, errIdempotencyReused
	}
	if prev.Status == nil {
		return false, errIdempotencyInUse
	}
	var headers map[string]string
	_ = json.Unmarshal(prev.Headers, &headers)
	for k, v := range headers {
		w.Header().Set(k, v)
	}
	w.Header().Set(replayedHeader, "true")
	w.WriteHeader(int(*prev.Status))
	_, _ = w.Write(prev.Body)
	return true, nil
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpUsage = "usage: yuva mcp stdio --url <server> --key <key>   (or YUVA_URL, YUVA_API_KEY)"

func mcpCLI(ctx context.Context, args []string, stdin io.ReadCloser, stdout io.WriteCloser, stderr io.Writer, log *slog.Logger) error {
	if len(args) == 0 || args[0] != "stdio" {
		return errors.New(mcpUsage)
	}
	fs := flag.NewFlagSet("mcp stdio", flag.ContinueOnError)
	fs.SetOutput(stderr)
	server := fs.String("url", os.Getenv("YUVA_URL"), "the Yuva server, e.g. https://yuva.example.com (env YUVA_URL)")
	key := fs.String("key", os.Getenv("YUVA_API_KEY"), "an API key or OAuth access token (env YUVA_API_KEY)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New(mcpUsage)
	}
	endpoint, err := mcpEndpoint(*server)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*key) == "" {
		return errors.New("--key or YUVA_API_KEY is required")
	}
	return bridgeMCP(ctx, endpoint, strings.TrimSpace(*key), stdin, stdout, log)
}

func mcpEndpoint(server string) (string, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return "", errors.New("--url or YUVA_URL is required")
	}
	u, err := url.Parse(server)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", fmt.Errorf("--url %q is not an http or https URL", server)
	}
	u.RawQuery, u.Fragment = "", ""
	u.Path = strings.TrimSuffix(u.Path, "/")
	if !strings.HasSuffix(u.Path, "/mcp") {
		u.Path += "/mcp"
	}
	return u.String(), nil
}

// bridgeMCP relays JSON-RPC messages between a stdio client and a server's Streamable HTTP
// endpoint without interpreting them, so every protocol version the server speaks passes through.
func bridgeMCP(ctx context.Context, endpoint, key string, stdin io.ReadCloser, stdout io.WriteCloser, log *slog.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	rt := &bridgeRoundTripper{key: key, base: http.DefaultTransport, log: log}
	remote, err := (&mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           &http.Client{Transport: rt},
		DisableStandaloneSSE: true,
	}).Connect(ctx)
	if err != nil {
		return err
	}
	defer remote.Close()
	local, err := (&mcp.IOTransport{Reader: stdin, Writer: stdout}).Connect(ctx)
	if err != nil {
		return err
	}
	defer local.Close()

	var (
		mu          sync.Mutex
		initializes = map[jsonrpc.ID]bool{}
		writes      sync.WaitGroup
	)
	done := make(chan error, 2)
	go func() {
		for {
			msg, err := local.Read(ctx)
			if err != nil {
				if errors.Is(err, io.EOF) || ctx.Err() != nil {
					err = nil
				}
				done <- err
				return
			}
			req, _ := msg.(*jsonrpc.Request)
			if req != nil && req.IsCall() && req.Method == "initialize" {
				mu.Lock()
				initializes[req.ID] = true
				mu.Unlock()
			}
			writes.Add(1)
			go func() {
				defer writes.Done()
				err := remote.Write(ctx, msg)
				if err == nil || ctx.Err() != nil {
					return
				}
				log.Error("mcp request failed", slog.String("endpoint", endpoint), slog.Any("error", err))
				if req == nil || !req.IsCall() {
					return
				}
				reply := &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: err.Error()}
				if wire := (*jsonrpc.Error)(nil); errors.As(err, &wire) {
					reply.Code, reply.Data = wire.Code, wire.Data
				}
				if err := local.Write(ctx, &jsonrpc.Response{ID: req.ID, Error: reply}); err != nil {
					log.Error("write to stdout", slog.Any("error", err))
				}
			}()
		}
	}()
	go func() {
		for {
			msg, err := remote.Read(ctx)
			if err != nil {
				writes.Wait()
				done <- err
				return
			}
			if resp, ok := msg.(*jsonrpc.Response); ok && resp.Error == nil {
				mu.Lock()
				isInit := initializes[resp.ID]
				delete(initializes, resp.ID)
				mu.Unlock()
				if isInit {
					var res struct {
						ProtocolVersion string `json:"protocolVersion"`
					}
					if json.Unmarshal(resp.Result, &res) == nil {
						rt.setProtocolVersion(res.ProtocolVersion)
					}
				}
			}
			if err := local.Write(ctx, msg); err != nil {
				done <- err
				return
			}
		}
	}()
	return <-done
}

type bridgeRoundTripper struct {
	key  string
	base http.RoundTripper
	log  *slog.Logger

	mu              sync.Mutex
	protocolVersion string
}

func (t *bridgeRoundTripper) setProtocolVersion(v string) {
	t.mu.Lock()
	t.protocolVersion = v
	t.mu.Unlock()
}

// RoundTrip adds the key, and the version negotiated by initialize, which the SDK's
// connection only learns when it runs the client session itself.
func (t *bridgeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.key)
	req.Header.Set("User-Agent", "yuva-mcp-stdio/"+buildVersion())
	t.mu.Lock()
	if v := t.protocolVersion; v != "" && req.Header.Get("Mcp-Protocol-Version") == "" {
		req.Header.Set("Mcp-Protocol-Version", v)
	}
	t.mu.Unlock()
	resp, err := t.base.RoundTrip(req)
	if err == nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
		t.log.Error("the server refused the key", slog.String("url", req.URL.String()), slog.Int("status", resp.StatusCode), slog.String("www_authenticate", resp.Header.Get("WWW-Authenticate")))
	}
	return resp, err
}

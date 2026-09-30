package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

// refusingCancelServer is an MCP server with one slow tool and one fast one
// that answers 400 to "notifications/cancelled" — what Grafana's MCP server
// was seen doing after a Loki query outlived its call.
func refusingCancelServer(t *testing.T) string {
	t.Helper()
	release := make(chan struct{})

	server := mcp.NewServer(&mcp.Implementation{Name: "grafana", Version: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "slow"},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			// Bounded, because a server that never hears the cancellation keeps
			// running the call, and closing the session waits for it.
			select {
			case <-ctx.Done():
			case <-release:
			case <-time.After(time.Second):
			}
			return &mcp.CallToolResult{}, nil, nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: "fast"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
		})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if bytes.Contains(body, []byte(`"notifications/cancelled"`)) {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(remote.Close)
	// Registered after remote.Close so it runs first: Close waits for the
	// slow handler, which only returns once released.
	t.Cleanup(func() { close(release) })
	return remote.URL
}

func TestConnectServer_aRefusedCancellation_leavesTheSessionUsable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	server := domain.MCPServer{
		Name: "grafana", Transport: domain.TransportHTTP, URL: refusingCancelServer(t), Enabled: true,
	}
	if err := connectServer(ctx, catalog, server, domain.MCPCredentials{}, nil, nil,
		credentialPolicy{}, nil); err != nil {
		t.Fatalf("connectServer: %v", err)
	}
	t.Cleanup(func() { _ = catalog.Close() })

	slowCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	if _, err := catalog.Invoke(slowCtx, engine.Call{RunID: "run-1", Seq: 1, Tool: "grafana.slow"}); err == nil {
		t.Fatal("the slow call finished inside its deadline; the test did not exercise a cancellation")
	}

	res, err := catalog.Invoke(ctx, engine.Call{RunID: "run-1", Seq: 2, Tool: "grafana.fast"})
	if err != nil {
		t.Fatalf("the call after a refused cancellation failed: %v", err)
	}
	if res.Failed {
		t.Fatalf("the call after a refused cancellation reported failure: %+v", res)
	}
}

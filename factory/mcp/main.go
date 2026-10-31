// Command factory-mcp is the software factory's own MCP server: the QA role's
// record_qa_verdict and the Merger's merge_if_ready (NT-013 § Guardrails).
//
// It is the factory's tooling, not part of the platform, and ships separately
// from agentd. Everything it needs comes from the environment; see README.md.
package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fuseone/agents/factory/guard"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "factory-mcp:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := guard.LoadConfig(os.Getenv, os.ReadFile)
	if err != nil {
		return err
	}
	tokens, err := guard.NewAppTokenSource(guard.AppTokenOptions{
		Credentials: cfg.App, APIURL: cfg.APIURL, Repo: cfg.Repo,
	})
	if err != nil {
		return err
	}
	gh := guard.NewClient(guard.ClientOptions{APIURL: cfg.APIURL, Repo: cfg.Repo, Tokens: tokens})
	server, err := guard.NewServer(cfg, gh)
	if err != nil {
		return err
	}
	addr := os.Getenv("FACTORY_MCP_ADDR")
	if addr == "" {
		return server.Run(ctx, &mcp.StdioTransport{})
	}
	return serveHTTP(ctx, server, addr, os.Getenv("FACTORY_MCP_TOKEN"))
}

// serveHTTP serves the streamable HTTP transport behind a bearer token. A
// merge tool reachable without one is refused outright, not warned about.
func serveHTTP(ctx context.Context, server *mcp.Server, addr, token string) error {
	if token == "" {
		return fmt.Errorf("%w: FACTORY_MCP_TOKEN is required when FACTORY_MCP_ADDR is set", guard.ErrConfig)
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	srv := &http.Server{
		Addr:              addr,
		Handler:           requireBearer(token, handler),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errs := make(chan error, 1)
	go func() { errs <- srv.ListenAndServe() }()
	fmt.Fprintf(os.Stderr, "factory-mcp listening on %s\n", addr)

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		if err := <-errs; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

func requireBearer(want string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, []byte("Bearer "+want)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

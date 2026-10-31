// Command factory-mcp is the software factory's own MCP server: the QA role's
// record_qa_verdict and the Merger's merge_if_ready (NT-013 § Guardrails).
//
// It is the factory's tooling, not part of the platform, and ships separately
// from agentd. Everything it needs comes from the environment; see README.md.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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
	return serveHTTP(ctx, server, httpOptions{
		Addr:         addr,
		Token:        os.Getenv("FACTORY_MCP_TOKEN"),
		CertFile:     os.Getenv("FACTORY_MCP_TLS_CERT"),
		KeyFile:      os.Getenv("FACTORY_MCP_TLS_KEY"),
		ClientCAFile: os.Getenv("FACTORY_MCP_CLIENT_CA"),
	})
}

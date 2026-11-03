// Command factory-mcp is the software factory's own MCP server: the board
// tools every role moves cards with, the QA role's record_qa_verdict and the
// Merger's merge_if_ready (NT-013 § Guardrails, #213).
//
// It is the factory's tooling, not part of the platform, and ships separately
// from agentd. Everything it needs comes from the environment; see README.md.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/fuseone/agents/factory/guard"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if err := healthcheck(ctx, os.Getenv, http.DefaultClient); err != nil {
			fmt.Fprintln(os.Stderr, "factory-mcp:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(ctx, os.Getenv, os.ReadFile); err != nil {
		fmt.Fprintln(os.Stderr, "factory-mcp:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, getenv func(string) string, readFile func(string) ([]byte, error)) error {
	cfg, err := guard.LoadConfig(getenv, readFile)
	if err != nil {
		return err
	}
	opts := httpOptionsFromEnv(cfg, getenv)
	if cfg.Auth == guard.AuthPassThrough {
		return servePassThrough(ctx, cfg, opts)
	}
	tokens, err := guard.NewAppTokenSource(guard.AppTokenOptions{
		Credentials: cfg.App, APIURL: cfg.APIURL, Repo: cfg.Repo,
	})
	if err != nil {
		return err
	}
	server, err := guard.NewServer(cfg, cfg.Role, newClient(cfg, tokens))
	if err != nil {
		return err
	}
	if opts.Addr == "" {
		return server.Run(ctx, &mcp.StdioTransport{})
	}
	return serveHTTP(ctx, mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil), opts)
}

// servePassThrough serves every role FACTORY_ROLE_BOTS lists, each request
// getting the surface of the identity behind its bearer. It needs HTTP:
// over stdio there is no bearer to pass through.
func servePassThrough(ctx context.Context, cfg guard.Config, opts httpOptions) error {
	if opts.Addr == "" {
		return fmt.Errorf("%w: FACTORY_MCP_AUTH=github-app-token needs FACTORY_MCP_ADDR: "+
			"over stdio there is no bearer to pass through", guard.ErrConfig)
	}
	handler, err := guard.NewPassThroughHandler(cfg, newClient(cfg, guard.RequestTokenSource{}), guard.SystemClock{})
	if err != nil {
		return err
	}
	return serveHTTP(ctx, handler, opts)
}

func newClient(cfg guard.Config, tokens guard.TokenSource) *guard.Client {
	return guard.NewClient(guard.ClientOptions{
		APIURL: cfg.APIURL, GraphQLURL: cfg.GraphQLURL, Repo: cfg.Repo, Tokens: tokens,
	})
}

// httpOptionsFromEnv reads the transport. In pass-through mode the shared
// FACTORY_MCP_TOKEN is not read: the bearer is the caller's GitHub token.
func httpOptionsFromEnv(cfg guard.Config, getenv func(string) string) httpOptions {
	opts := httpOptions{
		Addr:         getenv("FACTORY_MCP_ADDR"),
		CertFile:     getenv("FACTORY_MCP_TLS_CERT"),
		KeyFile:      getenv("FACTORY_MCP_TLS_KEY"),
		ClientCAFile: getenv("FACTORY_MCP_CLIENT_CA"),
		PassThrough:  cfg.Auth == guard.AuthPassThrough,
	}
	if !opts.PassThrough {
		opts.Token = getenv("FACTORY_MCP_TOKEN")
	}
	return opts
}

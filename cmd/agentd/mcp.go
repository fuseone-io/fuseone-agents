package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/tools"
)

// toolRefresh bounds how long a tool server configured in the console takes to
// start offering its tools.
//
// It exists because the alternative is worse than slow: before it, a server
// added from the administration area did nothing at all until somebody
// restarted the worker, and nothing on the screen said so. An operator
// configured a server, saw no tools, and had no way to tell a wrong address
// from a process that had not read the change yet.
const toolRefresh = 30 * time.Second

// connectServer reaches one tool server and imports its tools.
//
// The two transports are a real difference, not a detail. A command with
// arguments is code this installation executes inside the worker's container;
// a URL is a request it sends. Both end at the same AddServer, so a tool is a
// tool once it is here — but only one of them is remote code execution by
// configuration, and the form that offers them says so.
func connectServer(
	ctx context.Context, catalog *tools.Catalog, server domain.MCPServer, token string,
) error {
	client := mcp.NewClient(&mcp.Implementation{Name: "fuseone-agents", Version: version}, nil)

	transport, err := transportFor(ctx, server, token)
	if err != nil {
		return err
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("connect %s: %w", server.Name, err)
	}
	if err := catalog.AddServer(ctx, server.Name, session); err != nil {
		_ = session.Close()
		return fmt.Errorf("import tools from %s: %w", server.Name, err)
	}
	return nil
}

func transportFor(
	ctx context.Context, server domain.MCPServer, token string,
) (mcp.Transport, error) {
	switch server.TransportOf() {
	case domain.TransportHTTP:
		return &mcp.StreamableClientTransport{
			Endpoint:   server.URL,
			HTTPClient: bearerClient(token),
		}, nil

	case domain.TransportStdio:
		fields := strings.Fields(server.Command)
		if len(fields) == 0 {
			return nil, fmt.Errorf("%s: no command to run", server.Name)
		}
		cmd := exec.CommandContext(ctx, fields[0], append(fields[1:], server.Args...)...)
		cmd.Stderr = os.Stderr
		return &mcp.CommandTransport{Command: cmd}, nil

	default:
		return nil, fmt.Errorf("%s: unknown transport %q", server.Name, server.Transport)
	}
}

// bearerClient carries the token on every request to a remote server.
//
// A client of its own rather than the shared default: the token belongs to one
// server, and a transport installed on http.DefaultClient would send it to
// everything this process talks to.
func bearerClient(token string) *http.Client {
	if token == "" {
		return nil
	}
	return &http.Client{Transport: bearer{token: token}, Timeout: 60 * time.Second}
}

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	// Cloned: RoundTrip must not modify the request it is given, and the same
	// request is retried by the transport underneath.
	out := r.Clone(r.Context())
	out.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(out)
}

// fingerprint is what makes a change detectable without storing the config
// twice. Two servers with the same name and different addresses are different
// servers, and the one that is connected has to be replaced.
func fingerprint(server domain.MCPServer) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		server.TransportOf(), server.Command, strings.Join(server.Args, " "), server.URL,
		fmt.Sprint(server.Enabled),
	}, "\x00")))
	return hex.EncodeToString(sum[:8])
}

// Servers is where the configured set is read from, declared here by the
// consumer.
type Servers interface {
	MCPServers(ctx context.Context) ([]domain.MCPServer, error)
	MCPToken(ctx context.Context, name string) (string, error)
}

// reconciler keeps the connected servers matching the configured ones.
type reconciler struct {
	catalog *tools.Catalog
	servers Servers
	health  healthRecorder
	// connected maps a server name to the fingerprint of what was connected
	// under it, so a changed address is noticed rather than assumed stable.
	connected map[string]string
}

func newReconciler(catalog *tools.Catalog, servers Servers, health healthRecorder) *reconciler {
	return &reconciler{
		catalog: catalog, servers: servers, health: health,
		connected: make(map[string]string),
	}
}

// hold marks a server as connected by something other than the reconciler —
// a --mcp flag. It is never disconnected here: the flag owns it, and the
// console cannot see or change it.
func (r *reconciler) hold(name string) { r.connected[name] = "flag" }

// reconcile connects what is newly configured and disconnects what is not.
func (r *reconciler) reconcile(ctx context.Context) {
	configured, err := r.servers.MCPServers(ctx)
	if err != nil {
		slog.Error("could not read the configured tool servers", "err", err)
		return
	}

	wanted := make(map[string]domain.MCPServer, len(configured))
	for _, server := range configured {
		if server.Enabled {
			wanted[server.Name] = server
		}
	}

	for name, mark := range r.connected {
		if mark == "flag" {
			continue
		}
		if server, still := wanted[name]; still && fingerprint(server) == mark {
			continue
		}
		// Gone, switched off, or pointing somewhere else. All three mean the
		// session in hand is not the one configured.
		if err := r.catalog.RemoveServer(name); err != nil {
			slog.Error("could not disconnect a tool server", "server", name, "err", err)
		}
		delete(r.connected, name)
	}

	for name, server := range wanted {
		if _, already := r.connected[name]; already {
			continue
		}
		r.connect(ctx, server)
	}
}

func (r *reconciler) connect(ctx context.Context, server domain.MCPServer) {
	token, err := r.servers.MCPToken(ctx, server.Name)
	if err != nil {
		slog.Error("tool server has no readable token", "server", server.Name, "err", err)
		return
	}

	if err := connectServer(ctx, r.catalog, server, token); err != nil {
		// Recorded and skipped, never fatal. One broken integration used to
		// mean nothing on the installation ran, including every agent that
		// never touches it.
		slog.Error("tool server did not answer; its tools are unavailable",
			"server", server.Name, "transport", server.TransportOf(), "err", err)
		observe(ctx, r.health, server.Name, false, 0, err.Error())
		return
	}

	r.connected[server.Name] = fingerprint(server)
	count := r.catalog.CountFrom(server.Name)
	slog.Info("tool server connected",
		"server", server.Name, "transport", server.TransportOf(), "tools", count)
	observe(ctx, r.health, server.Name, true, count, "")
}

// watch keeps reconciling until ctx is cancelled. Its caller owns it.
func (r *reconciler) watch(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

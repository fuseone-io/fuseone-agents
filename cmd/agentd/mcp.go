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
	ctx context.Context, catalog *tools.Catalog, server domain.MCPServer,
	creds domain.MCPCredentials,
) error {
	client := mcp.NewClient(&mcp.Implementation{Name: "fuseone-agents", Version: version}, nil)

	transport, err := transportFor(ctx, server, creds)
	if err != nil {
		return err
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("connect %s: %w", server.Name, err)
	}
	if err := catalog.AddServer(ctx, server.Name, session, server.Surface); err != nil {
		_ = session.Close()
		return fmt.Errorf("import tools from %s: %w", server.Name, err)
	}
	return nil
}

func transportFor(
	ctx context.Context, server domain.MCPServer, creds domain.MCPCredentials,
) (mcp.Transport, error) {
	switch server.TransportOf() {
	case domain.TransportHTTP:
		return &mcp.StreamableClientTransport{
			Endpoint:   server.URL,
			HTTPClient: bearerClient(creds.Token),
		}, nil

	case domain.TransportStdio:
		cmd, err := commandFor(ctx, server, creds)
		if err != nil {
			return nil, err
		}
		return &mcp.CommandTransport{Command: cmd}, nil

	default:
		return nil, fmt.Errorf("%s: unknown transport %q", server.Name, server.Transport)
	}
}

// bearerClient carries the token on every request to a remote server. A local
// server never sees it: it is a bearer for an address, and a program started
// inside the worker has no address to be a bearer for.
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

/*
fingerprint is what makes a change detectable without storing the config twice.

Two servers with the same name and different addresses are different servers,
and the one connected has to be replaced.

When it was last written counts too, and it is the part that is easy to leave
out because it is not part of the address. A rotated token changes nothing this
can see — same command, same URL, same flags — so the session in hand kept
being used with the credential that was replaced, until a restart or an
unrelated edit. A credential is rotated most urgently when it has leaked, which
is exactly when "it takes effect at the next deploy" is the wrong answer.

The timestamp rather than the credential itself. Reading every server's secret
on every reconcile pass would unseal the vault on a timer to answer a question
the row already answers.
*/
func fingerprint(server domain.MCPServer) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		server.TransportOf(), server.Command, strings.Join(server.Args, " "), server.URL,
		fmt.Sprint(server.Enabled), server.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(sum[:8])
}

// Servers is where the configured set is read from, declared here by the
// consumer.
type Servers interface {
	MCPServers(ctx context.Context) ([]domain.MCPServer, error)
	MCPCredentials(ctx context.Context, name string) (domain.MCPCredentials, error)
}

// Publisher records what the installation now offers, so the administration
// area shows the tools of a server that connected after start-up. Optional: a
// worker with no database still connects, it just has nowhere to publish to.
type Publisher interface {
	Publish(ctx context.Context, entries []domain.ToolEntry) error
}

// reconciler keeps the connected servers matching the configured ones.
type reconciler struct {
	catalog   *tools.Catalog
	servers   Servers
	health    healthRecorder
	publisher Publisher
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

// publishingTo wires where the catalogue is published after it changes.
func (r *reconciler) publishingTo(p Publisher) *reconciler {
	r.publisher = p
	return r
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

	changed := false
	for name, server := range wanted {
		if _, already := r.connected[name]; already {
			continue
		}
		r.connect(ctx, server)
		changed = true
	}

	r.refreshHealth(ctx)

	// Published after the pass rather than only at start-up. Without this a
	// server that connected later offered its tools to every agent and
	// appeared on no screen, so the one place an operator goes to classify a
	// new tool would not list it.
	if changed && r.publisher != nil {
		if err := r.publisher.Publish(ctx, r.catalog.Entries()); err != nil {
			slog.Error("could not publish the tool catalogue", "err", err)
		}
	}
}

// refreshHealth restates what is connected right now.
//
// Without it an observation only ever records the moment a server was reached,
// so a server connected by a process that has since died reads as reachable
// for ever — and the console has no way to tell a live integration from the
// ghost of one.
func (r *reconciler) refreshHealth(ctx context.Context) {
	for name := range r.connected {
		observe(ctx, r.health, name, true, r.catalog.CountFrom(name), "")
	}
}

func (r *reconciler) connect(ctx context.Context, server domain.MCPServer) {
	creds, err := r.servers.MCPCredentials(ctx, server.Name)
	if err != nil {
		slog.Error("tool server has no readable credential", "server", server.Name, "err", err)
		return
	}

	if err := connectServer(ctx, r.catalog, server, creds); err != nil {
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

/*
commandFor builds the local process, with an environment it was given rather
than one it inherited.

`exec.Cmd` with a nil `Env` hands the child everything the parent holds. For
this parent that is `DATABASE_URL` and `FUSEONE_MASTER_KEY`, so a tool server
somebody configured could open the database and unseal the vault without
calling a single tool: no Gate decision, no ledger step, nothing to audit. The
Gate governs what a tool may do and has never governed what a process may read.

Scrubbing does not make stdio safe. The child still runs as the worker, on the
worker's filesystem, from inside the worker's network — that is what stdio *is*,
and it is why the form that offers it has to say so. What this removes is the
platform handing over its own credentials to do it with.

An allowlist and not a denylist. A denylist is correct only until somebody adds
the next secret to the deployment, and then it is silently wrong.
*/
func commandFor(
	ctx context.Context, server domain.MCPServer, creds domain.MCPCredentials,
) (*exec.Cmd, error) {
	if !server.AcceptsLocalExecution {
		// Checked again here, and not only where it is written. A row can
		// arrive by restore, by migration, or from a version of this that did
		// not ask — and the moment a program is about to be started is the
		// moment the answer matters.
		return nil, fmt.Errorf(
			"%s: a local server runs code inside the worker, and nobody has accepted that for this one",
			server.Name)
	}
	fields := strings.Fields(server.Command)
	if len(fields) == 0 {
		return nil, fmt.Errorf("%s: no command to run", server.Name)
	}
	cmd := exec.CommandContext(ctx, fields[0], append(fields[1:], server.Args...)...)
	cmd.Stderr = os.Stderr
	/*
		The allowlist, and then what this server was configured with.

		Its own variables last so they win a collision: a server told to use a
		particular PATH means it. What this never does is reopen inheritance —
		a variable in neither list does not reach the child, however much the
		worker holds.

		They come from the vault rather than from the settings value, because
		the reason a server needs a variable is almost always that the variable
		is a key, and a field that is sometimes a secret is stored as one
		always.
	*/
	cmd.Env = append(childEnv(), creds.Environ()...)
	return cmd, nil
}

/*
carried are the variables a program needs to be a program.

None of them is a credential, and each earns its place: PATH so a runtime can
start its own children, HOME because npx and pip write caches there and fail
without it, TMPDIR for the same reason, and the locale and zone so a server
formats dates the way the rest of the installation does.

HOME deserves the objection it invites — it points at files the child could
read. It could read them anyway: it runs as the worker's user. HOME tells it
where they are; it grants nothing it did not already have.
*/
var carried = []string{"PATH", "HOME", "TMPDIR", "LANG", "LC_ALL", "TZ"}

// childEnv is never nil, because nil is the inheriting one.
func childEnv() []string {
	env := make([]string, 0, len(carried))
	for _, name := range carried {
		// Unset stays unset. `PATH=` is not "no PATH" — it is a PATH that
		// finds nothing, which fails in a way nobody reads as configuration.
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}

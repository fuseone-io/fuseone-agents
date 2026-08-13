package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- the model provider -----------------------------------------------------

// modelServer speaks chat-completions, so the real client is exercised over
// real HTTP: request encoding, tool-call parsing and usage accounting all run
// the production path. Only the model's judgement is scripted.
type modelServer struct {
	*httptest.Server

	mu    sync.Mutex
	turn  int
	seen  []chatRequest
	reply func(turn int) chatReply
	// down makes the provider unavailable, the way a real one is: the address
	// answers, and answers badly.
	down bool
}

// fail takes the provider offline, and restore brings it back.
func (m *modelServer) fail() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.down = true
}

func (m *modelServer) restore() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.down = false
}

type chatReply struct {
	// Tool and Args make the model ask for a tool call; Text finishes the run.
	Tool, Args, Text string
	PromptTokens     int64
	CompletionTokens int64
	CachedTokens     int64
	FinishReason     string
}

func newModelServer(t *testing.T, reply func(turn int) chatReply) *modelServer {
	t.Helper()
	m := &modelServer{reply: reply}

	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		m.mu.Lock()
		down := m.down
		turn := m.turn
		if !down {
			m.turn++
			m.seen = append(m.seen, req)
		}
		m.mu.Unlock()

		if down {
			http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(responseFor(m.reply(turn)))
	}))
	t.Cleanup(m.Close)
	return m
}

func (m *modelServer) requests() []chatRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]chatRequest(nil), m.seen...)
}

func responseFor(r chatReply) map[string]any {
	message := map[string]any{"role": "assistant"}
	if r.Tool != "" {
		message["tool_calls"] = []map[string]any{{
			"id": "call_1", "type": "function",
			"function": map[string]any{"name": r.Tool, "arguments": r.Args},
		}}
	} else {
		message["content"] = r.Text
	}

	finish := r.FinishReason
	if finish == "" {
		finish = "stop"
	}
	return map[string]any{
		"choices": []map[string]any{{"finish_reason": finish, "message": message}},
		"usage": map[string]any{
			"prompt_tokens":         r.PromptTokens,
			"completion_tokens":     r.CompletionTokens,
			"prompt_tokens_details": map[string]any{"cached_tokens": r.CachedTokens},
		},
	}
}

// chatRequest mirrors only what the assertions read back.
type chatRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Tools []struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tools"`
}

// --- the MCP server ---------------------------------------------------------

type lookupInput struct {
	Email string `json:"email" jsonschema:"the customer's email address"`
}

type lookupOutput struct {
	Account string `json:"account"`
}

type noteInput struct {
	Text string `json:"text" jsonschema:"the note to record"`
}

type noteOutput struct {
	NoteID string `json:"note_id"`
}

// unnoteInput is what takes a note back. The compensation path has to reach a
// real server too — a ledger saying "compensated" proves the platform decided
// to undo something, never that the note is gone.
type unnoteInput struct {
	NoteID string `json:"note_id" jsonschema:"the note to remove"`
}

type unnoteOutput struct {
	Removed bool `json:"removed"`
}

// serverCalls is what the MCP server actually did. The ledger records what the
// platform decided; only this records what reached the outside world.
type serverCalls struct {
	mu      sync.Mutex
	lookups []lookupInput
	notes   []noteInput
	unnotes []unnoteInput
}

func (c *serverCalls) Unnotes() []unnoteInput {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]unnoteInput(nil), c.unnotes...)
}

func (c *serverCalls) Lookups() []lookupInput {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]lookupInput(nil), c.lookups...)
}

func (c *serverCalls) Notes() []noteInput {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]noteInput(nil), c.notes...)
}

// mcpSession starts a real MCP server over the SDK's in-memory transport and
// returns a connected client session. Nothing here is a double: the catalogue
// discovers tools and invokes them exactly as it would against a subprocess.
func mcpSession(t *testing.T, calls *serverCalls) *mcp.ClientSession {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "crm", Version: "1.0.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "lookup",
		Description: "Find a customer by email",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in lookupInput) (*mcp.CallToolResult, lookupOutput, error) {
		calls.mu.Lock()
		defer calls.mu.Unlock()
		calls.lookups = append(calls.lookups, in)
		return nil, lookupOutput{Account: "acct_4471"}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "note",
		Description: "Record an internal note on the account",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in noteInput) (*mcp.CallToolResult, noteOutput, error) {
		calls.mu.Lock()
		defer calls.mu.Unlock()
		calls.notes = append(calls.notes, in)
		return nil, noteOutput{NoteID: "note_9"}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "unnote",
		Description: "Remove an internal note from the account",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in unnoteInput) (*mcp.CallToolResult, unnoteOutput, error) {
		calls.mu.Lock()
		defer calls.mu.Unlock()
		calls.unnotes = append(calls.unnotes, in)
		return nil, unnoteOutput{Removed: true}, nil
	})

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("connect MCP server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "fuseone", Version: "test"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect MCP client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	return session
}

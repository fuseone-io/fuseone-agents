// Package tools connects registered MCP servers to the agent loop.
//
// It is the only place the platform reaches outside itself, and it enforces
// the rule that makes open authoring safe: what a tool does to the world is
// decided centrally by the Curator, never by the agent's author and never by
// the server that supplies the tool (PRD DE-12).
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

/*
Calling a tool: the one place in this package where something happens to the
world.

Everything above it decides; this does. It is also where a result becomes a
claim check — the bytes go to the content store and the step carries a
reference — because a tool result is the payload most likely to hold personal
data (PRD AU-04).
*/
// Invoke calls a tool and returns what came back.
//
// The result never lands in the ledger: it goes to the content store and the
// ledger records a reference. Tool output routinely carries personal data, and
// inlining it would make retention impossible to honour (PRD AU-04).
func (c *Catalog) Invoke(ctx context.Context, call engine.Call) (engine.ToolResult, error) {
	c.mu.RLock()
	entry, known := c.entries[call.Tool]
	session, connected := c.sessions[entry.Server]
	timeout := c.timeout
	c.mu.RUnlock()

	if !known || !entry.OnSurface {
		// The model is not the only caller. A resumed run replays a call the
		// ledger holds and a specification names tools by hand, so a surface
		// enforced only where the schemas are written is a surface with a way
		// round it.
		return engine.ToolResult{}, fmt.Errorf("%w: %s", ErrUnknownTool, call.Tool)
	}
	if !connected {
		return engine.ToolResult{}, fmt.Errorf("%w: %s", ErrUnknownServer, entry.Server)
	}

	var args any
	if len(call.Args) > 0 {
		if err := json.Unmarshal(call.Args, &args); err != nil {
			return engine.ToolResult{}, fmt.Errorf("tools: arguments for %s are not valid JSON: %w", call.Tool, err)
		}
	}

	// A tool that never returns would hold a worker's slot until the lease
	// expires; bound it here rather than relying on the server to behave.
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res, err := session.CallTool(callCtx, &mcp.CallToolParams{
		Name:      entry.RemoteName,
		Arguments: args,
	})
	if err != nil {
		return engine.ToolResult{}, fmt.Errorf("tools: call %s: %w", call.Tool, err)
	}

	out := engine.ToolResult{Failed: res.IsError}
	if res.IsError {
		out.ErrorCode = "tool_error"
	}
	// Output from a source the Curator has not vouched for is tainted from the
	// moment it enters. Everything derived from it inherits the label, which
	// is what stops an attacker-authored document steering a later action.
	if entry.Untrusted {
		out.Labels = domain.NewLabels(domain.LabelUntrusted)
	}

	text := flatten(res)
	if c.content != nil && text != "" {
		ref, err := c.content.Put(ctx, call.RunID, call.Seq, []byte(text))
		if err != nil {
			return engine.ToolResult{}, fmt.Errorf("tools: store result of %s: %w", call.Tool, err)
		}
		out.ResultRef = ref
	}
	return out, nil
}

// flatten renders a tool result as text for the model.
func flatten(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, content := range res.Content {
		if t, ok := content.(*mcp.TextContent); ok {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(t.Text)
		}
	}
	if b.Len() == 0 && res.StructuredContent != nil {
		if raw, err := json.Marshal(res.StructuredContent); err == nil {
			b.Write(raw)
		}
	}
	return b.String()
}

// schemaProperties pulls the properties map out of a tool's JSON Schema. The
// model needs the field descriptions; the surrounding envelope it does not.
func schemaProperties(schema any) map[string]any {
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var decoded struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	return decoded.Properties
}

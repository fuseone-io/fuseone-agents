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

const toolInvokeErrorLimit = 4096
const toolInvokeErrorTruncated = "\n\n[truncated by FuseOne: the tool failure diagnostic had %d bytes and this installation stores %d]"

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
	cache := c.caches[entry.Server]
	timeout := c.timeout
	content := c.content
	metrics := c.metrics
	health := c.health
	healthBy := c.healthBy
	clock := c.clock
	c.mu.RUnlock()
	now := clockNow(clock)

	if !known {
		// The model is not the only caller. A resumed run replays a call the
		// ledger holds and a specification names tools by hand, so a surface
		// enforced only where the schemas are written is a surface with a way
		// round it.
		return engine.ToolResult{}, fmt.Errorf("%w: %s", ErrUnknownTool, call.Tool)
	}
	if !entry.OnSurface {
		recordMCPToolHealth(ctx, health, healthBy, entry.Server, false, CodeMCPUnknownTool, now)
		return engine.ToolResult{}, fmt.Errorf("%w: %s", ErrUnknownTool, call.Tool)
	}
	if !connected {
		recordMCPToolHealth(ctx, health, healthBy, entry.Server, false, CodeMCPUnknownServer, now)
		return engine.ToolResult{}, fmt.Errorf("%w: %s", ErrUnknownServer, entry.Server)
	}

	out := engine.ToolResult{}
	// Output from a source the Curator has not vouched for is tainted from the
	// moment it enters. Everything derived from it inherits the label, which
	// is what stops an attacker-authored document steering a later action.
	if entry.Untrusted {
		out.Labels = domain.NewLabels(domain.LabelUntrusted)
	}

	var args any
	if len(call.Args) > 0 {
		if err := json.Unmarshal(call.Args, &args); err != nil {
			return engine.ToolResult{}, fmt.Errorf("tools: arguments for %s are not valid JSON: %w", call.Tool, err)
		}
	}

	cacheKey := resultCacheKeyOf(entry, call)
	if resultCacheable(entry, call, content, cache) {
		if cached, ok := cache.get(cacheKey, now); ok {
			ref, err := content.Put(ctx, call.RunID, call.Seq, cached.body)
			if err != nil {
				return engine.ToolResult{}, fmt.Errorf("tools: store cached result of %s: %w", call.Tool, err)
			}
			out.ResultRef = ref
			out.ResultDigest = engine.ResultDigest(cached.body)
			out.ResultBytes = int64(len(cached.body))
			out.Labels = cached.labels.Clone()
			out.Cached = true
			out.CachedFromRun = cached.sourceRun
			out.CachedFromSeq = cached.sourceSeq
			recordMCPToolCall(metrics, "ok", CodeMCPCacheHit, true)
			return out, nil
		}
	}

	// A tool that never returns would hold a worker's slot until the lease
	// expires; bound it here rather than relying on the server to behave.
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	callCtx = WithInvocation(callCtx)
	callCtx = WithCaller(callCtx, call.OnBehalfOf)

	res, err := session.CallTool(callCtx, &mcp.CallToolParams{
		Name:      entry.RemoteName,
		Arguments: args,
	})
	if err != nil {
		code := failureCodeOf(err, CodeMCPInvokeError)
		out.Failed = true
		out.ErrorCode = code
		if content != nil {
			ref, storeErr := content.Put(ctx, call.RunID, call.Seq, invocationErrorText(call.Tool, code, err))
			if storeErr != nil {
				return engine.ToolResult{}, fmt.Errorf("tools: store failure for %s: %w", call.Tool, storeErr)
			}
			out.ResultRef = ref
		}
		recordMCPToolCall(metrics, "error", code, false)
		recordMCPToolHealth(ctx, health, healthBy, entry.Server, false, code, clockNow(clock))
		return out, fmt.Errorf("tools: call %s: %w", call.Tool, err)
	}

	out.Failed = res.IsError
	if res.IsError {
		out.ErrorCode = CodeMCPToolError
		recordMCPToolCall(metrics, "error", CodeMCPToolError, false)
		recordMCPToolHealth(ctx, health, healthBy, entry.Server, false, CodeMCPToolError, clockNow(clock))
	} else {
		recordMCPToolCall(metrics, "ok", CodeMCPNoCode, false)
		recordMCPToolHealth(ctx, health, healthBy, entry.Server, true, CodeMCPNoCode, clockNow(clock))
	}

	text := flatten(res)
	body := []byte(text)
	if !out.Failed {
		out.ResultDigest = engine.ResultDigest(body)
		out.ResultBytes = int64(len(body))
	}
	if content != nil && len(body) > 0 {
		ref, err := content.Put(ctx, call.RunID, call.Seq, body)
		if err != nil {
			return engine.ToolResult{}, fmt.Errorf("tools: store result of %s: %w", call.Tool, err)
		}
		out.ResultRef = ref
		if resultCacheable(entry, call, content, cache) && !out.Failed {
			cache.put(cacheKey, body, out.Labels, call.RunID, call.Seq, clockNow(clock))
		}
	}
	return out, nil
}

func recordMCPToolCall(metrics Metrics, result, code string, cached bool) {
	if metrics != nil {
		metrics.MCPToolCall(result, code, cached)
	}
}

func invocationErrorText(tool domain.ToolID, code string, err error) []byte {
	text := fmt.Sprintf("the tool failed: %s\n%s: %v", code, tool, err)
	if len(text) <= toolInvokeErrorLimit {
		return []byte(text)
	}
	notice := fmt.Sprintf(toolInvokeErrorTruncated, len(text), toolInvokeErrorLimit)
	if len(notice) >= toolInvokeErrorLimit {
		return []byte(notice[:toolInvokeErrorLimit])
	}
	return []byte(text[:toolInvokeErrorLimit-len(notice)] + notice)
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

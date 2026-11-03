package guard

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is reported to MCP clients in the implementation info.
const Version = "0.2.0"

// NewServer builds the MCP server for one role: exactly the tools of its
// surface (ToolsFor), so QA cannot merge, the Merger cannot vouch for QA,
// and only Triage and the Architect can set a card's fields.
func NewServer(cfg Config, role Role, gh *Client) (*mcp.Server, error) {
	tools := ToolsFor(role)
	if len(tools) == 0 {
		return nil, fmt.Errorf("%w: unknown role %q", ErrConfig, role)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "factory", Version: Version}, nil)
	board := NewBoard(cfg.Board, role, gh)
	for _, tool := range tools {
		switch tool {
		case ToolMoveCard:
			registerMoveCard(server, board)
		case ToolSetCardFields:
			registerSetCardFields(server, board)
		case ToolRecordQAVerdict:
			registerQA(server, NewQA(cfg, gh))
		case ToolMergeIfReady:
			registerMerger(server, NewMerger(cfg, gh))
		}
	}
	return server, nil
}

func registerMerger(server *mcp.Server, merger *Merger) {
	mcp.AddTool(server, &mcp.Tool{
		Name: ToolMergeIfReady,
		Description: "Merge a pull request only if, re-read now, the given head SHA is still its head " +
			"and carries an authorised approval, a passing factory/qa from the QA App and every " +
			"expected check green, with the base up to date. Returns structured state only.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in MergeInput) (*mcp.CallToolResult, MergeResult, error) {
		res, err := merger.MergeIfReady(callContext(ctx, req), in)
		if err != nil {
			return nil, MergeResult{}, err
		}
		return &mcp.CallToolResult{IsError: !res.Merged}, res, nil
	})
}

func registerQA(server *mcp.Server, qa *QA) {
	mcp.AddTool(server, &mcp.Tool{
		Name: ToolRecordQAVerdict,
		Description: "Record the QA verdict on a pull request as the factory/qa commit status " +
			"on exactly the given head SHA. Refuses if that SHA is no longer the PR's head.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in VerdictInput) (*mcp.CallToolResult, VerdictResult, error) {
		res, err := qa.Record(callContext(ctx, req), in)
		if err != nil {
			return nil, VerdictResult{}, err
		}
		return &mcp.CallToolResult{IsError: !res.Recorded}, res, nil
	})
}

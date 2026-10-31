package guard

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is reported to MCP clients in the implementation info.
const Version = "0.1.0"

// Tool names. FuseOne addresses a tool as <registered server name>.<tool>.
const (
	ToolRecordQAVerdict = "record_qa_verdict"
	ToolMergeIfReady    = "merge_if_ready"
)

// NewServer builds the MCP server for the configured role. Each role's
// instance offers exactly one tool: the QA instance cannot merge, and the
// Merger's instance cannot set a QA verdict.
func NewServer(cfg Config, gh *Client) (*mcp.Server, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: "factory", Version: Version}, nil)
	switch cfg.Role {
	case RoleQA:
		registerQA(server, NewQA(cfg, gh))
	case RoleMerger:
	default:
		return nil, fmt.Errorf("%w: unknown role %q", ErrConfig, cfg.Role)
	}
	return server, nil
}

func registerQA(server *mcp.Server, qa *QA) {
	mcp.AddTool(server, &mcp.Tool{
		Name: ToolRecordQAVerdict,
		Description: "Record the QA verdict on a pull request as the factory/qa commit status " +
			"on exactly the given head SHA. Refuses if that SHA is no longer the PR's head.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in VerdictInput) (*mcp.CallToolResult, VerdictResult, error) {
		res, err := qa.Record(ctx, in)
		if err != nil {
			return nil, VerdictResult{}, err
		}
		return &mcp.CallToolResult{IsError: !res.Recorded}, res, nil
	})
}

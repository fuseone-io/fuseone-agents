package guard

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is reported to MCP clients in the implementation info.
const Version = "0.1.0"

// NewServer builds the MCP server for the configured role. Each role's
// instance offers exactly one tool: the QA instance cannot merge, and the
// Merger's instance cannot set a QA verdict.
func NewServer(cfg Config, gh *Client) (*mcp.Server, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: "factory", Version: Version}, nil)
	switch cfg.Role {
	case RoleQA:
	case RoleMerger:
	default:
		return nil, fmt.Errorf("%w: unknown role %q", ErrConfig, cfg.Role)
	}
	return server, nil
}

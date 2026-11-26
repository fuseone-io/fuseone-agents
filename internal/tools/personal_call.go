package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
)

// toolCall is where a call goes, all of it read in one snapshot with the
// entry it is for.
type toolCall struct {
	server   string
	shared   Session
	personal *personalPool
	who      domain.UserID
}

// callTool runs the call on the person's session when the server has them,
// else on the shared one; either way a dead connection retires what failed.
func (c *Catalog) callTool(ctx context.Context, to toolCall, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	server, shared, pool, who := to.server, to.shared, to.personal, to.who
	if pool == nil || who == "" {
		res, err := shared.CallTool(ctx, params)
		if err != nil {
			c.retireIfDead(server, shared, err)
		}
		return res, err
	}
	slot, err := pool.acquire(ctx, who)
	if err != nil {
		return nil, fmt.Errorf("open %s's session on %s: %w", who, server, err)
	}
	defer pool.release(slot)
	res, err := slot.session.CallTool(ctx, params)
	if err != nil {
		pool.retire(who, slot, err)
	}
	return res, err
}

// clockOrNow is the catalogue's clock as a pool reads it: taken once, so a
// pool never reaches back into the catalogue's lock.
func (c *Catalog) clockOrNow() func() time.Time {
	clock := c.clock
	return func() time.Time { return clockNow(clock) }
}

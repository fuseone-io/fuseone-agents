package admin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// toolKind is the settings row a discovered tool is published as.
const toolKind = "tool"

// Publish records what a server offers, so the catalogue is installation state
// rather than something each process rediscovers.
//
// Discovery and ruling are kept apart on purpose: publishing never changes an
// effect. A server that calls itself a write tool tomorrow still arrives
// unclassified, and the ruling that promotes it stays the separate, attributed
// act it has to be.
//
// What publishing does carry is the digest of the definition it found, because
// that is what a recorded ruling is matched against below. Without it this
// copy of the catalogue would apply rulings by name while the in-process one
// applied them by definition, and the two would disagree about the row a
// Curator is looking at.
func (c *Curator) Publish(ctx context.Context, pub domain.ToolPublication) error {
	if len(pub.Entries) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, e := range pub.Entries {
		value, err := encodeTool(e)
		if err != nil {
			return err
		}
		batch.Queue(`
			insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_at)
			values ('installation', '', '', $1, $2, $3, true, now())
			on conflict (scope_kind, company_id, area_id, kind, name) do update set
				value = excluded.value, updated_at = now()`,
			toolKind, string(e.ID), value)
	}

	if err := c.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("admin: publish tools: %w", err)
	}
	return nil
}

func encodeTool(e domain.ToolEntry) ([]byte, error) {
	value, err := json.Marshal(struct {
		Server      string `json:"server"`
		Description string `json:"description,omitempty"`
		Digest      string `json:"digest,omitempty"`
		// Whether this installation brought it in. Published rather than
		// derived, because the console reads this copy and a screen that
		// showed every discovered tool as taken would be offering the choice
		// while hiding the answer.
		OnSurface bool `json:"onSurface"`
	}{e.Server, e.Description, e.Digest, e.OnSurface})
	if err != nil {
		return nil, fmt.Errorf("admin: encode tool %s: %w", e.ID, err)
	}
	return value, nil
}

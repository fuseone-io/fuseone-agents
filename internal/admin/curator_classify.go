package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
Writing a ruling, at the platform's level or the owner's (NT-019).

A tool has at most two rulings, both named by the tool id and told apart by
the scope they are stored at: the platform's, the ceiling, at the
installation, and the owner's, at the scope where the tool's server is stored.
For a server of the installation the two are one row.
*/

// Classify records the platform's ruling and the fact that somebody made it.
//
// EffectUnknown is refused rather than stored: it is the zero value that makes
// an unclassified tool fail closed, and writing it deliberately would look
// like a decision when it is the absence of one.
func (c *Curator) Classify(ctx context.Context, scope domain.Scope, ruling domain.ToolClassification) error {
	return c.storeRuling(ctx, rulingWrite{
		audit: scope, kind: settings.ScopeInstallation, ruling: ruling,
	})
}

/*
ClassifyAsOwner records the owner's ruling, at the scope of the server the
door authorized.

The ruling lands on that exact row or nowhere. The server's row is held at the
key the door read, inside the write's transaction: a server removed and stored
again under the same name in another scope since then answers
ErrMCPServerUnknown, and its new owner does not inherit a ruling made by
somebody authorized for the old one.
*/
func (c *Curator) ClassifyAsOwner(ctx context.Context, owner ToolOwner, ruling domain.ToolClassification) error {
	if !strings.HasPrefix(string(ruling.Tool), owner.Server+".") {
		return fmt.Errorf("admin: %s is not a tool of %s", ruling.Tool, owner.Server)
	}
	return c.storeRuling(ctx, rulingWrite{
		audit: owner.At.Audit, kind: owner.At.Kind, stored: owner.At.Stored, ruling: ruling,
		hold: func(ctx context.Context, tx settings.DB) error {
			_, err := lockMCPServerAt(ctx, tx, owner.At, owner.Server)
			return err
		},
	})
}

// rulingWrite is one ruling, where it is stored and recorded, and what must
// still hold when it is written.
type rulingWrite struct {
	audit  domain.Scope
	kind   settings.ScopeKind
	stored domain.Scope
	ruling domain.ToolClassification
	// hold runs first inside the transaction, and refuses the write when what
	// authorized it no longer holds.
	hold func(ctx context.Context, tx settings.DB) error
}

func (c *Curator) storeRuling(ctx context.Context, w rulingWrite) error {
	value, err := encodeRuling(w.ruling)
	if err != nil {
		return err
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if w.hold != nil {
		if err := w.hold(ctx, tx); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by, updated_at)
		values ($1, $2, $3, $4, $5, $6, true, $7, now())
		on conflict (scope_kind, company_id, area_id, kind, name) do update set
			value = excluded.value, updated_by = excluded.updated_by, updated_at = now()`,
		string(w.kind), string(w.stored.Company), string(w.stored.Area),
		classificationKind, string(w.ruling.Tool), value, string(w.ruling.By)); err != nil {
		return fmt.Errorf("admin: store ruling for %s: %w", w.ruling.Tool, err)
	}

	// In the same transaction as the change it describes. A ruling that took
	// effect without a record of who made it is exactly what the trail exists
	// to make impossible.
	if err := Record(ctx, tx, Event{
		Principal: w.ruling.By, Scope: w.audit, Action: "tool.classified",
		Target: string(w.ruling.Tool), Detail: json.RawMessage(value),
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("admin: commit ruling: %w", err)
	}
	return nil
}

// encodeRuling validates a ruling and renders it as it is stored.
func encodeRuling(ruling domain.ToolClassification) ([]byte, error) {
	if !ruling.Effect.Valid() {
		return nil, fmt.Errorf("%w: %s", ErrNotClassifiable, ruling.Effect)
	}
	if err := ruling.Dedupe.Validate(); err != nil {
		return nil, fmt.Errorf("%w: invalid dedupe for %s: %w", ErrNotClassifiable, ruling.Tool, err)
	}
	if ruling.Tool == "" {
		return nil, errors.New("admin: a ruling needs a tool")
	}
	value, err := json.Marshal(struct {
		Effect        string `json:"effect"`
		Untrusted     bool   `json:"untrusted"`
		Reason        string `json:"reason,omitempty"`
		CompensatedBy string `json:"compensated_by,omitempty"`
		Dedupe        *struct {
			WindowSeconds int      `json:"window_seconds"`
			ArgPaths      []string `json:"arg_paths"`
		} `json:"dedupe,omitempty"`
		// The definition this was a judgement about. Omitted when the caller
		// did not name one, which reads as a ruling from before this was kept
		// rather than as a ruling about nothing.
		Digest string `json:"digest,omitempty"`
	}{
		ruling.Effect.String(), ruling.Untrusted, ruling.Reason,
		string(ruling.CompensatedBy), storedDedupe(ruling.Dedupe), ruling.Digest,
	})
	if err != nil {
		return nil, fmt.Errorf("admin: encode ruling: %w", err)
	}
	return value, nil
}

func storedDedupe(d domain.ToolDedupe) *struct {
	WindowSeconds int      `json:"window_seconds"`
	ArgPaths      []string `json:"arg_paths"`
} {
	if !d.Enabled() {
		return nil
	}
	return &struct {
		WindowSeconds int      `json:"window_seconds"`
		ArgPaths      []string `json:"arg_paths"`
	}{
		WindowSeconds: d.WindowSeconds,
		ArgPaths:      append([]string(nil), d.ArgPaths...),
	}
}

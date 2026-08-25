package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// classificationKind is the settings row a ruling is stored as. Reusing the
// settings table gives scope resolution for free: an area may tighten what its
// company allows without the company enumerating every area.
const classificationKind = "tool_classification"

var ErrNotClassifiable = errors.New("admin: unknown is not a ruling")

// Curator records what tools do to the world.
//
// This is the single point where any access enters the platform. A tool
// arrives from its server unclassified whatever the server claims about
// itself, and does nothing until this says what it does — which is why the
// ruling is durable and attributed rather than a flag on a running process.
type Curator struct {
	pool *pgxpool.Pool
}

func NewCurator(pool *pgxpool.Pool) *Curator {
	return &Curator{pool: pool}
}

// Classify records a ruling and the fact that somebody made it.
//
// EffectUnknown is refused rather than stored: it is the zero value that makes
// an unclassified tool fail closed, and writing it deliberately would look
// like a decision when it is the absence of one.
func (c *Curator) Classify(ctx context.Context, scope domain.Scope, ruling domain.ToolClassification) error {
	if !ruling.Effect.Valid() {
		return fmt.Errorf("%w: %s", ErrNotClassifiable, ruling.Effect)
	}
	if err := ruling.Dedupe.Validate(); err != nil {
		return fmt.Errorf("%w: invalid dedupe for %s: %w", ErrNotClassifiable, ruling.Tool, err)
	}
	if ruling.Tool == "" {
		return errors.New("admin: a ruling needs a tool")
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
		return fmt.Errorf("admin: encode ruling: %w", err)
	}

	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by, updated_at)
		values ('installation', '', '', $1, $2, $3, true, $4, now())
		on conflict (scope_kind, company_id, area_id, kind, name) do update set
			value = excluded.value, updated_by = excluded.updated_by, updated_at = now()`,
		classificationKind, string(ruling.Tool), value, string(ruling.By)); err != nil {
		return fmt.Errorf("admin: store ruling for %s: %w", ruling.Tool, err)
	}

	// In the same transaction as the change it describes. A ruling that took
	// effect without a record of who made it is exactly what the trail exists
	// to make impossible.
	if err := Record(ctx, tx, Event{
		Principal: ruling.By,
		Scope:     scope,
		Action:    "tool.classified",
		Target:    string(ruling.Tool),
		Detail:    json.RawMessage(value),
	}); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("admin: commit ruling: %w", err)
	}
	return nil
}

// List returns every ruling on record.
//
// The signature is what package tools declares as its Classifier, so the
// catalogue can consult the Curator without either package importing the
// other.
func (c *Curator) List(ctx context.Context, scope domain.Scope) ([]domain.ToolClassification, error) {
	rows, err := c.pool.Query(ctx, `
		select name, value, updated_by from settings
		where kind = $1 and enabled
		order by name`, classificationKind)
	if err != nil {
		return nil, fmt.Errorf("admin: list rulings: %w", err)
	}
	defer rows.Close()

	var out []domain.ToolClassification
	for rows.Next() {
		var (
			name, by string
			raw      []byte
			stored   struct {
				Effect        string `json:"effect"`
				Untrusted     bool   `json:"untrusted"`
				Reason        string `json:"reason"`
				CompensatedBy string `json:"compensated_by"`
				Dedupe        *struct {
					WindowSeconds int      `json:"window_seconds"`
					ArgPaths      []string `json:"arg_paths"`
				} `json:"dedupe"`
				Digest string `json:"digest"`
			}
		)
		if err := rows.Scan(&name, &raw, &by); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &stored); err != nil {
			return nil, fmt.Errorf("admin: decode ruling for %s: %w", name, err)
		}

		effect, err := domain.ParseEffect(stored.Effect)
		if err != nil {
			// A row nobody can read is not silently treated as anything:
			// reading a corrupt record as a classification would turn it into
			// a permission.
			return nil, fmt.Errorf("admin: ruling for %s: %w", name, err)
		}
		dedupe := domain.ToolDedupe{}
		if stored.Dedupe != nil {
			dedupe = domain.ToolDedupe{
				WindowSeconds: stored.Dedupe.WindowSeconds,
				ArgPaths:      append([]string(nil), stored.Dedupe.ArgPaths...),
			}
			if err := dedupe.Validate(); err != nil {
				return nil, fmt.Errorf("admin: ruling for %s: %w", name, err)
			}
		}

		out = append(out, domain.ToolClassification{
			Tool: domain.ToolID(name), Effect: effect,
			Untrusted: stored.Untrusted, By: domain.UserID(by), Reason: stored.Reason,
			CompensatedBy: domain.ToolID(stored.CompensatedBy),
			Dedupe:        dedupe,
			Digest:        stored.Digest,
		})
	}
	return out, rows.Err()
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

// Events reads the administrative trail, newest first.
//
// Append-only by construction: nothing in this package updates or deletes a
// row in admin_events, and a correction is a new event rather than an
// amendment to the one it corrects.
func (c *Curator) Events(ctx context.Context, target, cursor string, limit int) ([]domain.AdminEvent, string, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	after := decodeEventCursor(cursor)

	rows, err := c.pool.Query(ctx, `
		select event_id, at, principal_id, company_id, area_id, action, target, detail
		from admin_events
		where ($1 = '' or target = $1)
		  and ($2::bigint = 0 or event_id < $2)
		order by event_id desc
		limit $3`, target, after, limit+1)
	if err != nil {
		return nil, "", fmt.Errorf("admin: list events: %w", err)
	}
	defer rows.Close()

	var out []domain.AdminEvent
	for rows.Next() {
		var (
			e             domain.AdminEvent
			principal     string
			company, area string
		)
		if err := rows.Scan(&e.ID, &e.At, &principal, &company, &area, &e.Action, &e.Target, &e.Detail); err != nil {
			return nil, "", err
		}
		e.Principal = domain.UserID(principal)
		e.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(out) <= limit {
		return out, "", nil
	}
	next := encodeEventCursor(out[limit-1].ID)
	return out[:limit], next, nil
}

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
func (c *Curator) Publish(ctx context.Context, entries []domain.ToolEntry) error {
	if len(entries) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, e := range entries {
		value, err := json.Marshal(struct {
			Server      string `json:"server"`
			Description string `json:"description,omitempty"`
			Digest      string `json:"digest,omitempty"`
			// Whether this installation brought it in. Published rather than
			// derived, because the console reads this copy and a screen that
			// showed every discovered tool as taken would be offering the
			// choice while hiding the answer.
			OnSurface bool `json:"onSurface"`
		}{e.Server, e.Description, e.Digest, e.OnSurface})
		if err != nil {
			return fmt.Errorf("admin: encode tool %s: %w", e.ID, err)
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

// Tools returns the published catalogue with each ruling applied.
//
// A tool with no ruling reads as unclassified and untrusted, which is exactly
// what it is: imported, and nobody has said what it does. So does one whose
// ruling was overtaken by a new definition, and that one says so — refused for
// the same reason, but a decision to check rather than a decision to make.
//
// It used to read as EffectRead here — and the runtime refuses it, so the console, the interview
// and the flow check all showed "read, allowed" for a call the platform would
// stop. An author designs against the screen, which makes a permissive screen
// worse than no screen.
func (c *Curator) Tools(ctx context.Context) ([]domain.ToolEntry, error) {
	rows, err := c.pool.Query(ctx, `
		select name, value from settings
		where kind = $1 and enabled order by name`, toolKind)
	if err != nil {
		return nil, fmt.Errorf("admin: list tools: %w", err)
	}
	defer rows.Close()

	var entries []domain.ToolEntry
	for rows.Next() {
		var (
			name   string
			raw    []byte
			stored struct {
				Server      string `json:"server"`
				Description string `json:"description"`
				Digest      string `json:"digest"`
				OnSurface   bool   `json:"onSurface"`
			}
		)
		if err := rows.Scan(&name, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &stored); err != nil {
			return nil, fmt.Errorf("admin: decode tool %s: %w", name, err)
		}
		entries = append(entries, domain.ToolEntry{
			ID: domain.ToolID(name), Server: stored.Server, Description: stored.Description,
			Effect: domain.EffectUnknown, Untrusted: true, Digest: stored.Digest,
			OnSurface: stored.OnSurface,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rulings, err := c.List(ctx, domain.Scope{})
	if err != nil {
		return nil, err
	}
	byTool := make(map[domain.ToolID]domain.ToolClassification, len(rulings))
	for _, r := range rulings {
		byTool[r.Tool] = r
	}
	for i, e := range entries {
		r, ok := byTool[e.ID]
		if !ok {
			continue
		}
		/*
			The ruling has to be about the definition on offer.

			Both sides have to be able to answer. A ruling with no digest was
			recorded before this was kept, and a published tool with no digest
			has not been rediscovered since — neither is evidence of a change,
			and reading either as one would refuse every tool on an
			installation the day it upgrades.
		*/
		if r.Digest != "" && e.Digest != "" && r.Digest != e.Digest {
			entries[i].Stale = true
			continue
		}
		entries[i].Effect = r.Effect
		entries[i].Untrusted = r.Untrusted
		entries[i].CompensatedBy = r.CompensatedBy
	}
	return entries, nil
}

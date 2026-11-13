package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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
	// clock is what leases on unmanaged servers are written and judged by.
	clock func() time.Time
}

func NewCurator(pool *pgxpool.Pool) *Curator {
	return &Curator{pool: pool, clock: time.Now}
}

// List returns every ruling on record.
//
// The signature is what package tools declares as its Classifier, so the
// catalogue can consult the Curator without either package importing the
// other.
func (c *Curator) List(ctx context.Context, scope domain.Scope) ([]domain.ToolClassification, error) {
	// With where each is stored: the platform's ceiling and the owner's word
	// are two rows of one name, and only their scope tells them apart (NT-019).
	rows, err := c.pool.Query(ctx, `
		select name, value, updated_by, scope_kind, company_id, area_id from settings
		where kind = $1 and enabled
		order by name, scope_kind, company_id, area_id`, classificationKind)
	if err != nil {
		return nil, fmt.Errorf("admin: list rulings: %w", err)
	}
	defer rows.Close()

	var out []domain.ToolClassification
	for rows.Next() {
		var (
			name, by                 string
			scopeKind, company, area string
			raw                      []byte
			stored                   struct {
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
		if err := rows.Scan(&name, &raw, &by, &scopeKind, &company, &area); err != nil {
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
			Scope: storedRuntimeScope(scopeKind, company, area),
			Tool:  domain.ToolID(name), Effect: effect,
			Untrusted: stored.Untrusted, By: domain.UserID(by), Reason: stored.Reason,
			CompensatedBy: domain.ToolID(stored.CompensatedBy),
			Dedupe:        dedupe,
			Digest:        stored.Digest,
		})
	}
	return out, rows.Err()
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

	if err := c.placeTools(ctx, entries); err != nil {
		return nil, err
	}
	rulings, err := c.List(ctx, domain.Scope{})
	if err != nil {
		return nil, err
	}
	byTool, listed := rulingsInForce(entries, rulings)
	for i, e := range entries {
		entries[i].Rulings = listed[e.ID]
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
		entries[i].Dedupe = r.Dedupe.Clone()
	}
	return entries, nil
}

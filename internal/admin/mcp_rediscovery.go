package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
RequestMCPRediscovery asks every worker to ask a server again what it offers
(#287), and answers the generation the request became.

It is the fleet-wide counterpart of RequestMCPProbe, and deliberately not built
on it: a probe is consumed by the one worker that claims it, and this is owed by
every worker. Nor is it a write to the server's configuration. Saving the same
configuration used to make every worker reconnect by accident; #268 made that a
no-op, and this puts back the useful half without the reconnect. The request and
its line on the trail commit together.
*/
func (i *Integrations) RequestMCPRediscovery(
	ctx context.Context, by domain.UserID, scope domain.Scope, name string,
) (int64, error) {
	if strings.TrimSpace(name) == "" {
		return 0, ErrNoName
	}
	kind, at, err := i.mcpServerAt(ctx, name)
	if err != nil {
		return 0, err
	}
	server, err := i.settings.Get(ctx, kind, at, settings.KindMCPServer, name)
	switch {
	case errors.Is(err, settings.ErrNotFound):
		return 0, ErrMCPServerUnknown
	case err != nil:
		return 0, err
	case !server.Enabled:
		return 0, ErrMCPServerDisabled
	}

	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var generation int64
	if err := tx.QueryRow(ctx, `
		insert into mcp_rediscoveries (server, generation, requested_by, requested_at, scope_kind, company_id, area_id)
		values ($1, 1, $2, now(), $3, $4, $5)
		on conflict (server) do update set
			generation   = mcp_rediscoveries.generation + 1,
			requested_by = excluded.requested_by,
			requested_at = excluded.requested_at,
			scope_kind   = excluded.scope_kind,
			company_id   = excluded.company_id,
			area_id      = excluded.area_id
		returning generation`, name, string(by),
		string(kind), string(at.Company), string(at.Area)).Scan(&generation); err != nil {
		return 0, fmt.Errorf("admin: request rediscovery of %s: %w", name, err)
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: scope, Action: "mcp_server.rediscovery_requested", Target: name,
		Detail: map[string]any{"server": name, "generation": generation},
	}); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("admin: commit rediscovery request: %w", err)
	}
	return generation, nil
}

// MCPRediscoveries is the generation each server has been asked to, for a
// worker to compare with what it last answered.
//
// Each with where the server it was asked of was stored (NT-019): a worker
// holding a server of the same name stored elsewhere does not answer it.
func (i *Integrations) MCPRediscoveries(ctx context.Context) (map[string]MCPServerRequest, error) {
	rows, err := i.pool.Query(ctx, `
		select server, generation, scope_kind, company_id, area_id from mcp_rediscoveries`)
	if err != nil {
		return nil, fmt.Errorf("admin: read rediscovery requests: %w", err)
	}
	defer rows.Close()

	out := map[string]MCPServerRequest{}
	for rows.Next() {
		var server, kind, company, area string
		var generation int64
		if err := rows.Scan(&server, &generation, &kind, &company, &area); err != nil {
			return nil, fmt.Errorf("admin: read rediscovery requests: %w", err)
		}
		out[server] = MCPServerRequest{
			Name: server, Generation: generation, Scope: storedRuntimeScope(kind, company, area),
		}
	}
	return out, rows.Err()
}

// RecordMCPRediscovery keeps a worker's newest answer for a server. One for an
// older generation than the worker already answered is a late write, and it
// does not replace the newer one.
//
// Each with where the server answered was stored (NT-019).
func (i *Integrations) RecordMCPRediscovery(ctx context.Context, r domain.MCPRediscoveryReceipt) error {
	kind, stored := storedAt(r.Scope)
	if _, err := i.pool.Exec(ctx, `
		insert into mcp_rediscovery_receipts (server, worker, generation, succeeded, at, scope_kind, company_id, area_id)
		values ($1, $2, $3, $4, now(), $5, $6, $7)
		on conflict (server, worker) do update set
			generation = excluded.generation,
			succeeded  = excluded.succeeded,
			at         = excluded.at,
			scope_kind = excluded.scope_kind,
			company_id = excluded.company_id,
			area_id    = excluded.area_id
		where mcp_rediscovery_receipts.generation <= excluded.generation`,
		r.Server, r.Worker, r.Generation, r.Succeeded,
		string(kind), string(stored.Company), string(stored.Area)); err != nil {
		return fmt.Errorf("admin: record rediscovery of %s by %s: %w", r.Server, r.Worker, err)
	}
	return nil
}

// MCPRediscovery is the latest request for a server and each worker's newest
// receipt, newest first: only those about the row stored under the name now
// (NT-019). A request or receipt about a server of the name stored elsewhere,
// since removed, belongs to that server and is not shown.
func (i *Integrations) MCPRediscovery(ctx context.Context, name string) (domain.MCPRediscoveryStatus, error) {
	status := domain.MCPRediscoveryStatus{Server: name}
	kind, at, err := i.mcpServerAt(ctx, name)
	if err != nil {
		return status, err
	}
	if _, err := i.settings.Get(ctx, kind, at, settings.KindMCPServer, name); errors.Is(err, settings.ErrNotFound) {
		return status, ErrMCPServerUnknown
	} else if err != nil {
		return status, err
	}
	var by string
	err = i.pool.QueryRow(ctx, `
		select generation, requested_by, requested_at from mcp_rediscoveries
		where server = $1 and scope_kind = $2 and company_id = $3 and area_id = $4`,
		name, string(kind), string(at.Company), string(at.Area),
	).Scan(&status.Generation, &by, &status.RequestedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return status, fmt.Errorf("admin: read rediscovery of %s: %w", name, err)
	}
	status.RequestedBy = domain.UserID(by)

	rows, err := i.pool.Query(ctx, `
		select worker, generation, succeeded, at from mcp_rediscovery_receipts
		where server = $1 and scope_kind = $2 and company_id = $3 and area_id = $4
		order by at desc, worker`, name, string(kind), string(at.Company), string(at.Area))
	if err != nil {
		return status, fmt.Errorf("admin: read rediscovery receipts of %s: %w", name, err)
	}
	defer rows.Close()
	for rows.Next() {
		r := domain.MCPRediscoveryReceipt{Server: name, Scope: mcpServerRuntimeScope(kind, at)}
		if err := rows.Scan(&r.Worker, &r.Generation, &r.Succeeded, &r.At); err != nil {
			return status, err
		}
		status.Receipts = append(status.Receipts, r)
	}
	return status, rows.Err()
}

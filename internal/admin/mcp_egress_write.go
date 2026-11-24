package admin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
EgressPolicyWrite is one write of a policy and what it read. A policy is
replaced whole, so a write that did not see what is stored erases it: Create
refuses one over a policy that exists, and Expect one over a policy that
changed since it was read. Neither given, the write replaces, as before.
*/
type EgressPolicyWrite struct {
	By     domain.UserID
	Audit  domain.Scope
	Policy MCPEgressPolicy
	Expect domain.IfMatch
	Create bool
}

// EgressPolicyRemoval is one removal of a policy and what it read.
type EgressPolicyRemoval struct {
	By     domain.UserID
	Audit  domain.Scope
	At     domain.Scope
	Expect domain.IfMatch
}

type storedEgressPolicy struct {
	Company string   `json:"company"`
	Area    string   `json:"area,omitempty"`
	Origins []string `json:"origins"`
}

// PutMCPEgressPolicy stores the policy for a company or an area, replacing
// any it had, with no precondition. The caller is the platform's
// administration; the door says so.
func (i *Integrations) PutMCPEgressPolicy(ctx context.Context, by domain.UserID, audit domain.Scope, p MCPEgressPolicy) error {
	_, err := i.WriteMCPEgressPolicy(ctx, EgressPolicyWrite{By: by, Audit: audit, Policy: p})
	return err
}

// WriteMCPEgressPolicy stores a policy under its precondition and answers the
// revision it leaves. Judged under the policies' lock, so no write of the same
// policy lands between the judgement and this one.
func (i *Integrations) WriteMCPEgressPolicy(ctx context.Context, w EgressPolicyWrite) (domain.Revision, error) {
	p := w.Policy
	if err := egressPolicyScope(p.Scope); err != nil {
		return "", err
	}
	origins, err := egressOrigins(p.Origins)
	if err != nil {
		return "", err
	}
	value, err := json.Marshal(storedEgressPolicy{
		Company: string(p.Scope.Company), Area: string(p.Scope.Area), Origins: origins,
	})
	if err != nil {
		return "", fmt.Errorf("admin: encode MCP egress policy: %w", err)
	}
	return writeRevised(ctx, i.pool, i.settings, folded{
		guard: func(ctx context.Context, tx settings.DB) error { return LockMCPEgressPolicies(ctx, tx) },
		by:    w.By, scope: w.Audit, action: "mcp_egress_policy.configured", target: egressPolicyName(p.Scope),
		set: settings.Setting{
			ScopeKind: settings.ScopeInstallation, Kind: kindMCPEgressPolicy, Name: egressPolicyName(p.Scope),
			Value: value, Enabled: true, UpdatedBy: string(w.By),
		},
		detail: map[string]any{
			"company": string(p.Scope.Company), "area": string(p.Scope.Area), "origins": origins,
		},
		revised: &revised{expect: w.Expect, absent: w.Create, of: i.egressRevision, lockedByGuard: true},
	})
}

// egressRevision names a stored policy: its origins and scope, nothing else.
func (i *Integrations) egressRevision(set settings.Setting) domain.Revision {
	return i.settings.Revision(set, nil, nil)
}

// DeleteMCPEgressPolicy removes a company's or an area's policy. An area's
// servers inherit the company's again, or reach nothing; to deny an area what
// its company allows, store an empty list instead.
func (i *Integrations) DeleteMCPEgressPolicy(ctx context.Context, by domain.UserID, audit domain.Scope, at domain.Scope) error {
	return i.RemoveMCPEgressPolicy(ctx, EgressPolicyRemoval{By: by, Audit: audit, At: at})
}

// RemoveMCPEgressPolicy removes a policy under its precondition: one that
// changed since it was read is not removed.
func (i *Integrations) RemoveMCPEgressPolicy(ctx context.Context, r EgressPolicyRemoval) error {
	at := r.At
	if err := egressPolicyScope(at); err != nil {
		return err
	}
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// The policies' lock first, then the row, as a PUT of a policy takes them
	// (its guard runs before it reads or writes the row): two writes of one
	// policy then never wait on each other crosswise.
	if err := LockMCPEgressPolicies(ctx, tx); err != nil {
		return err
	}
	if r.Expect.Present() {
		row, err := readStored(ctx, tx, i.settings, folded{set: settings.Setting{
			ScopeKind: settings.ScopeInstallation, Kind: kindMCPEgressPolicy, Name: egressPolicyName(at),
		}})
		if err != nil {
			return err
		}
		if _, err := (&revised{expect: r.Expect, of: i.egressRevision}).judge(row); err != nil {
			return err
		}
	}
	if err := i.settings.DeleteTx(ctx, tx, settings.ScopeInstallation, domain.Scope{},
		kindMCPEgressPolicy, egressPolicyName(at)); err != nil {
		return err
	}
	if err := Record(ctx, tx, Event{
		Principal: r.By, Scope: r.Audit, Action: "mcp_egress_policy.removed", Target: egressPolicyName(at),
		Detail: map[string]any{"company": string(at.Company), "area": string(at.Area)},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

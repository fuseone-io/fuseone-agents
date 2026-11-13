package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/netguard"
	"github.com/fuseone/agents/internal/settings"
)

/*
Where MCP servers outside the installation may send requests (NT-019, decided
2026-11-13).

The platform keeps one policy per company or area: a list of exact origins.
Stored at the installation, so only the platform's administration writes it;
an area can neither create nor widen its own. A server of an area is held to
its area's policy, or else to its company's, never to both together; with
neither it reaches nothing. The installation's servers carry no policy.
*/

const kindMCPEgressPolicy settings.Kind = "mcp_egress_policy"

// ErrEgressPolicyInvalid is a policy that cannot be stored as given.
var ErrEgressPolicyInvalid = errors.New("admin: invalid MCP egress policy")

// MCPEgressPolicy is the origins servers stored at Scope may reach.
type MCPEgressPolicy struct {
	Scope     domain.Scope
	Origins   []string
	UpdatedBy string
	UpdatedAt time.Time
}

type storedEgressPolicy struct {
	Company string   `json:"company"`
	Area    string   `json:"area,omitempty"`
	Origins []string `json:"origins"`
}

// PutMCPEgressPolicy stores the policy for a company or an area, replacing
// any it had. The caller is the platform's administration; the door says so.
func (i *Integrations) PutMCPEgressPolicy(ctx context.Context, by domain.UserID, audit domain.Scope, p MCPEgressPolicy) error {
	if err := egressPolicyScope(p.Scope); err != nil {
		return err
	}
	origins, err := egressOrigins(p.Origins)
	if err != nil {
		return err
	}
	value, err := json.Marshal(storedEgressPolicy{
		Company: string(p.Scope.Company), Area: string(p.Scope.Area), Origins: origins,
	})
	if err != nil {
		return fmt.Errorf("admin: encode MCP egress policy: %w", err)
	}
	return writeSetting(ctx, i.pool, i.settings, by, audit, settings.Setting{
		ScopeKind: settings.ScopeInstallation, Kind: kindMCPEgressPolicy, Name: egressPolicyName(p.Scope),
		Value: value, Enabled: true, UpdatedBy: string(by),
	}, "mcp_egress_policy.configured", egressPolicyName(p.Scope), map[string]any{
		"company": string(p.Scope.Company), "area": string(p.Scope.Area), "origins": origins,
	})
}

// DeleteMCPEgressPolicy removes a company's or an area's policy. An area's
// servers inherit the company's again, or reach nothing; to deny an area what
// its company allows, store an empty list instead.
func (i *Integrations) DeleteMCPEgressPolicy(ctx context.Context, by domain.UserID, audit domain.Scope, at domain.Scope) error {
	if err := egressPolicyScope(at); err != nil {
		return err
	}
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := i.settings.DeleteTx(ctx, tx, settings.ScopeInstallation, domain.Scope{},
		kindMCPEgressPolicy, egressPolicyName(at)); err != nil {
		return err
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: audit, Action: "mcp_egress_policy.removed", Target: egressPolicyName(at),
		Detail: map[string]any{"company": string(at.Company), "area": string(at.Area)},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MCPEgressPolicies is every stored policy.
func (i *Integrations) MCPEgressPolicies(ctx context.Context) ([]MCPEgressPolicy, error) {
	rows, err := i.settings.List(ctx, kindMCPEgressPolicy)
	if err != nil {
		return nil, err
	}
	out := make([]MCPEgressPolicy, 0, len(rows))
	for _, row := range rows {
		var stored storedEgressPolicy
		if err := json.Unmarshal(row.Value, &stored); err != nil {
			return nil, fmt.Errorf("admin: decode MCP egress policy %s: %w", row.Name, err)
		}
		out = append(out, MCPEgressPolicy{
			Scope:   domain.Scope{Company: domain.CompanyID(stored.Company), Area: domain.AreaID(stored.Area)},
			Origins: stored.Origins, UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt,
		})
	}
	return out, nil
}

// egressFor is the policy a server stored at runtime scope s is held to: nil
// for the installation's, else its area's, else its company's, else one that
// allows nothing.
func egressFor(policies []MCPEgressPolicy, s domain.Scope) *domain.MCPEgress {
	if s == (domain.Scope{}) || s.Company == domain.Installation {
		return nil
	}
	for _, at := range []domain.Scope{s, {Company: s.Company}} {
		if i := slices.IndexFunc(policies, func(p MCPEgressPolicy) bool { return p.Scope == at }); i >= 0 {
			return &domain.MCPEgress{Origins: slices.Clone(policies[i].Origins)}
		}
	}
	return &domain.MCPEgress{}
}

func egressPolicyName(s domain.Scope) string {
	if s.Area == "" {
		return string(s.Company)
	}
	return string(s.Company) + "/" + string(s.Area)
}

// egressPolicyScope refuses a scope no company or area could have: a policy
// is a company's or an area's, never the installation's.
func egressPolicyScope(s domain.Scope) error {
	if err := domain.ValidCompanyID(string(s.Company)); err != nil {
		return fmt.Errorf("%w: %v", ErrEgressPolicyInvalid, err)
	}
	if strings.TrimSpace(string(s.Company)) != string(s.Company) {
		return fmt.Errorf("%w: company %q has surrounding spaces", ErrEgressPolicyInvalid, s.Company)
	}
	if s.Area == "" {
		return nil
	}
	if folded, err := domain.NormalizeAreaID(string(s.Area)); err != nil || folded != s.Area {
		return fmt.Errorf("%w: area %q is not an area identifier", ErrEgressPolicyInvalid, s.Area)
	}
	return nil
}

// egressOrigins is the canonical, sorted, deduplicated list, refusing any
// origin netguard would never let a request reach. An empty list is a policy
// too: it allows nothing, and overrides the company's for an area.
func egressOrigins(given []string) ([]string, error) {
	out := []string{}
	for _, raw := range given {
		origin, err := domain.CanonicalOrigin(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrEgressPolicyInvalid, err)
		}
		if err := netguard.ValidateHTTPURL(origin); err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrEgressPolicyInvalid, origin, err)
		}
		out = append(out, origin)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

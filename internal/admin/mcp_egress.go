package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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
	// Revision names the policy as stored; a write's If-Match compares it.
	Revision domain.Revision
}

// MCPEgressPolicies is every stored policy.
func (i *Integrations) MCPEgressPolicies(ctx context.Context) ([]MCPEgressPolicy, error) {
	rows, err := i.settings.List(ctx, kindMCPEgressPolicy)
	if err != nil {
		return nil, err
	}
	policies, err := policiesFrom(rows)
	if err != nil {
		return nil, err
	}
	// In the rows' order, one policy each: the revision a write's If-Match names.
	for k := range policies {
		policies[k].Revision = i.egressRevision(rows[k])
	}
	return policies, nil
}

// policiesFrom decodes stored policy rows.
func policiesFrom(rows []settings.Setting) ([]MCPEgressPolicy, error) {
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
//
// Nil is origins not given, a body that forgot the field or sent null: not a
// deny-all, which would replace a working list by accident. It is refused.
func egressOrigins(given []string) ([]string, error) {
	if given == nil {
		return nil, fmt.Errorf("%w: origins are required; an empty list allows nothing", ErrEgressPolicyInvalid)
	}
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

// ErrEgressNotAllowed is an area's server that would send a request outside
// the platform's egress policy for its scope, or whose scope it allows nothing.
var ErrEgressNotAllowed = errors.New("admin: outside the platform's egress policy for this server's scope")

// ErrEgressAddressInvalid is a server's OAuth token endpoint or GitHub App API
// base that is not an http(s) address with a host: there is no origin a policy
// could list, so it is refused as invalid input, not as an egress refusal.
var ErrEgressAddressInvalid = errors.New("admin: the OAuth token endpoint and the GitHub App API base must be http or https addresses with a host")

// EgressRefusal is ErrEgressNotAllowed naming the origin refused: canonical,
// with no path or query, since a token endpoint's query may carry a secret.
// It is never empty: an address with no origin is ErrEgressAddressInvalid.
type EgressRefusal struct {
	Origin string
}

func (e *EgressRefusal) Error() string {
	return ErrEgressNotAllowed.Error() + ": " + e.Origin
}

func (e *EgressRefusal) Is(target error) bool { return target == ErrEgressNotAllowed }

// egressAtTx is the policy a server stored at kind/stored is held to, nil for
// the installation's, read inside the server write's transaction under the
// policies' lock held shared: no policy can change between this read and the
// write's commit.
func (i *Integrations) egressAtTx(ctx context.Context, tx settings.DB, kind settings.ScopeKind, stored domain.Scope) (*domain.MCPEgress, error) {
	if kind == settings.ScopeInstallation {
		return nil, nil
	}
	if err := LockMCPEgressPoliciesShared(ctx, tx); err != nil {
		return nil, err
	}
	rows, err := i.settings.ListTx(ctx, tx, kindMCPEgressPolicy)
	if err != nil {
		return nil, err
	}
	policies, err := policiesFrom(rows)
	if err != nil {
		return nil, err
	}
	return egressFor(policies, mcpServerRuntimeScope(kind, stored)), nil
}

/*
LockMCPEgressPolicies holds the policies for a write of one, until the
transaction ends. A server write holds the same lock shared while it reads the
policy for its scope and until it commits (LockMCPEgressPoliciesShared), so a
policy narrowed meanwhile either waits for the server's commit or is the one
the server is judged by. Every policy write takes this lock before its row,
and a server write never locks a policy row, so no two of them can each hold
what the other waits for.
*/
func LockMCPEgressPolicies(ctx context.Context, tx settings.DB) error {
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtextextended('mcp_egress_policies', 0))`); err != nil {
		return fmt.Errorf("admin: lock the MCP egress policies: %w", err)
	}
	return nil
}

// LockMCPEgressPoliciesShared is the server write's side of the policies' lock.
func LockMCPEgressPoliciesShared(ctx context.Context, tx settings.DB) error {
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock_shared(hashtextextended('mcp_egress_policies', 0))`); err != nil {
		return fmt.Errorf("admin: lock the MCP egress policies shared: %w", err)
	}
	return nil
}

// withinEgress refuses a server whose URL, OAuth token endpoint or GitHub App
// API base its policy does not list: the worker would refuse each of them on
// every connection (0.109.0).
func withinEgress(egress *domain.MCPEgress, serverURL string, creds domain.MCPCredentials) error {
	if egress == nil {
		return nil
	}
	targets := []string{serverURL}
	if creds.OAuth != nil && creds.OAuth.TokenURL != "" {
		targets = append(targets, creds.OAuth.TokenURL)
	}
	if creds.GitHubApp != nil && !creds.GitHubApp.Empty() {
		base := creds.GitHubApp.APIBaseURL
		if base == "" {
			base = domain.DefaultGitHubAPIBase
		}
		targets = append(targets, base)
	}
	return originsWithin(egress, targets)
}

// originsWithin refuses the first address the policy does not list, naming
// its origin alone. Nil is the installation's, held to none.
func originsWithin(egress *domain.MCPEgress, targets []string) error {
	if egress == nil {
		return nil
	}
	for _, raw := range targets {
		u, err := url.Parse(raw)
		if err != nil {
			return ErrEgressAddressInvalid
		}
		origin, ok := domain.OriginOf(u)
		if !ok {
			return ErrEgressAddressInvalid
		}
		if !egress.Allows(u) {
			return &EgressRefusal{Origin: origin}
		}
	}
	return nil
}

package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
)

/*
The reduced answers an author picks from, asked for one target scope (NT-019,
rule 5, phase 2 writers part B2a).

ListTools and GET /providers answer, for a target, every row whose runtime
scope contains it: what an agent stored at the target can run, which is the
Gate's checkScope over the same scopes. A union of the caller's scopes would
offer a cx agent finance's tools; the caller's grants instead of the target
would lose cx's own rows for a company author.

Without a target, the platform's administration keeps every row: its screen
lists every tool and every owner's ruling. Anyone else gets only the
installation's rows, which every scope inherits, so nothing of any area is
answered without naming it.

The filter lives here, in the door. The rulings the worker syncs from are read
whole (Curator.List), whatever a listing asked for.
*/

// errTargetMalformed is a target that is not a real company or area as stored.
var errTargetMalformed = errors.New("malformed target scope")

// errTargetHidden is a target the caller may not read; it answers as one that
// does not exist, since a refusal would confirm it does.
var errTargetHidden = errors.New("target scope not found")

// reach says whether a row of the given runtime scope is in a reduced answer.
type reach func(runtime domain.Scope) bool

// readerReach is the reach of a reduced answer for raw, the optional target.
// The caller is already known to hold tool:read somewhere.
func readerReach(ctx context.Context, raw *string) (reach, error) {
	p, _ := auth.PrincipalFrom(ctx)
	platform := p.Can(domain.PermToolRead, adminScope)
	if raw == nil {
		if platform {
			return func(domain.Scope) bool { return true }, nil
		}
		return isInstallations, nil
	}
	target, err := parseTarget(*raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errTargetMalformed, err)
	}
	if !platform && !p.Can(domain.PermToolRead, target) {
		return nil, errTargetHidden
	}
	return func(runtime domain.Scope) bool { return runtime.Contains(target) }, nil
}

// isInstallations keeps the rows every scope inherits.
func isInstallations(runtime domain.Scope) bool {
	return runtime == domain.Scope{Company: domain.Installation}
}

// parseTarget reads `company` or `company/area` exactly as stored, by part A's
// realScope: nothing is folded into an identifier, and the installation's `*`
// is no company identifier, so it is not a target.
func parseTarget(raw string) (domain.Scope, error) {
	company, area, hasArea := strings.Cut(raw, "/")
	if company == "" || (hasArea && area == "") {
		return domain.Scope{}, fmt.Errorf("scope %q: `company` or `company/area`", raw)
	}
	target := domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	if err := realScope(target); err != nil {
		return domain.Scope{}, err
	}
	return target, nil
}

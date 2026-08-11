package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// rows is whatever can read: the pool, or a transaction when a snapshot has to
// see writes that have not committed yet.
type rows interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// readAll reads every policy, ordered by code.
//
// Ordered because the set's hash is taken over it: a hash that moved with row
// order would make every restart look like a policy change, and every step
// would seal a decision to a name nobody could reproduce.
func readAll(ctx context.Context, db rows) ([]domain.Policy, error) {
	result, err := db.Query(ctx, `
		select code, name, owner, reason, resource, effects, reach,
		       scopes, agents, conditions, effect, mode, enabled
		from policies order by code`)
	if err != nil {
		return nil, fmt.Errorf("policy: read: %w", err)
	}
	defer result.Close()

	// Never nil: an installation with no policies hashes an empty set, and a
	// nil slice would encode as `null` while an empty one encodes as `[]` —
	// two different hashes for the same absence of rules.
	out := []domain.Policy{}
	for result.Next() {
		var (
			p                  domain.Policy
			effects, agents    []string
			reach, effect      string
			mode               string
			scopes, conditions []byte
		)
		if err := result.Scan(&p.Code, &p.Name, &p.Owner, &p.Reason, &p.Resource,
			&effects, &reach, &scopes, &agents, &conditions,
			&effect, &mode, &p.Enabled); err != nil {
			return nil, err
		}

		p.Reach, p.Effect, p.Mode = domain.PolicyReach(reach), domain.PolicyEffect(effect), domain.PolicyMode(mode)
		for _, e := range effects {
			// An effect nobody can parse becomes Unknown, which never
			// executes — a stored value that drifted must not read as a read.
			effect, _ := domain.ParseEffect(e)
			p.Effects = append(p.Effects, effect)
		}
		for _, a := range agents {
			p.Agents = append(p.Agents, domain.AgentID(a))
		}
		if err := json.Unmarshal(scopes, &p.Scopes); err != nil {
			return nil, fmt.Errorf("policy: decode scopes of %s: %w", p.Code, err)
		}
		if err := json.Unmarshal(conditions, &p.Conditions); err != nil {
			return nil, fmt.Errorf("policy: decode conditions of %s: %w", p.Code, err)
		}
		out = append(out, p)
	}
	if err := result.Err(); err != nil {
		return nil, err
	}

	slices.SortFunc(out, func(a, b domain.Policy) int { return strings.Compare(a.Code, b.Code) })
	return out, nil
}

// encode prepares the JSON and array columns.
func encode(p domain.Policy) (scopes, conditions []byte, effects []string, err error) {
	if scopes, err = json.Marshal(orEmpty(p.Scopes)); err != nil {
		return nil, nil, nil, fmt.Errorf("policy: encode scopes: %w", err)
	}
	if conditions, err = json.Marshal(orEmpty(p.Conditions)); err != nil {
		return nil, nil, nil, fmt.Errorf("policy: encode conditions: %w", err)
	}

	effects = []string{}
	for _, e := range p.Effects {
		effects = append(effects, e.String())
	}
	return scopes, conditions, effects, nil
}

func agentIDs(agents []domain.AgentID) []string {
	out := []string{}
	for _, a := range agents {
		out = append(out, string(a))
	}
	return out
}

// orEmpty keeps nil out of the encoding. A nil slice encodes as `null` and an
// empty one as `[]`, and the two would hash differently for the same policy.
func orEmpty[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}

package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/policy"
)

// Where the rules a run is decided under come from.

// policySource is where the set comes from, or a source of nothing when this
// worker has no database. An installation running on the in-memory ledger
// decides under the built-in ladder, which is the safe default rather than an
// absence of rules.
func policySource(pool *pgxpool.Pool) policy.Source {
	if pool == nil {
		return emptyPolicies{}
	}
	return policy.NewStore(pool)
}

type emptyPolicies struct{}

func (emptyPolicies) Active(context.Context) (policy.Set, error) {
	return policy.Set{Hash: "builtin", Policies: nil}, nil
}

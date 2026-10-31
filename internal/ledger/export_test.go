package ledger

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// The fallback filters, exported to the plan test so it explains the exact
// text the aggregates run rather than a copy that could drift from it.
const (
	DeclaredPairRunsFilter  = declaredPairRuns
	DeclaredPairStepsFilter = declaredPairSteps
)

// AppendInTx seals a step inside a transaction the test holds open, so a race
// can be staged on the production append rather than on a copy of its SQL.
func AppendInTx(ctx context.Context, tx pgx.Tx, s domain.Step) (domain.Step, error) {
	return appendTx(ctx, tx, nil, nil, s)
}

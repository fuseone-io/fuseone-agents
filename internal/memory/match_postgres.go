package memory

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
Match answers what is already here, and never writes.

Reads only, and outside any transaction: nothing here decides anything, so
holding an identity while a person reads the answer would block writers for as
long as somebody leaves a form open.

byIdentityTx does the looking, which is what makes this find rows written before
the canonical key existed — it matches the key or the assertion id, and a legacy
row still answers to the second. What it will not do is repair one: keying a row
is a write, and this is the read a screen makes on every keystroke.

Deliberately not on the Gate's path. A run recalls memory through Find; this
exists so a person composing one is shown what they are about to duplicate.
*/
func (p *Postgres) Match(ctx context.Context, in MatchInput) (Match, error) {
	var out Match
	own, err := byIdentityTx(ctx, p.pool, in.identityOf(in.AgentID))
	if err != nil {
		return Match{}, err
	}
	out.Own = own

	if in.AgentID != "" {
		shared, err := byIdentityTx(ctx, p.pool, in.identityOf(""))
		if err != nil {
			return Match{}, err
		}
		out.Shared = shared
	}

	pending, err := pendingForIdentity(ctx, p.pool, in)
	if err != nil {
		return Match{}, err
	}
	out.Pending = pending
	return out, nil
}

// pendingForIdentity is the proposal nobody has decided yet. By assertion id
// because that is what a suggestion carries — the queue predates the canonical
// key and a pending row has no column for it that a match could index on.
func pendingForIdentity(
	ctx context.Context, tx db, in MatchInput,
) (*domain.MemorySuggestion, error) {
	found, err := scanSuggestion(tx.QueryRow(ctx, `
		select `+suggestionColumns+` from memory_suggestions
		where company_id = $1 and area_id = $2 and agent_id = $3
		  and assertion_id = $4 and status = 'pending'
		order by updated_at desc, suggestion_id
		limit 1`,
		string(in.Scope.Company), string(in.Scope.Area), string(in.AgentID),
		in.identityOf(in.AgentID).ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memory: read pending for identity: %w", err)
	}
	return &found, nil
}

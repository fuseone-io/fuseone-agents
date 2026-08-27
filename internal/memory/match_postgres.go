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

Deliberately not on the Gate's path. A run recalls memory through Find; this
exists so a person composing one is shown what they are about to duplicate.
*/
func (p *Postgres) Match(ctx context.Context, in MatchInput) (Match, error) {
	var out Match
	own, err := p.matchOne(ctx, in.identityOf(in.AgentID))
	if err != nil {
		return Match{}, err
	}
	out.Own = asOf(own, in.Now)

	if in.AgentID != "" {
		shared, err := p.matchOne(ctx, in.identityOf(""))
		if err != nil {
			return Match{}, err
		}
		out.Shared = asOf(shared, in.Now)
	}

	pending, err := p.matchPending(ctx, in)
	if err != nil {
		return Match{}, err
	}
	out.Pending = pending
	return out, nil
}

/*
matchOne looks by both names an identity has, and then by neither.

byIdentityTx matches the canonical key or the raw assertion id, which finds a
row written before the key existed only when the person is spelling it the same
way they did then. The whole reason to ask is that they might not be — so a row
with no key falls back to the same comparison the write path makes, in Go,
because the rule is NFC and then case folding and Postgres has neither.

What it will not do is fill the key in. Keying a row is a write, and this is the
read a screen makes on every keystroke; the sweep and the next write are what
repair it.
*/
func (p *Postgres) matchOne(
	ctx context.Context, identity domain.MemoryAssertion,
) (*domain.MemoryAssertion, error) {
	found, err := byIdentityTx(ctx, p.pool, identity)
	if err != nil || found != nil {
		return found, err
	}
	ids, err := unkeyedRowsOf(ctx, p.pool, identity)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	legacy, err := readAssertionTx(ctx, p.pool, ids[0], identity.Scope)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &legacy, nil
}

/*
matchPending is the proposal nobody has decided yet.

By the canonical key, the raw assertion id, or — for a proposal recorded before
either was written to this table — by comparing the identity in Go, the same
three ways an assertion is found. A queue that could only be searched by exact
spelling would let somebody teach a fact an agent proposed an hour ago in
slightly different words.
*/
func (p *Postgres) matchPending(
	ctx context.Context, in MatchInput,
) (*domain.MemorySuggestion, error) {
	identity := in.identityOf(in.AgentID)
	found, err := scanSuggestion(p.pool.QueryRow(ctx, `
		select `+suggestionColumns+` from memory_suggestions
		where company_id = $1 and area_id = $2 and agent_id = $3 and status = 'pending'
		  and (canonical_identity_key = $4 or assertion_id = $5)
		order by updated_at desc, suggestion_id
		limit 1`,
		string(in.Scope.Company), string(in.Scope.Area), string(in.AgentID),
		domain.CanonicalIdentityKey(identity), identity.ID))
	if err == nil {
		return &found, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("memory: read pending for identity: %w", err)
	}
	return p.unkeyedPending(ctx, identity)
}

func (p *Postgres) unkeyedPending(
	ctx context.Context, identity domain.MemoryAssertion,
) (*domain.MemorySuggestion, error) {
	rows, err := p.pool.Query(ctx, `
		select `+suggestionColumns+` from memory_suggestions
		where company_id = $1 and area_id = $2 and agent_id = $3
		  and status = 'pending' and canonical_identity_key is null
		order by updated_at desc, suggestion_id`,
		string(identity.Scope.Company), string(identity.Scope.Area),
		string(identity.AgentID))
	if err != nil {
		return nil, fmt.Errorf("memory: read unkeyed pending: %w", err)
	}
	defer rows.Close()

	key := domain.CanonicalIdentityKey(identity)
	for rows.Next() {
		held, err := scanSuggestion(rows)
		if err != nil {
			return nil, fmt.Errorf("memory: read unkeyed pending: %w", err)
		}
		if domain.CanonicalIdentityKey(assertionOf(held)) == key {
			return &held, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: read unkeyed pending: %w", err)
	}
	return nil, nil
}

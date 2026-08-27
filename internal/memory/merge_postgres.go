package memory

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

/*
mergeInto is the only way an assertion reaches the table.

Every path that writes one — a person correcting it, a suggestion accepted, a
policy confirming repeated observations — comes through here, so the decision in
Merge is made once and the coordination around it is made once.

The order is the point. The lock is taken before anything is read, because a
merge computed from a row somebody else has since changed is a merge that
overwrites them. And the event is recorded after the merge rather than before,
so what the trail shows is the projection that resulted and not the write that
arrived.
*/
func mergeInto(
	ctx context.Context, tx db, incoming domain.MemoryAssertion,
	origin MergeOrigin, by domain.UserID, reason, action string,
) (domain.MemoryAssertion, MergeOutcome, error) {
	if err := lockIdentity(ctx, tx, incoming); err != nil {
		return domain.MemoryAssertion{}, "", err
	}
	if err := keyLegacyIdentities(ctx, tx, incoming); err != nil {
		return domain.MemoryAssertion{}, "", err
	}
	stored, covering, err := neighboursTx(ctx, tx, incoming)
	if err != nil {
		return domain.MemoryAssertion{}, "", err
	}

	merged, outcome, err := Merge(MergeInput{
		Incoming: incoming, Stored: stored, Covering: covering,
		Origin: origin, Now: incoming.UpdatedAt,
	})
	if err != nil {
		return domain.MemoryAssertion{}, "", err
	}
	if outcome == Covered {
		// Nothing changed. Writing the projection or touching updated_at would
		// record a mutation of shared memory that did not happen, and the
		// caller is being told to go and improve it deliberately instead.
		return merged, outcome, nil
	}

	if err := upsertAssertion(ctx, tx, merged); err != nil {
		return domain.MemoryAssertion{}, "", err
	}
	if err := recordEvent(ctx, tx, merged, by, reason, action); err != nil {
		return domain.MemoryAssertion{}, "", err
	}
	return merged, outcome, nil
}

/*
lockIdentity holds the canonical identity for the rest of the transaction.

Transaction-scoped, so a rollback or a failure never leaves a lock behind on the
session. Keyed on the canonical identity rather than on the assertion id,
because "Slack Alerts" and " slack   alerts " are different ids and the same
fact: locking the id would let two writers each take their own lock and both
insert.

An agent-scoped write also holds the shared key, since shared memory is what
decides whether this write is covered. Both are taken in sorted order, which is
what stops two writers holding one each and waiting for the other.
*/
func lockIdentity(ctx context.Context, tx db, a domain.MemoryAssertion) error {
	for _, identity := range lockedIdentities(a) {
		if _, err := tx.Exec(ctx,
			`select pg_advisory_xact_lock(hashtextextended($1, 0))`,
			domain.CanonicalIdentityKey(identity)); err != nil {
			return fmt.Errorf("memory: lock identity: %w", err)
		}
	}
	return nil
}

// lockedIdentities is every identity this write depends on, in the order every
// writer takes them.
func lockedIdentities(a domain.MemoryAssertion) []domain.MemoryAssertion {
	out := []domain.MemoryAssertion{a}
	if a.AgentID != "" {
		shared := a
		shared.AgentID = ""
		out = append(out, shared)
	}
	slices.SortFunc(out, func(x, y domain.MemoryAssertion) int {
		return strings.Compare(domain.CanonicalIdentityKey(x), domain.CanonicalIdentityKey(y))
	})
	return out
}

/*
keyLegacyIdentities gives the canonical key to the rows this write is about.

A row written before the key has nothing to be found by except an assertion id
nobody will type again, so a second spelling of the same fact matches nothing and
inserts a duplicate. The sweep shortens that queue; it cannot close it, because a
pod still running the old image writes keyless rows after the job has passed. So
the write path repairs what it is about to look at, and the sweep is what stops
the repair from being paid for one row at a time for ever.

Only the rows whose identity is one this transaction already holds. Keying a row
of some other identity would be arithmetic nobody can argue with, performed while
holding the wrong lock — a write racing that identity's writer.

No event, and nothing else touched. The key is derived from the row's own
fields, changes nothing anybody decided, and is recalculable from the row.
*/
func keyLegacyIdentities(ctx context.Context, tx db, a domain.MemoryAssertion) error {
	for _, identity := range lockedIdentities(a) {
		ids, err := unkeyedRowsOf(ctx, tx, identity)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `
			update memory_assertions set canonical_identity_key = $2
			where assertion_id = any($1)`,
			ids, domain.CanonicalIdentityKey(identity)); err != nil {
			return fmt.Errorf("memory: key legacy rows: %w", err)
		}
	}
	return nil
}

// unkeyedRowsOf is the keyless rows in this namespace that are this identity.
//
// The comparison is in Go and cannot be pushed into the query: the rule is NFC
// then case folding, and Postgres has no NFC — an accent typed as a combining
// mark would compare unequal to the same letter precomposed, which in Portuguese
// is an ordinary Tuesday. Only the fields that form the key are read, so the
// scan the partial index serves stays narrow.
func unkeyedRowsOf(ctx context.Context, tx db, a domain.MemoryAssertion) ([]string, error) {
	rows, err := tx.Query(ctx, `
		select assertion_id, kind, subject, signature from memory_assertions
		where company_id = $1 and area_id = $2 and agent_id = $3
		  and canonical_identity_key is null
		order by assertion_id`,
		string(a.Scope.Company), string(a.Scope.Area), string(a.AgentID))
	if err != nil {
		return nil, fmt.Errorf("memory: read unkeyed rows: %w", err)
	}
	defer rows.Close()

	key := domain.CanonicalIdentityKey(a)
	var ids []string
	for rows.Next() {
		var id string
		held := domain.MemoryAssertion{Scope: a.Scope, AgentID: a.AgentID}
		if err := rows.Scan(&id, &held.Kind, &held.Subject, &held.Signature); err != nil {
			return nil, fmt.Errorf("memory: read unkeyed rows: %w", err)
		}
		if domain.CanonicalIdentityKey(held) == key {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: read unkeyed rows: %w", err)
	}
	return ids, nil
}

// neighboursTx reads the row this write may merge into and the shared row that
// may already cover it. Both are read inside the lock: a value read before it
// is a value somebody may have changed since.
func neighboursTx(
	ctx context.Context, tx db, a domain.MemoryAssertion,
) (stored, covering *domain.MemoryAssertion, err error) {
	stored, err = byIdentityTx(ctx, tx, a)
	if err != nil || a.AgentID == "" || stored != nil {
		return stored, nil, err
	}
	shared := a
	shared.AgentID = ""
	covering, err = byIdentityTx(ctx, tx, shared)
	return stored, covering, err
}

/*
byIdentityTx finds the row that is this identity, by either name it has.

The canonical key is the identity a duplicate check can trust, and the assertion
id is what a row written before that key existed is called. Matching both is
what lets the two live side by side while the older rows are filled in.

Two rows, not one. "Which row is this identity" has no answer the moment there
is more than one, and a limit of one cannot tell the only row from the first of
several — it returned the keyed row and hid the legacy one behind an ordering
nobody chose. Reading one more is what turns a silent choice into a refusal.

The order is what makes the refusal stable: the same pair is named every time,
and by the id both stores sort on, so two people reading the same error are
looking at the same two rows whichever store answered.
*/
func byIdentityTx(
	ctx context.Context, tx db, a domain.MemoryAssertion,
) (*domain.MemoryAssertion, error) {
	key := domain.CanonicalIdentityKey(a)
	rows, err := tx.Query(ctx, `
		select `+columns+` from memory_assertions
		where company_id = $1 and area_id = $2 and agent_id = $3
		  and (canonical_identity_key = $4 or assertion_id = $5)
		order by assertion_id
		limit 2`,
		string(a.Scope.Company), string(a.Scope.Area), string(a.AgentID),
		key, a.ID)
	if err != nil {
		return nil, fmt.Errorf("memory: read identity: %w", err)
	}
	defer rows.Close()

	var found []domain.MemoryAssertion
	for rows.Next() {
		one, err := scanAssertion(rows)
		if err != nil {
			return nil, fmt.Errorf("memory: read identity: %w", err)
		}
		found = append(found, one)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: read identity: %w", err)
	}
	return oneOf(found, key)
}

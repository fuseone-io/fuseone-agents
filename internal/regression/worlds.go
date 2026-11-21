package regression

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

/*
Worlds reads the world a replayed case is answered with (NT-018 phase 5).

A reader of its own and not the Store: it takes no fence and holds no connection outside the
pool, so the simulation pool can have one without a second Store, whose fence budget means
something only while there is one per process.
*/
type Worlds struct{ pool *pgxpool.Pool }

func NewWorlds(pool *pgxpool.Pool) *Worlds { return &Worlds{pool: pool} }

// kept is one row of content as a world needs it checked.
type kept struct {
	found, erased, truncated bool
	kind, owner, digest      string
	fullBytes                int64
	bytes                    []byte
}

// ownedBy says whether the content is whole, readable, and the case's own.
func (k kept) ownedBy(owner string) bool {
	return k.found && !k.erased && !k.truncated && k.kind == WorldKind && k.owner == owner
}

/*
WorldOf answers the world of a case as the version a run was opened on recorded it: nil for a
case with none. The version and never the working set, so what is replayed is what the run
says it checked.

Everything the replay will hand out is checked here, before anything plans: the object the
version fixed, kept whole as the case's own; its entries, in order from one and each naming a
call; and every answer, present, whole, the case's own and with the digest and size recorded.
Any of it that fails is ErrWorldUnreadable for the whole world: an answer found missing on a
later turn would fail the run as an unexplained error instead of saying what was lost.
*/
func (w *Worlds) WorldOf(
	ctx context.Context, agent domain.AgentID, version int64, caseID string,
) (*domain.CaseWorld, error) {
	owner := string(agent) + "/" + caseID
	object, ref, err := w.object(ctx, agent, version, caseID)
	if err != nil || ref == "" {
		return nil, err
	}
	var world domain.CaseWorld
	if err := json.Unmarshal(object, &world); err != nil || world.Case != caseID {
		return nil, fmt.Errorf("%w: %s does not name %s", domain.ErrWorldUnreadable, ref, caseID)
	}
	if err := entriesInOrder(world.Entries); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", domain.ErrWorldUnreadable, ref, err)
	}
	if err := w.answersKept(ctx, owner, world.Entries); err != nil {
		return nil, err
	}
	return &world, nil
}

// object reads the world object the version fixed for the case, and its reference: none for a
// case with no world.
func (w *Worlds) object(ctx context.Context, agent domain.AgentID, version int64, caseID string) ([]byte, string, error) {
	var ref, fixed string
	var k kept
	err := w.pool.QueryRow(ctx, `
		select v.world_ref, v.world_digest, rc.ref is not null, coalesce(rc.erased_at is not null, false),
		       coalesce(rc.truncated, false), coalesce(rc.owner_kind, ''), coalesce(rc.run_id, ''),
		       coalesce(rc.digest, ''), rc.bytes
		from dataset_version_cases v
		left join run_content rc on rc.ref = v.world_ref and v.world_ref <> ''
		where v.dataset_id = $1 and v.version = $2 and v.case_id = $3`,
		defaultDatasetOf(agent), version, caseID).Scan(&ref, &fixed, &k.found, &k.erased, &k.truncated,
		&k.kind, &k.owner, &k.digest, &k.bytes)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", fmt.Errorf("%w: %s is not in version %d of %s", domain.ErrWorldUnreadable, caseID, version, agent)
	}
	if err != nil {
		return nil, "", fmt.Errorf("regression: read the world of %s: %w", caseID, err)
	}
	if ref == "" {
		return nil, "", nil
	}
	if !k.ownedBy(string(agent)+"/"+caseID) || k.digest != fixed {
		return nil, "", fmt.Errorf("%w: %s is not the world version %d fixed", domain.ErrWorldUnreadable, ref, version)
	}
	return k.bytes, ref, nil
}

// entriesInOrder holds a world's entries to the order they were asked in, from one, each
// naming the call it answers and the answer it hands back.
func entriesInOrder(entries []domain.WorldEntry) error {
	for i, e := range entries {
		switch {
		case e.Position != i+1:
			return fmt.Errorf("entry %d is at position %d", i+1, e.Position)
		case e.Tool == "" || e.ArgsDigest == "" || e.AnswerRef == "" || e.AnswerDigest == "":
			return fmt.Errorf("entry %d does not name its call and its answer", e.Position)
		}
	}
	return nil
}

// answersKept checks every answer a world names is kept, whole, as the case's own, with the
// digest and size its entry recorded.
func (w *Worlds) answersKept(ctx context.Context, owner string, entries []domain.WorldEntry) error {
	refs := make([]string, 0, len(entries))
	for _, e := range entries {
		refs = append(refs, e.AnswerRef)
	}
	rows, err := w.pool.Query(ctx, `
		select ref, true, erased_at is not null, truncated, owner_kind, run_id, digest, full_bytes
		from run_content where ref = any($1::text[])`, refs)
	if err != nil {
		return fmt.Errorf("regression: read the answers of %s: %w", owner, err)
	}
	defer rows.Close()
	held := make(map[string]kept, len(refs))
	for rows.Next() {
		var ref string
		var k kept
		if err := rows.Scan(&ref, &k.found, &k.erased, &k.truncated, &k.kind, &k.owner, &k.digest, &k.fullBytes); err != nil {
			return fmt.Errorf("regression: scan an answer of %s: %w", owner, err)
		}
		held[ref] = k
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("regression: read the answers of %s: %w", owner, err)
	}
	for _, e := range entries {
		k := held[e.AnswerRef]
		if !k.ownedBy(owner) || k.digest != e.AnswerDigest || k.fullBytes != e.AnswerBytes {
			return fmt.Errorf("%w: answer %d (%s) is not kept as recorded", domain.ErrWorldUnreadable, e.Position, e.AnswerRef)
		}
	}
	return nil
}

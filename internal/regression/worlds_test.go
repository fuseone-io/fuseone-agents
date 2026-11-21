package regression_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
)

/*
The world a replayed case is answered with, read where its run's opening says: the case, in
the version of the corpus the run was opened on, never the working set (NT-018 phase 5). A
case with no world has none to read. A world that cannot be read, or that names another case,
is refused: replaying the case without it would answer a situation the author never corrected.
*/

func recordedWithWorld(t *testing.T, pool *pgxpool.Pool, id string, world domain.CaseWorld) string {
	t.Helper()
	body, err := json.Marshal(world)
	if err != nil {
		t.Fatalf("encode the world: %v", err)
	}
	ref, err := ledger.NewContent(pool).PutFor(t.Context(), regression.WorldKind, "suporte/"+id, 1, body)
	if err != nil {
		t.Fatalf("keep the world: %v", err)
	}
	if err := regression.NewStore(pool).Record(t.Context(), withWorld(aCase(id, never("x")), ref)); err != nil {
		t.Fatalf("Record: %v", err)
	}
	return ref
}

func aWorld(id string) domain.CaseWorld {
	return domain.CaseWorld{Case: id, Entries: []domain.WorldEntry{
		{Position: 1, Tool: "crm.lookup", ArgsDigest: "sha256:a", AnswerRef: "case_world://suporte/" + id + "/2/x", AnswerDigest: "x", AnswerBytes: 3},
	}}
}

func TestWorldOf_aCaseWithAWorld_readsItAtTheVersionTheRunWasOpenedOn(t *testing.T) {
	pool := activatedCorpus(t)
	recordedWithWorld(t, pool, "reg-3", aWorld("reg-3"))

	got, err := regression.NewWorlds(pool).WorldOf(t.Context(), "suporte", 2, "reg-3")
	if err != nil {
		t.Fatalf("WorldOf: %v", err)
	}
	if got == nil || got.Case != "reg-3" || len(got.Entries) != 1 || got.Entries[0].Tool != "crm.lookup" {
		t.Errorf("WorldOf = %+v, want the recorded world", got)
	}
}

// Version 1 was written before the case had a world: a run opened on it replays none, even
// though the working set has one now.
func TestWorldOf_aVersionFromBeforeTheWorld_readsNone(t *testing.T) {
	pool := activatedCorpus(t)
	recordedWithWorld(t, pool, "reg-1", aWorld("reg-1"))

	got, err := regression.NewWorlds(pool).WorldOf(t.Context(), "suporte", 1, "reg-1")
	if err != nil || got != nil {
		t.Errorf("WorldOf = %+v, %v; want no world and no error", got, err)
	}
}

func TestWorldOf_aWorldThatCannotBeHonoured_isRefused(t *testing.T) {
	for name, tc := range map[string]struct {
		world   domain.CaseWorld
		version int64
		erase   bool
	}{
		"its object was erased":           {world: aWorld("reg-3"), version: 2, erase: true},
		"its object names another case":   {world: aWorld("reg-9"), version: 2},
		"the case is not in that version": {world: aWorld("reg-3"), version: 1},
	} {
		t.Run(name, func(t *testing.T) {
			pool := activatedCorpus(t)
			ref := recordedWithWorld(t, pool, "reg-3", tc.world)
			if tc.erase {
				if _, err := pool.Exec(t.Context(), `
					update run_content set bytes = null, erased_at = now(), erased_reason = 'subject'
					where ref = $1`, ref); err != nil {
					t.Fatalf("erase: %v", err)
				}
			}
			_, err := regression.NewWorlds(pool).WorldOf(t.Context(), "suporte", tc.version, "reg-3")
			if !errors.Is(err, domain.ErrWorldUnreadable) {
				t.Errorf("err = %v, want the world refused as unreadable", err)
			}
		})
	}
}

package regression_test

import (
	"crypto/sha256"
	"encoding/hex"
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
case with no world has none to read.

Everything a replay will hand out is checked before anything plans: the object the version
fixed, owned by its case, and each answer it names, present, whole, owned by the case and with
the digest and size recorded. A world any part of which cannot be honoured is refused whole:
replaying it in part would answer a situation the author never corrected, and an answer found
missing mid-run fails the turn as an unexplained error instead of saying what was lost.
*/

// keptAnswer stores one answer as a case's world keeps it, and names it as an entry does.
func keptAnswer(t *testing.T, pool *pgxpool.Pool, owner string, position int, body string) domain.WorldEntry {
	t.Helper()
	ref, err := ledger.NewContent(pool).PutFor(t.Context(), regression.WorldKind, owner, int64(position+1), []byte(body))
	if err != nil {
		t.Fatalf("keep answer %d: %v", position, err)
	}
	sum := sha256.Sum256([]byte(body))
	return domain.WorldEntry{
		Position: position, Tool: "crm.lookup", ArgsDigest: "sha256:a",
		AnswerRef: ref, AnswerDigest: hex.EncodeToString(sum[:]), AnswerBytes: int64(len(body)),
	}
}

// aWorld is a world of two answers, both kept for the case.
func aWorld(t *testing.T, pool *pgxpool.Pool, id string) domain.CaseWorld {
	t.Helper()
	return domain.CaseWorld{Case: id, Entries: []domain.WorldEntry{
		keptAnswer(t, pool, "suporte/"+id, 1, `{"plan":"gold"}`),
		keptAnswer(t, pool, "suporte/"+id, 2, `{"ok":true}`),
	}}
}

// recordedWithWorld keeps the world as the case's own object and records the case naming it.
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

func eraseContent(t *testing.T, pool *pgxpool.Pool, ref string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `
		update run_content set bytes = null, erased_at = now(), erased_reason = 'subject' where ref = $1`, ref); err != nil {
		t.Fatalf("erase %s: %v", ref, err)
	}
}

func TestWorldOf_aCaseWithAWorld_readsItAtTheVersionTheRunWasOpenedOn(t *testing.T) {
	pool := activatedCorpus(t)
	want := aWorld(t, pool, "reg-3")
	recordedWithWorld(t, pool, "reg-3", want)

	got, err := regression.NewWorlds(pool).WorldOf(t.Context(), "suporte", 2, "reg-3")
	if err != nil {
		t.Fatalf("WorldOf: %v", err)
	}
	if got == nil || got.Case != "reg-3" || len(got.Entries) != 2 || got.Entries[1] != want.Entries[1] {
		t.Errorf("WorldOf = %+v, want the recorded world", got)
	}
}

// Version 1 was written before the case had a world: a run opened on it replays none, even
// though the working set has one now.
func TestWorldOf_aVersionFromBeforeTheWorld_readsNone(t *testing.T) {
	pool := activatedCorpus(t)
	recordedWithWorld(t, pool, "reg-1", aWorld(t, pool, "reg-1"))

	got, err := regression.NewWorlds(pool).WorldOf(t.Context(), "suporte", 1, "reg-1")
	if err != nil || got != nil {
		t.Errorf("WorldOf = %+v, %v; want no world and no error", got, err)
	}
}

func TestWorldOf_aWorldThatCannotBeHonoured_isRefusedWhole(t *testing.T) {
	type tamper struct {
		world   func(t *testing.T, pool *pgxpool.Pool, w *domain.CaseWorld)
		after   func(t *testing.T, pool *pgxpool.Pool, w domain.CaseWorld, ref string)
		version int64
	}
	for name, tc := range map[string]tamper{
		"its object was erased":           {after: func(t *testing.T, pool *pgxpool.Pool, _ domain.CaseWorld, ref string) { eraseContent(t, pool, ref) }},
		"its object names another case":   {world: func(_ *testing.T, _ *pgxpool.Pool, w *domain.CaseWorld) { w.Case = "reg-9" }},
		"the case is not in that version": {version: 1},
		// Only the second answer goes: the first would replay, the second would not.
		"one answer was erased": {after: func(t *testing.T, pool *pgxpool.Pool, w domain.CaseWorld, _ string) {
			eraseContent(t, pool, w.Entries[1].AnswerRef)
		}},
		// The digest and the size are the whole answer's even when only a prefix was kept.
		"an answer was kept only in part": {after: func(t *testing.T, pool *pgxpool.Pool, w domain.CaseWorld, _ string) {
			if _, err := pool.Exec(t.Context(), `update run_content set truncated = true where ref = $1`, w.Entries[1].AnswerRef); err != nil {
				t.Fatalf("truncate: %v", err)
			}
		}},
		"an answer was never kept": {world: func(_ *testing.T, _ *pgxpool.Pool, w *domain.CaseWorld) {
			w.Entries[1].AnswerRef = "case_world://suporte/reg-3/9/none"
		}},
		"an answer is another case's": {world: func(t *testing.T, pool *pgxpool.Pool, w *domain.CaseWorld) {
			w.Entries[1] = keptAnswer(t, pool, "suporte/reg-8", 2, `{"ok":true}`)
		}},
		"an answer's digest is not the one recorded": {world: func(_ *testing.T, _ *pgxpool.Pool, w *domain.CaseWorld) {
			w.Entries[1].AnswerDigest = "0000"
		}},
		"an answer's size is not the one recorded": {world: func(_ *testing.T, _ *pgxpool.Pool, w *domain.CaseWorld) {
			w.Entries[1].AnswerBytes++
		}},
		"the positions skip one":      {world: func(_ *testing.T, _ *pgxpool.Pool, w *domain.CaseWorld) { w.Entries[1].Position = 3 }},
		"an entry names no tool":      {world: func(_ *testing.T, _ *pgxpool.Pool, w *domain.CaseWorld) { w.Entries[0].Tool = "" }},
		"an entry names no arguments": {world: func(_ *testing.T, _ *pgxpool.Pool, w *domain.CaseWorld) { w.Entries[0].ArgsDigest = "" }},
	} {
		t.Run(name, func(t *testing.T) {
			pool := activatedCorpus(t)
			world := aWorld(t, pool, "reg-3")
			if tc.world != nil {
				tc.world(t, pool, &world)
			}
			ref := recordedWithWorld(t, pool, "reg-3", world)
			if tc.after != nil {
				tc.after(t, pool, world, ref)
			}
			version := tc.version
			if version == 0 {
				version = 2
			}
			_, err := regression.NewWorlds(pool).WorldOf(t.Context(), "suporte", version, "reg-3")
			if !errors.Is(err, domain.ErrWorldUnreadable) {
				t.Errorf("err = %v, want the world refused as unreadable", err)
			}
		})
	}
}

// The object read is the one the version fixed: one kept under another kind, or whose digest
// is not the one the version recorded, is not the case's world even when its reference is the
// one the case names.
func TestWorldOf_anObjectThatIsNotTheOneTheVersionFixed_isRefused(t *testing.T) {
	for name, statement := range map[string]string{
		"kept under another kind": `update run_content set owner_kind = 'regression' where ref = $1`,
		"another digest":          `update run_content set digest = 'other' where ref = $1`,
	} {
		t.Run(name, func(t *testing.T) {
			pool := activatedCorpus(t)
			ref := recordedWithWorld(t, pool, "reg-3", aWorld(t, pool, "reg-3"))
			if _, err := pool.Exec(t.Context(), statement, ref); err != nil {
				t.Fatalf("tamper: %v", err)
			}
			_, err := regression.NewWorlds(pool).WorldOf(t.Context(), "suporte", 2, "reg-3")
			if !errors.Is(err, domain.ErrWorldUnreadable) {
				t.Errorf("err = %v, want the world refused as unreadable", err)
			}
		})
	}
}

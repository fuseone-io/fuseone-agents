package regression_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
)

/*
A correction copies what it needs into the corpus in the transaction that records it (NT-018
phase 5b, C2): the input, and for a case kept from a rehearsal the answers the author gave and
the world naming them, with the expectations, the case and the version. A recording that fails
anywhere keeps none of it. The world written is the one a replay reads: the round trip goes
through the reader the simulation pool uses.
*/

func uniqueBytes(what string) []byte {
	return []byte(fmt.Sprintf(`{"%s":%d}`, what, time.Now().UnixNano()))
}

func withAnswers(answers ...string) *regression.WorldCopy {
	w := &regression.WorldCopy{}
	for i, a := range answers {
		w.Answers = append(w.Answers, regression.Answer{
			Tool: "crm.lookup", ArgsDigest: fmt.Sprintf("sha256:%d", i), Bytes: []byte(a),
		})
	}
	return w
}

// kept counts what a case owns as content: its input copy, and its world.
func kept(t *testing.T, pool *pgxpool.Pool, id string, input []byte) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `
		select count(*) from run_content
		where (owner_kind = 'regression' and run_id = 'suporte' and bytes = $1)
		   or (owner_kind = 'case_world' and run_id = 'suporte/' || $2)`, input, id).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestRecordCopying_aCaseWithAWorld_isReadBackByTheReplaysReader(t *testing.T) {
	pool := activatedCorpus(t)
	input := uniqueBytes("ask")
	recorded, err := regression.NewStore(pool).RecordCopying(t.Context(), aCase("reg-3", never("x")),
		regression.Copying{Input: input, World: withAnswers(`{"plan":"gold"}`, `{"ok":true}`)})
	if err != nil {
		t.Fatalf("RecordCopying: %v", err)
	}
	world, err := regression.NewWorlds(pool).WorldOf(t.Context(), "suporte", 2, "reg-3")
	if err != nil || world == nil || len(world.Entries) != 2 || world.Entries[1].ArgsDigest != "sha256:1" {
		t.Fatalf("WorldOf = %+v (%v), want the two answers recorded", world, err)
	}
	if got, err := ledger.NewContent(pool).Get(t.Context(), world.Entries[0].AnswerRef); err != nil || string(got) != `{"plan":"gold"}` {
		t.Errorf("answer 1 = %q (%v)", got, err)
	}
	if got, err := ledger.NewContent(pool).Get(t.Context(), recorded.InputRef); err != nil || string(got) != string(input) {
		t.Errorf("input = %q (%v), want the occurrence copied", got, err)
	}
}

// A rehearsal that made no call has a world too: one with nothing in it, so a replay that
// calls anything has left it.
func TestRecordCopying_aRehearsalThatCalledNothing_keepsAnEmptyWorld(t *testing.T) {
	pool := activatedCorpus(t)
	if _, err := regression.NewStore(pool).RecordCopying(t.Context(), aCase("reg-3", never("x")),
		regression.Copying{Input: uniqueBytes("ask"), World: withAnswers()}); err != nil {
		t.Fatalf("RecordCopying: %v", err)
	}
	world, err := regression.NewWorlds(pool).WorldOf(t.Context(), "suporte", 2, "reg-3")
	if err != nil || world == nil || len(world.Entries) != 0 {
		t.Errorf("WorldOf = %+v (%v), want an empty world, not none", world, err)
	}
}

func TestRecordCopying_aRecordingThatFails_keepsNothing(t *testing.T) {
	pool := activatedCorpus(t)
	input := uniqueBytes("ask")
	huge := aCase("reg-3", domain.Expectation{Kind: domain.ExpectNeverCalls, Value: strings.Repeat("x", domain.DefaultContentLimit)})
	_, err := regression.NewStore(pool).RecordCopying(t.Context(), huge,
		regression.Copying{Input: input, World: withAnswers(`{"plan":"gold"}`)})
	if !errors.Is(err, regression.ErrExpectationsTooLarge) {
		t.Fatalf("err = %v, want the expectations refused", err)
	}
	if n := kept(t, pool, "reg-3", input); n != 0 {
		t.Errorf("%d objects kept by a recording that failed, want none", n)
	}
}

// A corpus is fixed to the scope it was first recorded in (D3): a case from another scope
// would replay there content that was never read there.
func TestRecordCopying_intoADatasetOfAnotherScope_isRefusedAndKeepsNothing(t *testing.T) {
	pool := activatedCorpus(t)
	input := uniqueBytes("ask")
	elsewhere := aCase("reg-3", never("x"))
	elsewhere.Scope = domain.Scope{Company: "acme", Area: "finance"}
	_, err := regression.NewStore(pool).RecordCopying(t.Context(), elsewhere,
		regression.Copying{Input: input, World: withAnswers(`{"plan":"gold"}`)})
	if !errors.Is(err, regression.ErrDatasetElsewhere) {
		t.Fatalf("err = %v, want %v", err, regression.ErrDatasetElsewhere)
	}
	if n := kept(t, pool, "reg-3", input); n != 0 {
		t.Errorf("%d objects kept, want none", n)
	}
}

// The world came from the same rehearsal as the input: an erasure that reaches the input
// copy reaches the answers and the object naming them.
func TestEraseInputCopy_takesTheCasesWorldWithIt(t *testing.T) {
	pool := activatedCorpus(t)
	input := uniqueBytes("ask")
	recorded, err := regression.NewStore(pool).RecordCopying(t.Context(), aCase("reg-3", never("x")),
		regression.Copying{Input: input, World: withAnswers(`{"plan":"gold"}`)})
	if err != nil {
		t.Fatalf("RecordCopying: %v", err)
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.NewContent(pool).EraseRefsTx(t.Context(), tx, []string{recorded.InputRef}, "subject"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	var readable int
	if err := pool.QueryRow(t.Context(), `
		select count(*) from run_content
		where owner_kind = 'case_world' and run_id = 'suporte/reg-3' and erased_at is null`).Scan(&readable); err != nil {
		t.Fatal(err)
	}
	if readable != 0 {
		t.Errorf("%d world objects still readable after the input was erased, want none", readable)
	}
}

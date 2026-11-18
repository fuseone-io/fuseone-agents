package ledger_test

import (
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/testdb"
)

// A run opened on a candidate names its seal, and a rehearsal its workbench
// thread, in run_started. The projection keeps both so erasure can follow
// them to the runs derived from a seal or a thread (NT-018 phase 2).

func TestAppend_aCandidateRunStart_projectsItsSealAndThread(t *testing.T) {
	pool := testdb.Pool(t)
	run := domain.RunID("cand_" + time.Now().Format("150405.000000000"))
	const seal = "0b7c3e52-8a43-4b55-9a3f-0d6a3c5f9e11"
	const thread = "5d1f0c7a-2b9e-4c11-8f2d-3a9e6b7c8d10"
	store := ledger.NewPostgres(pool)
	steps := []domain.Step{{
		RunID: run, Seq: 1, Kind: domain.StepRunStarted,
		Scope: domain.Scope{Company: "c", Area: "a"}, AgentID: "ag", VersionID: "v",
		At: time.Now(), IdemKey: string(run) + ":1",
		Payload: []byte(`{"trigger":"simulation","simulated":true,"experiment":"exp_1",` +
			`"seal_id":"` + seal + `","authoring_thread":"` + thread + `"}`),
	}, {
		RunID: run, Seq: 2, Kind: domain.StepRunFinished,
		Scope: domain.Scope{Company: "c", Area: "a"}, AgentID: "ag", VersionID: "v",
		At: time.Now(), IdemKey: string(run) + ":2", Payload: []byte(`{}`),
	}}
	for _, s := range steps {
		if _, err := store.Append(t.Context(), s); err != nil {
			t.Fatalf("append %d: %v", s.Seq, err)
		}
	}
	var gotSeal, gotThread string
	if err := pool.QueryRow(t.Context(), `
		select coalesce(seal_id::text, ''), coalesce(authoring_thread::text, '')
		from runs where run_id = $1`, string(run)).Scan(&gotSeal, &gotThread); err != nil {
		t.Fatalf("read: %v", err)
	}
	if gotSeal != seal || gotThread != thread {
		t.Fatalf("projection seal=%q thread=%q, want %q and %q", gotSeal, gotThread, seal, thread)
	}
}

func TestAppend_anOrdinaryRunStart_hasNoSeal(t *testing.T) {
	pool := testdb.Pool(t)
	run := domain.RunID("plain_" + time.Now().Format("150405.000000000"))
	if _, err := ledger.NewPostgres(pool).Append(t.Context(), domain.Step{
		RunID: run, Seq: 1, Kind: domain.StepRunStarted,
		Scope: domain.Scope{Company: "c", Area: "a"}, AgentID: "ag", VersionID: "v",
		At: time.Now(), IdemKey: string(run) + ":1", Payload: []byte(`{"trigger":"manual"}`),
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	var sealNull, threadNull bool
	if err := pool.QueryRow(t.Context(), `
		select seal_id is null, authoring_thread is null from runs where run_id = $1`,
		string(run)).Scan(&sealNull, &threadNull); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !sealNull || !threadNull {
		t.Fatalf("ordinary run projected seal null=%v thread null=%v", sealNull, threadNull)
	}
}


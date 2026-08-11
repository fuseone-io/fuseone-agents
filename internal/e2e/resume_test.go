package e2e_test

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
)

// A run that has already called a tool has to be resumable by a process that
// did not make the call. Rebuilding the transcript needs the earlier arguments
// and results, and holding them in a map inside one worker meant a restart —
// or a second pod — could not continue the run at all (PRD NF-02, DE-15).

func TestResume_anotherProcessCanRebuildTheTranscript(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the cross-process resume")
	}

	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `truncate run_steps, runs, run_content`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	store := ledger.NewPostgres(pool)

	// The first process writes the tool's arguments and result.
	first := ledger.NewContent(pool)
	ref, err := first.Put(t.Context(), "run-1", 5, []byte(`{"email":"cliente@exemplo.com.br"}`))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	steps := []domain.Step{
		{RunID: "run-1", Kind: domain.StepRunStarted, Scope: domain.Scope{Company: "acme", Area: "cx"},
			AgentID: "triage", VersionID: "v1"},
		{RunID: "run-1", Kind: domain.StepToolCalled, Scope: domain.Scope{Company: "acme", Area: "cx"},
			AgentID: "triage", VersionID: "v1",
			Payload: []byte(`{"tool":"crm.lookup","args_ref":"` + ref + `"}`)},
	}
	for _, step := range steps {
		if _, err := store.Append(t.Context(), step); err != nil {
			t.Fatalf("Append(%s): %v", step.Kind, err)
		}
	}

	written, err := store.Read(t.Context(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	// A second process, sharing only the database.
	second := ledger.NewContent(pool)
	if _, err := engine.BuildTranscript(t.Context(), second, written); err != nil {
		t.Fatalf("a process that did not make the call could not rebuild the transcript: %v", err)
	}

	// And the same run against a fresh in-memory store is exactly the failure
	// this replaced — kept as the reason the durable one exists.
	if _, err := engine.BuildTranscript(t.Context(), engine.NewMemoryContent(), written); err == nil {
		t.Error("an empty in-memory store resolved content it never held")
	}
}

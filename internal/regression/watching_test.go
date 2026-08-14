package regression_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/regression"
)

/*
Which corpora the clock is allowed to spend money on.

Every entry here is a nightly set of model calls at a real provider, so the
two ways this can be wrong are both expensive: naming an agent with no corpus
buys a report that says nothing, and naming one that was never published buys
runs of a version that does not exist.
*/

func TestWatching_anAgentWithCorrections_isNamedWithItsCurrentVersion(t *testing.T) {
	store, pool := watchable(t)
	ctx := context.Background()

	if err := store.Record(ctx, aCase("estorno",
		domain.Expectation{Kind: domain.ExpectCalls, Value: "crm.lookup"})); err != nil {
		t.Fatalf("Record: %v", err)
	}
	published(t, pool, "suporte", "v7")

	watching, err := store.Watching(ctx)
	if err != nil {
		t.Fatalf("Watching: %v", err)
	}
	if len(watching) != 1 {
		t.Fatalf("watching = %+v, want the one agent with a corpus", watching)
	}
	if watching[0].Agent != "suporte" || watching[0].Version != "v7" {
		t.Errorf("watched = %+v, want suporte at its current version", watching[0])
	}
	// The scope comes from the correction, because the notice about it has to
	// reach that area's conversation and not the installation's.
	if watching[0].Scope.Area != "cx" {
		t.Errorf("scope = %+v, want the area the correction was made in", watching[0].Scope)
	}
}

func TestWatching_anAgentNeverPublished_isNotWatched(t *testing.T) {
	store, _ := watchable(t)
	ctx := context.Background()

	// A corpus against nothing. Running it would open runs of a version that
	// does not exist, nightly, for ever.
	if err := store.Record(ctx, aCase("estorno",
		domain.Expectation{Kind: domain.ExpectCalls, Value: "crm.lookup"})); err != nil {
		t.Fatalf("Record: %v", err)
	}

	watching, err := store.Watching(ctx)
	if err != nil {
		t.Fatalf("Watching: %v", err)
	}
	if len(watching) != 0 {
		t.Errorf("watching = %+v, want nothing", watching)
	}
}

func watchable(t *testing.T) (*regression.Store, *pgxpool.Pool) {
	t.Helper()
	store := storeFor(t)
	pool := poolFor(t)
	if _, err := pool.Exec(t.Context(), `delete from agent_state`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	return store, pool
}

func published(t *testing.T, pool *pgxpool.Pool, agent, version string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `
		insert into agent_state (agent_id, paused, current_version)
		values ($1, true, $2)
		on conflict (agent_id) do update set current_version = excluded.current_version`,
		agent, version); err != nil {
		t.Fatalf("publish %s: %v", agent, err)
	}
}

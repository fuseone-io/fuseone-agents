package spec_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/spec"
)

/*
What the workbench's draft reads from the published agent, inside its own
transaction (NT-018 1b, part 3): the current version, chosen as every reader
chooses it, and the event graph's lock taken shared, so a draft is never
judged against an agent a publication is about to change.
*/

func TestCurrentIn_isTheVersionSomebodyMadeCurrent(t *testing.T) {
	pool := openSpecPool(t)
	ctx := context.Background()
	first, second := twoVersions(t, pool)
	if _, err := spec.NewPublisher(pool, fixedClock{}).Publish(ctx, spec.Publication{
		Spec: second, By: "usr_ana", Company: "acme"}); err != nil {
		t.Fatalf("publish the second: %v", err)
	}
	if err := spec.NewRegistry(pool).MakeCurrent(ctx, first.ID, first.Version); err != nil {
		t.Fatalf("roll back to the first: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	got, found, err := spec.CurrentIn(ctx, tx, first.ID)
	if err != nil || !found {
		t.Fatalf("CurrentIn = %v, found %v", err, found)
	}
	if got.Version != first.Version || got.Instructions != first.Instructions || got.Company != "acme" {
		t.Fatalf("current = %s (%s), want the first, whole", got.Version, got.Company)
	}
}

func TestCurrentIn_anAgentNeverPublished_isNotFound(t *testing.T) {
	pool := openSpecPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, found, err := spec.CurrentIn(ctx, tx, "nobody"); err != nil || found {
		t.Fatalf("CurrentIn = found %v, %v; want nothing", found, err)
	}
}

// The shared lock waits for a publication holding the graph, and returns
// once it commits.
func TestShareEventGraph_waitsForAPublicationInFlight(t *testing.T) {
	pool := openSpecPool(t)
	ctx := context.Background()
	_, second := twoVersions(t, pool)
	reader := namedConn(t, "nt018-share-graph")

	finished := make(chan error, 1)
	publisher := spec.PauseAfterJudging(spec.NewPublisher(pool, fixedClock{}), func() {
		go func() {
			tx, err := reader.Begin(ctx)
			if err != nil {
				finished <- err
				return
			}
			// Rolled back before it reports, so the test never returns, and
			// closes the connection, while this goroutine still uses it.
			shared := spec.ShareEventGraph(ctx, tx)
			if err := tx.Rollback(ctx); err != nil && shared == nil {
				shared = err
			}
			finished <- shared
		}()
		waitForBackendLockWait(t, pool, "nt018-share-graph", finished)
	})
	if _, err := publisher.Publish(ctx, spec.Publication{Spec: second, By: "usr_ana", Company: "acme"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := <-finished; err != nil {
		t.Fatalf("ShareEventGraph after the publication: %v", err)
	}
}

package drafts_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
)

/*
A draft published from the workbench (NT-018, contract 4). Its ETag is judged
in the publication's own transaction, under the event graph's lock, the
agent's row and then the draft's: a draft that moved since it was read undoes
the publication, and one published has its base reset to the version it
became.
*/

func specFor(t *testing.T, agent domain.AgentID) spec.Spec {
	t.Helper()
	s, err := spec.Parse("test", []byte(fmt.Sprintf(
		"---\nid: %s\nname: Triagem\ncompany: %s\narea: %s\nprovider: openai\nmodel: m\ntools: [crm.lookup]\n"+
			"budget: {micros: 500000, steps: 60}\n---\n\nNunca\nPrometer prazo.", agent, cx.Company, cx.Area)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return s
}

func publishDraft(t *testing.T, store *drafts.Store, agent domain.AgentID, expect domain.Revision) (spec.Spec, error) {
	t.Helper()
	s := specFor(t, agent)
	_, err := spec.NewPublisher(testdb.Pool(t), engine.SystemClock{}).Publish(context.Background(), spec.Publication{
		Spec: s, By: "usr_ana", Company: cx.Company,
		Within: store.WithinPublication(drafts.Publishing{Agent: agent, Expect: expect, Caller: as("usr_ana", cx)}),
	})
	return s, err
}

func TestWithinPublication_aDraftReadAtItsETag_isPublishedAndBasedOnTheNewVersion(t *testing.T) {
	store := drafts.NewStore(testdb.Pool(t))
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)

	published, err := publishDraft(t, store, agent, o.Revision)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	after, err := store.Get(t.Context(), agent, as("usr_ana", cx))
	if err != nil || after.BaseVersion != published.Version || after.Revision != o.Revision {
		t.Errorf("draft after = %+v (%v), want based on %s, its own ETag unchanged", after, err, published.Version)
	}
}

func TestWithinPublication_aDraftThatMoved_undoesThePublication(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	openedWithBlocks(t, store, agent)

	_, err := publishDraft(t, store, agent, `"v1-older"`)
	var stale *domain.StaleError
	if !errors.As(err, &stale) {
		t.Fatalf("Publish = %v, want the draft's precondition to fail", err)
	}
	if versions, err := spec.NewRegistry(pool).Versions(t.Context(), agent); err != nil || len(versions) != 0 {
		t.Errorf("versions = %+v (%v), want the publication undone", versions, err)
	}
}

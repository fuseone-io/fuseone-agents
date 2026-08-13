package httpapi

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// The two probes answer different questions, and an orchestrator acts on them
// differently: liveness restarts, readiness withdraws.

func TestReady_aStoreThatCannotBeRead_refusesWithTheReason(t *testing.T) {
	t.Parallel()

	// Readiness reaches the database and liveness does not. A pod that cannot
	// read the ledger has nothing to answer with: it should leave the
	// rotation, and it should not be restarted for it.
	got, err := NewServer(brokenStore{}, "test").Ready(t.Context(), openapi.ReadyRequestObject{})
	if err != nil {
		t.Fatalf("Ready: %v", err)
	}

	refused, ok := got.(openapi.Ready503ApplicationProblemPlusJSONResponse)
	if !ok {
		t.Fatalf("Ready = %T, want a 503", got)
	}
	if refused.Detail == nil || *refused.Detail == "" {
		t.Error("the refusal says nothing; kubectl describe would show a bare failure")
	}
}

func TestHealth_doesNotTouchTheStore(t *testing.T) {
	t.Parallel()

	// A liveness probe that asked the database would restart every pod during
	// a database blip, turning a recoverable outage into a crash loop.
	got, err := NewServer(brokenStore{}, "test").Health(t.Context(), openapi.HealthRequestObject{})
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if _, ok := got.(openapi.Health200JSONResponse); !ok {
		t.Errorf("Health = %T, want it alive whatever the database is doing", got)
	}
}

// brokenStore is a database nobody can reach.
type brokenStore struct{ Store }

func (brokenStore) Runs(context.Context) ([]domain.RunID, error) {
	return nil, errors.New("dial tcp: connection refused")
}

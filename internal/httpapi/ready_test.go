package httpapi

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
Readiness answers a probe, not a person.

It has to be reachable without a credential — the kubelet holds none — which
makes it the one endpoint anybody who can reach the Service can read. So it
says whether this process can serve and nothing else. The driver's own error
carries the connection string it tried: user, database, host and port, handed
to an unauthenticated caller on a refusal.

Where the detail belongs is the log, which needs a credential on the cluster to
read and is where an operator diagnosing this is already looking.
*/
type deadStore struct {
	*ledger.Memory
}

func (deadStore) Runs(context.Context) ([]domain.RunID, error) {
	return nil, errors.New(
		"failed to connect to `user=agents database=agents`: 10.43.205.154:5432 " +
			"(postgres): dial error: dial tcp 10.43.205.154:5432: connect: connection refused")
}

func TestReady_databaseUnreachable_refusesWithoutNamingTheDatabase(t *testing.T) {
	t.Parallel()
	s := NewServer(deadStore{ledger.NewMemory()}, "test")

	resp, err := s.Ready(t.Context(), openapi.ReadyRequestObject{})
	if err != nil {
		t.Fatalf("Ready: %v", err)
	}

	refused, ok := resp.(openapi.Ready503ApplicationProblemPlusJSONResponse)
	if !ok {
		t.Fatalf("response = %T, want a 503", resp)
	}

	body := ""
	if refused.Detail != nil {
		body = *refused.Detail
	}
	for _, leaked := range []string{"user=agents", "10.43.205.154", "5432", "database=agents"} {
		if strings.Contains(body, leaked) {
			t.Errorf("the refusal handed %q to an unauthenticated caller: %s", leaked, body)
		}
	}
	if body == "" {
		t.Error("the refusal says nothing at all; a probe should still name what is unavailable")
	}
}

func TestReady_storeAnswers_reportsReady(t *testing.T) {
	t.Parallel()
	s := NewServer(ledger.NewMemory(), "test")

	resp, err := s.Ready(t.Context(), openapi.ReadyRequestObject{})
	if err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if _, ok := resp.(openapi.Ready200JSONResponse); !ok {
		t.Fatalf("response = %T, want 200", resp)
	}
}

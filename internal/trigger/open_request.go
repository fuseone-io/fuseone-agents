package trigger

import (
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// The trusted context a request carries, copied before it is sealed so a
// caller that keeps its pointer cannot change what the ledger records.

// workItemOf is the inheritance the request carries, validated: refused
// before anything is stored, because the ledger would refuse the start and
// leave the input behind with no run to own it.
func workItemOf(req Request) (*domain.RunWorkItem, error) {
	if req.WorkItem == nil {
		return nil, nil
	}
	item := *req.WorkItem
	if err := item.Validate(); err != nil {
		return nil, fmt.Errorf("trigger: %w", err)
	}
	return &item, nil
}

func cloneTicket(in *domain.TicketContext) *domain.TicketContext {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneContextArtifacts(in []domain.ContextArtifact) []domain.ContextArtifact {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.ContextArtifact, len(in))
	for i, artifact := range in {
		out[i] = artifact
		out[i].Labels = artifact.Labels.Clone()
	}
	return out
}

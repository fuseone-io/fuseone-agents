package spec

import (
	"context"
	"errors"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/worker"
)

// ErrSealNotFound is a seal id no agent_seals row carries.
var ErrSealNotFound = errors.New("no such seal")

// SealRow is what agent_seals and the version's claim say about one seal.
// Digest is the claim's; empty is a claim without one.
type SealRow struct {
	Agent      domain.AgentID
	Version    domain.VersionID
	Scope      domain.Scope
	ContentRef string
	Digest     string
}

// SealRows reads one seal's metadata, declared here by the consumer.
type SealRows interface {
	Seal(ctx context.Context, id string) (SealRow, error)
}

// Candidates resolves runs opened on a candidate (NT-018 phase 2).
type Candidates struct {
	resolver *Resolver
	seals    SealRows
	content  Content
}

// Candidates is the resolver of candidate runs over seals and the content store.
func (r *Resolver) Candidates(seals SealRows, content Content) *Candidates {
	return &Candidates{resolver: r, seals: seals, content: content}
}

var _ worker.Candidates = (*Candidates)(nil)

// ResolveCandidate is what a run pinned to a seal plans with.
func (c *Candidates) ResolveCandidate(
	ctx context.Context, claim domain.Claim, started domain.RunStartedPayload,
) (worker.Resolution, error) {
	sealed, err := ReadSeal(ctx, c.seals, c.content, started.SealID, SealExpectation{
		Agent: claim.AgentID, Version: claim.VersionID, Scope: claim.Scope,
	})
	if err != nil {
		return worker.Resolution{}, err
	}
	var config domain.ExperimentConfig
	if started.ExperimentConfig != nil {
		config = *started.ExperimentConfig
	}
	return c.resolver.resolveDefinition(sealed, claim, config)
}

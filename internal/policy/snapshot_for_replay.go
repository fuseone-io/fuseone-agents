package policy

import (
	"context"

	"github.com/fuseone/agents/internal/domain"
)

// Policies returns the set behind a hash, for a replay (PRD AU-07).
//
// A thinner shape than Snapshot on purpose: a replay wants the rules and has
// no use for when they were taken, and the interface it declares should not
// name a type it does not read.
func (s *Store) Policies(ctx context.Context, hash string) ([]domain.Policy, error) {
	set, err := s.Snapshot(ctx, hash)
	if err != nil {
		return nil, err
	}
	return set.Policies, nil
}

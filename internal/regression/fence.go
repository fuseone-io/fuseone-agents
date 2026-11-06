package regression

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// FenceSlots is how many fences one store can have open at once. Each is a
// connection the database counts, and these are short and rare: a person recording
// a correction, a battery opening its runs, an erasure.
const FenceSlots = 4

/*
fence is the lock an erasure and anything copying a corpus share, per agent.

Reading an occurrence and writing it somewhere else are two steps, and if the
occurrence goes between them, the second puts it back while the erasure reports
success. So copying holds this shared, and everything that makes an occurrence go
holds it exclusively: a person's request, retention, and deleting a case.

On a connection of its own rather than one from the pool. What is inside a fence
needs the pool, so a fence holding a pool connection could wait for itself: as many
fences as the pool has connections, each waiting for one nobody will return. Nothing
here takes a second slot or a pool connection while holding a slot, and nobody holds
a pool connection while waiting for one, so there is no cycle to wait in.
*/
type fence struct {
	config *pgx.ConnConfig
	slots  chan struct{}
}

func newFence(config *pgx.ConnConfig) *fence {
	return &fence{config: config, slots: make(chan struct{}, FenceSlots)}
}

func (f *fence) enter(
	ctx context.Context, lock string, agents []domain.AgentID, inside func(context.Context) error,
) error {
	if len(agents) == 0 {
		return inside(ctx)
	}
	select {
	case f.slots <- struct{}{}:
		defer func() { <-f.slots }()
	case <-ctx.Done():
		return fmt.Errorf("regression: wait for a corpus fence: %w", ctx.Err())
	}
	conn, err := pgx.ConnectConfig(ctx, f.config)
	if err != nil {
		return fmt.Errorf("regression: connect the corpus fence: %w", err)
	}
	// Not the caller's context: a cancelled request still has to say goodbye.
	defer func() { _ = conn.Close(context.Background()) }()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("regression: begin the corpus fence: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	// In a stable order, so two fences over the same agents cannot deadlock.
	ordered := slices.Sorted(slices.Values(agents))
	for _, agent := range slices.Compact(ordered) {
		if _, err := tx.Exec(ctx, lock, domain.CorpusFence(agent)); err != nil {
			return fmt.Errorf("regression: wait at the corpus fence of %s: %w", agent, err)
		}
	}
	return inside(ctx)
}

// Hold runs inside while the agent's corpus is being copied. Copies do not wait for
// each other. Whatever makes an occurrence go waits for this to finish, and
// anything starting after that reads the tombstone.
func (s *Store) Hold(
	ctx context.Context, agent domain.AgentID, inside func(context.Context) error,
) error {
	return s.fence.enter(ctx, `select pg_advisory_xact_lock_shared(hashtextextended($1, 0))`,
		[]domain.AgentID{agent}, inside)
}

// Exclusive runs inside with these corpora fenced against anything copying them.
func (s *Store) Exclusive(
	ctx context.Context, agents []domain.AgentID, inside func(context.Context) error,
) error {
	return s.fence.enter(ctx, `select pg_advisory_xact_lock(hashtextextended($1, 0))`, agents, inside)
}

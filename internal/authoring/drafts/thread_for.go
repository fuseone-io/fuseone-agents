package drafts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
ThreadFor is author's conversation on the draft, opened the first time it is asked for: the
same thread their turns and proposals use. A rehearsal belongs to it (NT-018 phase 4), so
erasing the conversation reaches the rehearsal's run. The draft is named by its id, as a seal
records it, and may have been discarded since: a thread outlives its draft.
*/
func (s *Store) ThreadFor(ctx context.Context, agent domain.AgentID, draftID string, author domain.UserID) (string, error) {
	var thread string
	err := s.within(ctx, func(tx pgx.Tx) error {
		var err error
		thread, err = threadOf(ctx, tx, draftID, agent, author)
		return err
	})
	return thread, err
}

package drafts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fuseone/agents/internal/domain"
)

func readTurn(ctx context.Context, tx pgx.Tx, where string, args ...any) (Turn, bool, error) {
	var t Turn
	var read string
	err := tx.QueryRow(ctx, `
		select turn_id::text, thread_id::text, idempotency_key, seq, draft_revision, state,
		       reserved_micros, cost_micros, pasted, created_at, message_ref, reply_ref,
		       (erased or subject_erased), dropped, stale
		from agent_draft_turns where `+where, args...).Scan(
		&t.ID, &t.Thread, &t.Key, &t.Seq, &read, &t.State, &t.Reserved, &t.Cost, &t.Pasted, &t.At,
		&t.messageRef, &t.replyRef, &t.Erased, &t.Dropped, &t.Stale)
	if errors.Is(err, pgx.ErrNoRows) {
		return Turn{}, false, nil
	}
	if err != nil {
		return Turn{}, false, fmt.Errorf("drafts: read the turn: %w", err)
	}
	t.Read = domain.Revision(read)
	return t, true, nil
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

// Turns is the caller's own thread over the agent's draft, oldest first, with
// what was said when it is still kept. A reading: it locks nothing.
func (s *Store) Turns(ctx context.Context, agent domain.AgentID, c Caller) ([]Turn, error) {
	var out []Turn
	err := s.within(ctx, func(tx pgx.Tx) error {
		r, err := s.reachableRow(ctx, tx, agent, c, false)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			select t.turn_id::text from agent_draft_turns t
			join agent_draft_threads th on th.thread_id = t.thread_id
			where th.draft_id = $1::uuid and th.author = $2 order by t.seq`, r.draftID, string(c.By))
		if err != nil {
			return fmt.Errorf("drafts: read the thread's turns: %w", err)
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		out, err = saidOf(ctx, tx, ids)
		return err
	})
	return out, err
}

// readSaid is a turn with its message and reply read from the content store.
func readSaid(ctx context.Context, tx pgx.Tx, id string) (Turn, error) {
	turn, _, err := readTurn(ctx, tx, `turn_id = $1::uuid`, id)
	if err != nil {
		return Turn{}, err
	}
	message, kept, err := body(ctx, tx, turn.messageRef)
	if err != nil {
		return Turn{}, err
	}
	turn.Message, turn.Erased = string(message), turn.Erased || !kept
	if turn.replyRef != "" {
		reply, _, err := body(ctx, tx, turn.replyRef)
		if err != nil {
			return Turn{}, err
		}
		turn.Reply = string(reply)
	}
	return turn, nil
}

/*
EraseThread erases what was said in the caller's own thread over the draft:
every message, reply and proposal body the thread owns in the content store,
in one transaction, and every turn of it fenced, so a call still on its way
brings nothing back into it. The turns, proposals and decisions stay, as references to
nothing, and a proposal whose body is gone can never be accepted. An author
with no thread has nothing to erase.
*/
func (s *Store) EraseThread(ctx context.Context, agent domain.AgentID, c Caller) error {
	return s.fence.Exclusive(ctx, []domain.AgentID{agent}, func(ctx context.Context) error {
		return s.within(ctx, func(tx pgx.Tx) error {
			r, err := s.lockReachable(ctx, tx, agent, c)
			if err != nil {
				return err
			}
			var thread string
			err = tx.QueryRow(ctx, `select thread_id::text from agent_draft_threads where draft_id = $1::uuid and author = $2`,
				r.draftID, string(c.By)).Scan(&thread)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("drafts: find the thread: %w", err)
			}
			return s.eraseThreadsTx(ctx, tx, threadErasure{
				agent: agent, draft: r, threads: []string{thread}, by: c.By, reason: "thread erased",
			})
		})
	})
}

// TurnSize is a turn of the caller's thread and the bytes of what was said in it.
type TurnSize struct {
	ID    string
	Bytes int
}

/*
TurnSizes is every turn of the caller's own thread, oldest first, with the bytes of its
message and reply as the content store keeps them, and none of the content itself (#422):
enough to choose a window before any turn's words are read. What was erased counts as
nothing, as a turn read whole would have it. A reading: it locks nothing.
*/
func (s *Store) TurnSizes(ctx context.Context, agent domain.AgentID, c Caller) ([]TurnSize, error) {
	var out []TurnSize
	err := s.within(ctx, func(tx pgx.Tx) error {
		r, err := s.reachableRow(ctx, tx, agent, c, false)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			select t.turn_id::text,
			       coalesce((select octet_length(m.bytes) from run_content m where m.ref = t.message_ref and m.erased_at is null), 0)
			     + coalesce((select octet_length(p.bytes) from run_content p where p.ref = t.reply_ref and p.erased_at is null), 0)
			from agent_draft_turns t
			join agent_draft_threads th on th.thread_id = t.thread_id
			where th.draft_id = $1::uuid and th.author = $2 order by t.seq`, r.draftID, string(c.By))
		if err != nil {
			return fmt.Errorf("drafts: read the thread's turn sizes: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (TurnSize, error) {
			var ts TurnSize
			return ts, row.Scan(&ts.ID, &ts.Bytes)
		})
		return err
	})
	return out, err
}

// TurnsByID is the given turns of the caller's own thread, oldest first, with what was
// said; an id that is not one of them is left out. Only these turns' content is read.
func (s *Store) TurnsByID(ctx context.Context, agent domain.AgentID, c Caller, ids []string) ([]Turn, error) {
	var out []Turn
	err := s.within(ctx, func(tx pgx.Tx) error {
		r, err := s.reachableRow(ctx, tx, agent, c, false)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			select t.turn_id::text from agent_draft_turns t
			join agent_draft_threads th on th.thread_id = t.thread_id
			where th.draft_id = $1::uuid and th.author = $2 and t.turn_id = any($3::uuid[])
			order by t.seq`, r.draftID, string(c.By), ids)
		if err != nil {
			return fmt.Errorf("drafts: read the thread's chosen turns: %w", err)
		}
		mine, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		out, err = saidOf(ctx, tx, mine)
		return err
	})
	return out, err
}

// saidOf is each turn, in the order given, with what was said.
func saidOf(ctx context.Context, tx pgx.Tx, ids []string) ([]Turn, error) {
	out := make([]Turn, 0, len(ids))
	for _, id := range ids {
		turn, err := readSaid(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, turn)
	}
	return out, nil
}

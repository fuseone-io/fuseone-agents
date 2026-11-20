package drafts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
)

/*
A turn's settlement (NT-018 phase 3, contracts 3 and 7).

What the call came back with is stored whole or not at all: the reply, every
proposal the answer carried, and the turn's state and real cost, in one
transaction under the draft's lock and then the turn's. A turn is done only
with all of it; a store that could not judge a proposal stores none of it,
and the turn is left for its caller to fail at its cost. A thread erased
while the model was thinking keeps nothing the call brought back.
*/

// Settlement is what a turn's call came back with.
type Settlement struct {
	Agent  domain.AgentID
	Caller Caller
	Turn   string
	Cost   int64
	Reply  string
	Failed bool
	// Proposed are the answer's operations, each with the model's reason,
	// proposed against the ETag the turn read; Validator and Now judge them as
	// operations by proposal.
	Proposed []Proposed
	// Dropped is what the answer carried that could not be read as operations, or past
	// the limit per answer: kept on the turn with what settling itself discards.
	Dropped   int
	Validator draft.Validator
	Now       time.Time
}

// Proposed is one operation the model proposed, and its sentence on why.
type Proposed struct {
	Op     draft.Operation
	Reason string
}

// Settled is a settled turn and what it proposed. Stale counts operations
// made against a draft that moved during the call; Withheld those of a thread
// erased during it. Neither is kept.
type Settled struct {
	Turn            Turn
	Proposals       []Proposal
	Stale, Withheld int
}

// SettleTurn replaces a turn's reservation with what its call really cost,
// and keeps its reply and proposals. A turn made unknown by a retry while the
// call was on its way settles too: the real cost is the better figure.
func (s *Store) SettleTurn(ctx context.Context, st Settlement) (Settled, error) {
	var out Settled
	err := s.within(ctx, func(tx pgx.Tx) error {
		r, err := s.lockReachable(ctx, tx, st.Agent, st.Caller)
		if err != nil {
			return err
		}
		turn, found, err := readTurn(ctx, tx, `turn_id = $1::uuid and draft_id = $2::uuid for update`, st.Turn, r.draftID)
		if err != nil || !found {
			return firstOrErr(err, ErrNotFound)
		}
		if turn.State != TurnInProgress && turn.State != TurnUnknown {
			return fmt.Errorf("drafts: turn %s is %s already", turn.ID, turn.State)
		}
		ref := ""
		if out, ref, err = s.keepWhatWasSaid(ctx, tx, r, turn, st); err != nil {
			return err
		}
		state := TurnDone
		if st.Failed {
			state = TurnFailed
		}
		if _, err := tx.Exec(ctx, `
			update agent_draft_turns set state = $2, cost_micros = $3, reply_ref = $4, settled_at = now(),
			  dropped = $5, stale = $6
			where turn_id = $1::uuid`, turn.ID, state, st.Cost, ref, st.Dropped+out.Withheld, out.Stale); err != nil {
			return fmt.Errorf("drafts: settle the turn: %w", err)
		}
		out.Turn, _, err = readTurn(ctx, tx, `turn_id = $1::uuid`, turn.ID)
		return err
	})
	return out, err
}

// keepWhatWasSaid stores the reply and the proposals of a thread not erased,
// and answers the reply's reference.
func (s *Store) keepWhatWasSaid(ctx context.Context, tx pgx.Tx, r row, turn Turn, st Settlement) (Settled, string, error) {
	if turn.Erased {
		return Settled{Withheld: len(st.Proposed)}, "", nil
	}
	ref := ""
	if st.Reply != "" {
		var err error
		if ref, err = s.content.RestoreForTx(ctx, tx, "authoring", turn.Thread,
			turnContentSeq(int64(turn.Seq), 1), []byte(st.Reply)); err != nil {
			return Settled{}, "", fmt.Errorf("drafts: keep the reply: %w", err)
		}
	}
	if len(st.Proposed) == 0 {
		return Settled{}, ref, nil
	}
	if r.revision != turn.Read {
		return Settled{Stale: len(st.Proposed)}, ref, nil
	}
	d, err := draft.Decode(r.stored)
	if err != nil {
		return Settled{}, "", err
	}
	var out Settled
	for _, proposed := range st.Proposed {
		refusal, err := verdictOf(st.Validator.Validate(ctx, d, proposed.Op, draft.FromProposal, st.Now))
		if err != nil {
			return Settled{}, "", err
		}
		p, err := s.record(ctx, tx, r, Proposing{Agent: st.Agent, Op: proposed.Op, Read: turn.Read, Caller: st.Caller},
			refusal, said{turn: turn.ID, reason: proposed.Reason})
		if err != nil {
			return Settled{}, "", err
		}
		out.Proposals = append(out.Proposals, p)
	}
	return out, ref, nil
}

// Replaying is a key to look for in the caller's thread.
type Replaying struct {
	Agent  domain.AgentID
	Key    string
	Caller Caller
}

// ReplayTurn answers the turn a key already names in the caller's thread,
// before anything about a new turn is judged: a stored turn is what was
// stored, whatever changed in the installation since. Found false is a key
// never used; ErrTurnInFlight one still within its lease.
func (s *Store) ReplayTurn(ctx context.Context, rp Replaying) (Started, bool, error) {
	var out Started
	var found bool
	err := s.within(ctx, func(tx pgx.Tx) error {
		r, err := s.lockReachable(ctx, tx, rp.Agent, rp.Caller)
		if err != nil {
			return err
		}
		var thread string
		err = tx.QueryRow(ctx, `select thread_id::text from agent_draft_threads where draft_id = $1::uuid and author = $2`,
			r.draftID, string(rp.Caller.By)).Scan(&thread)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("drafts: find the thread: %w", err)
		}
		out, found, err = s.byKey(ctx, tx, thread, rp.Key)
		return err
	})
	return out, found, err
}

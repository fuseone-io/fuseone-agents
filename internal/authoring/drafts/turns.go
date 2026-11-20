package drafts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
The conversation's turns (NT-018 phase 3, contract 7).

BeginTurn reserves a turn's worst case against the day's ceiling and records
it in progress, in a short transaction: no lock is held while the model is
called. SettleTurn replaces the reservation with the real cost. A key already
used answers what was stored; one whose lease ran out before it was stored is
unknown, and is never called again.
*/

// Turn states.
const (
	TurnInProgress = "in_progress"
	TurnDone       = "done"
	TurnFailed     = "failed"
	TurnUnknown    = "unknown"
)

// CeilingLockKey serialises reservations against the day's ceiling: two turns
// judged at once could each fit what only one of them does.
const CeilingLockKey int64 = 0x0018_c0de_7e11_9a5e

var (
	// ErrTurnInFlight is a turn sent while another is in progress in the same
	// thread, or its own key still within its lease.
	ErrTurnInFlight = errors.New("drafts: a turn is already in progress in this thread")
	// ErrOverCeiling is a turn whose reservation does not fit what is left of
	// the day's authoring ceiling.
	ErrOverCeiling = errors.New("drafts: the turn does not fit the day's authoring ceiling")
	// ErrNoCeiling is a turn asked for with no ceiling to judge it against.
	ErrNoCeiling = errors.New("drafts: a turn needs a daily ceiling")
)

// Turn is one message and the model's answer to it.
type Turn struct {
	ID, Thread, Key string
	Seq             int
	Read            domain.Revision
	State           string
	// Reserved is the worst case set aside before the call; Cost is what the
	// call really cost, once settled.
	Reserved, Cost int64
	Pasted         bool
	At             time.Time
	// Message and Reply are what was said, read from the content store when
	// the turn is listed; Erased once retention or the thread's erasure took
	// them.
	Message, Reply string
	Erased         bool
	// Dropped and Stale are what the answer carried that never became a card: operations
	// discarded on the way in, and those made against a draft that moved during the call.
	Dropped, Stale int
	messageRef     string
	replyRef       string
}

// Beginning is one turn to begin.
type Beginning struct {
	Agent   domain.AgentID
	Key     string
	Message string
	Pasted  bool
	// Read is the draft's ETag the turn's prompt was built from.
	Read domain.Revision
	// Reserve is the turn's worst case, in micros; DailyMicros the ceiling.
	Reserve, DailyMicros int64
	Lease                time.Duration
	Caller               Caller
}

// Started is a turn begun, or the stored turn its key already named.
type Started struct {
	Turn     Turn
	Replayed bool
}

// BeginTurn reserves and records a turn, or answers the one its key names.
func (s *Store) BeginTurn(ctx context.Context, b Beginning) (Started, error) {
	if b.DailyMicros <= 0 {
		return Started{}, ErrNoCeiling
	}
	var out Started
	err := s.within(ctx, func(tx pgx.Tx) error {
		r, err := s.lockReachable(ctx, tx, b.Agent, b.Caller)
		if err != nil {
			return err
		}
		thread, err := threadOf(ctx, tx, r.draftID, b.Agent, b.Caller.By)
		if err != nil {
			return err
		}
		if earlier, found, err := s.byKey(ctx, tx, thread, b.Key); err != nil || found {
			out = earlier
			return err
		}
		if r.revision != b.Read {
			return &domain.StaleError{Current: r.revision}
		}
		if err := lapse(ctx, tx, thread); err != nil {
			return err
		}
		if err := withinCeiling(ctx, tx, b.Reserve, b.DailyMicros); err != nil {
			return err
		}
		turn, err := s.insertTurn(ctx, tx, r, thread, b)
		out = Started{Turn: turn}
		return err
	})
	return out, err
}

// byKey answers a key already used in the thread: the stored turn, made
// unknown if its lease ran out before it was stored, or ErrTurnInFlight while
// it is still within its lease.
func (s *Store) byKey(ctx context.Context, tx pgx.Tx, thread, key string) (Started, bool, error) {
	// Locked first, judged second: a settlement holding the row is waited
	// for, and what it wrote is what is read.
	turn, found, err := readTurn(ctx, tx, `thread_id = $1::uuid and idempotency_key = $2 for update`, thread, key)
	if err != nil || !found {
		return Started{}, false, err
	}
	if turn.State == TurnInProgress {
		var expired bool
		if err := tx.QueryRow(ctx, `
			update agent_draft_turns set state = 'unknown'
			where turn_id = $1::uuid and state = 'in_progress' and lease_until < now() returning true`,
			turn.ID).Scan(&expired); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Started{}, false, ErrTurnInFlight
			}
			return Started{}, false, fmt.Errorf("drafts: judge the turn's lease: %w", err)
		}
		turn.State = TurnUnknown
	}
	return Started{Turn: turn, Replayed: true}, true, nil
}

// lapse makes unknown the thread's turn in progress whose lease ran out, so
// it holds the thread no longer, whatever key comes next. Locked first,
// judged second, like byKey.
func lapse(ctx context.Context, tx pgx.Tx, thread string) error {
	var id string
	err := tx.QueryRow(ctx, `
		select turn_id::text from agent_draft_turns where thread_id = $1::uuid and state = 'in_progress' for update`,
		thread).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("drafts: read the turn in progress: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		update agent_draft_turns set state = 'unknown'
		where turn_id = $1::uuid and state = 'in_progress' and lease_until < now()`, id); err != nil {
		return fmt.Errorf("drafts: judge the turn's lease: %w", err)
	}
	return nil
}

// withinCeiling judges a reservation against what the day already holds,
// under a lock every reservation takes.
func withinCeiling(ctx context.Context, tx pgx.Tx, reserve, daily int64) error {
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock($1)`, CeilingLockKey); err != nil {
		return fmt.Errorf("drafts: lock the ceiling: %w", err)
	}
	spent, err := SpentToday(ctx, tx)
	if err != nil {
		return err
	}
	if spent+reserve > daily {
		return fmt.Errorf("%w: %d spent or held, %d more asked, %d a day", ErrOverCeiling, spent, reserve, daily)
	}
	return nil
}

func (s *Store) insertTurn(ctx context.Context, tx pgx.Tx, r row, thread string, b Beginning) (Turn, error) {
	var seq int64
	if err := tx.QueryRow(ctx, `select coalesce(max(seq), 0) + 1 from agent_draft_turns where thread_id = $1::uuid`,
		thread).Scan(&seq); err != nil {
		return Turn{}, fmt.Errorf("drafts: number the turn: %w", err)
	}
	ref, err := s.content.RestoreForTx(ctx, tx, "authoring", thread, turnContentSeq(seq, 0), []byte(b.Message))
	if err != nil {
		return Turn{}, fmt.Errorf("drafts: keep the message: %w", err)
	}
	_, err = tx.Exec(ctx, `
		insert into agent_draft_turns (thread_id, idempotency_key, seq, draft_id, agent_id, author, draft_revision,
			state, reserved_micros, lease_until, message_ref, pasted, created_at)
		values ($1::uuid, $2, $3, $4::uuid, $5, $6, $7, 'in_progress', $8, now() + $9::interval, $10, $11, now())`,
		thread, b.Key, seq, r.draftID, string(b.Agent), string(b.Caller.By), string(b.Read),
		b.Reserve, fmt.Sprintf("%d milliseconds", b.Lease.Milliseconds()), ref, b.Pasted)
	if isUniqueViolation(err) {
		return Turn{}, ErrTurnInFlight
	}
	if err != nil {
		return Turn{}, fmt.Errorf("drafts: record the turn: %w", err)
	}
	turn, _, err := readTurn(ctx, tx, `thread_id = $1::uuid and idempotency_key = $2`, thread, b.Key)
	return turn, err
}

// turnContentSeq keeps a turn's message and reply apart from the thread's
// proposals in the content store: the thread owns all three, and the seq is
// what makes each reference its own.
func turnContentSeq(turn int64, part int64) int64 { return 1_000_000 + turn*2 + part }

/*
SpentToday is what authoring holds against today's ceiling: the confirmed cost
of every call, the reservation of every turn still in progress, and the
reservation kept for every turn whose result is unknown, which counts and is
never recorded as a cost. Interview calls record their cost in the trail;
turns keep theirs on their own row.
*/
func SpentToday(ctx context.Context, q interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}) (int64, error) {
	var micros int64
	err := q.QueryRow(ctx, `
		select
		  (select coalesce(sum((detail->>'micros')::bigint), 0) from admin_events
		    where action = 'authoring.spent' and at >= date_trunc('day', now()))
		+ (select coalesce(sum(case when state in ('in_progress', 'unknown') then reserved_micros
		                            else cost_micros end), 0)
		    from agent_draft_turns where created_at >= date_trunc('day', now()))`).Scan(&micros)
	if err != nil {
		return 0, fmt.Errorf("drafts: read the day's authoring spend: %w", err)
	}
	return micros, nil
}

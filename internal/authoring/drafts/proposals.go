package drafts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
)

/*
The conversation's proposals (NT-018 phase 3, contract 3).

The model only proposes: Propose records an operation in the proposing
author's thread, bound to the draft and the ETag the model read, and judged
then as an operation by proposal. Only that author accepting it, by its id,
applies it; Accept loads what was stored rather than anything the caller
sends.
*/

var (
	// ErrProposalInvalid is a proposal that failed validation when it was
	// made: shown crossed out, and never applied.
	ErrProposalInvalid = errors.New("drafts: the proposal is invalid and cannot be accepted")
	// ErrDecided is a proposal already accepted or rejected.
	ErrDecided = errors.New("drafts: the proposal was decided already")
)

// Proposal is one operation the model proposed.
type Proposal struct {
	ID, Thread, Draft string
	Agent             domain.AgentID
	Author            domain.UserID
	// Kind is the operation's kind, kept on the row: known even once the
	// body that says the rest was erased.
	Kind string
	// Read is the draft's ETag the model read when it proposed this.
	Read    domain.Revision
	Op      draft.Operation
	Valid   bool
	Refusal string
	// Turn is the conversation's turn that proposed it; empty for none.
	Turn string
	// Reason is the model's sentence on why, read from the content store;
	// empty for none, or once retention or erasing the thread took it.
	Reason    string
	reasonRef string
	// Decision is accepted, rejected, or empty while it waits.
	Decision string
	// Current is whether the draft, as the thread was read, is still one this waiting
	// proposal can be accepted on: the one the model read, or one only its own turn's
	// acceptances moved (#412). What applying it would leave is judged on acceptance.
	Current bool
	BodyRef string
	At      time.Time
}

// Proposing is one operation the model proposes, for the author whose turn
// it was.
type Proposing struct {
	Agent     domain.AgentID
	Op        draft.Operation
	Read      domain.Revision
	Caller    Caller
	Validator draft.Validator
	Now       time.Time
}

type keptBody struct{ ref, digest string }

// proposalBody is what the content store keeps: the operation's Body, and the
// reason when it is invalid. A valid proposal's body is exactly Body(op), so
// its digest is the operation's.
func proposalBody(op draft.Operation, refusal string) []byte {
	if refusal == "" {
		return draft.Body(op)
	}
	body, _ := json.Marshal(struct {
		Kind      string          `json:"kind"`
		Operation json.RawMessage `json:"operation"`
		Refused   string          `json:"refused"`
	}{op.Kind(), operationOf(draft.Body(op)), refusal})
	return body
}

func operationOf(body []byte) json.RawMessage {
	var head struct {
		Operation json.RawMessage `json:"operation"`
	}
	_ = json.Unmarshal(body, &head)
	return head.Operation
}

/*
Propose records one proposal in its author's thread.

Made against the draft as it is now, or not at all: a model that read an
older draft proposes against a state the author no longer has (412). It is
validated as an operation by proposal, on a copy, and kept either way: an
invalid one is shown with its reason and can never be accepted.
*/
func (s *Store) Propose(ctx context.Context, p Proposing) (Proposal, error) {
	var out Proposal
	err := s.within(ctx, func(tx pgx.Tx) error {
		r, err := s.lockReachable(ctx, tx, p.Agent, p.Caller)
		if err != nil {
			return err
		}
		if r.revision != p.Read {
			return &domain.StaleError{Current: r.revision}
		}
		d, err := draft.Decode(r.stored)
		if err != nil {
			return err
		}
		refusal, err := verdictOf(p.Validator.Validate(ctx, d, p.Op, draft.FromProposal, p.Now))
		if err != nil {
			return err
		}
		out, err = s.record(ctx, tx, r, p, refusal, said{})
		return err
	})
	return out, err
}

// said is where a proposal came from in the conversation: its turn and the
// model's reason, both empty for a proposal no turn made.
type said struct{ turn, reason string }

// reasonContentSeq keeps a proposal's reason apart from its body in the
// content store: the thread owns both, and the seq makes each reference its own.
func reasonContentSeq(seq int64) int64 { return 2_000_000 + seq }

// record keeps one proposal.
func (s *Store) record(ctx context.Context, tx pgx.Tx, r row, p Proposing, refusal string, from said) (Proposal, error) {
	thread, err := threadOf(ctx, tx, r.draftID, p.Agent, p.Caller.By)
	if err != nil {
		return Proposal{}, err
	}
	var seq int64
	if err := tx.QueryRow(ctx, `
		select coalesce(max(seq), 0) + 1 from agent_draft_proposals where thread_id = $1::uuid`,
		thread).Scan(&seq); err != nil {
		return Proposal{}, fmt.Errorf("drafts: number the proposal: %w", err)
	}
	body := proposalBody(p.Op, refusal)
	if len(body) > draft.MaxDefinitionBytes {
		return Proposal{}, fmt.Errorf("%w: a proposal of %d bytes", draft.ErrTooLarge, len(body))
	}
	ref, err := s.content.RestoreForTx(ctx, tx, "authoring", thread, seq, body)
	if err != nil {
		return Proposal{}, fmt.Errorf("drafts: keep the proposal: %w", err)
	}
	reasonRef := ""
	if from.reason != "" {
		if reasonRef, err = s.content.RestoreForTx(ctx, tx, "authoring", thread, reasonContentSeq(seq), []byte(from.reason)); err != nil {
			return Proposal{}, fmt.Errorf("drafts: keep the proposal's reason: %w", err)
		}
	}
	sum := sha256.Sum256(body)
	out := Proposal{
		Thread: thread, Draft: r.draftID, Agent: p.Agent, Author: p.Caller.By, Read: p.Read, Kind: p.Op.Kind(),
		Op: p.Op, Valid: refusal == "", Refusal: refusal, BodyRef: ref, Turn: from.turn, Reason: from.reason,
	}
	err = tx.QueryRow(ctx, `
		insert into agent_draft_proposals (thread_id, seq, draft_id, agent_id, author, draft_revision,
			kind, digest, body_ref, valid, created_at, turn_id, reason_ref)
		values ($1::uuid, $2, $3::uuid, $4, $5, $6, $7, $8, $9, $10, now(), nullif($11, '')::uuid, $12)
		returning proposal_id::text, created_at`,
		thread, seq, r.draftID, string(p.Agent), string(p.Caller.By), string(p.Read),
		p.Op.Kind(), hex.EncodeToString(sum[:]), ref, out.Valid, from.turn, reasonRef).Scan(&out.ID, &out.At)
	if err != nil {
		return Proposal{}, fmt.Errorf("drafts: record the proposal: %w", err)
	}
	return out, nil
}

// threadOf is the author's thread over the draft, opened the first time.
func threadOf(ctx context.Context, tx pgx.Tx, draftID string, agent domain.AgentID, author domain.UserID) (string, error) {
	if _, err := tx.Exec(ctx, `
		insert into agent_draft_threads (draft_id, agent_id, author, created_at)
		values ($1::uuid, $2, $3, now()) on conflict (draft_id, author) do nothing`,
		draftID, string(agent), string(author)); err != nil {
		return "", fmt.Errorf("drafts: open the thread: %w", err)
	}
	var thread string
	if err := tx.QueryRow(ctx, `
		select thread_id::text from agent_draft_threads where draft_id = $1::uuid and author = $2`,
		draftID, string(author)).Scan(&thread); err != nil {
		return "", fmt.Errorf("drafts: read the thread: %w", err)
	}
	return thread, nil
}

/*
verdictOf reads a validation's answer as a proposal's verdict. Only what the
validator decides about the operation makes a proposal invalid; an error that
kept it from deciding at all — a catalogue that could not be read — is not a
verdict, and is returned, so nothing is recorded and the turn can try again.
*/
func verdictOf(_ draft.Applied, err error) (string, error) {
	switch {
	case err == nil:
		return "", nil
	case errors.Is(err, draft.ErrRefused), errors.Is(err, draft.ErrInvalidOperation),
		errors.Is(err, draft.ErrHumanOnly), errors.Is(err, draft.ErrConversationOnly),
		errors.Is(err, draft.ErrTooLarge):
		return err.Error(), nil
	}
	return "", err
}

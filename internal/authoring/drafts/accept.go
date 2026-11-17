package drafts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
)

// Acceptance is a person accepting one proposal, named by its id alone.
type Acceptance struct {
	Agent     domain.AgentID
	Proposal  string
	Caller    Caller
	Validator draft.Validator
	Now       time.Time
}

// stored is a proposal's row, read under its lock.
type stored struct {
	Proposal
	digest string
}

func readProposal(ctx context.Context, tx pgx.Tx, agent domain.AgentID, id string, lock bool) (stored, bool, error) {
	var p stored
	var read string
	query := `
		select p.proposal_id::text, p.thread_id::text, p.draft_id::text, p.author, p.draft_revision,
		       p.kind, p.valid, p.digest, p.body_ref, p.created_at, coalesce(d.decision, ''),
		       coalesce(p.turn_id::text, ''), p.reason_ref
		from agent_draft_proposals p
		left join agent_draft_proposal_decisions d on d.proposal_id = p.proposal_id
		where p.proposal_id::text = $1 and p.agent_id = $2`
	if lock {
		query += ` for update of p`
	}
	err := tx.QueryRow(ctx, query, id, string(agent)).Scan(
		&p.ID, &p.Thread, &p.Draft, &p.Author, &read, &p.Kind, &p.Valid, &p.digest, &p.BodyRef, &p.At, &p.Decision, &p.Turn, &p.reasonRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return stored{}, false, nil
	}
	if err != nil {
		return stored{}, false, fmt.Errorf("drafts: read the proposal: %w", err)
	}
	p.Agent, p.Read = agent, domain.Revision(read)
	return p, true, nil
}

// body is what the content store keeps for a proposal, or nothing once
// retention or erasing the thread took it.
func body(ctx context.Context, tx pgx.Tx, ref string) ([]byte, bool, error) {
	var data []byte
	var erased bool
	err := tx.QueryRow(ctx, `select bytes, erased_at is not null from run_content where ref = $1`, ref).Scan(&data, &erased)
	if errors.Is(err, pgx.ErrNoRows) || erased {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("drafts: read the proposal's body: %w", err)
	}
	return data, true, nil
}

/*
Accept applies one proposal, by its id, for its author.

In order: the proposal is its author's, or it is no proposal at all (404);
the draft is reachable, on its scope and where its agent lives (404); it was
not decided already (409); it was valid when made (400); its body is still
kept (404); the scope it moves the draft to (403); the draft is still the
one, at the ETag, the model read (412); then its stored operation, validated
again on the draft as it is.
*/
func (s *Store) Accept(ctx context.Context, a Acceptance) (Opened, error) {
	var out Opened
	err := s.within(ctx, func(tx pgx.Tx) error {
		p, r, err := s.lockDecidable(ctx, tx, a.Agent, a.Proposal, a.Caller)
		if err != nil {
			return err
		}
		// Settled when it was made, on the scope the author already
		// reaches: an invalid proposal says nothing about any other.
		if !p.Valid {
			return ErrProposalInvalid
		}
		apply, d, err := applicationOf(ctx, tx, p, r, a)
		if err != nil {
			return err
		}
		applied, err := a.Validator.Validate(ctx, d, apply.Op, draft.FromProposal, a.Now)
		if err != nil {
			return err
		}
		if out, err = s.update(ctx, tx, r, apply, applied); err != nil {
			return err
		}
		return decide(ctx, tx, p.ID, "accepted", out.Revision, a.Caller.By)
	})
	return out, err
}

// applicationOf is the stored operation as an application to the draft,
// once its body is still kept (404), the scope it moves the draft to is the
// caller's (403), and the draft is the one, at the ETag, the model read (412).
func applicationOf(ctx context.Context, tx pgx.Tx, p stored, r row, a Acceptance) (Application, draft.Draft, error) {
	data, found, err := body(ctx, tx, p.BodyRef)
	if err != nil || !found {
		return Application{}, draft.Draft{}, firstOrErr(err, ErrNotFound)
	}
	op, err := draft.ParseBody(data)
	if err != nil {
		return Application{}, draft.Draft{}, err
	}
	apply := Application{
		Agent: a.Agent, Op: op, Origin: draft.FromProposal, Caller: a.Caller,
		Validator: a.Validator, Now: a.Now, kept: &keptBody{ref: p.BodyRef, digest: p.digest},
	}
	d, err := draft.Decode(r.stored)
	if err != nil {
		return Application{}, draft.Draft{}, err
	}
	if err := destinationAllowed(d, r.scope, apply); err != nil {
		return Application{}, draft.Draft{}, err
	}
	if r.draftID != p.Draft || r.revision != p.Read {
		return Application{}, draft.Draft{}, &domain.StaleError{Current: r.revision}
	}
	return apply, d, nil
}

// Reject decides a proposal against, for its author. The draft is untouched.
func (s *Store) Reject(ctx context.Context, agent domain.AgentID, id string, c Caller) error {
	return s.within(ctx, func(tx pgx.Tx) error {
		p, _, err := s.lockDecidable(ctx, tx, agent, id, c)
		if err != nil {
			return err
		}
		return decide(ctx, tx, p.ID, "rejected", "", c.By)
	})
}

/*
lockDecidable locks the draft, then a proposal its caller may decide.

The draft's row first, then the proposal's, as every path takes them: a
decision that locked the proposal before the draft deadlocked against a
reading of the thread that held the draft.
*/
func (s *Store) lockDecidable(ctx context.Context, tx pgx.Tx, agent domain.AgentID, id string, c Caller) (stored, row, error) {
	r, err := s.lockReachable(ctx, tx, agent, c)
	if err != nil {
		return stored{}, row{}, err
	}
	p, found, err := readProposal(ctx, tx, agent, id, true)
	if err != nil {
		return stored{}, row{}, err
	}
	// Another author's proposal is not there for this caller: threads are
	// each author's own, and a proposal is decided only by whose turn made it.
	if !found || p.Author != c.By {
		return stored{}, row{}, ErrNotFound
	}
	if p.Decision != "" {
		return stored{}, row{}, ErrDecided
	}
	return p, r, nil
}

func decide(ctx context.Context, tx pgx.Tx, id, decision string, revision domain.Revision, by domain.UserID) error {
	if _, err := tx.Exec(ctx, `
		insert into agent_draft_proposal_decisions (proposal_id, decision, revision, by_user, at)
		values ($1::uuid, $2, $3, $4, now())`, id, decision, string(revision), string(by)); err != nil {
		return fmt.Errorf("drafts: decide the proposal: %w", err)
	}
	return nil
}

// Proposals is the caller's own thread over the agent's draft, oldest first,
// each with its operation and reason when the body is still kept. A reading:
// it locks nothing, so it never stands in a decision's way.
func (s *Store) Proposals(ctx context.Context, agent domain.AgentID, c Caller) ([]Proposal, error) {
	var out []Proposal
	err := s.within(ctx, func(tx pgx.Tx) error {
		r, err := s.reachableRow(ctx, tx, agent, c, false)
		if err != nil {
			return err
		}
		ids, err := threadProposals(ctx, tx, r.draftID, c.By)
		if err != nil {
			return err
		}
		for _, id := range ids {
			p, _, err := readProposal(ctx, tx, agent, id, false)
			if err != nil {
				return err
			}
			if data, found, err := body(ctx, tx, p.BodyRef); err != nil {
				return err
			} else if found {
				p.Op, _ = draft.ParseBody(data)
				p.Refusal = refusalOf(data)
			}
			if p.reasonRef != "" {
				reason, _, err := body(ctx, tx, p.reasonRef)
				if err != nil {
					return err
				}
				p.Reason = string(reason)
			}
			out = append(out, p.Proposal)
		}
		return nil
	})
	return out, err
}

func threadProposals(ctx context.Context, tx pgx.Tx, draftID string, author domain.UserID) ([]string, error) {
	rows, err := tx.Query(ctx, `
		select p.proposal_id::text from agent_draft_proposals p
		join agent_draft_threads t on t.thread_id = p.thread_id
		where t.draft_id = $1::uuid and t.author = $2 order by p.seq`, draftID, string(author))
	if err != nil {
		return nil, fmt.Errorf("drafts: read the thread: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func refusalOf(data []byte) string {
	var head struct {
		Refused string `json:"refused"`
	}
	_ = json.Unmarshal(data, &head)
	return head.Refused
}

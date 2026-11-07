// Package regression keeps the corrections an author made, so every future
// version is checked against them (PRD FU-12).
//
// It is the half of simulation that makes it a loop rather than a diagnosis:
// running a set of occurrences tells an author what would happen once, and a
// corpus tells them whether the thing they already fixed is still fixed.
package regression

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

// ErrNothingToCheck means a correction arrived with no expectation on it.
var ErrNothingToCheck = errors.New("regression: a case with no expectation cannot be checked")

type Store struct {
	pool    *pgxpool.Pool
	fence   *fence
	content *ledger.Content
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		pool: pool, fence: newFence(pool.Config().ConnConfig), content: ledger.NewContent(pool),
	}
}

// Record stores one correction, replacing a case of the same id.
//
// A case with no expectation is refused. A correction nothing can check is a
// note — notes are welcome, and they are not a regression case: letting one in
// makes the battery report as passing something that was never checked.
func (s *Store) Record(ctx context.Context, c domain.RegressionCase) error {
	switch {
	case strings.TrimSpace(c.ID) == "" || strings.TrimSpace(string(c.Agent)) == "":
		return fmt.Errorf("regression: a case needs an agent and an id")
	case len(c.Expectations) == 0:
		return ErrNothingToCheck
	}
	for _, e := range c.Expectations {
		if !e.Kind.Valid() {
			return fmt.Errorf("regression: %q is not something that can be checked", e.Kind)
		}
	}

	expectations, err := json.Marshal(c.Expectations)
	if err != nil {
		return fmt.Errorf("regression: encode expectations: %w", err)
	}

	return s.write(ctx, func(tx pgx.Tx) error {
		return s.recordCase(ctx, tx, c, expectations)
	})
}

// caseRow is what one recorded case writes.
type caseRow struct {
	domain.RegressionCase
	expectations []byte
	ref          string
}

func upsertCase(ctx context.Context, tx pgx.Tx, c caseRow) error {
	_, err := tx.Exec(ctx, `
		insert into regression_cases (
			agent_id, case_id, company_id, area_id,
			input_ref, expectations, from_run, note, created_by, model, effort, expectations_ref
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		on conflict (agent_id, case_id) do update set
			input_ref = excluded.input_ref,
			expectations = excluded.expectations,
			expectations_ref = excluded.expectations_ref,
			from_run = excluded.from_run,
			note = excluded.note,
			-- Re-recorded, because a correction made today held against
			-- today's model. Keeping the old one would report drift for ever
			-- after somebody deliberately moved.
			model = excluded.model,
			effort = excluded.effort`,
		string(c.Agent), c.ID, string(c.Scope.Company), string(c.Scope.Area),
		c.InputRef, c.expectations, string(c.FromRun), c.Note, string(c.CreatedBy),
		c.Model, c.Effort, c.ref)
	if err != nil {
		return fmt.Errorf("regression: record %s: %w", c.ID, err)
	}
	return nil
}

// List returns an agent's corpus, oldest first.
//
// The order is stable across reads because two reports of the same battery
// have to be comparable case by case, and they are not if case three is case
// one on the second read.
func (s *Store) List(ctx context.Context, agent domain.AgentID) ([]domain.RegressionCase, error) {
	rows, err := s.pool.Query(ctx, `
		select c.case_id, c.company_id, c.area_id, c.input_ref, c.expectations,
		       c.from_run, c.note, c.created_by, c.created_at, c.model, c.effort,
		       coalesce(rc.erased_at is not null, false),
		       c.expectations_ref, ex.ref is not null, ex.bytes,
		       coalesce(ex.erased_at is not null, false)
		from regression_cases c
		left join run_content rc on rc.ref = c.input_ref
		left join run_content ex on ex.ref = c.expectations_ref
		where c.agent_id = $1
		order by c.created_at, c.case_id`, string(agent))
	if err != nil {
		return nil, fmt.Errorf("regression: list %s: %w", agent, err)
	}
	defer rows.Close()

	var out []domain.RegressionCase
	for rows.Next() {
		c := domain.RegressionCase{Agent: agent}
		var held heldExpectations
		var company, area, fromRun, by string
		var inputErased bool
		if err := rows.Scan(&c.ID, &company, &area, &c.InputRef, &held.column,
			&fromRun, &c.Note, &by, &c.CreatedAt, &c.Model, &c.Effort, &inputErased,
			&held.ref, &held.found, &held.object, &held.erased); err != nil {
			return nil, fmt.Errorf("regression: scan case: %w", err)
		}
		var err error
		if c.Expectations, err = held.read(); err != nil {
			return nil, fmt.Errorf("regression: decode expectations of %s: %w", c.ID, err)
		}
		c.Erased = inputErased || held.erased
		c.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
		c.FromRun, c.CreatedBy = domain.RunID(fromRun), domain.UserID(by)
		out = append(out, c)
	}
	return out, rows.Err()
}

/*
Delete removes one case from the corpus, which writes a new version without it.

It erases nothing. An earlier version still names the case, because it records what
was checked, so the copy and the replays stay until retention or a subject erasure
takes them. The fence is still taken: it keeps this apart from anything copying the
corpus, like every other write.
*/
func (s *Store) Delete(ctx context.Context, agent domain.AgentID, id string, by domain.UserID) error {
	return s.Exclusive(ctx, []domain.AgentID{agent}, func(ctx context.Context) error {
		return s.write(ctx, func(tx pgx.Tx) error {
			return deleteCase(ctx, tx, agent, id, by)
		})
	})
}

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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// ErrNothingToCheck means a correction arrived with no expectation on it.
var ErrNothingToCheck = errors.New("regression: a case with no expectation cannot be checked")

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

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

	_, err = s.pool.Exec(ctx, `
		insert into regression_cases (
			agent_id, case_id, company_id, area_id,
			input_ref, expectations, from_run, note, created_by
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		on conflict (agent_id, case_id) do update set
			input_ref = excluded.input_ref,
			expectations = excluded.expectations,
			from_run = excluded.from_run,
			note = excluded.note`,
		string(c.Agent), c.ID, string(c.Scope.Company), string(c.Scope.Area),
		c.InputRef, expectations, string(c.FromRun), c.Note, string(c.CreatedBy))
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
		select case_id, company_id, area_id, input_ref, expectations,
		       from_run, note, created_by, created_at
		from regression_cases
		where agent_id = $1
		order by created_at, case_id`, string(agent))
	if err != nil {
		return nil, fmt.Errorf("regression: list %s: %w", agent, err)
	}
	defer rows.Close()

	var out []domain.RegressionCase
	for rows.Next() {
		c := domain.RegressionCase{Agent: agent}
		var expectations []byte
		var company, area, fromRun, by string
		if err := rows.Scan(&c.ID, &company, &area, &c.InputRef, &expectations,
			&fromRun, &c.Note, &by, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("regression: scan case: %w", err)
		}
		if err := json.Unmarshal(expectations, &c.Expectations); err != nil {
			return nil, fmt.Errorf("regression: decode expectations of %s: %w", c.ID, err)
		}
		c.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
		c.FromRun, c.CreatedBy = domain.RunID(fromRun), domain.UserID(by)
		out = append(out, c)
	}
	return out, rows.Err()
}

// Delete removes one case from the corpus.
func (s *Store) Delete(ctx context.Context, agent domain.AgentID, id string) error {
	if _, err := s.pool.Exec(ctx,
		`delete from regression_cases where agent_id = $1 and case_id = $2`,
		string(agent), id); err != nil {
		return fmt.Errorf("regression: delete %s: %w", id, err)
	}
	return nil
}

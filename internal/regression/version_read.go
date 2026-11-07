package regression

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// ErrNoSuchVersion means a battery names a version of the corpus that is not there.
// An empty answer would read as a corpus with nothing broken.
var ErrNoSuchVersion = errors.New("regression: no such version of this corpus")

/*
Pinned answers the latest version of an agent's corpus, with its cases.

A battery replays these and names the version. Read from the version and not from the
working set, so what ran and what the run says it ran cannot differ, whoever records a
correction in between: writers do not wait for a battery opening.

An agent with no version has no corpus, and that is zero and no cases.
*/
func (s *Store) Pinned(
	ctx context.Context, agent domain.AgentID,
) (int64, []domain.RegressionCase, error) {
	var version int64
	err := s.pool.QueryRow(ctx, `
		select version from dataset_versions where dataset_id = $1
		order by version desc limit 1`, defaultDatasetOf(agent)).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, fmt.Errorf("regression: read the latest version of %s: %w", agent, err)
	}
	cases, err := s.AtVersion(ctx, agent, version)
	return version, cases, err
}

/*
AtVersion answers an agent's corpus as one version recorded it, ordered by case id.

Erasure wins over a pin: a version keeps naming a case and never brings back what it
said, so content gone since reads as erased. Model and effort are not part of a version,
so they come from the working row, and a case deleted since has neither.
*/
func (s *Store) AtVersion(
	ctx context.Context, agent domain.AgentID, version int64,
) ([]domain.RegressionCase, error) {
	var exists bool
	dataset := defaultDatasetOf(agent)
	if err := s.pool.QueryRow(ctx, `
		select exists (select 1 from dataset_versions where dataset_id = $1 and version = $2)`,
		dataset, version).Scan(&exists); err != nil {
		return nil, fmt.Errorf("regression: look up version %d of %s: %w", version, agent, err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: %s version %d", ErrNoSuchVersion, agent, version)
	}

	rows, err := s.pool.Query(ctx, `
		select v.case_id, v.input_ref, v.from_run, d.company_id, d.area_id,
		       coalesce(i.erased_at is not null, false),
		       v.expectations_ref, ex.ref is not null, ex.bytes,
		       coalesce(ex.erased_at is not null, false),
		       coalesce(c.model, ''), coalesce(c.effort, '')
		from dataset_version_cases v
		join datasets d on d.dataset_id = v.dataset_id
		left join run_content i on i.ref = v.input_ref
		left join run_content ex on ex.ref = v.expectations_ref
		left join regression_cases c on c.agent_id = d.agent_id and c.case_id = v.case_id
		where v.dataset_id = $1 and v.version = $2
		order by v.case_id collate "C"`, dataset, version)
	if err != nil {
		return nil, fmt.Errorf("regression: read version %d of %s: %w", version, agent, err)
	}
	defer rows.Close()

	var out []domain.RegressionCase
	for rows.Next() {
		c, err := scanVersionCase(rows)
		if err != nil {
			return nil, err
		}
		c.Agent = agent
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanVersionCase(rows pgx.Rows) (domain.RegressionCase, error) {
	var c domain.RegressionCase
	var held heldExpectations
	var company, area, fromRun string
	var inputErased bool
	if err := rows.Scan(&c.ID, &c.InputRef, &fromRun, &company, &area, &inputErased,
		&held.ref, &held.found, &held.object, &held.erased, &c.Model, &c.Effort); err != nil {
		return c, fmt.Errorf("regression: scan a version's case: %w", err)
	}
	expectations, err := held.read()
	if err != nil {
		return c, fmt.Errorf("regression: decode expectations of %s: %w", c.ID, err)
	}
	c.Expectations, c.Erased = expectations, inputErased || held.erased
	c.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	c.FromRun = domain.RunID(fromRun)
	return c, nil
}

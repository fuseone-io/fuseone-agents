package memory

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// A row becoming a value.
//
// Its own file because it is the one place the column order in the queries and
// the field order here have to agree, and the way that agreement breaks is
// somebody adding a column beside a statement without looking at what reads it.

func scan(rows pgx.Rows, now time.Time) ([]domain.MemoryAssertion, error) {
	defer rows.Close()
	var out []domain.MemoryAssertion
	for rows.Next() {
		a, err := scanAssertion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, effectiveStatus(a, now))
	}
	return out, rows.Err()
}

func scanSuggestions(rows pgx.Rows) ([]domain.MemorySuggestion, error) {
	defer rows.Close()
	var out []domain.MemorySuggestion
	for rows.Next() {
		s, err := scanSuggestion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanAssertion(row scanner) (domain.MemoryAssertion, error) {
	var a domain.MemoryAssertion
	var company, area, agent, status, createdBy, updatedBy string
	var evidence []byte
	var labels []string
	err := row.Scan(
		&a.ID, &company, &area, &agent, &a.Kind, &a.Subject, &a.Signature,
		&a.Claim, &evidence, &a.Observations, &a.Confirmed, &labels,
		&status, &a.ExpiresAt, &createdBy, &a.CreatedAt, &updatedBy, &a.UpdatedAt)
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	a.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	a.AgentID = domain.AgentID(agent)
	a.Status = domain.MemoryStatus(status)
	a.CreatedBy, a.UpdatedBy = domain.UserID(createdBy), domain.UserID(updatedBy)
	if err := json.Unmarshal(evidence, &a.Evidence); err != nil {
		return domain.MemoryAssertion{}, fmt.Errorf("memory: decode evidence: %w", err)
	}
	a.Labels = domain.NewLabels(labels...)
	return a, nil
}

func scanSuggestion(row scanner) (domain.MemorySuggestion, error) {
	var s domain.MemorySuggestion
	var company, area, agent, status, createdBy, updatedBy string
	var coveredBy *string
	var evidence []byte
	var labels []string
	err := row.Scan(
		&s.ID, &s.AssertionID, &company, &area, &agent, &s.Kind, &s.Subject,
		&s.Signature, &s.Claim, &evidence, &s.Observations, &labels,
		&status, &s.ExpiresAt, &createdBy, &s.CreatedAt, &updatedBy, &s.UpdatedAt,
		&coveredBy)
	if err != nil {
		return domain.MemorySuggestion{}, err
	}
	s.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	s.AgentID = domain.AgentID(agent)
	s.Status = domain.MemorySuggestionStatus(status)
	if coveredBy != nil {
		s.CoveredBy = *coveredBy
	}
	s.CreatedBy, s.UpdatedBy = domain.UserID(createdBy), domain.UserID(updatedBy)
	if err := json.Unmarshal(evidence, &s.Evidence); err != nil {
		return domain.MemorySuggestion{}, fmt.Errorf("memory: decode suggestion evidence: %w", err)
	}
	s.Labels = domain.NewLabels(labels...)
	return s, nil
}

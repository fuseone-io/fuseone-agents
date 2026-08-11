package ledger

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// stepColumns is the single definition of the read shape. Every query selects
// through it so scanStep never has to guess at column order.
const stepColumns = `
	run_id, seq, kind, company_id, area_id, agent_id, version_id, on_behalf_of,
	payload, labels,
	input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_micros,
	idem_key, policy_hash, at, prev_hash, hash`

func scanSteps(rows pgx.Rows) ([]domain.Step, error) {
	var out []domain.Step
	for rows.Next() {
		var (
			s        domain.Step
			runID    string
			kind     string
			company  string
			area     string
			agentID  string
			version  string
			onBehalf string
			payload  []byte
			labels   []string
		)
		if err := rows.Scan(
			&runID, &s.Seq, &kind, &company, &area, &agentID, &version, &onBehalf,
			&payload, &labels,
			&s.Cost.InputTokens, &s.Cost.OutputTokens,
			&s.Cost.CacheReadTokens, &s.Cost.CacheWriteTokens, &s.Cost.Micros,
			&s.IdemKey, &s.PolicyHash, &s.At, &s.PrevHash, &s.Hash,
		); err != nil {
			return nil, fmt.Errorf("scan step: %w", err)
		}

		s.RunID = domain.RunID(runID)
		s.Kind = domain.StepKind(kind)
		s.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
		s.AgentID = domain.AgentID(agentID)
		s.VersionID = domain.VersionID(version)
		s.OnBehalfOf = domain.UserID(onBehalf)
		s.Labels = domain.NewLabels(labels...)

		// jsonb reorders keys and strips whitespace, so these are not the bytes
		// that were written. domain.NewStep hashed the canonical form, so
		// canonicalising again here reproduces exactly that.
		s.Payload = domain.CanonicalJSON(payload)

		// Postgres stores timestamptz at microsecond precision and returns it
		// in the session's zone. The hash was computed over UTC micros.
		s.At = s.At.UTC()

		out = append(out, s)
	}
	return out, rows.Err()
}

// rowScanner is what both pgx.Rows and pgx.Row satisfy for a single row.
type rowScanner interface{ Scan(dest ...any) error }

func scanRunSummary(row rowScanner) (domain.RunSummary, error) {
	var (
		s                        domain.RunSummary
		runID, company, area     string
		agent, version, onBehalf string
		labels                   []string
		pendingTool, pendingRule *string
		pendingReason            *string
		pendingAtSeq             *int64
		endedAt                  *time.Time
	)

	if err := row.Scan(&runID, &company, &area, &agent, &version, &onBehalf,
		&s.Phase, &s.Seq,
		&s.Cost.Micros, &s.Cost.InputTokens, &s.Cost.OutputTokens,
		&s.Cost.CacheReadTokens, &s.Cost.CacheWriteTokens,
		&s.ReservedMicros, &s.ToolCalls, &labels,
		&pendingTool, &pendingRule, &pendingReason, &pendingAtSeq,
		&s.StartedAt, &endedAt, &s.UpdatedAt); err != nil {
		return domain.RunSummary{}, err
	}

	s.RunID = domain.RunID(runID)
	s.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	s.AgentID = domain.AgentID(agent)
	s.VersionID = domain.VersionID(version)
	s.OnBehalfOf = domain.UserID(onBehalf)
	s.Labels = domain.NewLabels(labels...)

	if endedAt != nil {
		s.EndedAt = *endedAt
	}
	if pendingTool != nil {
		s.PendingApproval = &domain.PendingApprovalSummary{
			Tool: domain.ToolID(*pendingTool), AtSeq: derefInt64(pendingAtSeq),
			Rule: derefString(pendingRule), Reason: derefString(pendingReason),
		}
	}
	return s, nil
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

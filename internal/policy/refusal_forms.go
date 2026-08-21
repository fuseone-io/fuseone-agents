package policy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// ErrRefusalClaimLost means another worker now owns the alert this worker
// tried to finish. The message may have left; retrying is safer than marking a
// conversation quiet when nobody knows whether it heard.
var ErrRefusalClaimLost = errors.New("policy: refusal alert claim lost")

// RefusalForm is the low-cardinality shape of a Gate block that people should
// hear about once. A run id is still kept so the notice links to the first
// concrete example instead of to an abstract rule.
type RefusalForm struct {
	Scope      domain.Scope
	RuleKey    string
	Rule       string
	PolicyCode string
	Tool       domain.ToolID
	Effect     domain.Effect
	Verdict    domain.Verdict

	FirstRunID   domain.RunID
	FirstSeq     int64
	FirstAgentID domain.AgentID
	FirstSeenAt  time.Time

	LastRunID   domain.RunID
	LastSeq     int64
	LastAgentID domain.AgentID
	LastSeenAt  time.Time
}

// RefusalForms stores the projection used to announce a new Gate block once.
type RefusalForms struct{ pool *pgxpool.Pool }

func NewRefusalForms(pool *pgxpool.Pool) *RefusalForms { return &RefusalForms{pool: pool} }

// Import folds newly recorded Gate blocks into the projection.
//
// It reads run_steps rather than asking the Gate again: the alert is about the
// decision that happened, under the policy hash and tool digest of that time.
func (s *RefusalForms) Import(ctx context.Context, until time.Time, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("policy: begin refusal import: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cursor, err := readRefusalCursor(ctx, tx)
	if err != nil {
		return 0, err
	}

	found, err := readBlockedDecisions(ctx, tx, cursor, until, limit)
	if err != nil {
		return 0, err
	}
	for _, f := range found {
		if err := upsertRefusalForm(ctx, tx, f); err != nil {
			return 0, err
		}
	}

	next := cursor
	if len(found) == 0 {
		next = refusalCursor{at: until.UTC()}
	} else if len(found) < limit {
		next = refusalCursor{at: until.UTC()}
	} else {
		last := found[len(found)-1]
		next = refusalCursor{at: last.FirstSeenAt, runID: string(last.FirstRunID), seq: last.FirstSeq}
	}
	if err := writeRefusalCursor(ctx, tx, next); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("policy: commit refusal import: %w", err)
	}
	return len(found), nil
}

// Claim leases pending forms so multiple workers can run the announcer.
func (s *RefusalForms) Claim(
	ctx context.Context, owner string, lease time.Duration, limit int,
) ([]RefusalForm, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		with picked as (
			select company_id, area_id, rule_key, tool, effect, verdict
			from gate_refusal_forms
			where announced_at is null
			  and (lease_until is null or lease_until < now())
			order by first_seen_at
			limit $2
			for update skip locked
		)
		update gate_refusal_forms f
		   set lease_owner = $1,
		       lease_until = now() + ($3::bigint * interval '1 millisecond')
		  from picked p
		 where f.company_id = p.company_id
		   and f.area_id = p.area_id
		   and f.rule_key = p.rule_key
		   and f.tool = p.tool
		   and f.effect = p.effect
		   and f.verdict = p.verdict
		returning `+refusalFormColumns("f")+`
		`, owner, limit, lease.Milliseconds())
	if err != nil {
		return nil, fmt.Errorf("policy: claim refusal alerts: %w", err)
	}
	defer rows.Close()
	return scanRefusalForms(rows)
}

// MarkAnnounced finishes the claim after the message has left.
func (s *RefusalForms) MarkAnnounced(
	ctx context.Context, f RefusalForm, owner string, at time.Time,
) error {
	tag, err := s.pool.Exec(ctx, `
		update gate_refusal_forms
		   set announced_at = $8,
		       lease_owner = '',
		       lease_until = null
		 where company_id = $1
		   and area_id = $2
		   and rule_key = $3
		   and tool = $4
		   and effect = $5
		   and verdict = $6
		   and lease_owner = $7
		   and announced_at is null`,
		string(f.Scope.Company), string(f.Scope.Area), f.RuleKey, string(f.Tool),
		int16(f.Effect), int16(f.Verdict), owner, at.UTC())
	if err != nil {
		return fmt.Errorf("policy: mark refusal announced: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRefusalClaimLost
	}
	return nil
}

type refusalCursor struct {
	at    time.Time
	runID string
	seq   int64
}

func readRefusalCursor(ctx context.Context, tx pgx.Tx) (refusalCursor, error) {
	var c refusalCursor
	err := tx.QueryRow(ctx, `
		select scanned_at, scanned_run_id, scanned_seq
		from gate_refusal_alert_cursor
		where id = true
		for update`).Scan(&c.at, &c.runID, &c.seq)
	if err == nil {
		c.at = c.at.UTC()
		return c, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return refusalCursor{}, fmt.Errorf("policy: read refusal cursor: %w", err)
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		insert into gate_refusal_alert_cursor (id, scanned_at) values (true, $1)
		on conflict (id) do nothing`, now); err != nil {
		return refusalCursor{}, fmt.Errorf("policy: initialise refusal cursor: %w", err)
	}
	return refusalCursor{at: now}, nil
}

func writeRefusalCursor(ctx context.Context, tx pgx.Tx, c refusalCursor) error {
	_, err := tx.Exec(ctx, `
		update gate_refusal_alert_cursor
		   set scanned_at = $1,
		       scanned_run_id = $2,
		       scanned_seq = $3
		 where id = true`, c.at.UTC(), c.runID, c.seq)
	if err != nil {
		return fmt.Errorf("policy: advance refusal cursor: %w", err)
	}
	return nil
}

func readBlockedDecisions(
	ctx context.Context, tx pgx.Tx, cursor refusalCursor, until time.Time, limit int,
) ([]RefusalForm, error) {
	rows, err := tx.Query(ctx, `
		select company_id, area_id,
		       coalesce(nullif(payload->>'policy_code', ''), nullif(payload->>'rule', ''), 'unknown') as rule_key,
		       coalesce(payload->>'rule', '') as rule,
		       coalesce(payload->>'policy_code', '') as policy_code,
		       payload->>'tool' as tool,
		       coalesce((payload->>'effect')::smallint, 0::smallint) as effect,
		       coalesce((payload->>'verdict')::smallint, 0::smallint) as verdict,
		       run_id, seq, agent_id, at
		  from run_steps
		 where kind = $1
		   and coalesce((payload->>'verdict')::smallint, 0::smallint) = $2
		   and (at, run_id, seq) > ($3, $4, $5)
		   and at <= $6
		   and not exists (
		       select 1 from runs r where r.run_id = run_steps.run_id and r.simulated
		   )
		 order by at, run_id, seq
		 limit $7`,
		string(domain.StepGateDecided), int16(domain.VerdictBlock),
		cursor.at.UTC(), cursor.runID, cursor.seq, until.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("policy: read blocked Gate decisions: %w", err)
	}
	defer rows.Close()
	return scanDecisionRows(rows)
}

func upsertRefusalForm(ctx context.Context, tx pgx.Tx, f RefusalForm) error {
	_, err := tx.Exec(ctx, `
		insert into gate_refusal_forms (
			company_id, area_id, rule_key, rule, policy_code,
			tool, effect, verdict,
			first_run_id, first_seq, first_agent_id, first_seen_at,
			last_run_id, last_seq, last_agent_id, last_seen_at
		) values (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$9,$10,$11,$12
		)
		on conflict (company_id, area_id, rule_key, tool, effect, verdict)
		do update set
			rule = excluded.rule,
			policy_code = excluded.policy_code,
			last_run_id = excluded.last_run_id,
			last_seq = excluded.last_seq,
			last_agent_id = excluded.last_agent_id,
			last_seen_at = excluded.last_seen_at`,
		string(f.Scope.Company), string(f.Scope.Area), f.RuleKey, f.Rule, f.PolicyCode,
		string(f.Tool), int16(f.Effect), int16(f.Verdict),
		string(f.FirstRunID), f.FirstSeq, string(f.FirstAgentID), f.FirstSeenAt.UTC())
	if err != nil {
		return fmt.Errorf("policy: upsert refusal form: %w", err)
	}
	return nil
}

func refusalFormColumns(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return `
	` + prefix + `company_id, ` + prefix + `area_id, ` + prefix + `rule_key,
	` + prefix + `rule, ` + prefix + `policy_code,
	` + prefix + `tool, ` + prefix + `effect, ` + prefix + `verdict,
	` + prefix + `first_run_id, ` + prefix + `first_seq,
	` + prefix + `first_agent_id, ` + prefix + `first_seen_at,
	` + prefix + `last_run_id, ` + prefix + `last_seq,
	` + prefix + `last_agent_id, ` + prefix + `last_seen_at`
}

func scanRefusalForms(rows pgx.Rows) ([]RefusalForm, error) {
	var out []RefusalForm
	for rows.Next() {
		f, err := scanRefusalForm(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func scanDecisionRows(rows pgx.Rows) ([]RefusalForm, error) {
	var out []RefusalForm
	for rows.Next() {
		f, err := scanRefusalDecision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func scanRefusalForm(rows pgx.Rows) (RefusalForm, error) {
	var (
		f             RefusalForm
		company, area string
		tool          string
		effect        int16
		verdict       int16
		firstRun      string
		firstAgent    string
		lastRun       string
		lastAgent     string
	)
	if err := rows.Scan(
		&company, &area, &f.RuleKey, &f.Rule, &f.PolicyCode,
		&tool, &effect, &verdict,
		&firstRun, &f.FirstSeq, &firstAgent, &f.FirstSeenAt,
		&lastRun, &f.LastSeq, &lastAgent, &f.LastSeenAt,
	); err != nil {
		return RefusalForm{}, fmt.Errorf("policy: scan refusal form: %w", err)
	}
	f.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	f.Tool = domain.ToolID(tool)
	f.Effect = domain.Effect(effect)
	f.Verdict = domain.Verdict(verdict)
	f.FirstRunID = domain.RunID(firstRun)
	f.FirstAgentID = domain.AgentID(firstAgent)
	f.LastRunID = domain.RunID(lastRun)
	f.LastAgentID = domain.AgentID(lastAgent)
	f.FirstSeenAt = f.FirstSeenAt.UTC()
	f.LastSeenAt = f.LastSeenAt.UTC()
	return f, nil
}

func scanRefusalDecision(rows pgx.Rows) (RefusalForm, error) {
	var (
		f             RefusalForm
		company, area string
		tool          string
		effect        int16
		verdict       int16
		runID         string
		agentID       string
	)
	if err := rows.Scan(
		&company, &area, &f.RuleKey, &f.Rule, &f.PolicyCode,
		&tool, &effect, &verdict,
		&runID, &f.FirstSeq, &agentID, &f.FirstSeenAt,
	); err != nil {
		return RefusalForm{}, fmt.Errorf("policy: scan blocked decision: %w", err)
	}
	f.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	f.Tool = domain.ToolID(tool)
	f.Effect = domain.Effect(effect)
	f.Verdict = domain.Verdict(verdict)
	f.FirstRunID = domain.RunID(runID)
	f.FirstAgentID = domain.AgentID(agentID)
	f.FirstSeenAt = f.FirstSeenAt.UTC()
	f.LastRunID = f.FirstRunID
	f.LastSeq = f.FirstSeq
	f.LastAgentID = f.FirstAgentID
	f.LastSeenAt = f.FirstSeenAt
	return f, nil
}

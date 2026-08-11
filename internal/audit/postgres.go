package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// Postgres reads both trails as one ordered stream.
//
// A union in the database rather than two queries merged in Go: merging pages
// from two sources in the process cannot be paginated correctly — the tenth
// page of a merge is not the merge of two tenth pages — and an audit trail
// that silently drops entries between pages is not one.
type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

// verbs maps a ledger step to the past-tense verb the trail reads in. Only
// steps a person would audit are here; the rest are the interpreter's
// bookkeeping and would bury the decisions among them.
const ledgerVerbs = `
	case
		when kind = 'approval_decided' then
			case when payload->>'approved' = 'true' then 'approval.granted' else 'approval.refused' end
		when payload->>'verdict' = '1' then 'gate.allowed'
		when payload->>'verdict' = '2' then 'gate.constrained'
		when payload->>'verdict' = '3' then 'gate.escalated'
		when payload->>'verdict' = '4' then 'gate.blocked'
		else 'gate.decided'
	end`

func (p *Postgres) Read(ctx context.Context, filter Filter, limit int) ([]Entry, error) {
	where, args := predicate(filter)

	// Both halves produce the same columns so one ORDER BY covers the union.
	query := `
		with trail as (
			select at, 'ledger' as source,
			       case when kind = 'approval_decided'
			            then coalesce(payload->>'by', '') else agent_id end as actor,
			       ` + ledgerVerbs + ` as verb,
			       coalesce(payload->>'tool', run_id) as target,
			       company_id, area_id, payload as detail,
			       run_id, seq, encode(hash, 'hex') as hash
			from run_steps
			where kind in ('gate_decided', 'approval_decided')

			union all

			select at, 'admin' as source, principal_id as actor,
			       action as verb, target,
			       company_id, area_id, detail,
			       '' as run_id, 0 as seq, '' as hash
			from admin_events
		)
		select at, source, actor, verb, target, company_id, area_id, detail, run_id, seq, hash
		from trail ` + where + `
		order by at desc, seq desc
		limit $` + fmt.Sprint(len(args)+1)

	args = append(args, limit)

	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("audit: read trail: %w", err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var (
			entry         Entry
			company, area string
			detail        []byte
		)
		if err := rows.Scan(&entry.At, &entry.Source, &entry.Actor, &entry.Verb, &entry.Target,
			&company, &area, &detail, &entry.RunID, &entry.Seq, &entry.Hash); err != nil {
			return nil, err
		}
		entry.At = entry.At.UTC()
		entry.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
		if len(detail) > 0 {
			_ = json.Unmarshal(detail, &entry.Detail)
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// predicate builds the shared narrowing. Every value is bound: an audit filter
// comes from a query string, and this one runs over both trails at once.
func predicate(f Filter) (string, []any) {
	var (
		clauses []string
		args    []any
	)
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}

	if !f.Since.IsZero() {
		add("at >= $%d", f.Since.UTC())
	}
	if !f.Until.IsZero() {
		add("at < $%d", f.Until.UTC())
	}
	if f.Actor != "" {
		// Partial, like every other search in this console. An audit trail
		// whose actor filter needed an exact `usr_5tfnqizly5wccgic` would be
		// filterable only by somebody who already had it on the clipboard.
		add("actor ilike $%d", "%"+f.Actor+"%")
	}
	if len(f.Sources) > 0 {
		sources := make([]string, 0, len(f.Sources))
		for _, s := range f.Sources {
			sources = append(sources, string(s))
		}
		add("source = any($%d::text[])", sources)
	}

	// Scope narrowing is the one that matters: an audit trail that showed an
	// area somebody cannot otherwise see would be a way around every other
	// check on this platform.
	if len(f.Scopes) > 0 {
		var reachable []string
		for _, scope := range f.Scopes {
			args = append(args, string(scope.Company))
			company := fmt.Sprintf("company_id = $%d", len(args))
			if scope.Area == "" {
				// A grant with no area covers the whole company. Installation
				// -wide administrative rows carry no company at all, and a
				// caller granted across a company sees those too.
				reachable = append(reachable, "("+company+" or company_id = '')")
				continue
			}
			args = append(args, string(scope.Area))
			reachable = append(reachable,
				fmt.Sprintf("(%s and (area_id = $%d or area_id = ''))", company, len(args)))
		}
		clauses = append(clauses, "("+strings.Join(reachable, " or ")+")")
	}

	if len(clauses) == 0 {
		return "", nil
	}
	return "where " + strings.Join(clauses, " and "), args
}

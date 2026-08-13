package audit

import (
	"fmt"
	"strings"
)

/*
Building the query that reads both trails as one.

The shape matters more than it looks. The obvious form — union the two records,
then order and limit the result — makes the database read every matching row
before it can answer a page: measured on a modest ledger, sixteen thousand rows
sorted to return one screenful, and the cost grows with the installation.

Top-N of a union is the union of each side's top-N, so the ordering and the
limit go inside each branch where an index can supply them, and the outer query
merges two short lists. That identity is also what makes the cursor correct: a
boundary pushed into both branches cannot lose an entry that falls between them.
*/

// actorExpr is who acted, in the ledger.
//
// A gate decision is the agent's and an approval is the person's. Reading the
// agent as the approver would credit a machine with somebody's consent.
const actorExpr = `case when kind = 'approval_decided'
		then coalesce(payload->>'by', '') else agent_id end`

// columns is the shape both branches produce, in order. The trailing mark is
// the branch's own position and never leaves this package.
const columns = `at, source, actor, verb, target, company_id, area_id,
	detail, run_id, seq, hash, mark`

type builder struct{ args []any }

// bind records a value and returns the placeholder that reads it.
func (b *builder) bind(v any) string {
	b.args = append(b.args, v)
	return fmt.Sprintf("$%d", len(b.args))
}

// buildTrail assembles the page query and the values it reads.
func buildTrail(f Filter, limit int) (string, []any) {
	b := &builder{}
	page := b.bind(limit)
	from := DecodeCursor(f.Cursor)

	var branches []string
	if wants(f, SourceLedger) {
		branches = append(branches, b.ledger(f, from, page))
	}
	if wants(f, SourceAdmin) {
		branches = append(branches, b.admin(f, from, page))
	}
	if len(branches) == 0 {
		return "", nil
	}

	return fmt.Sprintf("select %s from (%s) trail order by at desc, mark desc limit %s",
		columns, strings.Join(branches, " union all "), page), b.args
}

// wants answers whether a record belongs in the answer at all.
//
// Narrowing by source drops the branch rather than filtering its rows, so
// asking for one record does not pay for reading the other.
func wants(f Filter, s Source) bool {
	if len(f.Sources) == 0 {
		return true
	}
	for _, want := range f.Sources {
		if want == s {
			return true
		}
	}
	return false
}

func (b *builder) ledger(f Filter, from Cursor, page string) string {
	where := []string{"kind in ('gate_decided', 'approval_decided')"}
	where = append(where, b.narrow(f, actorExpr)...)
	if m := from.Ledger; m != nil {
		// Strictly past the last entry the previous page carried. Rows are
		// compared as a tuple so the boundary is one place, not three cases.
		where = append(where, fmt.Sprintf("(at, run_id, seq) < (%s, %s, %s)",
			b.bind(m.At.UTC()), b.bind(m.Run), b.bind(m.Seq)))
	}

	return fmt.Sprintf(`(select at, 'ledger' as source, %s as actor, %s as verb,
			coalesce(payload->>'tool', run_id) as target,
			company_id, area_id, payload as detail,
			run_id, seq, encode(hash, 'hex') as hash,
			run_id || ':' || lpad(seq::text, 12, '0') as mark
		from run_steps where %s
		order by at desc, run_id desc, seq desc limit %s)`,
		actorExpr, ledgerVerbs, strings.Join(where, " and "), page)
}

func (b *builder) admin(f Filter, from Cursor, page string) string {
	where := append([]string{"true"}, b.narrow(f, "principal_id")...)
	if m := from.Admin; m != nil {
		where = append(where, fmt.Sprintf("(at, event_id) < (%s, %s)",
			b.bind(m.At.UTC()), b.bind(m.ID)))
	}

	return fmt.Sprintf(`(select at, 'admin' as source, principal_id as actor,
			action as verb, target, company_id, area_id, detail,
			'' as run_id, 0::bigint as seq, '' as hash,
			lpad(event_id::text, 20, '0') as mark
		from admin_events where %s
		order by at desc, event_id desc limit %s)`,
		strings.Join(where, " and "), page)
}

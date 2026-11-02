package ledger

import (
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
workItemSource is what every trail statement reads by (#204): the resolution
of the reference asked for, as a CTE, and the FROM of the card's runs.

card_of is one row whatever the links say — an aggregate with no grouping —
holding the card the trail is of and, when the reference asked for was an
alias the caller may read, that reference. It is computed inside the statement
that reads by it, so the page and the summary each resolve in their own
snapshot — the first page's shared one — and no statement is added for it.
Aliases are found by their key, (company, ref) and (company, root_ref), with
no company in hand: there is at most one row per company, and the leading
column is skipped over.

The FROM is the claims of the card and of its readable aliases, joined to the
runs the filter reads. Who and where are the run's, from the projection the
filter was applied to — never the claim row's, which gives only the join, the
reference and the moment of the claim. An alias joins only in its own company,
and only by a link the caller may read. The claims are found by the (ref)
index on run_work_items, over the card and its aliases as one array.
*/
func workItemSource(filter domain.RunFilter, ref domain.WorkItemRef) (with, from string, args []any) {
	where, args := runFilterSQL(filter)
	where = whereAnd(where, realRuns)
	readable, args := aliasScopeSQL(filter, args)
	args = append(args, string(ref))
	asked := fmt.Sprintf("$%d::text", len(args))
	with = `card_of as (
			select case when count(distinct root_ref) = 1 then min(root_ref) else ` + asked + ` end as card,
			       case when count(distinct root_ref) = 1 then ` + asked + ` else '' end as resolved_from
			from work_item_aliases where ref = ` + asked + ` and ` + readable + `
		)`
	aliases := `select company_id, ref from work_item_aliases
		            where root_ref = (select card from card_of) and ` + readable
	from = `
		from run_work_items c
		join (select run_id, agent_id, company_id, area_id, phase, started_at, ended_at, cost_micros
		      from runs ` + where + `) r
		  on r.run_id = c.run_id
		left join (` + aliases + `) a on a.company_id = c.company_id and a.ref = c.ref
		where c.ref = any(array(select card from card_of union all select ref from (` + aliases + `) x))
		  and (c.ref = (select card from card_of) or a.ref is not null)`
	return with, from, args
}

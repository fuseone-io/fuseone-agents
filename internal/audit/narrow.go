package audit

import (
	"fmt"
	"strings"
)

// narrow builds the clauses that apply to one branch.
//
// Every value is bound: an audit filter comes from a query string. The actor
// expression differs per record — a person's identifier in one, the agent or
// the approver in the other — so it is passed in rather than assumed.
func (b *builder) narrow(f Filter, actor string) []string {
	var clauses []string

	if !f.Since.IsZero() {
		clauses = append(clauses, "at >= "+b.bind(f.Since.UTC()))
	}
	if !f.Until.IsZero() {
		clauses = append(clauses, "at < "+b.bind(f.Until.UTC()))
	}
	if f.Actor != "" {
		// Partial, like every other search in this console. An actor filter
		// needing an exact `usr_5tfnqizly5wccgic` would be usable only by
		// somebody who already had it on the clipboard.
		clauses = append(clauses, fmt.Sprintf("(%s) ilike %s", actor, b.bind("%"+f.Actor+"%")))
	}
	if reach := b.reachable(f); reach != "" {
		clauses = append(clauses, reach)
	}
	return clauses
}

// reachable is the scope narrowing, and it is the one that matters: a trail
// showing an area somebody cannot otherwise see would be a way around every
// other check on this platform.
func (b *builder) reachable(f Filter) string {
	if len(f.Scopes) == 0 {
		return ""
	}
	var reach []string
	for _, scope := range f.Scopes {
		company := "company_id = " + b.bind(string(scope.Company))
		if scope.Area == "" {
			// A grant with no area covers the whole company. Installation-wide
			// administrative rows carry no company at all, and a caller granted
			// across a company sees those too.
			reach = append(reach, "("+company+" or company_id = '')")
			continue
		}
		reach = append(reach, fmt.Sprintf("(%s and (area_id = %s or area_id = ''))",
			company, b.bind(string(scope.Area))))
	}
	return "(" + strings.Join(reach, " or ") + ")"
}

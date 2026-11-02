package ledger

import (
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

/*
How a trail joins a card's pull requests (#204, slice 2b), as both stores read
it.

Resolution is one level deep. The trail asked by ref is of target: the root of
ref's aliases, when the links the caller may read name exactly one, and ref
itself otherwise. A run is on target's trail when the reference it claimed is
target (a claim), or is an alias of target in the run's own company (an alias).

Links are read under the caller's scopes, by the linking run's own company and
area, exactly as runs are. A link the caller cannot read neither joins a trail
nor resolves one: the trail would otherwise say that work on the card happens
somewhere the caller cannot see. Two readable links of one pull request to two
roots — two companies on one repository, each with its own link — resolve to
neither, rather than to one picked at random; each company's runs still join
their own root's trail.
*/

// trailRef is the card a trail is of, and the reference it was resolved from
// when that was an alias.
type trailRef struct {
	target domain.WorkItemRef
	from   domain.WorkItemRef
}

// trailTarget resolves ref against the roots its readable links name.
func trailTarget(ref domain.WorkItemRef, roots []domain.WorkItemRef) (domain.WorkItemRef, domain.WorkItemRef) {
	if len(roots) != 1 {
		return ref, ""
	}
	return roots[0], ref
}

// trailLinkOf says how a run that claimed held, whose company links held to
// aliasRoot ("" for none), is on target's trail — or that it is not.
func trailLinkOf(held, aliasRoot, target domain.WorkItemRef) (domain.WorkItemLink, bool) {
	switch {
	case held == target:
		return domain.WorkItemLinkClaim, true
	case aliasRoot != "" && aliasRoot == target:
		return domain.WorkItemLinkAlias, true
	}
	return "", false
}

// readableAliases is the links the caller may read: each alias's root, per
// company.
type readableAliases map[aliasKey]domain.WorkItemRef

func visibleAliases(rows []aliasRow, filter domain.RunFilter) readableAliases {
	out := readableAliases{}
	for _, r := range rows {
		if scopeReadable(r.scope, filter) {
			out[aliasKey{r.scope.Company, r.ref}] = r.root
		}
	}
	return out
}

// rootsOf is the distinct roots ref is a readable alias of, across companies.
func (a readableAliases) rootsOf(ref domain.WorkItemRef) []domain.WorkItemRef {
	var out []domain.WorkItemRef
	for k, root := range a {
		if k.ref == ref && !containsRef(out, root) {
			out = append(out, root)
		}
	}
	return out
}

func containsRef(refs []domain.WorkItemRef, ref domain.WorkItemRef) bool {
	for _, r := range refs {
		if r == ref {
			return true
		}
	}
	return false
}

// scopeReadable is the scope half of matches, for a record that has a scope
// and nothing else the filter could narrow by.
func scopeReadable(scope domain.Scope, f domain.RunFilter) bool {
	switch {
	case f.Scope.Company != "" && scope.Company != f.Scope.Company:
		return false
	case f.Scope.Area != "" && scope.Area != f.Scope.Area:
		return false
	case len(f.Scopes) > 0 && !withinAny(scope, f.Scopes):
		return false
	}
	return true
}

// aliasScopeSQL is scopeReadable as a predicate on work_item_aliases' own
// company and area, its arguments appended to args. Always a predicate, never
// empty: "true" when the filter narrows nothing.
func aliasScopeSQL(f domain.RunFilter, args []any) (string, []any) {
	clauses := []string{"true"}
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if f.Scope.Company != "" {
		add("company_id = $%d", string(f.Scope.Company))
	}
	if f.Scope.Area != "" {
		add("area_id = $%d", string(f.Scope.Area))
	}
	if len(f.Scopes) > 0 && !readsEveryCompany(f.Scopes) {
		var anyOf []string
		for _, scope := range f.Scopes {
			args = append(args, string(scope.Company))
			company := fmt.Sprintf("company_id = $%d", len(args))
			if scope.Area == "" {
				anyOf = append(anyOf, company)
				continue
			}
			args = append(args, string(scope.Area))
			anyOf = append(anyOf, fmt.Sprintf("(%s and area_id = $%d)", company, len(args)))
		}
		clauses = append(clauses, "("+strings.Join(anyOf, " or ")+")")
	}
	return strings.Join(clauses, " and "), args
}

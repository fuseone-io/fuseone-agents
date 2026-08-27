package memory

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// A filter turned into SQL, and the scoring kept out of the predicate.
//
// The distinction the whole file exists for: what a row must match belongs in
// where, and how well it matched belongs in order by. Putting the score in the
// predicate is what made the trigram indexes unreachable and every search a
// sequential scan.

func listWhere(f Filter) (string, []any) {
	var clauses []string
	var args []any
	clauses = append(clauses, scopedClause(f.Scopes, &args))
	if f.AgentID != "" {
		args = append(args, string(f.AgentID))
		clauses = append(clauses, "(agent_id = $"+fmt.Sprint(len(args))+" or agent_id = '')")
	}
	if f.Status.Valid() {
		clauses = append(clauses, statusClause(f.Status, f.Now, &args))
	}
	clauses = appendSearchClauses(clauses, &args, f.Search)
	return "where " + strings.Join(clauses, " and "), args
}

func suggestionWhere(f SuggestionFilter) (string, []any) {
	var clauses []string
	var args []any
	clauses = append(clauses, scopedClause(f.Scopes, &args))
	if f.AgentID != "" {
		args = append(args, string(f.AgentID))
		clauses = append(clauses, "(agent_id = $"+fmt.Sprint(len(args))+" or agent_id = '')")
	}
	if f.Status.Valid() {
		args = append(args, string(f.Status))
		clauses = append(clauses, "status = $"+fmt.Sprint(len(args)))
	}
	clauses = appendSearchClauses(clauses, &args, f.Search)
	return "where " + strings.Join(clauses, " and "), args
}

func findWhere(q domain.MemoryQuery) (string, []any, string) {
	var clauses []string
	var args []any
	args = append(args, string(q.Scope.Company))
	clauses = append(clauses, "company_id = $"+fmt.Sprint(len(args)))
	args = append(args, string(q.Scope.Area))
	clauses = append(clauses, "area_id = $"+fmt.Sprint(len(args)))
	args = append(args, string(q.AgentID))
	clauses = append(clauses, "(agent_id = $"+fmt.Sprint(len(args))+" or agent_id = '')")
	if kind := strings.TrimSpace(q.Kind); kind != "" {
		args = append(args, kind)
		clauses = append(clauses, "kind = $"+fmt.Sprint(len(args)))
	}
	if subject := strings.TrimSpace(q.Subject); subject != "" {
		args = append(args, subject)
		clauses = append(clauses, "subject = $"+fmt.Sprint(len(args)))
	}
	if signature := strings.TrimSpace(q.Signature); signature != "" {
		args = append(args, signature)
		clauses = append(clauses, "signature = $"+fmt.Sprint(len(args)))
	}
	var searchOrder string
	clauses, searchOrder = appendFindSearchClause(clauses, &args, q.Search)
	args = append(args, q.Now.UTC())
	clauses = append(clauses, "status = 'active'")
	clauses = append(clauses, "(expires_at is null or expires_at > $"+fmt.Sprint(len(args))+")")
	return "where " + strings.Join(clauses, " and "), args, searchOrder
}

func appendSearchClauses(clauses []string, args *[]any, search string) []string {
	parsed := parseSearchTerms(search)
	if !parsed.hasInput {
		return clauses
	}
	if len(parsed.terms) == 0 {
		return append(clauses, "false")
	}
	for _, term := range parsed.terms {
		*args = append(*args, searchPattern(term))
		n := fmt.Sprint(len(*args))
		clauses = append(clauses,
			searchTermCondition(n, term))
	}
	return clauses
}

func appendFindSearchClause(clauses []string, args *[]any, search string) ([]string, string) {
	parsed := parseSearchTerms(search)
	terms := parsed.terms
	if !parsed.hasInput {
		return clauses, ""
	}
	if len(terms) == 0 {
		return append(clauses, "false"), ""
	}
	conds := make([]string, 0, len(terms))
	scoreParts := make([]string, 0, len(terms))
	strongConds := []string{}
	for _, term := range terms {
		*args = append(*args, searchPattern(term))
		cond := searchTermCondition(fmt.Sprint(len(*args)), term)
		conds = append(conds, cond)
		scoreParts = append(scoreParts,
			fmt.Sprintf("(case when %s then %d else 0 end)", cond, searchTermWeight(term)))
		if strongSearchTerm(term) {
			strongConds = append(strongConds, cond)
		}
	}
	// The broad OR is deliberately separate from the match-count predicate:
	// PostgreSQL can turn this plain boolean shape into BitmapOr over the
	// trigram indexes, then apply the stricter count as a filter.
	clauses = append(clauses, "("+strings.Join(conds, " or ")+")")
	if len(strongConds) > 0 {
		clauses = append(clauses, "("+strings.Join(strongConds, " or ")+")")
	}
	clauses = append(clauses, searchTermsMatchedClause(conds, minFindSearchMatches(len(terms))))
	score := strings.Join(scoreParts, " + ")
	return clauses, score
}

func searchTermsMatchedClause(conds []string, required int) string {
	if required <= 1 || len(conds) == 1 {
		return "(" + strings.Join(conds, " or ") + ")"
	}
	if required >= len(conds) {
		return "(" + strings.Join(conds, " and ") + ")"
	}
	var pairs []string
	for i := range conds {
		for j := i + 1; j < len(conds); j++ {
			pairs = append(pairs, "("+conds[i]+" and "+conds[j]+")")
		}
	}
	return "(" + strings.Join(pairs, " or ") + ")"
}

func searchTermCondition(n string, term string) string {
	if shortSearchTerm(term) {
		return "(subject ~* $" + n + " or " +
			"signature ~* $" + n + " or " +
			"claim ~* $" + n + ")"
	}
	return "(subject ilike $" + n + " escape '\\' or " +
		"signature ilike $" + n + " escape '\\' or " +
		"claim ilike $" + n + " escape '\\')"
}

func searchPattern(term string) string {
	if shortSearchTerm(term) {
		return searchRegexPattern(term)
	}
	return likePattern(term)
}

func searchRegexPattern(term string) string {
	return "(^|[^[:alnum:]])" + regexp.QuoteMeta(term) + "([^[:alnum:]]|$)"
}

func likePattern(term string) string {
	var b strings.Builder
	b.Grow(len(term) + 2)
	b.WriteByte('%')
	for _, r := range term {
		if r == '%' || r == '_' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('%')
	return b.String()
}

func statusClause(status domain.MemoryStatus, now time.Time, args *[]any) string {
	switch status {
	case domain.MemoryActive:
		*args = append(*args, now.UTC())
		return "(status = 'active' and (expires_at is null or expires_at > $" + fmt.Sprint(len(*args)) + "))"
	case domain.MemoryExpired:
		*args = append(*args, now.UTC())
		return "(status = 'expired' or (status = 'active' and expires_at <= $" + fmt.Sprint(len(*args)) + "))"
	default:
		*args = append(*args, string(status))
		return "status = $" + fmt.Sprint(len(*args))
	}
}

func scopedClause(scopes []domain.Scope, args *[]any) string {
	if len(scopes) == 0 {
		return "false"
	}
	var parts []string
	for _, scope := range scopes {
		parts = append(parts, scopeClause(scope, args))
	}
	return "(" + strings.Join(parts, " or ") + ")"
}

func scopeClause(scope domain.Scope, args *[]any) string {
	if scope.Company == domain.Installation && scope.Area == "" {
		return "true"
	}
	*args = append(*args, string(scope.Company))
	company := len(*args)
	if scope.Area == "" {
		return fmt.Sprintf("(company_id = $%d)", company)
	}
	*args = append(*args, string(scope.Area))
	return fmt.Sprintf("(company_id = $%d and area_id = $%d)", company, len(*args))
}

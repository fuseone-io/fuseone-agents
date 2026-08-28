package memory

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/fuseone/agents/internal/domain"
)

// The vocabulary of a search: what counts as a term, which terms carry weight,
// and what it means for one to be found in a field.
//
// The rules are shared with the durable store, which turns the same terms into
// predicates. Everything here is about the words; nothing here decides how a
// result is ordered.

const (
	maxSearchTerms     = 6
	maxSearchTermBytes = 64
)

func suggestionContainsSearch(s domain.MemorySuggestion, q string) bool {
	parsed := parseSearchTerms(q)
	terms := parsed.terms
	if len(terms) == 0 {
		return !parsed.hasInput
	}
	return fieldsContainTerms(terms, s.Subject, s.Signature, s.Claim)
}

func containsSearch(a domain.MemoryAssertion, q string) bool {
	parsed := parseSearchTerms(q)
	terms := parsed.terms
	if len(terms) == 0 {
		return !parsed.hasInput
	}
	return fieldsContainTerms(terms, a.Subject, a.Signature, a.Claim)
}

func findSearchMatches(a domain.MemoryAssertion, q string) bool {
	parsed := parseSearchTerms(q)
	terms := parsed.terms
	if len(terms) == 0 {
		return !parsed.hasInput
	}
	matched, strongMatched := searchMatchCounts(a, terms)
	if hasStrongSearchTerm(terms) && strongMatched == 0 {
		return false
	}
	return matched >= minFindSearchMatches(len(terms))
}

func fieldsContainTerms(terms []string, fields ...string) bool {
	haystack := strings.ToLower(strings.Join(fields, "\n"))
	for _, term := range terms {
		if !searchTermMatches(haystack, term) {
			return false
		}
	}
	return true
}

type parsedSearchTerms struct {
	terms    []string
	hasInput bool
	omitted  int
}

func searchTerms(q string) []string {
	return parseSearchTerms(q).terms
}

func parseSearchTerms(q string) parsedSearchTerms {
	raw := strings.Fields(strings.ToLower(strings.TrimSpace(q)))
	if len(raw) == 0 {
		return parsedSearchTerms{}
	}
	seen := map[string]bool{}
	var strong []string
	var ordinary []string
	for _, rawTerm := range raw {
		term := normalizeSearchTerm(rawTerm)
		if term == "" || seen[term] {
			continue
		}
		seen[term] = true
		if strongSearchTerm(term) {
			strong = append(strong, term)
			continue
		}
		if searchStopword(term) {
			continue
		}
		ordinary = append(ordinary, term)
	}
	out, omitted := boundedSearchTerms(strong, ordinary)
	return parsedSearchTerms{terms: out, hasInput: true, omitted: omitted}
}

func normalizeSearchTerm(raw string) string {
	term := strings.Trim(raw, `"'.,;:()[]{}<>!?`)
	if term == "" {
		term = strings.TrimSpace(raw)
	}
	return truncateSearchTerm(term)
}

func boundedSearchTerms(groups ...[]string) ([]string, int) {
	out := make([]string, 0, maxSearchTerms)
	var omitted int
	for _, group := range groups {
		for _, term := range group {
			if len(out) == maxSearchTerms {
				omitted++
				continue
			}
			out = append(out, term)
		}
	}
	return out, omitted
}

func searchStopword(term string) bool {
	if utf8.RuneCountInString(term) < 2 {
		return true
	}
	switch term {
	case "about", "algum", "alguma", "algumas", "alguns", "anything", "aquela",
		"aquele", "aqueles", "aquilo", "aqui", "are", "com", "coisa",
		"coisas", "como", "consegue", "could", "das", "de", "delas",
		"deles", "do", "dos", "essa", "essas", "esse", "esses", "esta",
		"estas", "este", "estes", "eu", "favor", "find", "for", "from",
		"isso", "isto", "know", "me", "na", "nas", "need", "no", "nos",
		"para", "pela", "pelas", "pelo", "pelos", "please", "por",
		"preciso", "problem", "problema", "procure", "procurar", "quais",
		"qual", "qualquer", "que", "queria", "quero", "saber", "search",
		"sem", "sobre", "some", "something", "tell", "that", "the",
		"these", "thing", "this", "those", "uma", "umas", "uns", "voce",
		"você", "want", "were", "with", "without":
		return true
	default:
		return false
	}
}

func searchMatchCounts(a domain.MemoryAssertion, terms []string) (matched int, strongMatched int) {
	haystack := strings.ToLower(strings.Join([]string{a.Subject, a.Signature, a.Claim}, "\n"))
	for _, term := range terms {
		if !searchTermMatches(haystack, term) {
			continue
		}
		matched++
		if strongSearchTerm(term) {
			strongMatched++
		}
	}
	return matched, strongMatched
}

func searchScore(a domain.MemoryAssertion, terms []string) int {
	haystack := strings.ToLower(strings.Join([]string{a.Subject, a.Signature, a.Claim}, "\n"))
	var score int
	for _, term := range terms {
		if searchTermMatches(haystack, term) {
			score += searchTermWeight(term)
		}
	}
	return score
}

func searchTermMatches(haystack string, term string) bool {
	if shortSearchTerm(term) {
		return containsBoundedSearchTerm(haystack, term)
	}
	return strings.Contains(haystack, term)
}

func containsBoundedSearchTerm(haystack string, term string) bool {
	for start := 0; ; {
		i := strings.Index(haystack[start:], term)
		if i < 0 {
			return false
		}
		i += start
		end := i + len(term)
		if searchBoundary(haystack[:i], false) && searchBoundary(haystack[end:], true) {
			return true
		}
		start = end
	}
}

func searchBoundary(s string, before bool) bool {
	if s == "" {
		return true
	}
	var r rune
	if before {
		r, _ = utf8.DecodeRuneInString(s)
	} else {
		r, _ = utf8.DecodeLastRuneInString(s)
	}
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

func hasStrongSearchTerm(terms []string) bool {
	for _, term := range terms {
		if strongSearchTerm(term) {
			return true
		}
	}
	return false
}

func strongSearchTerm(term string) bool {
	if len(term) < 6 {
		return false
	}
	if strings.ContainsAny(term, "_.-/:=") {
		return true
	}
	var hasLetter, hasDigit bool
	for _, r := range term {
		hasLetter = hasLetter || unicode.IsLetter(r)
		hasDigit = hasDigit || unicode.IsDigit(r)
	}
	return hasLetter && hasDigit
}

func shortSearchTerm(term string) bool {
	return utf8.RuneCountInString(term) == 2
}

func searchTermWeight(term string) int {
	if strongSearchTerm(term) {
		return 4
	}
	return 1
}

func minFindSearchMatches(terms int) int {
	if terms <= 1 {
		return terms
	}
	return 2
}

func truncateSearchTerm(term string) string {
	if len(term) <= maxSearchTermBytes {
		return term
	}
	end := 0
	for i, r := range term {
		next := i + utf8.RuneLen(r)
		if next > maxSearchTermBytes {
			break
		}
		end = next
	}
	return term[:end]
}

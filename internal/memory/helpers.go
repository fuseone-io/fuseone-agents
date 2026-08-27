package memory

import (
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/fuseone/agents/internal/domain"
)

const (
	maxSearchTerms     = 6
	maxSearchTermBytes = 64
)

func prepareAssertion(
	a domain.MemoryAssertion, by domain.UserID, now time.Time,
) (domain.MemoryAssertion, error) {
	a.Kind = clean(a.Kind)
	a.Subject = clean(a.Subject)
	a.Signature = clean(a.Signature)
	a.Claim = clean(a.Claim)
	if a.Status == "" {
		a.Status = domain.MemoryActive
	}
	if err := a.Validate(); err != nil {
		return domain.MemoryAssertion{}, err
	}
	a.ID = domain.MemoryAssertionID(a)
	a.CreatedBy, a.UpdatedBy = by, by
	a.CreatedAt, a.UpdatedAt = now.UTC(), now.UTC()
	a.Labels = a.Labels.Union(domain.ScopeLabels(a.Scope))
	a.Evidence = slices.Clone(a.Evidence)
	return a, nil
}

func prepareSuggestion(
	s domain.MemorySuggestion, by domain.UserID, now time.Time,
) (domain.MemorySuggestion, error) {
	s.Kind = clean(s.Kind)
	s.Subject = clean(s.Subject)
	s.Signature = clean(s.Signature)
	s.Claim = clean(s.Claim)
	if s.Status == "" {
		s.Status = domain.MemorySuggestionPending
	}
	if s.Observations <= 0 {
		s.Observations = 1
	}
	s.AssertionID = domain.MemoryAssertionID(domain.MemoryAssertion{
		Scope: s.Scope, AgentID: s.AgentID, Kind: s.Kind,
		Subject: s.Subject, Signature: s.Signature, Claim: s.Claim,
		Evidence: s.Evidence, Observations: s.Observations,
		Status: domain.MemoryActive,
	})
	s.ID = domain.MemorySuggestionID(s)
	s.CreatedBy, s.UpdatedBy = by, by
	s.CreatedAt, s.UpdatedAt = now.UTC(), now.UTC()
	s.Labels = s.Labels.Union(domain.ScopeLabels(s.Scope))
	s.Evidence = boundedEvidence(s.Evidence)
	if err := s.Validate(); err != nil {
		return domain.MemorySuggestion{}, err
	}
	return s, nil
}

/*
identitiesForSuggestion is where an equivalent memory could already be: the
agent's own namespace first, then the shared one every agent in the scope reads.

Identities rather than assertion ids. The id hashes the strings as typed, so a
memory somebody wrote as "Grafana Datasource" was invisible to an agent that
proposed "grafana  datasource" — and the queue filled with proposals for a fact
the platform already knew, each of which a person had to read and dismiss. The
id travels along because a row written before the canonical key answers to
nothing else.
*/
func identitiesForSuggestion(s domain.MemorySuggestion) []domain.MemoryAssertion {
	own := domain.MemoryAssertion{
		Scope: s.Scope, AgentID: s.AgentID, Kind: s.Kind,
		Subject: s.Subject, Signature: s.Signature,
	}
	own.ID = domain.MemoryAssertionID(own)
	if s.AgentID == "" {
		return []domain.MemoryAssertion{own}
	}
	shared := own
	shared.AgentID = ""
	shared.ID = domain.MemoryAssertionID(shared)
	return []domain.MemoryAssertion{own, shared}
}

func memoryFindMatches(a domain.MemoryAssertion, q domain.MemoryQuery) bool {
	if a.Scope != q.Scope || !(a.AgentID == "" || a.AgentID == q.AgentID) {
		return false
	}
	if a.Status != domain.MemoryActive || expired(a, q.Now) {
		return false
	}
	return matchesField(a.Kind, q.Kind) && matchesField(a.Subject, q.Subject) &&
		matchesField(a.Signature, q.Signature) && findSearchMatches(a, q.Search)
}

func suggestionMatches(s domain.MemorySuggestion, f SuggestionFilter) bool {
	if !containsScope(f.Scopes, s.Scope) {
		return false
	}
	if f.AgentID != "" && !(s.AgentID == "" || s.AgentID == f.AgentID) {
		return false
	}
	if f.Status.Valid() && s.Status != f.Status {
		return false
	}
	return suggestionContainsSearch(s, f.Search)
}

func suggestionContainsSearch(s domain.MemorySuggestion, q string) bool {
	parsed := parseSearchTerms(q)
	terms := parsed.terms
	if len(terms) == 0 {
		return !parsed.hasInput
	}
	return fieldsContainTerms(terms, s.Subject, s.Signature, s.Claim)
}

func listMatches(a domain.MemoryAssertion, f Filter) bool {
	if !containsScope(f.Scopes, a.Scope) {
		return false
	}
	if f.AgentID != "" && !(a.AgentID == "" || a.AgentID == f.AgentID) {
		return false
	}
	if f.Status.Valid() && effectiveStatus(a, f.Now).Status != f.Status {
		return false
	}
	return containsSearch(a, f.Search)
}

func effectiveStatus(a domain.MemoryAssertion, now time.Time) domain.MemoryAssertion {
	if a.Status == domain.MemoryActive && expired(a, now) {
		a.Status = domain.MemoryExpired
	}
	return a
}

func expired(a domain.MemoryAssertion, now time.Time) bool {
	return a.ExpiresAt != nil && !a.ExpiresAt.After(now.UTC())
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

func containsScope(scopes []domain.Scope, scope domain.Scope) bool {
	for _, s := range scopes {
		if s.Contains(scope) {
			return true
		}
	}
	return false
}

func matchesField(got, want string) bool {
	return want == "" || got == strings.TrimSpace(want)
}

func sortAssertions(out []domain.MemoryAssertion) {
	slices.SortFunc(out, func(a, b domain.MemoryAssertion) int {
		if a.Confirmed != b.Confirmed {
			return int(b.Confirmed - a.Confirmed)
		}
		if a.Observations != b.Observations {
			return int(b.Observations - a.Observations)
		}
		if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

func sortFindAssertions(out []domain.MemoryAssertion, search string) {
	terms := searchTerms(search)
	if len(terms) == 0 {
		sortAssertions(out)
		return
	}
	slices.SortFunc(out, func(a, b domain.MemoryAssertion) int {
		if scoreA, scoreB := searchScore(a, terms), searchScore(b, terms); scoreA != scoreB {
			return scoreB - scoreA
		}
		if a.Confirmed != b.Confirmed {
			return int(b.Confirmed - a.Confirmed)
		}
		if a.Observations != b.Observations {
			return int(b.Observations - a.Observations)
		}
		if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

func sortByUpdated(out []domain.MemoryAssertion) {
	slices.SortFunc(out, func(a, b domain.MemoryAssertion) int {
		if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

func sortSuggestionsByUpdated(out []domain.MemorySuggestion) {
	slices.SortFunc(out, func(a, b domain.MemorySuggestion) int {
		if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

func first(in []domain.MemoryAssertion, n int) []domain.MemoryAssertion {
	if len(in) <= n {
		return in
	}
	return in[:n]
}

func firstSuggestions(in []domain.MemorySuggestion, n int) []domain.MemorySuggestion {
	if len(in) <= n {
		return in
	}
	return in[:n]
}

func cloneAssertion(a domain.MemoryAssertion) domain.MemoryAssertion {
	a.Labels = a.Labels.Clone()
	a.Evidence = cloneEvidence(a.Evidence)
	return a
}

func cloneSuggestion(s domain.MemorySuggestion) domain.MemorySuggestion {
	s.Labels = s.Labels.Clone()
	s.Evidence = cloneEvidence(s.Evidence)
	return s
}

// cloneEvidence copies the labels inside each citation too. Cloning only the
// slice of records leaves every Labels sharing its backing array, and the
// in-memory store hands out what it holds — so a reader mutating a returned
// citation would be editing the stored taint in place.
func cloneEvidence(in []domain.MemoryEvidence) []domain.MemoryEvidence {
	out := slices.Clone(in)
	for i := range out {
		out[i].Labels = out[i].Labels.Clone()
	}
	return out
}

/*
boundedEvidence folds repeated citations and holds the record count to the cap.

Folded by MemoryEvidence.Key rather than by the whole record, because the labels
are resolved from the ledger at the moment somebody asks: the same step comes
back clean today and tainted once the run that produced it gained a label, and
those are one citation read twice. Keeping both would spend the budget on a
duplicate of itself; keeping only the first would discard the later, fuller
reading. So the labels are unioned, which is also the direction that never loses
a taint.
*/
func boundedEvidence(in []domain.MemoryEvidence) []domain.MemoryEvidence {
	out := make([]domain.MemoryEvidence, 0, min(len(in), domain.MaxMemoryEvidence))
	at := map[string]int{}
	for _, ev := range in {
		if i, seen := at[ev.Key()]; seen {
			out[i].Labels = out[i].Labels.Union(ev.Labels)
			continue
		}
		if len(out) == domain.MaxMemoryEvidence {
			continue
		}
		at[ev.Key()] = len(out)
		out = append(out, ev)
	}
	return out
}

func clean(v string) string { return strings.TrimSpace(v) }

func nowOrWall(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}

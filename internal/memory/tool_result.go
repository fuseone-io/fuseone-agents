package memory

import (
	"encoding/json"
	"slices"

	"github.com/fuseone/agents/internal/domain"
)

// What goes back to the model, and what is left out of it.
//
// Bounded on purpose: a result that grew with the memory would spend a run's
// context on rows nobody asked for. What is dropped is named in the payload —
// a shorter answer that says it is shorter beats a shorter answer that does
// not.

type resultAssertion struct {
	ID           string                  `json:"id"`
	Kind         string                  `json:"kind"`
	Subject      string                  `json:"subject"`
	Signature    string                  `json:"signature"`
	Claim        string                  `json:"claim"`
	Evidence     []domain.MemoryEvidence `json:"evidence"`
	Observations int64                   `json:"observations,omitempty"`
	Confirmed    int64                   `json:"confirmed,omitempty"`
	ExpiresAt    *string                 `json:"expires_at,omitempty"`
	UpdatedAt    string                  `json:"updated_at"`
}

type memoryResultPayload struct {
	Assertions               []resultAssertion `json:"assertions"`
	Omitted                  int               `json:"omitted,omitempty"`
	OmittedReason            string            `json:"omitted_reason,omitempty"`
	ByteBudget               int               `json:"byte_budget,omitempty"`
	SearchTermsUsed          []string          `json:"search_terms_used,omitempty"`
	SearchTermsOmitted       int               `json:"search_terms_omitted,omitempty"`
	SearchTermsOmittedReason string            `json:"search_terms_omitted_reason,omitempty"`
	SearchTermBudget         int               `json:"search_term_budget,omitempty"`
}

type memoryResultStats struct {
	Returned int
	Omitted  int
}

type memorySuggestPayload struct {
	Status       string `json:"status"`
	SuggestionID string `json:"suggestion_id,omitempty"`
	AssertionID  string `json:"assertion_id,omitempty"`
	Observations int64  `json:"observations,omitempty"`
	Confirmed    int64  `json:"confirmed,omitempty"`
}

const maxMemoryResultBytes = 16 * 1024

func memoryResult(
	found []domain.MemoryAssertion, search parsedSearchTerms,
) ([]byte, domain.Labels, memoryResultStats, error) {
	assertions := make([]resultAssertion, 0, len(found))
	var labels domain.Labels
	for _, a := range found {
		next := append(assertions, resultAssertionFrom(a))
		body, err := marshalMemoryResult(next, len(found), search)
		if err != nil {
			return nil, domain.Labels{}, memoryResultStats{}, err
		}
		if len(body) > maxMemoryResultBytes {
			if len(assertions) == 0 {
				body, err := marshalMemoryResult(nil, len(found), search)
				return body, labels, memoryResultStats{Omitted: len(found)}, err
			}
			break
		}
		assertions = next
		labels = labels.Union(a.Labels)
	}
	body, err := marshalMemoryResult(assertions, len(found), search)
	stats := memoryResultStats{Returned: len(assertions), Omitted: len(found) - len(assertions)}
	return body, labels, stats, err
}

func resultAssertionFrom(a domain.MemoryAssertion) resultAssertion {
	expires := ""
	var expiresAt *string
	if a.ExpiresAt != nil {
		expires = a.ExpiresAt.UTC().Format(timeFormat)
		expiresAt = &expires
	}
	return resultAssertion{
		ID: a.ID, Kind: a.Kind, Subject: a.Subject,
		Signature: a.Signature, Claim: a.Claim,
		Evidence:     slices.Clone(a.Evidence),
		Observations: a.Observations, Confirmed: a.Confirmed,
		ExpiresAt: expiresAt, UpdatedAt: a.UpdatedAt.UTC().Format(timeFormat),
	}
}

func marshalMemoryResult(
	assertions []resultAssertion, total int, search parsedSearchTerms,
) ([]byte, error) {
	if assertions == nil {
		assertions = []resultAssertion{}
	}
	out := memoryResultPayload{Assertions: assertions}
	if omitted := total - len(assertions); omitted > 0 {
		out.Omitted = omitted
		out.OmittedReason = "result_byte_budget"
		out.ByteBudget = maxMemoryResultBytes
	}
	if search.omitted > 0 {
		out.SearchTermsUsed = slices.Clone(search.terms)
		out.SearchTermsOmitted = search.omitted
		out.SearchTermsOmittedReason = "search_term_budget"
		out.SearchTermBudget = maxSearchTerms
	}
	return json.Marshal(out)
}

func memorySuggestResult(out domain.MemorySuggestionOutcome) memorySuggestPayload {
	payload := memorySuggestPayload{
		Status:       string(out.Result),
		SuggestionID: out.Suggestion.ID,
		AssertionID:  out.Suggestion.AssertionID,
		Observations: out.Suggestion.Observations,
	}
	if out.Assertion != nil {
		payload.AssertionID = out.Assertion.ID
		payload.Confirmed = out.Assertion.Confirmed
	}
	return payload
}

const timeFormat = "2006-01-02T15:04:05Z"

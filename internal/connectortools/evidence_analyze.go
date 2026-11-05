package connectortools

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/engine"
)

const (
	EvidenceObserved      = "observed"
	EvidenceNotObserved   = "not_observed"
	EvidenceNotObservable = "not_observable"

	defaultEvidenceTop    = 20
	maxEvidenceTop        = 50
	maxEvidenceDimensions = 5
	maxEvidenceGroupValue = 256
	// maxEvidenceDistinct bounds a group's memory. Past it the answer says the
	// distinct count is a minimum rather than growing with an attacker's URLs.
	maxEvidenceDistinct = 100_000
)

type evidenceAnalyzeArgs struct {
	Evidence string            `json:"evidence"`
	Window   *evidenceArgument `json:"window,omitempty"`
	GroupBy  []string          `json:"groupBy,omitempty"`
	Sum      []string          `json:"sum,omitempty"`
	Match    *evidenceMatchArg `json:"match,omitempty"`
	Top      int               `json:"top,omitempty"`
}

// evidenceMatchArg selects events by one kept field: an exact value, or a
// hostname and its subdomains.
type evidenceMatchArg struct {
	Field  string `json:"field"`
	Equals string `json:"equals,omitempty"`
	Suffix string `json:"suffix,omitempty"`
}

/*
evidenceOutcome says only what the coverage supports.

Absence needs all of it: every event was read, the source was shown to answer over
this window, and nothing in the set was marked as unmeasured, because the event
that would have matched may be exactly the one the source could not see. Without
that the honest answer is that nobody can tell, never that nothing happened.
*/
func evidenceOutcome(state evidenceState, matched, unmeasured int64) string {
	switch {
	case matched > 0:
		return EvidenceObserved
	case state.Coverage == EvidenceComplete && state.Control.Passed && unmeasured == 0:
		return EvidenceNotObserved
	default:
		return EvidenceNotObservable
	}
}

// suffixMatches compares on a label boundary. A substring match would count
// attacker.example.com.evil.test as example.com.
func suffixMatches(value, suffix string) bool {
	value = strings.TrimSuffix(strings.ToLower(value), ".")
	suffix = strings.TrimSuffix(strings.ToLower(suffix), ".")
	return value == suffix || strings.HasSuffix(value, "."+suffix)
}

func (m *evidenceMatchArg) matches(row map[string]json.RawMessage) bool {
	if m == nil {
		return true
	}
	value := evidenceScalar(row[m.Field])
	if m.Suffix != "" {
		return suffixMatches(value, m.Suffix)
	}
	return value == m.Equals
}

func decodeEvidenceAnalyze(raw []byte) (evidenceAnalyzeArgs, bool) {
	var args evidenceAnalyzeArgs
	if !decodeStrict(raw, &args) || args.Evidence == "" || args.Top < 0 || args.Top > maxEvidenceTop {
		return evidenceAnalyzeArgs{}, false
	}
	if len(args.GroupBy) > maxEvidenceDimensions || len(args.Sum) > maxEvidenceDimensions {
		return evidenceAnalyzeArgs{}, false
	}
	return args, true
}

// evidenceAnalyzeAllowed checks the fields against the query that collected:
// what was never kept cannot be asked about.
func evidenceAnalyzeAllowed(args evidenceAnalyzeArgs, query EvidenceQuery) bool {
	kept := func(field string) bool { return slices.Contains(query.Fields, field) }
	if !allKept(args.GroupBy, kept) || !allKept(args.Sum, kept) {
		return false
	}
	if args.Window != nil && query.TimeField == "" {
		return false
	}
	match := args.Match
	if match == nil {
		return true
	}
	oneCriterion := (match.Equals == "") != (match.Suffix == "")
	return kept(match.Field) && oneCriterion && evidenceValueAllowed(match.Equals+match.Suffix)
}

func allKept(fields []string, kept func(string) bool) bool {
	for _, field := range fields {
		if !kept(field) {
			return false
		}
	}
	return true
}

// evidenceCut is the part of a collection asked about, in epoch seconds.
type evidenceCut struct {
	active   bool
	from, to int64
}

func resolveEvidenceCut(asked *evidenceArgument, collected evidenceSpan) (evidenceCut, bool) {
	if asked == nil {
		return evidenceCut{}, true
	}
	span, ok := resolveEvidenceRange(*asked)
	if !ok {
		return evidenceCut{}, false
	}
	// Nobody looked outside what was collected, so nothing can be said about it.
	if collected.From != "" && (span.From < collected.From || span.To > collected.To) {
		return evidenceCut{}, false
	}
	from, _ := time.Parse(time.RFC3339, span.From)
	to, _ := time.Parse(time.RFC3339, span.To)
	return evidenceCut{active: true, from: from.Unix(), to: to.Unix()}, true
}

func (l *Layer) evidenceAnalyze(
	ctx context.Context, instance Instance, call engine.Call,
) (engine.ToolResult, error) {
	args, ok := decodeEvidenceAnalyze(call.Args)
	if !ok {
		return failed(CodeConnectorBadArguments), nil
	}
	state, code := l.openEvidence(ctx, call, instance, args.Evidence)
	if code != "" {
		return failed(code), nil
	}
	query, found := evidenceQueryByID(instance.Evidence, state.Query)
	if !found || state.Contract != evidenceContractDigest(instance.Evidence, query) {
		return failed(CodeConnectorContractChanged), nil
	}
	cut, ok := resolveEvidenceCut(args.Window, state.Window)
	if !ok || !evidenceAnalyzeAllowed(args, query) {
		return failed(CodeConnectorBadArguments), nil
	}
	tally := newEvidenceTally(args, query, instance.Evidence.Gap, cut)
	for _, block := range state.Blocks {
		raw, err := l.content.Get(ctx, block)
		if err != nil {
			if ctx.Err() != nil {
				return engine.ToolResult{}, ctx.Err()
			}
			return failed(CodeConnectorArtifactMissing), nil
		}
		tally.read(raw)
	}
	return l.storeJSON(ctx, call, evidenceLabels(), tally.answer(call, state))
}

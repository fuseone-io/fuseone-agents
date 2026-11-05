package connectortools

import (
	"bytes"
	"encoding/json"
	"strconv"
	"unicode/utf8"
)

// evidenceSumTally is what one summed field added up to, and what it could not.
type evidenceSumTally struct {
	Total   float64 `json:"total"`
	Missing int64   `json:"missing"`
	Zero    int64   `json:"zero"`
}

type evidenceBucket struct {
	Value string             `json:"value"`
	Count int64              `json:"count"`
	Sums  map[string]float64 `json:"sums,omitempty"`
}

type evidenceGroup struct {
	buckets map[string]*evidenceBucket
	missing int64
	capped  bool
}

// evidenceTally folds events. It holds no event, only counts and the ids needed
// to count each one once.
type evidenceTally struct {
	args       evidenceAnalyzeArgs
	query      EvidenceQuery
	gap        EvidenceMatch
	cut        evidenceCut
	seen       map[string]struct{}
	groups     map[string]*evidenceGroup
	sums       map[string]*evidenceSumTally
	events     int64
	duplicates int64
	outside    int64
	undated    int64
	unmeasured int64
}

func newEvidenceTally(
	args evidenceAnalyzeArgs, query EvidenceQuery, gap EvidenceMatch, cut evidenceCut,
) *evidenceTally {
	tally := &evidenceTally{
		args: args, query: query, gap: gap, cut: cut, seen: map[string]struct{}{},
		groups: map[string]*evidenceGroup{}, sums: map[string]*evidenceSumTally{},
	}
	for _, field := range args.GroupBy {
		tally.groups[field] = &evidenceGroup{buckets: map[string]*evidenceBucket{}}
	}
	for _, field := range args.Sum {
		tally.sums[field] = &evidenceSumTally{}
	}
	return tally
}

func (t *evidenceTally) read(block []byte) {
	for line := range bytes.SplitSeq(block, []byte{'\n'}) {
		var row map[string]json.RawMessage
		if len(line) == 0 || json.Unmarshal(line, &row) != nil {
			continue
		}
		if t.repeated(row) || !t.inCut(row) {
			continue
		}
		if t.gap.Field != "" && evidenceScalar(row[t.gap.Field]) == t.gap.Value {
			t.unmeasured++
		}
		if t.args.Match.matches(row) {
			t.count(row)
		}
	}
}

func (t *evidenceTally) repeated(row map[string]json.RawMessage) bool {
	id := evidenceScalar(row[t.query.IDField])
	if t.query.IDField == "" || id == "" {
		return false
	}
	if _, held := t.seen[id]; held {
		t.duplicates++
		return true
	}
	t.seen[id] = struct{}{}
	return false
}

func (t *evidenceTally) inCut(row map[string]json.RawMessage) bool {
	if !t.cut.active {
		return true
	}
	at, ok := evidenceInstantOf(row[t.query.TimeField])
	if !ok {
		t.undated++
		return false
	}
	if at < t.cut.from || at >= t.cut.to {
		t.outside++
		return false
	}
	return true
}

func (t *evidenceTally) count(row map[string]json.RawMessage) {
	t.events++
	amounts := make(map[string]float64, len(t.sums))
	for field, sum := range t.sums {
		amount, err := strconv.ParseFloat(evidenceScalar(row[field]), 64)
		if err != nil {
			sum.Missing++
			continue
		}
		if amount == 0 {
			sum.Zero++
		}
		sum.Total += amount
		amounts[field] = amount
	}
	for field, group := range t.groups {
		group.add(evidenceScalar(row[field]), amounts)
	}
}

func (g *evidenceGroup) add(value string, amounts map[string]float64) {
	if value == "" {
		g.missing++
		return
	}
	value = boundEvidenceValue(value)
	bucket, held := g.buckets[value]
	if !held {
		if len(g.buckets) >= maxEvidenceDistinct {
			g.capped = true
			return
		}
		bucket = &evidenceBucket{Value: value, Sums: map[string]float64{}}
		g.buckets[value] = bucket
	}
	bucket.Count++
	for field, amount := range amounts {
		bucket.Sums[field] += amount
	}
}

func boundEvidenceValue(value string) string {
	if len(value) <= maxEvidenceGroupValue {
		return value
	}
	cut := maxEvidenceGroupValue
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut]
}

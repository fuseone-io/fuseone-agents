package domain

import (
	"slices"
	"strings"
)

// Standard data-flow labels (PRD 10.4). An installation may define its own;
// these three carry meaning for the Gate.
const (
	// LabelUntrusted marks data that entered from an untrusted source — a tool
	// response, an email body, a fetched web page. A high-effect action whose
	// arguments derive from it requires human approval.
	LabelUntrusted = "untrusted"
	LabelPersonal  = "personal"
	LabelSecret    = "secret"
)

// Labels is a sorted set with no duplicates.
//
// The ordering is not cosmetic: the set feeds the step's hash chain, and two
// orderings of the same set would hash identical states differently.
type Labels []string

func NewLabels(values ...string) Labels {
	if len(values) == 0 {
		return nil
	}
	out := make(Labels, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	out = slices.Compact(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func (l Labels) Has(v string) bool {
	_, found := slices.BinarySearch(l, v)
	return found
}

// HasAny reports whether any of the given labels is present.
func (l Labels) HasAny(values ...string) bool {
	for _, v := range values {
		if l.Has(v) {
			return true
		}
	}
	return false
}

// Union is taint propagation: a derived value carries the union of the labels
// of everything it came from. This is what stops dirty data read at step 2
// from becoming a clean premise for the action at step 6.
func (l Labels) Union(other Labels) Labels {
	if len(l) == 0 {
		return other.Clone()
	}
	if len(other) == 0 {
		return l.Clone()
	}
	merged := make(Labels, 0, len(l)+len(other))
	merged = append(merged, l...)
	merged = append(merged, other...)
	slices.Sort(merged)
	return slices.Compact(merged)
}

// UnionAll propagates from several source steps at once.
func UnionAll(sets ...Labels) Labels {
	var out Labels
	for _, s := range sets {
		out = out.Union(s)
	}
	return out
}

func (l Labels) Clone() Labels {
	if len(l) == 0 {
		return nil
	}
	return slices.Clone(l)
}

func (l Labels) String() string {
	return strings.Join(l, ",")
}

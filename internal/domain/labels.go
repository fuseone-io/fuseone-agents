package domain

import (
	"fmt"
	"slices"
	"strings"
)

// Standard data-flow labels (PRD 10.4). An installation may define its own;
// these carry meaning for the Gate.
const (
	// LabelUntrusted marks data that entered from an untrusted source — a tool
	// response, an email body, a fetched web page. A high-effect action whose
	// arguments derive from it requires human approval.
	LabelUntrusted = "untrusted"
	LabelPersonal  = "personal"
	LabelSecret    = "secret"

	labelCompanyPrefix = "company:"
	labelAreaPrefix    = "area:"
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

// ScopeLabels names the scope whose data a run or artifact carries.
//
// Company and area are both present on purpose. Area identifiers are scoped by
// company, and "platform" inside one company must not be read as "platform" in
// another.
func ScopeLabels(scope Scope) Labels {
	if scope.Company == "" || scope.Company == Installation {
		return nil
	}
	if scope.Area == "" {
		return NewLabels(LabelCompany(scope.Company))
	}
	return NewLabels(LabelCompany(scope.Company), LabelArea(scope))
}

func LabelCompany(company CompanyID) string {
	return labelCompanyPrefix + string(company)
}

func LabelArea(scope Scope) string {
	return labelAreaPrefix + scope.String()
}

// ScopeBoundaryViolation is a data-flow label the target scope may not carry.
type ScopeBoundaryViolation struct {
	Label  string
	Origin Scope
	Target Scope
}

func (v ScopeBoundaryViolation) Error() string {
	if v.Origin == (Scope{}) {
		return fmt.Sprintf("data label %q is not a valid scope label", v.Label)
	}
	return fmt.Sprintf("data from %s cannot enter scope %s", v.Origin, v.Target)
}

// ScopeBoundaryViolation reports the first scope label the target does not
// reach.
//
// This is a data barrier, not a query filter. A company-wide scope can carry
// data from one of its areas, and the installation scope can carry data from
// any company. An area cannot carry data from a sibling area or another
// company. Reserved labels that do not parse fail closed: a malformed platform
// label is more suspicious than useful.
func (l Labels) ScopeBoundaryViolation(target Scope) (ScopeBoundaryViolation, bool) {
	companiesWithArea := map[CompanyID]bool{}
	for _, label := range l {
		origin, ok, reserved := scopeFromLabel(label)
		if !reserved {
			continue
		}
		if !ok {
			return ScopeBoundaryViolation{Label: label, Target: target}, true
		}
		if origin.Area != "" {
			companiesWithArea[origin.Company] = true
		}
	}
	for _, label := range l {
		origin, ok, reserved := scopeFromLabel(label)
		if !reserved {
			continue
		}
		if !ok {
			return ScopeBoundaryViolation{Label: label, Target: target}, true
		}
		if origin.Area == "" && companiesWithArea[origin.Company] {
			continue
		}
		if !target.Contains(origin) {
			return ScopeBoundaryViolation{Label: label, Origin: origin, Target: target}, true
		}
	}
	return ScopeBoundaryViolation{}, false
}

func scopeFromLabel(label string) (Scope, bool, bool) {
	if company, ok := strings.CutPrefix(label, labelCompanyPrefix); ok {
		if company == "" || CompanyID(company) == Installation {
			return Scope{}, false, true
		}
		return Scope{Company: CompanyID(company)}, true, true
	}
	if raw, ok := strings.CutPrefix(label, labelAreaPrefix); ok {
		scope, parsed := ParseScope(raw)
		return scope, parsed, true
	}
	return Scope{}, false, false
}

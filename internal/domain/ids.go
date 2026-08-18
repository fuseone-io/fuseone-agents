package domain

import "strings"

// Typed identifiers. Distinct string types stop one from being passed where
// another is expected — a mistake the compiler catches and review does not.
type (
	CompanyID string
	AreaID    string
	AgentID   string
	VersionID string
	RunID     string
	ToolID    string
	UserID    string
)

// SystemWorker is the principal used for automatic maintenance the worker
// performs on stored configuration. It is not a user and should only appear
// where the platform is preserving an operator's previous decision.
const SystemWorker UserID = "system:worker"

// Scope locates any record within the installation hierarchy.
//
// The hierarchy is Installation -> Company -> Area -> Agent (PRD 3.1). Company
// exists from phase one with a single value: the column costs nothing now and
// avoids migrating the ledger, budgets and role grants when the group gains
// its second legal entity.
type Scope struct {
	Company CompanyID
	Area    AreaID
}

func (s Scope) Valid() bool {
	return s.Company != "" && s.Area != ""
}

func (s Scope) String() string {
	return string(s.Company) + "/" + string(s.Area)
}

// Contains reports whether a grant in this scope reaches another.
//
// A scope with no area is the whole company, which is what makes the hierarchy
// in PRD §3.1 real for grants and not only for budgets: installation → company
// → area, inheriting downwards and never widening. Without it the first
// administrator of an installation — granted where no areas exist yet — could
// govern nothing, and every area created afterwards would be invisible to
// them until somebody granted each one by hand.
//
// It only ever widens downwards. An area never reaches its siblings, and never
// reaches the company above it.
func (s Scope) Contains(other Scope) bool {
	// The one scope above every company. Checked first and by name, never by
	// emptiness: the zero scope must go on reaching nothing.
	if s.Company == Installation {
		return true
	}
	if s.Company != other.Company {
		return false
	}
	return s.Area == "" || s.Area == other.Area
}

// ParseScope reads the "company/area" form.
func ParseScope(v string) (Scope, bool) {
	company, area, found := strings.Cut(v, "/")
	if !found || company == "" || area == "" {
		return Scope{}, false
	}
	// Never the scope above every company. This reads what somebody typed or
	// what arrived in a query string, and a caller who could name the
	// installation could ask to be checked against it.
	if CompanyID(company) == Installation {
		return Scope{}, false
	}
	return Scope{Company: CompanyID(company), Area: AreaID(area)}, true
}

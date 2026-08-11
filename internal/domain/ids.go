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

// Contains reports whether s covers other. An area covers only itself; the
// company comparison is what enforces the boundary between legal entities.
func (s Scope) Contains(other Scope) bool {
	return s.Company == other.Company && s.Area == other.Area
}

// ParseScope reads the "company/area" form.
func ParseScope(v string) (Scope, bool) {
	company, area, found := strings.Cut(v, "/")
	if !found || company == "" || area == "" {
		return Scope{}, false
	}
	return Scope{Company: CompanyID(company), Area: AreaID(area)}, true
}

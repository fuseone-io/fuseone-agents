package domain

// RecordedScope is a scope as a step records it. Its own type, so the ledger's
// JSON keeps one shape whatever happens to Scope's encoding.
type RecordedScope struct {
	Company CompanyID `json:"company"`
	Area    AreaID    `json:"area,omitempty"`
}

// RecordScope is s, as a step records it.
func RecordScope(s Scope) *RecordedScope {
	return &RecordedScope{Company: s.Company, Area: s.Area}
}

// Scope is the recorded scope back.
func (r RecordedScope) Scope() Scope { return Scope{Company: r.Company, Area: r.Area} }

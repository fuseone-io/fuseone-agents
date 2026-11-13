package domain

// RulingLevel is whose word a ruling is (NT-019): the platform's, the ceiling,
// stored at the installation, or the owner's, stored at the tool's server's
// scope. For a server of the installation the two are one row, the platform's.
type RulingLevel string

const (
	RulingPlatform RulingLevel = "platform"
	RulingOwner    RulingLevel = "owner"
)

// ToolRuling is one ruling on a tool as the console lists it: where it is
// stored, what it says, and whether it decides the tool.
type ToolRuling struct {
	Level     RulingLevel
	Scope     Scope
	Effect    Effect
	Untrusted bool
	// InForce marks the ruling the worker applies, chosen as RulingsInForce
	// chooses it. Whether it still matches the tool's definition is the
	// tool's Stale, not this.
	InForce bool
	// Overridden marks an owner's ruling while the platform has one on the
	// same tool: listed, so the owner sees their word, and not applied.
	Overridden bool
}

/*
RulingsOf lists each tool's rulings, the platform's first (NT-019).

The same reading as RulingsInForce: a ruling stored anywhere but the
installation or the tool's server's scope is nobody's word and is left out,
and so is a ruling for a tool scopeOf does not know.
*/
func RulingsOf(given []ToolClassification, scopeOf func(ToolID) (Scope, bool)) map[ToolID][]ToolRuling {
	installation := Scope{Company: Installation}
	platform := map[ToolID]*ToolRuling{}
	owner := map[ToolID]*ToolRuling{}
	for _, r := range given {
		server, ok := scopeOf(r.Tool)
		if !ok {
			continue
		}
		listed := ToolRuling{Scope: r.Scope, Effect: r.Effect, Untrusted: r.Untrusted}
		switch {
		case r.Scope == (Scope{}) || r.Scope == installation:
			listed.Level, listed.Scope = RulingPlatform, installation
			platform[r.Tool] = &listed
		case r.Scope == server:
			listed.Level = RulingOwner
			owner[r.Tool] = &listed
		}
	}
	out := map[ToolID][]ToolRuling{}
	for id, p := range platform {
		p.InForce = true
		out[id] = append(out[id], *p)
	}
	for id, o := range owner {
		_, overridden := platform[id]
		o.Overridden, o.InForce = overridden, !overridden
		out[id] = append(out[id], *o)
	}
	return out
}

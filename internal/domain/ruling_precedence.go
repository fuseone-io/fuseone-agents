package domain

/*
RulingsInForce picks, for each tool, the one ruling that decides it (NT-019).

A tool has at most two: its owner's, stored at its server's scope, and the
platform's, stored at the installation, which wins whatever order they were
read in. A ruling stored anywhere else is nobody's word about this tool and is
left out, and so is a ruling for a tool scopeOf does not know. A ruling with no
scope was read before rulings had one, when every ruling was the platform's.

One function for every reader, the worker's catalogue and the console's
listing, so the two cannot disagree about which ruling a tool is under.
*/
func RulingsInForce(given []ToolClassification, scopeOf func(ToolID) (Scope, bool)) []ToolClassification {
	installation := Scope{Company: Installation}
	chosen := map[ToolID]ToolClassification{}
	platform := map[ToolID]bool{}
	var order []ToolID
	for _, r := range given {
		server, ok := scopeOf(r.Tool)
		if !ok {
			continue
		}
		atPlatform := r.Scope == (Scope{}) || r.Scope == installation
		if !atPlatform && r.Scope != server {
			continue
		}
		if _, seen := chosen[r.Tool]; !seen {
			order = append(order, r.Tool)
		}
		if atPlatform || !platform[r.Tool] {
			chosen[r.Tool] = r
			platform[r.Tool] = platform[r.Tool] || atPlatform
		}
	}
	out := make([]ToolClassification, 0, len(order))
	for _, id := range order {
		out = append(out, chosen[id])
	}
	return out
}

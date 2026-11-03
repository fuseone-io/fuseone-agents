package guard

// Role is a factory role. It decides two things and nothing else: which tools
// a caller sees, and which lane changes move_card accepts from it. Both
// tables are switches, not maps, so no package-level state can be mutated.
type Role string

const (
	RoleTriage    Role = "triage"
	RoleArchitect Role = "architect"
	RoleDeveloper Role = "developer"
	RoleReviewer  Role = "reviewer"
	RoleQA        Role = "qa"
	RoleMerger    Role = "merger"
)

// Roles is the closed set, in the order the factory's flow meets them.
func Roles() []Role {
	return []Role{RoleTriage, RoleArchitect, RoleDeveloper, RoleReviewer, RoleQA, RoleMerger}
}

func knownRole(r Role) bool {
	for _, k := range Roles() {
		if r == k {
			return true
		}
	}
	return false
}

// Tool names. FuseOne addresses a tool as <registered server name>.<tool>.
const (
	ToolMoveCard        = "move_card"
	ToolSetCardFields   = "set_card_fields"
	ToolRecordQAVerdict = "record_qa_verdict"
	ToolMergeIfReady    = "merge_if_ready"
	ToolSendBack        = "send_back"
	ToolListReadyPRs    = "list_ready_prs"
)

// ToolsFor is the role's surface: exactly the tools a caller with that role
// may list and call. An unknown role has none.
func ToolsFor(r Role) []string {
	switch r {
	case RoleTriage, RoleArchitect:
		return []string{ToolMoveCard, ToolSetCardFields}
	case RoleDeveloper, RoleReviewer:
		return []string{ToolMoveCard}
	case RoleQA:
		return []string{ToolMoveCard, ToolRecordQAVerdict}
	case RoleMerger:
		return []string{ToolMoveCard, ToolListReadyPRs, ToolMergeIfReady, ToolSendBack}
	}
	return nil
}

func hasTool(r Role, tool string) bool {
	for _, t := range ToolsFor(r) {
		if t == tool {
			return true
		}
	}
	return false
}

// judgeMove is the transition table (NT-013 § The board). Nothing leaves
// Travado: only a person moves a card out of it. Nothing leaves Done. Moving
// to the lane a card is already in is an idempotent success. Every other
// change is allowed only on the row of the role that owns it; nobody acts in
// Preparado, so no agent moves a card out of it.
func judgeMove(r Role, from, to Lane) Reason {
	switch {
	case from == to:
		return ReasonAlreadyInLane
	case from == LaneTravado:
		return ReasonTravadoIsHuman
	case from == LaneDone:
		return ReasonDoneIsFinal
	case roleMoves(r, from, to):
		return ""
	}
	return ReasonTransitionRefused
}

func roleMoves(r Role, from, to Lane) bool {
	switch r {
	case RoleTriage, RoleArchitect:
		return from == LaneTriagem && (to == LanePreparado || to == LaneTravado)
	case RoleDeveloper, RoleReviewer, RoleQA:
		return from == LaneDoing && to == LaneTravado
	case RoleMerger:
		// Travado is the Merger's exit when only a person can unblock a merge:
		// a base the head is behind, a conflict, a refusal GitHub gave.
		return from == LaneDoing && (to == LaneDone || to == LaneTravado)
	}
	return false
}

package guard

import "testing"

// allowedMoves is D2 written out by hand, independently of judgeMove: the
// only lane changes each role may make.
var allowedMoves = map[Role][][2]Lane{
	RoleTriage:    {{LaneTriagem, LanePreparado}, {LaneTriagem, LaneTravado}},
	RoleArchitect: {{LaneTriagem, LanePreparado}, {LaneTriagem, LaneTravado}},
	RoleDeveloper: {{LaneDoing, LaneTravado}},
	RoleReviewer:  {{LaneDoing, LaneTravado}},
	RoleQA:        {{LaneDoing, LaneTravado}},
	RoleMerger:    {{LaneDoing, LaneDone}, {LaneDoing, LaneTravado}},
}

func allowed(r Role, from, to Lane) bool {
	for _, m := range allowedMoves[r] {
		if m == [2]Lane{from, to} {
			return true
		}
	}
	return false
}

// refusalFor is the reason a move outside the role's row must carry.
func refusalFor(from, to Lane) Reason {
	switch {
	case from == to:
		return ReasonAlreadyInLane
	case from == LaneTravado:
		return ReasonTravadoIsHuman
	case from == LaneDone:
		return ReasonDoneIsFinal
	}
	return ReasonTransitionRefused
}

func TestJudgeMove_everyRoleAndPair_matchesTheTable(t *testing.T) {
	t.Parallel()

	for _, role := range Roles() {
		for _, from := range Lanes() {
			for _, to := range Lanes() {
				want := Reason("")
				if !allowed(role, from, to) {
					want = refusalFor(from, to)
				}
				if got := judgeMove(role, from, to); got != want {
					t.Errorf("%s %s -> %s = %q, want %q", role, from, to, got, want)
				}
			}
		}
	}
}

func TestJudgeMove_unknownRole_movesNothing(t *testing.T) {
	t.Parallel()

	for _, from := range Lanes() {
		for _, to := range Lanes() {
			if r := judgeMove("curator", from, to); r == "" {
				t.Fatalf("an unknown role moved %s -> %s", from, to)
			}
		}
	}
}

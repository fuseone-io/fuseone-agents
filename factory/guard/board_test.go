package guard

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"
)

const testItem = "PVTI_lADOcard1"

// roleBot is each role's bot user id, and roleToken the installation token
// the fake knows it by. QA's bot is the one the merge harness trusts.
var roleBot = map[Role]int64{
	RoleTriage: 101, RoleArchitect: 102, RoleDeveloper: 103, RoleReviewer: 104, RoleQA: qaBotID, RoleMerger: 106,
}

func roleToken(r Role) string { return string(r) + "-token" }

type boardHarness struct {
	t    *testing.T
	fake *fakeGraphQL
	srv  *httptest.Server
	cfg  BoardConfig
}

// newBoardHarness holds one card of the configured project, in lane, with
// no Work type or Size set, and knows every role's bot token.
func newBoardHarness(t *testing.T, lane Lane) *boardHarness {
	t.Helper()
	cfg := testBoardConfig()
	fake, srv := newFakeGraphQL(t, cfg)
	fake.items[testItem] = &fakeItem{ID: testItem, ProjectID: cfg.ProjectID, Status: cfg.Lanes[lane]}
	for role, id := range roleBot {
		fake.bots[roleToken(role)] = id
	}
	return &boardHarness{t: t, fake: fake, srv: srv, cfg: cfg}
}

func (h *boardHarness) client(tokens TokenSource) *Client {
	return NewClient(ClientOptions{GraphQLURL: h.srv.URL + "/graphql", HTTP: h.srv.Client(), Tokens: tokens})
}

func (h *boardHarness) board(role Role) *Board {
	return NewBoard(h.cfg, role, h.client(staticToken(roleToken(role))))
}

func (h *boardHarness) edit(change func(f *fakeGraphQL)) {
	h.fake.mu.Lock()
	defer h.fake.mu.Unlock()
	change(h.fake)
}

func (h *boardHarness) mutations() []fakeMutation {
	h.fake.mu.Lock()
	defer h.fake.mu.Unlock()
	return append([]fakeMutation(nil), h.fake.mutations...)
}

func (h *boardHarness) move(role Role, lane string) MoveResult {
	h.t.Helper()
	res, err := h.board(role).Move(h.t.Context(), MoveInput{ItemID: testItem, Lane: lane})
	if err != nil {
		h.t.Fatalf("move_card returned an error: %v", err)
	}
	return res
}

func (h *boardHarness) expectNoMutation(res MoveResult, want Reason) {
	h.t.Helper()
	if res.Moved || res.Reason != want {
		h.t.Fatalf("result = %+v, want %s", res, want)
	}
	if got := h.mutations(); len(got) != 0 {
		h.t.Fatalf("refused move still wrote: %+v", got)
	}
}

// All 25 from -> to pairs, for every role. A pair on the role's row moves
// with exactly one mutation, on the configured project and Status field;
// every other pair is refused with its reason and writes nothing.
func TestMoveCard_everyRoleAndPair_movesOnlyAlongTheRolesRow(t *testing.T) {
	t.Parallel()

	for _, role := range Roles() {
		for _, from := range Lanes() {
			for _, to := range Lanes() {
				t.Run(fmt.Sprintf("%s/%s->%s", role, from, to), func(t *testing.T) {
					t.Parallel()
					h := newBoardHarness(t, from)
					res := h.move(role, string(to))
					if !allowed(role, from, to) {
						h.expectNoMutation(res, refusalFor(from, to))
						return
					}
					h.expectMoved(role, res, from, to)
				})
			}
		}
	}
}

func (h *boardHarness) expectMoved(role Role, res MoveResult, from, to Lane) {
	h.t.Helper()
	if !res.Moved || res.Reason != ReasonMoved || res.From != from || res.To != to || res.ItemID != testItem {
		h.t.Fatalf("result = %+v, want moved %s -> %s", res, from, to)
	}
	want := fakeMutation{Token: roleToken(role), Project: h.cfg.ProjectID, Item: testItem,
		Field: h.cfg.StatusFieldID, Option: h.cfg.Lanes[to]}
	if got := h.mutations(); len(got) != 1 || got[0] != want {
		h.t.Fatalf("mutations = %+v, want exactly %+v", got, want)
	}
	var status string
	h.edit(func(f *fakeGraphQL) { status = f.items[testItem].Status })
	if status != h.cfg.Lanes[to] {
		h.t.Fatalf("card is in option %q, want %s", status, to)
	}
}

// A card of another project is refused before anything is written, even on
// a move the role's row allows.
func TestMoveCard_itemOfAnotherProject_wrongProjectAndNoMutation(t *testing.T) {
	t.Parallel()

	h := newBoardHarness(t, LaneTriagem)
	h.edit(func(f *fakeGraphQL) { f.items[testItem].ProjectID = otherProjectID })
	h.expectNoMutation(h.move(RoleTriage, "Preparado"), ReasonWrongProject)
}

func TestMoveCard_lanesOutsideTheClosedSet_invalidInput(t *testing.T) {
	t.Parallel()

	for _, lane := range []string{"Backlog", "travado", "TRIAGEM", "", " Preparado", "opt-preparado"} {
		t.Run(lane, func(t *testing.T) {
			t.Parallel()
			h := newBoardHarness(t, LaneTriagem)
			h.expectNoMutation(h.move(RoleTriage, lane), ReasonInvalidInput)
		})
	}
}

func TestMoveCard_malformedItemID_invalidInput(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"", "PVTI_", "PVT_kwDOproject", "I_kwDOissue", "PVTI_a b", "PVTI_x\n", "PVTI_" + string(make([]byte, 65))} {
		h := newBoardHarness(t, LaneTriagem)
		res, err := h.board(RoleTriage).Move(t.Context(), MoveInput{ItemID: id, Lane: "Preparado"})
		if err != nil || res.Reason != ReasonInvalidInput || res.ItemID != "" {
			t.Fatalf("id %q: result %+v, err %v, want invalid_input without echoing the id", id, res, err)
		}
	}
}

// A Status that is unset, or an option the configuration does not name, is
// no lane: the move is not judged at all.
func TestMoveCard_currentStatusUnknownOrUnset_laneUnknown(t *testing.T) {
	t.Parallel()

	for name, option := range map[string]string{"unset": "", "unconfigured option": "opt-backlog"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newBoardHarness(t, LaneTriagem)
			h.edit(func(f *fakeGraphQL) { f.items[testItem].Status = option })
			h.expectNoMutation(h.move(RoleTriage, "Preparado"), ReasonLaneUnknown)
		})
	}
}

// Triage proposed Triagem -> Preparado; before it executed, a person moved
// the card to Travado. The move is judged on the card as it is now.
func TestMoveCard_personMovedToTravadoAfterTheProposal_travadoIsHuman(t *testing.T) {
	t.Parallel()

	h := newBoardHarness(t, LaneTriagem)
	h.edit(func(f *fakeGraphQL) { f.items[testItem].Status = h.cfg.Lanes[LaneTravado] })
	h.expectNoMutation(h.move(RoleTriage, "Preparado"), ReasonTravadoIsHuman)
}

func TestMoveCard_archivedOrMissingItem_refused(t *testing.T) {
	t.Parallel()

	cases := map[Reason]func(f *fakeGraphQL){
		ReasonItemArchived: func(f *fakeGraphQL) { f.items[testItem].Archived = true },
		ReasonItemNotFound: func(f *fakeGraphQL) { delete(f.items, testItem) },
	}
	for want, change := range cases {
		t.Run(string(want), func(t *testing.T) {
			t.Parallel()
			h := newBoardHarness(t, LaneTriagem)
			h.edit(change)
			h.expectNoMutation(h.move(RoleTriage, "Preparado"), want)
		})
	}
}

func TestMoveCard_alreadyInLane_successWithoutMutation(t *testing.T) {
	t.Parallel()

	h := newBoardHarness(t, LaneTravado)
	res := h.move(RoleDeveloper, "Travado")
	h.expectNoMutation(res, ReasonAlreadyInLane)
	if res.From != LaneTravado || res.To != LaneTravado {
		t.Fatalf("result = %+v", res)
	}
}

func TestMoveCard_updateNotApplied_reportedByOutcome(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		change func(f *fakeGraphQL)
		want   Reason
		status int
	}{
		"errors in the answer": {func(f *fakeGraphQL) { f.refuseField = testStatusField }, ReasonMoveRefused, 0},
		"http status":          {func(f *fakeGraphQL) { f.mutationStatus = 502 }, ReasonMoveRefused, 502},
		"answer lost":          {func(f *fakeGraphQL) { f.dropMutation = true }, ReasonOutcomeUnknown, 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newBoardHarness(t, LaneTriagem)
			h.edit(tc.change)
			res := h.move(RoleTriage, "Preparado")
			if res.Moved || res.Reason != tc.want || res.HTTPStatus != tc.status || len(h.mutations()) != 1 {
				t.Fatalf("result = %+v after %d mutations, want %s/%d", res, len(h.mutations()), tc.want, tc.status)
			}
		})
	}
}

// tokenFailsAtWrite hands out the token for the read and fails on the next
// request: the update's. Nothing leaves the process.
type tokenFailsAtWrite struct{ calls *int }

func (s tokenFailsAtWrite) Token(context.Context) (string, error) {
	*s.calls++
	if *s.calls > 1 {
		return "", errors.New("token unavailable")
	}
	return roleToken(RoleTriage), nil
}

func TestMoveCard_updateNeverSent_moveNotSent(t *testing.T) {
	t.Parallel()

	h := newBoardHarness(t, LaneTriagem)
	calls := 0
	res, err := NewBoard(h.cfg, RoleTriage, h.client(tokenFailsAtWrite{&calls})).
		Move(t.Context(), MoveInput{ItemID: testItem, Lane: "Preparado"})
	if err != nil || res.Moved || res.Reason != ReasonMoveNotSent || calls != 2 {
		t.Fatalf("result %+v, err %v, token calls %d, want move_not_sent at the write", res, err, calls)
	}
	if len(h.mutations()) != 0 {
		t.Fatal("an update reached GitHub")
	}
}

// A read GitHub refuses for any reason but NOT_FOUND fails the call: it is
// not a missing card, and nothing is written.
func TestMoveCard_readRefused_errorAndNoMutation(t *testing.T) {
	t.Parallel()

	h := newBoardHarness(t, LaneTriagem)
	h.edit(func(f *fakeGraphQL) { f.readErrors = true })
	_, err := h.board(RoleTriage).Move(t.Context(), MoveInput{ItemID: testItem, Lane: "Preparado"})
	if !errors.Is(err, ErrGitHub) || len(h.mutations()) != 0 {
		t.Fatalf("err = %v, mutations %d", err, len(h.mutations()))
	}
}

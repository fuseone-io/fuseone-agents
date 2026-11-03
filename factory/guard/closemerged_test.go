package guard

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

/*
A merge and the card's move to Done are two calls, each approved by a person.
A run that ends between them — it crashed, the approval was refused or never
given, the move's answer was lost — leaves a merged pull request whose card
sits in Doing, and list_ready_prs lists open pull requests only, so no later
run would ever see it. close_merged is the repair: a merged pull request on
the base still labelled factory:ready-to-merge is listed under merged, and
close_merged moves its card to Done and only then drops the label, so a
failure at any point leaves it listed for the next run.
*/

// gh is the Merger's client on both fakes: REST for the pull request and
// its labels, GraphQL for the listing and the board.
func (h *readyHarness) gh() *Client {
	return NewClient(ClientOptions{
		APIURL: h.cfg.APIURL, GraphQLURL: h.board.srv.URL + "/graphql", Repo: h.cfg.Repo,
		HTTP: h.srv.Client(), Tokens: staticToken("merger-token"),
	})
}

// mirror copies testPR's state on the REST fake into the GraphQL one, the
// way GitHub serves one pull request through both APIs.
func (h *readyHarness) mirror() {
	h.t.Helper()
	var open, merged bool
	var labels []string
	h.edit(func(f *fakeGitHub) {
		pr := f.prs[testPR]
		open, merged, labels = pr.State == "open", pr.Merged, slices.Clone(pr.Labels)
	})
	h.board.edit(func(f *fakeGraphQL) {
		f.prs[0].Open, f.prs[0].Merged, f.prs[0].Labels = open, merged, labels
	})
}

// merged puts testPR in the state a merge leaves it in, card untouched.
func (h *readyHarness) merged() {
	h.t.Helper()
	h.edit(func(f *fakeGitHub) {
		f.prs[testPR].State, f.prs[testPR].Merged, f.prs[testPR].MergeCommit = "closed", true, mergeSHA
	})
	h.mirror()
}

func (h *readyHarness) closeMerged(pr int) CloseResult {
	h.t.Helper()
	res, err := NewReadyLister(h.cfg, h.gh()).CloseMerged(h.t.Context(), CloseInput{PR: pr})
	if err != nil {
		h.t.Fatalf("close_merged: %v", err)
	}
	return res
}

func (h *readyHarness) lane() Lane {
	h.t.Helper()
	var lane Lane
	h.board.edit(func(f *fakeGraphQL) { lane, _ = h.board.cfg.laneOf(f.items[issueItem].Status) })
	return lane
}

// The review's scenario, interrupted exactly there: merge_if_ready merged,
// and nothing moved the card. The next listing shows it under merged, and
// close_merged repairs it.
func TestMerger_interruptedBetweenMergeAndMove_nextRunRepairsTheCard(t *testing.T) {
	t.Parallel()

	h := newReadyHarness(t)
	got, err := NewMerger(h.cfg, h.gh()).MergeIfReady(t.Context(), MergeInput{PR: testPR, HeadSHA: headA})
	if err != nil || !got.Merged {
		t.Fatalf("merge_if_ready = %+v, %v; want merged", got, err)
	}
	h.mirror()

	listed := h.list()
	want := MergedPR{PR: testPR, ItemID: issueItem, Lane: LaneDoing}
	if len(listed.PRs) != 0 || len(listed.Merged) != 1 || listed.Merged[0] != want {
		t.Fatalf("listed %+v, merged %+v; want only %+v under merged", listed.PRs, listed.Merged, want)
	}

	res := h.closeMerged(testPR)
	if !res.Closed || res.Reason != ReasonClosedOut || !res.Moved || h.lane() != LaneDone {
		t.Fatalf("close_merged = %+v, card in %s; want closed_out with the card in Done", res, h.lane())
	}
	if ops := h.fake.labelOps; !slices.Equal(ops, []string{"-" + LabelReadyToMerge}) {
		t.Fatalf("label writes = %v, want only the ready-to-merge label dropped", ops)
	}
	h.mirror()
	if again := h.list(); len(again.Merged) != 0 || len(again.PRs) != 0 {
		t.Fatalf("after the repair the listing still shows %+v / %+v", again.PRs, again.Merged)
	}
}

func TestCloseMerged_cardAlreadyInDone_onlyDropsTheLabel(t *testing.T) {
	t.Parallel()

	h := newReadyHarness(t)
	h.merged()
	h.board.edit(func(f *fakeGraphQL) { f.items[issueItem].Status = h.board.cfg.Lanes[LaneDone] })

	res := h.closeMerged(testPR)
	if !res.Closed || res.Moved || len(h.board.mutations()) != 0 {
		t.Fatalf("close_merged = %+v with %d board writes; want closed with no move", res, len(h.board.mutations()))
	}
	if !slices.Equal(h.fake.labelOps, []string{"-" + LabelReadyToMerge}) {
		t.Fatalf("label writes = %v", h.fake.labelOps)
	}
}

// A merged pull request with no card has nothing to move; the stale label
// still goes, so merged pull requests never pile up in the listing.
func TestCloseMerged_noCard_onlyDropsTheLabel(t *testing.T) {
	t.Parallel()

	h := newReadyHarness(t)
	h.board.edit(func(f *fakeGraphQL) { f.prs[0].Closing = nil })
	h.merged()

	if listed := h.list(); len(listed.Merged) != 1 || listed.Merged[0] != (MergedPR{PR: testPR}) {
		t.Fatalf("merged = %+v, want testPR with no card", listed.Merged)
	}
	res := h.closeMerged(testPR)
	if !res.Closed || res.Moved || len(h.board.mutations()) != 0 || len(h.fake.labelOps) != 1 {
		t.Fatalf("close_merged = %+v, board writes %d, label writes %v", res, len(h.board.mutations()), h.fake.labelOps)
	}
}

// Nothing is written for a pull request close_merged is not for.
func TestCloseMerged_notClosable_refusedAndNothingWritten(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		setup func(h *readyHarness)
		want  Reason
	}{
		"still open": {func(h *readyHarness) {}, ReasonPRNotMerged},
		"label gone": {func(h *readyHarness) {
			h.merged()
			h.board.edit(func(f *fakeGraphQL) { f.prs[0].Labels = nil })
		}, ReasonNotReadyToMerge},
		"card in Travado": {func(h *readyHarness) {
			h.merged()
			h.board.edit(func(f *fakeGraphQL) { f.items[issueItem].Status = h.board.cfg.Lanes[LaneTravado] })
		}, ReasonCardNotClosable},
		"two cards": {func(h *readyHarness) {
			h.merged()
			h.board.edit(func(f *fakeGraphQL) {
				f.items[secondItem] = &fakeItem{ID: secondItem, ProjectID: testProjectID, Status: f.board.Lanes[LaneDoing]}
				f.prs[0].Items = []string{secondItem}
			})
		}, ReasonCardNotClosable},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newReadyHarness(t)
			c.setup(h)
			res := h.closeMerged(testPR)
			if res.Closed || res.Reason != c.want || len(h.board.mutations()) != 0 || len(h.fake.labelOps) != 0 {
				t.Fatalf("close_merged = %+v, board writes %d, label writes %v; want %s and nothing written",
					res, len(h.board.mutations()), h.fake.labelOps, c.want)
			}
		})
	}
}

// The label is dropped only after the card reached Done: a move that fails
// keeps it, and the next listing still offers the repair.
func TestCloseMerged_moveFails_keepsTheLabelForTheNextRun(t *testing.T) {
	t.Parallel()

	h := newReadyHarness(t)
	h.merged()
	h.board.edit(func(f *fakeGraphQL) { f.mutationStatus = 502 })

	res := h.closeMerged(testPR)
	if res.Closed || res.Reason != ReasonMoveRefused || len(h.fake.labelOps) != 0 {
		t.Fatalf("close_merged = %+v, label writes %v; want move_refused and the label kept", res, h.fake.labelOps)
	}
	h.board.edit(func(f *fakeGraphQL) { f.mutationStatus = 0 })
	if listed := h.list(); len(listed.Merged) != 1 {
		t.Fatalf("merged = %+v, want testPR still offered", listed.Merged)
	}
	if again := h.closeMerged(testPR); !again.Closed || h.lane() != LaneDone {
		t.Fatalf("retry = %+v, card in %s", again, h.lane())
	}
}

// A merged pull request whose card a person holds is counted, not offered.
func TestListReadyPRs_mergedWithItsCardInTravado_heldNotOffered(t *testing.T) {
	t.Parallel()

	h := newReadyHarness(t)
	h.merged()
	h.board.edit(func(f *fakeGraphQL) { f.items[issueItem].Status = h.board.cfg.Lanes[LaneTravado] })
	if got := h.list(); len(got.Merged) != 0 || got.Skipped.MergedHeld != 1 {
		t.Fatalf("merged %+v, skipped %+v; want it held", got.Merged, got.Skipped)
	}
}

// Through the MCP server, close_merged answers no text GitHub served, closed
// out or refused: the fakes plant the canary in every title, body and name.
func TestCloseMergedTool_result_carriesOnlyReasonsNumbersAndIDs(t *testing.T) {
	t.Parallel()

	for name, lane := range map[string]Lane{"closed out": LaneDoing, "refused": LaneTravado} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newReadyHarness(t)
			h.merged()
			h.board.edit(func(f *fakeGraphQL) { f.items[issueItem].Status = h.board.cfg.Lanes[lane] })
			server, err := NewServer(h.cfg, RoleMerger, h.gh())
			if err != nil {
				t.Fatal(err)
			}
			res := h.call(connect(t, server), ToolCloseMerged, map[string]any{"pr": testPR})
			raw, _ := json.Marshal(res)
			if strings.Contains(string(raw), canary) || strings.Contains(string(raw), "canary") {
				t.Fatalf("result carries GitHub's free text: %s", raw)
			}
			if got := structured[CloseResult](t, res); got.Closed != (lane == LaneDoing) || got.PR != testPR {
				t.Fatalf("close_merged = %+v", got)
			}
		})
	}
}

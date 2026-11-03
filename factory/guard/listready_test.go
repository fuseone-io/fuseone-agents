package guard

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	issueItem  = "PVTI_lADOissue"
	secondItem = "PVTI_lADOsecond"
)

// readyHarness joins the two fakes: GitHub's REST API (pull request,
// reviews, statuses, checks) as merge_if_ready reads it, and GraphQL (the
// ready listing and the board) as list_ready_prs reads it. testPR is ready
// on headA, labelled, and its card, the issue it closes, sits in Doing.
type readyHarness struct {
	*harness
	board *boardHarness
}

func newReadyHarness(t *testing.T) *readyHarness {
	t.Helper()
	h := &readyHarness{harness: newHarness(t), board: newBoardHarness(t, LaneDoing)}
	h.board.edit(func(f *fakeGraphQL) {
		delete(f.items, testItem)
		f.items[issueItem] = &fakeItem{ID: issueItem, ProjectID: testProjectID, Status: h.board.cfg.Lanes[LaneDoing]}
		f.prs = []*fakeGQLPR{{Number: testPR, Head: headA, Base: "main", Open: true,
			Labels: []string{LabelReadyToMerge}, Closing: [][]string{{issueItem}}}}
	})
	return h
}

func (h *readyHarness) list() ReadyList {
	h.t.Helper()
	gh := NewClient(ClientOptions{
		APIURL: h.cfg.APIURL, GraphQLURL: h.board.srv.URL + "/graphql", Repo: h.cfg.Repo,
		HTTP: h.srv.Client(), Tokens: staticToken("merger-token"),
	})
	res, err := NewReadyLister(h.cfg, gh).List(h.t.Context())
	if err != nil {
		h.t.Fatalf("list_ready_prs: %v", err)
	}
	return res
}

func TestListReadyPRs_readyPullRequest_listedWithItsCardAndReady(t *testing.T) {
	t.Parallel()

	h := newReadyHarness(t)
	got := h.list()
	want := ReadyPR{PR: testPR, HeadSHA: headA, ItemID: issueItem, Ready: true}
	if len(got.PRs) != 1 || got.PRs[0] != want {
		t.Fatalf("listed %+v, want exactly %+v", got.PRs, want)
	}
}

// The pre-judgement is merge_if_ready's own: a pull request that would be
// refused is listed with the reason the Merger then sends back.
func TestListReadyPRs_notReady_listedWithMergeIfReadysReason(t *testing.T) {
	t.Parallel()

	h := newReadyHarness(t)
	h.edit(func(f *fakeGitHub) { f.reviews[testPR][0].CommitID = headB })
	got := h.list()
	if len(got.PRs) != 1 || got.PRs[0].Ready || got.PRs[0].Reason != ReasonReviewStale {
		t.Fatalf("listed %+v, want testPR not ready with review_stale", got.PRs)
	}
}

func TestListReadyPRs_cardOutsideDoing_skipped(t *testing.T) {
	t.Parallel()

	for _, lane := range []Lane{LaneTriagem, LanePreparado, LaneTravado, LaneDone} {
		t.Run(string(lane), func(t *testing.T) {
			t.Parallel()
			h := newReadyHarness(t)
			h.board.edit(func(f *fakeGraphQL) { f.items[issueItem].Status = h.board.cfg.Lanes[lane] })
			got := h.list()
			if len(got.PRs) != 0 || got.Skipped.NotInDoing != 1 {
				t.Fatalf("listed %+v, skipped %+v: a card in %s is no Merger's", got.PRs, got.Skipped, lane)
			}
		})
	}
}

func TestListReadyPRs_cardCount_mustBeExactlyOne(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		mutate func(f *fakeGraphQL)
		want   ReadySkipped
	}{
		"no card": {func(f *fakeGraphQL) { f.prs[0].Closing = nil }, ReadySkipped{NoCard: 1}},
		"card in another project": {func(f *fakeGraphQL) { f.items[issueItem].ProjectID = otherProjectID },
			ReadySkipped{NoCard: 1}},
		"archived card": {func(f *fakeGraphQL) { f.items[issueItem].Archived = true }, ReadySkipped{NoCard: 1}},
		"two cards": {func(f *fakeGraphQL) {
			f.items[secondItem] = &fakeItem{ID: secondItem, ProjectID: testProjectID, Status: f.board.Lanes[LaneDoing]}
			f.prs[0].Items = []string{secondItem}
		}, ReadySkipped{ManyCards: 1}},
		"the PR's own card and its issue's are the same item": {func(f *fakeGraphQL) {
			f.prs[0].Items = []string{issueItem}
		}, ReadySkipped{}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newReadyHarness(t)
			h.board.edit(c.mutate)
			got := h.list()
			if got.Skipped != c.want {
				t.Fatalf("skipped %+v, want %+v (listed %+v)", got.Skipped, c.want, got.PRs)
			}
			if listed := len(got.PRs) == 1; listed != (c.want == ReadySkipped{}) {
				t.Fatalf("listed %+v with skipped %+v", got.PRs, got.Skipped)
			}
		})
	}
}

// Oldest first: the Merger takes the first one it can act on.
func TestListReadyPRs_severalPullRequests_oldestFirst(t *testing.T) {
	t.Parallel()

	h := newReadyHarness(t)
	yes := true
	h.edit(func(f *fakeGitHub) {
		f.prs[7] = &fakePR{Head: headB, BaseRef: "main", BaseRepo: "fuseone-io/fuseone-agents", State: "open", Mergeable: &yes}
	})
	h.board.edit(func(f *fakeGraphQL) {
		f.items[secondItem] = &fakeItem{ID: secondItem, ProjectID: testProjectID, Status: f.board.Lanes[LaneDoing]}
		f.prs = append([]*fakeGQLPR{{Number: 7, Head: headB, Base: "main", Open: true,
			Labels: []string{LabelReadyToMerge}, Items: []string{secondItem}}}, f.prs...)
	})
	got := h.list()
	if len(got.PRs) != 2 || got.PRs[0].PR != 7 || got.PRs[1].PR != testPR {
		t.Fatalf("listed %+v, want #7 then #%d", got.PRs, testPR)
	}
}

// A listing longer than one page is not decided on: the guard fails closed.
func TestListReadyPRs_morePullRequestsThanOnePage_failsClosed(t *testing.T) {
	t.Parallel()

	h := newReadyHarness(t)
	h.board.edit(func(f *fakeGraphQL) { f.morePRs = true })
	gh := NewClient(ClientOptions{
		APIURL: h.cfg.APIURL, GraphQLURL: h.board.srv.URL + "/graphql", Repo: h.cfg.Repo,
		HTTP: h.srv.Client(), Tokens: staticToken("merger-token"),
	})
	if _, err := NewReadyLister(h.cfg, gh).List(t.Context()); err == nil {
		t.Fatal("a partial listing was answered")
	}
}

func TestListReadyPRs_otherLabelOrBase_notListed(t *testing.T) {
	t.Parallel()

	cases := map[string]func(f *fakeGraphQL){
		"needs review": func(f *fakeGraphQL) { f.prs[0].Labels = []string{LabelNeedsReview} },
		"other base":   func(f *fakeGraphQL) { f.prs[0].Base = "develop" },
		"closed":       func(f *fakeGraphQL) { f.prs[0].Open = false },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newReadyHarness(t)
			h.board.edit(mutate)
			if got := h.list(); len(got.PRs) != 0 || got.Skipped != (ReadySkipped{}) {
				t.Fatalf("listed %+v, skipped %+v", got.PRs, got.Skipped)
			}
		})
	}
}

// Through the MCP server, the listing carries no text GitHub served: the
// fake plants the canary in every title, body and project name.
func TestListReadyPRsTool_result_carriesOnlyNumbersSHAsIDsAndReasons(t *testing.T) {
	t.Parallel()

	h := newReadyHarness(t)
	h.edit(func(f *fakeGitHub) { f.checks[headA][1].Conclusion = "failure" })
	gh := NewClient(ClientOptions{
		APIURL: h.cfg.APIURL, GraphQLURL: h.board.srv.URL + "/graphql", Repo: h.cfg.Repo,
		HTTP: h.srv.Client(), Tokens: staticToken("merger-token"),
	})
	server, err := NewServer(h.cfg, RoleMerger, gh)
	if err != nil {
		t.Fatal(err)
	}
	res := h.call(connect(t, server), ToolListReadyPRs, map[string]any{})
	raw, _ := json.Marshal(res)
	if strings.Contains(string(raw), canary) || strings.Contains(string(raw), "canary") {
		t.Fatalf("result carries GitHub's free text: %s", raw)
	}
	got := structured[ReadyList](t, res)
	want := ReadyPR{PR: testPR, HeadSHA: headA, ItemID: issueItem, Reason: ReasonCheckNotSuccess, Check: "postgres"}
	if res.IsError || len(got.PRs) != 1 || got.PRs[0] != want {
		t.Fatalf("isError %v, listed %+v, want %+v", res.IsError, got.PRs, want)
	}
}

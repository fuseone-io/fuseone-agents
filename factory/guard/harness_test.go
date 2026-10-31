package guard

import (
	"context"
	"net/http/httptest"
	"testing"
)

const testPR = 42

type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }

type harness struct {
	t    *testing.T
	fake *fakeGitHub
	srv  *httptest.Server
	cfg  Config
}

// newHarness starts a fake GitHub holding one PR that is ready to merge on
// headA: approved there by an authorised reviewer, QA passed by the QA App,
// every expected check green from GitHub Actions, base up to date. Each test
// breaks exactly one of those.
//
// The fake's ruleset is seeded from the same reviewer and check lists as the
// guard's configuration: one list, read by both, as #207 requires.
func newHarness(t *testing.T) *harness {
	t.Helper()
	fake, srv := newFakeGitHub(t)
	cfg := Config{
		Role:        RoleMerger,
		APIURL:      srv.URL,
		WebURL:      "https://github.com",
		Repo:        Repo{Owner: "fuseone-io", Name: "fuseone-agents"},
		Base:        "main",
		Reviewers:   []string{"fuseone-factory-reviewer[bot]", "klinux"},
		QABotID:     qaBotID,
		Checks:      []string{"go", "postgres"},
		ChecksAppID: actionsAppID,
		MergeMethod: "squash",
	}
	yes := true
	fake.prs[testPR] = &fakePR{
		Head: headA, BaseRef: "main", BaseRepo: "fuseone-io/fuseone-agents",
		State: "open", Mergeable: &yes, Labels: []string{"factory:ready-to-merge"},
	}
	fake.reviews[testPR] = []fakeReview{
		{ID: 1, Login: "fuseone-factory-reviewer[bot]", State: "APPROVED", CommitID: headA},
	}
	fake.statuses[headA] = []fakeStatus{
		{ID: 10, State: "success", Context: "factory/qa", Creator: qaBot},
	}
	fake.checks[headA] = []fakeCheck{
		{ID: 20, Name: "go", Status: "completed", Conclusion: "success", AppID: actionsAppID, HeadSHA: headA},
		{ID: 21, Name: "postgres", Status: "completed", Conclusion: "success", AppID: actionsAppID, HeadSHA: headA},
	}
	fake.ruleset = fakeRuleset{
		Reviewers: cfg.Reviewers, QABotID: cfg.QABotID,
		Checks: cfg.Checks, ChecksAppID: cfg.ChecksAppID,
	}
	return &harness{t: t, fake: fake, srv: srv, cfg: cfg}
}

func (h *harness) client(token string) *Client {
	return NewClient(ClientOptions{
		APIURL: h.cfg.APIURL, Repo: h.cfg.Repo, HTTP: h.srv.Client(), Tokens: staticToken(token),
	})
}

func (h *harness) qa(token string) *QA {
	cfg := h.cfg
	cfg.Role = RoleQA
	return NewQA(cfg, h.client(token))
}

// edit changes the fake's state under its lock.
func (h *harness) edit(change func(f *fakeGitHub)) {
	h.fake.mu.Lock()
	defer h.fake.mu.Unlock()
	change(h.fake)
}

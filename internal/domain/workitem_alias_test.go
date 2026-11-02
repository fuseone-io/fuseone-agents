package domain_test

import (
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
The link's writer, as the domain states it (#204, slice 2b): the tool id, the
pull request a create_pull_request answer names, and what makes a returned
call a proof of the link.
*/

func TestToolID_workItemLink_isAPlatformOwnedCoordinationWithNoServer(t *testing.T) {
	t.Parallel()

	id := domain.ToolWorkItemLink
	if id != "$fuseone.work_item.link" {
		t.Fatalf("link id = %q, want the id the role specs name", id)
	}
	if !id.PlatformOwned() || !id.Coordination() || id.Server() != "" {
		t.Fatalf("%s: PlatformOwned %v, Coordination %v, Server %q; want platform-owned coordination with no server",
			id, id.PlatformOwned(), id.Coordination(), id.Server())
	}
}

func TestToolID_pullRequestTool_isCreatePullRequestOnAnyServer(t *testing.T) {
	t.Parallel()

	for id, want := range map[domain.ToolID]bool{
		"github-developer.create_pull_request": true,
		"github.create_pull_request":           true,
		"github.create_pull_request_review":    false,
		"github.merge_pull_request":            false,
		"create_pull_request":                  false,
		".create_pull_request":                 false,
		domain.ToolWorkItemLink:                false,
		domain.ToolWorkItemClaim:               false,
	} {
		if got := id.CreatesPullRequest(); got != want {
			t.Errorf("%q.CreatesPullRequest() = %v, want %v", id, got, want)
		}
	}
}

func TestPullRequestNamed_readsTheURLTheAnswerGives(t *testing.T) {
	t.Parallel()

	const want = domain.WorkItemRef("github:fuseone-io/fuseone-agents#240")
	for name, body := range map[string]string{
		"minimal answer":     `{"id":"9","url":"https://github.com/FuseOne-IO/fuseone-agents/pull/240"}`,
		"full pull request":  `{"number":240,"html_url":"https://github.com/fuseone-io/fuseone-agents/pull/240"}`,
		"both, in agreement": `{"url":"https://github.com/fuseone-io/fuseone-agents/pull/240","html_url":"https://github.com/fuseone-io/fuseone-agents/pull/240"}`,
	} {
		if got, ok := domain.PullRequestNamed([]byte(body)); !ok || got != want {
			t.Errorf("%s: PullRequestNamed = %q, %v; want %q", name, got, ok, want)
		}
	}
}

func TestPullRequestNamed_anythingElse_namesNone(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"an issue":          `{"url":"https://github.com/fuseone-io/fuseone-agents/issues/240"}`,
		"another host":      `{"url":"https://github.example.com/fuseone-io/fuseone-agents/pull/240"}`,
		"a sub-page":        `{"url":"https://github.com/fuseone-io/fuseone-agents/pull/240/files"}`,
		"the API URL":       `{"url":"https://api.github.com/repos/fuseone-io/fuseone-agents/pulls/240"}`,
		"two disagree":      `{"url":"https://github.com/fuseone-io/fuseone-agents/pull/240","html_url":"https://github.com/fuseone-io/fuseone-agents/pull/241"}`,
		"no url":            `{"number":240}`,
		"not JSON":          `opened https://github.com/fuseone-io/fuseone-agents/pull/240`,
		"a prefix cut off":  `{"url":"https://github.com/fuseone-io/fuseone-agents/pu`,
		"number zero":       `{"url":"https://github.com/fuseone-io/fuseone-agents/pull/0"}`,
		"url not a string":  `{"url":240}`,
		"an array of them":  `[{"url":"https://github.com/fuseone-io/fuseone-agents/pull/240"}]`,
		"empty":             ``,
		"query string kept": `{"url":"https://github.com/fuseone-io/fuseone-agents/pull/240?x=1"}`,
	} {
		if got, ok := domain.PullRequestNamed([]byte(body)); ok {
			t.Errorf("%s: PullRequestNamed = %q; want none", name, got)
		}
	}
}

func returnedPR() domain.ToolReturnedPayload {
	return domain.ToolReturnedPayload{
		Tool: "github-developer.create_pull_request", ResultRef: "content:pr",
		PullRequest: &domain.PullRequestStamp{Ref: linkAlias, Via: domain.ProvenanceHostedGitHubApp},
	}
}

func TestJudgePullRequestProof_theRunsOwnSuccessfulCreate_proves(t *testing.T) {
	t.Parallel()

	if err := domain.JudgePullRequestProof(returnedPR(), domain.Labels{}, linkAlias); err != nil {
		t.Fatalf("JudgePullRequestProof = %v, want the proof accepted", err)
	}
}

// An answer labelled untrusted proves nothing, whatever it names and however
// it was stamped: a server classified untrusted can name a pull request it
// never opened.
func TestJudgePullRequestProof_anUntrustedResult_unproven(t *testing.T) {
	t.Parallel()

	untrusted := domain.NewLabels(domain.LabelUntrusted)
	if err := domain.JudgePullRequestProof(returnedPR(), untrusted, linkAlias); !errors.Is(err, domain.ErrWorkItemLinkUnproven) {
		t.Fatalf("JudgePullRequestProof(untrusted) = %v, want ErrWorkItemLinkUnproven", err)
	}
}

func TestJudgePullRequestProof_anyPartMissing_unproven(t *testing.T) {
	t.Parallel()

	cases := map[string]func(p *domain.ToolReturnedPayload){
		"another tool":       func(p *domain.ToolReturnedPayload) { p.Tool = "github-developer.pull_request_read" },
		"the link tool":      func(p *domain.ToolReturnedPayload) { p.Tool = domain.ToolWorkItemLink },
		"a failed call":      func(p *domain.ToolReturnedPayload) { p.Failed, p.ErrorCode = true, "tool_error" },
		"another number":     func(p *domain.ToolReturnedPayload) { p.PullRequest.Ref = "github:fuseone-io/fuseone-agents#241" },
		"nothing named":      func(p *domain.ToolReturnedPayload) { p.PullRequest = nil },
		"served from cache":  func(p *domain.ToolReturnedPayload) { p.Cached = true },
		"another repository": func(p *domain.ToolReturnedPayload) { p.PullRequest.Ref = "github:fuseone-io/fuseone-site#240" },
		"no provenance":      func(p *domain.ToolReturnedPayload) { p.PullRequest.Via = "" },
		"another provenance": func(p *domain.ToolReturnedPayload) { p.PullRequest.Via = "github_local" },
	}
	for name, mutate := range cases {
		p := returnedPR()
		mutate(&p)
		if err := domain.JudgePullRequestProof(p, domain.Labels{}, linkAlias); !errors.Is(err, domain.ErrWorkItemLinkUnproven) {
			t.Errorf("%s: JudgePullRequestProof = %v, want ErrWorkItemLinkUnproven", name, err)
		}
	}
}

// The sentinels are distinct, so a caller can tell the model which fence
// refused it.
func TestWorkItemLinkErrors_areDistinct(t *testing.T) {
	t.Parallel()

	errs := []error{
		domain.ErrWorkItemLinkInvalid, domain.ErrWorkItemLinkedElsewhere,
		domain.ErrWorkItemLinkUnclaimed, domain.ErrWorkItemLinkNested,
		domain.ErrWorkItemLinkUnproven,
	}
	for i, a := range errs {
		for j, b := range errs {
			if i != j && errors.Is(a, b) {
				t.Errorf("%v is %v", a, b)
			}
		}
	}
}

func TestJudgeApprovalProof_onlyAnApprovalOfTheLinkProves(t *testing.T) {
	t.Parallel()

	approved := domain.ApprovalDecidedPayload{Approved: true, By: "ana", AtSeq: 7}
	link := domain.ApprovalRequestedPayload{Tool: domain.ToolWorkItemLink, ArgsDigest: domain.LinkArgsDigest(linkAlias)}
	if err := domain.JudgeApprovalProof(approved, link, 8, linkAlias); err != nil {
		t.Fatalf("an approval of the link = %v, want it accepted", err)
	}
	refused := approved
	refused.Approved = false
	unanchored := approved
	unanchored.AtSeq = 0
	later := approved
	later.AtSeq = 8
	for name, c := range map[string]struct {
		decided   domain.ApprovalDecidedPayload
		requested domain.ApprovalRequestedPayload
	}{
		"a refusal":             {refused, link},
		"a decision on nothing": {unanchored, link},
		"a request not before":  {later, link},
		"an approval of a write": {approved, domain.ApprovalRequestedPayload{Tool: "github.issue_write",
			ArgsDigest: domain.LinkArgsDigest(linkAlias)}},
		// Review round 1, P2: the decision approved one pull request, and
		// proves a link of that one alone.
		"an approval of another pull request": {approved, domain.ApprovalRequestedPayload{Tool: domain.ToolWorkItemLink,
			ArgsDigest: domain.LinkArgsDigest("github:fuseone-io/fuseone-agents#241")}},
		"an approval naming no arguments": {approved, domain.ApprovalRequestedPayload{Tool: domain.ToolWorkItemLink}},
	} {
		if err := domain.JudgeApprovalProof(c.decided, c.requested, 8, linkAlias); !errors.Is(err, domain.ErrWorkItemLinkUnproven) {
			t.Errorf("%s: JudgeApprovalProof = %v, want ErrWorkItemLinkUnproven", name, err)
		}
	}
}

func TestJudgeAlias_oneRootOneLevel(t *testing.T) {
	t.Parallel()

	p := validLink()
	cases := []struct {
		name      string
		aliasRoot domain.WorkItemRef
		hasAlias  bool
		rootAlias bool
		want      error
	}{
		{"a new alias", "", false, false, nil},
		{"the same link again", linkRoot, false, false, nil},
		{"another card's alias", "github:fuseone-io/fuseone-agents#7", false, false, domain.ErrWorkItemLinkedElsewhere},
		{"an alias that is a root", "", true, false, domain.ErrWorkItemLinkNested},
		{"a root that is an alias", "", false, true, domain.ErrWorkItemLinkNested},
	}
	for _, c := range cases {
		err := domain.JudgeAlias(p, c.aliasRoot, c.hasAlias, c.rootAlias)
		if (c.want == nil && err != nil) || (c.want != nil && !errors.Is(err, c.want)) {
			t.Errorf("%s: JudgeAlias = %v, want %v", c.name, err, c.want)
		}
	}
}

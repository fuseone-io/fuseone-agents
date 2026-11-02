package ledger_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
The link's fence, against both stores (#204, slice 2b).

The run supplies the alias; the ledger holds the rest against the record while
it seals the step: the root is the card the run holds, the proof is the run's
own create_pull_request naming the alias or a person's approval of the link,
an alias has one root for good, and links are one level deep.
*/

const (
	otherAlias = domain.WorkItemRef("github:fuseone-io/fuseone-agents#241")
	thirdCard  = domain.WorkItemRef("github:fuseone-io/fuseone-agents#125")
)

// returnPullRequest records a create_pull_request call of the run and its
// return naming pr, as the engine writes them, and returns the return's seq.
func returnPullRequest(t *testing.T, s Store, runID domain.RunID, agent domain.AgentID, pr domain.WorkItemRef) int64 {
	t.Helper()
	return returnOf(t, s, runID, agent, domain.ToolReturnedPayload{
		Tool: "github-developer.create_pull_request", ResultRef: "content:pr", PullRequest: hosted(pr),
	})
}

// hosted is a pull request stamped as the engine stamps a trusted answer of
// the hosted GitHub server reached as an App. None for an empty reference.
func hosted(pr domain.WorkItemRef) *domain.PullRequestStamp {
	if pr == "" {
		return nil
	}
	return &domain.PullRequestStamp{Ref: pr, Via: domain.ProvenanceHostedGitHubApp}
}

func returnOf(t *testing.T, s Store, runID domain.RunID, agent domain.AgentID, p domain.ToolReturnedPayload) int64 {
	t.Helper()
	return returnLabelled(t, s, runID, agent, p, domain.Labels{})
}

// returnLabelled is returnOf with the labels the tool_returned step is sealed
// with, as the engine seals a result's taint.
func returnLabelled(
	t *testing.T, s Store, runID domain.RunID, agent domain.AgentID, p domain.ToolReturnedPayload, labels domain.Labels,
) int64 {
	t.Helper()
	called := withPayload(t, step(runID, domain.StepToolCalled),
		domain.ToolCalledPayload{Tool: p.Tool, Effect: domain.EffectWrite})
	called.AgentID = agent
	mustAppend(t, s, called)
	returned := withPayload(t, step(runID, domain.StepToolReturned), p)
	returned.AgentID, returned.Labels = agent, labels
	sealed, err := s.Append(context.Background(), returned)
	if err != nil {
		t.Fatalf("return %s: %v", p.Tool, err)
	}
	return sealed.Seq
}

func openClaimed(t *testing.T, s Store, runID domain.RunID, agent domain.AgentID, ref domain.WorkItemRef) {
	t.Helper()
	openRunOf(t, s, runID, agent)
	if err := claim(t, s, runID, agent, ref); err != nil {
		t.Fatalf("claim %s for %s: %v", ref, runID, err)
	}
}

func linkWith(t *testing.T, s Store, runID domain.RunID, agent domain.AgentID, p domain.WorkItemLinkedPayload) error {
	t.Helper()
	_, err := s.Append(context.Background(), linkStepOf(t, runID, agent, p))
	return err
}

func linked(alias, root domain.WorkItemRef, proof int64) domain.WorkItemLinkedPayload {
	return domain.WorkItemLinkedPayload{
		Alias: alias, Root: root,
		Proof: domain.WorkItemLinkProof{Kind: domain.StepToolReturned, Seq: proof},
	}
}

// openLinked is a developer run that claimed root, opened alias and linked it.
func openLinked(t *testing.T, s Store, runID domain.RunID, agent domain.AgentID, alias, root domain.WorkItemRef) {
	t.Helper()
	openClaimed(t, s, runID, agent, root)
	proof := returnPullRequest(t, s, runID, agent, alias)
	if err := linkWith(t, s, runID, agent, linked(alias, root, proof)); err != nil {
		t.Fatalf("link %s to %s in %s: %v", alias, root, runID, err)
	}
}

func TestWorkItemLink_withoutAHeldClaim_refused(t *testing.T) {
	run(t, "a link from a run that holds no card", func(t *testing.T, s Store) {
		openRunOf(t, s, "run-a", "developer")
		proof := returnPullRequest(t, s, "run-a", "developer", linkAlias)

		err := linkWith(t, s, "run-a", "developer", linked(linkAlias, card, proof))
		if !errors.Is(err, domain.ErrWorkItemLinkUnclaimed) {
			t.Fatalf("link with no claim = %v, want ErrWorkItemLinkUnclaimed", err)
		}
	})
}

// The root is the run's claim, never a card the model names.
func TestWorkItemLink_aRootOtherThanTheHeldClaim_refused(t *testing.T) {
	run(t, "a link to a card the run does not hold", func(t *testing.T, s Store) {
		openClaimed(t, s, "run-a", "developer", card)
		proof := returnPullRequest(t, s, "run-a", "developer", linkAlias)

		err := linkWith(t, s, "run-a", "developer", linked(linkAlias, otherCard, proof))
		if !errors.Is(err, domain.ErrWorkItemLinkUnclaimed) {
			t.Fatalf("link to another root = %v, want ErrWorkItemLinkUnclaimed", err)
		}
	})
}

func TestWorkItemLink_underAnotherAgentThanItsRun_refused(t *testing.T) {
	run(t, "a link stamped with another agent", func(t *testing.T, s Store) {
		openClaimed(t, s, "run-a", "developer", card)
		proof := returnPullRequest(t, s, "run-a", "developer", linkAlias)

		err := linkWith(t, s, "run-a", "reviewer", linked(linkAlias, card, proof))
		if !errors.Is(err, domain.ErrWorkItemLinkInvalid) {
			t.Fatalf("link under another agent = %v, want ErrWorkItemLinkInvalid", err)
		}
	})
}

func TestWorkItemLink_sameAliasToTheSameRootTwice_isIdempotent(t *testing.T) {
	run(t, "the same link again", func(t *testing.T, s Store) {
		openLinked(t, s, "run-a", "developer", linkAlias, card)
		proof := returnPullRequest(t, s, "run-a", "developer", linkAlias)
		if err := linkWith(t, s, "run-a", "developer", linked(linkAlias, card, proof)); err != nil {
			t.Fatalf("the same link again in the run = %v, want success", err)
		}

		// Another agent's run that holds the same card links the same pull
		// request: one root, nothing new.
		openClaimed(t, s, "run-b", "qa", card)
		again := returnPullRequest(t, s, "run-b", "qa", linkAlias)
		if err := linkWith(t, s, "run-b", "qa", linked(linkAlias, card, again)); err != nil {
			t.Fatalf("the same link from another run = %v, want success", err)
		}
	})
}

func TestWorkItemLink_anAliasOfAnotherRoot_isLinkedElsewhere(t *testing.T) {
	run(t, "an alias another card owns", func(t *testing.T, s Store) {
		openLinked(t, s, "run-a", "developer", linkAlias, card)

		openClaimed(t, s, "run-b", "developer", otherCard)
		proof := returnPullRequest(t, s, "run-b", "developer", linkAlias)
		err := linkWith(t, s, "run-b", "developer", linked(linkAlias, otherCard, proof))
		if !errors.Is(err, domain.ErrWorkItemLinkedElsewhere) {
			t.Fatalf("relinking an alias = %v, want ErrWorkItemLinkedElsewhere", err)
		}
	})
}

// Keyed per company: another company's link of the same pull request is its
// own, and conflicts with nothing here.
func TestWorkItemLink_anotherCompanysAlias_doesNotConflict(t *testing.T) {
	run(t, "links are per company", func(t *testing.T, s Store) {
		openLinked(t, s, "run-a", "developer", linkAlias, card)

		other := domain.Scope{Company: "globex", Area: "cx"}
		started := step("run-g", domain.StepRunStarted)
		started.AgentID, started.Scope = "developer", other
		mustAppend(t, s, started)
		claimed := claimStep(t, "run-g", "developer", otherCard)
		claimed.Scope = other
		mustAppend(t, s, claimed)
		called := withPayload(t, step("run-g", domain.StepToolCalled),
			domain.ToolCalledPayload{Tool: "github.create_pull_request", Effect: domain.EffectWrite})
		returned := withPayload(t, step("run-g", domain.StepToolReturned),
			domain.ToolReturnedPayload{Tool: "github.create_pull_request", PullRequest: hosted(linkAlias)})
		for _, st := range []*domain.Step{&called, &returned} {
			st.AgentID, st.Scope = "developer", other
		}
		mustAppend(t, s, called)
		sealed := mustAppendSeq(t, s, returned)
		l := linkStepOf(t, "run-g", "developer", linked(linkAlias, otherCard, sealed))
		l.Scope = other
		if _, err := s.Append(context.Background(), l); err != nil {
			t.Fatalf("another company's link of the same pull request = %v, want success", err)
		}
	})
}

func TestWorkItemLink_aRootWithAliasesUsedAsAnAlias_refused(t *testing.T) {
	run(t, "a root cannot become an alias", func(t *testing.T, s Store) {
		openLinked(t, s, "run-a", "developer", linkAlias, card)

		openClaimed(t, s, "run-b", "developer", thirdCard)
		proof := returnPullRequest(t, s, "run-b", "developer", card)
		err := linkWith(t, s, "run-b", "developer", linked(card, thirdCard, proof))
		if !errors.Is(err, domain.ErrWorkItemLinkNested) {
			t.Fatalf("linking a root as an alias = %v, want ErrWorkItemLinkNested", err)
		}
	})
}

func TestWorkItemLink_anAliasUsedAsARoot_refused(t *testing.T) {
	run(t, "an alias cannot become a root", func(t *testing.T, s Store) {
		openLinked(t, s, "run-a", "developer", linkAlias, card)

		// The reviewer holds the pull request, and a pull request it opened
		// would be an alias of an alias.
		openClaimed(t, s, "run-r", "reviewer", linkAlias)
		proof := returnPullRequest(t, s, "run-r", "reviewer", otherAlias)
		err := linkWith(t, s, "run-r", "reviewer", linked(otherAlias, linkAlias, proof))
		if !errors.Is(err, domain.ErrWorkItemLinkNested) {
			t.Fatalf("linking to an alias = %v, want ErrWorkItemLinkNested", err)
		}
	})
}

func TestWorkItemLink_proofThatDoesNotVerify_refused(t *testing.T) {
	cases := map[string]func(t *testing.T, s Store) domain.WorkItemLinkedPayload{
		"a proof at a step of another kind": func(t *testing.T, s Store) domain.WorkItemLinkedPayload {
			returnPullRequest(t, s, "run-a", "developer", linkAlias)
			return linked(linkAlias, card, 2) // the claim
		},
		"a proof naming the wrong kind": func(t *testing.T, s Store) domain.WorkItemLinkedPayload {
			proof := returnPullRequest(t, s, "run-a", "developer", linkAlias)
			p := linked(linkAlias, card, proof)
			p.Proof.Kind = domain.StepApprovalDecided
			return p
		},
		"a return of another tool": func(t *testing.T, s Store) domain.WorkItemLinkedPayload {
			proof := returnOf(t, s, "run-a", "developer", domain.ToolReturnedPayload{
				Tool: "github-developer.pull_request_read", PullRequest: hosted(linkAlias),
			})
			return linked(linkAlias, card, proof)
		},
		"a result naming another pull request": func(t *testing.T, s Store) domain.WorkItemLinkedPayload {
			proof := returnPullRequest(t, s, "run-a", "developer", otherAlias)
			return linked(linkAlias, card, proof)
		},
		"a result naming none": func(t *testing.T, s Store) domain.WorkItemLinkedPayload {
			proof := returnPullRequest(t, s, "run-a", "developer", "")
			return linked(linkAlias, card, proof)
		},
		"a failed create": func(t *testing.T, s Store) domain.WorkItemLinkedPayload {
			proof := returnOf(t, s, "run-a", "developer", domain.ToolReturnedPayload{
				Tool: "github-developer.create_pull_request", PullRequest: hosted(linkAlias),
				Failed: true, ErrorCode: "tool_error",
			})
			return linked(linkAlias, card, proof)
		},
		// A stamp with no provenance proves nothing (#239 review), so a forged
		// stamp cannot pass at append either.
		// Untrusted content carries no stamp of its own: one with no
		// provenance is no trusted proof, whatever the content's label.
		"an untrusted result with no provenance": func(t *testing.T, s Store) domain.WorkItemLinkedPayload {
			proof := returnLabelled(t, s, "run-a", "developer", domain.ToolReturnedPayload{
				Tool: "github-developer.create_pull_request", PullRequest: &domain.PullRequestStamp{Ref: linkAlias},
			}, domain.NewLabels(domain.LabelUntrusted))
			return linked(linkAlias, card, proof)
		},
		"a stamp with no provenance": func(t *testing.T, s Store) domain.WorkItemLinkedPayload {
			proof := returnOf(t, s, "run-a", "developer", domain.ToolReturnedPayload{
				Tool: "github-developer.create_pull_request", PullRequest: &domain.PullRequestStamp{Ref: linkAlias},
			})
			return linked(linkAlias, card, proof)
		},
		"an approval of another call": func(t *testing.T, s Store) domain.WorkItemLinkedPayload {
			return approvalProof(t, s, "github-developer.issue_write", true)
		},
		"a refusal of the link": func(t *testing.T, s Store) domain.WorkItemLinkedPayload {
			return approvalProof(t, s, domain.ToolWorkItemLink, false)
		},
		// Review round 1, P2: an approval of one pull request proves no
		// link of another.
		"an approval of another pull request": func(t *testing.T, s Store) domain.WorkItemLinkedPayload {
			p := approvalProofOf(t, s, domain.ToolWorkItemLink, otherAlias, true)
			p.Alias = linkAlias
			return p
		},
	}
	for name, build := range cases {
		run(t, "a link with "+name, func(t *testing.T, s Store) {
			openClaimed(t, s, "run-a", "developer", card)
			p := build(t, s)
			before := headSeq(t, s, "run-a")
			if err := linkWith(t, s, "run-a", "developer", p); !errors.Is(err, domain.ErrWorkItemLinkUnproven) {
				t.Fatalf("link with %s = %v, want ErrWorkItemLinkUnproven", name, err)
			}
			if after := headSeq(t, s, "run-a"); after != before {
				t.Fatalf("a refused link moved the head from %d to %d", before, after)
			}
		})
	}
}

// The trusted proof is the stamp, not the content (review round 2): a
// create_pull_request whose answer is labelled untrusted — the default the
// factory keeps — still proves the link its stamp names.
func TestWorkItemLink_anUntrustedResultWithAHostedStamp_proves(t *testing.T) {
	run(t, "an untrusted answer's stamp proves", func(t *testing.T, s Store) {
		openClaimed(t, s, "run-a", "developer", card)
		proof := returnLabelled(t, s, "run-a", "developer", domain.ToolReturnedPayload{
			Tool: "github-developer.create_pull_request", PullRequest: hosted(linkAlias),
		}, domain.NewLabels(domain.LabelUntrusted))
		if err := linkWith(t, s, "run-a", "developer", linked(linkAlias, card, proof)); err != nil {
			t.Fatalf("link proved by an untrusted answer's hosted stamp = %v, want it sealed", err)
		}
	})
}

// A proof cannot be a step that comes after the link: the shape refuses it.
func TestWorkItemLink_proofAfterTheLink_refused(t *testing.T) {
	run(t, "a proof not yet written", func(t *testing.T, s Store) {
		openClaimed(t, s, "run-a", "developer", card)
		head := headSeq(t, s, "run-a")
		err := linkWith(t, s, "run-a", "developer", linked(linkAlias, card, head+1))
		if !errors.Is(err, domain.ErrWorkItemLinkInvalid) {
			t.Fatalf("link proved by itself = %v, want ErrWorkItemLinkInvalid", err)
		}
	})
}

// The path without a create_pull_request: a person approved the link.
func TestWorkItemLink_approvedByAPerson_sealed(t *testing.T) {
	run(t, "a link a person approved", func(t *testing.T, s Store) {
		openClaimed(t, s, "run-a", "developer", card)
		if err := linkWith(t, s, "run-a", "developer", approvalProof(t, s, domain.ToolWorkItemLink, true)); err != nil {
			t.Fatalf("link a person approved = %v, want it sealed", err)
		}
	})
}

// approvalProof records a request for tool, with the link of linkAlias as its
// arguments, and a person's decision on it, and returns a link of linkAlias to
// card that names that decision.
func approvalProof(t *testing.T, s Store, tool domain.ToolID, approved bool) domain.WorkItemLinkedPayload {
	t.Helper()
	return approvalProofOf(t, s, tool, linkAlias, approved)
}

// approvalProofOf is approvalProof for a request that approved alias.
func approvalProofOf(t *testing.T, s Store, tool domain.ToolID, alias domain.WorkItemRef, approved bool) domain.WorkItemLinkedPayload {
	t.Helper()
	requested := withPayload(t, step("run-a", domain.StepApprovalRequested),
		domain.ApprovalRequestedPayload{Tool: tool, Effect: domain.EffectWrite, ArgsDigest: domain.LinkArgsDigest(alias)})
	requested.AgentID = "developer"
	at := mustAppendSeq(t, s, requested)
	decided := withPayload(t, step("run-a", domain.StepApprovalDecided),
		domain.ApprovalDecidedPayload{Approved: approved, By: "ana", AtSeq: at})
	decided.AgentID = "developer"
	seq := mustAppendSeq(t, s, decided)
	p := linked(linkAlias, card, seq)
	p.Proof.Kind = domain.StepApprovalDecided
	return p
}

func mustAppendSeq(t *testing.T, s Store, st domain.Step) int64 {
	t.Helper()
	sealed, err := s.Append(context.Background(), st)
	if err != nil {
		t.Fatalf("append %s: %v", st.Kind, err)
	}
	return sealed.Seq
}

// A simulation rehearses the fence in its own run and takes nothing: its link
// conflicts with no real one.
func TestWorkItemLink_simulatedRun_isJudgedButHoldsNoAlias(t *testing.T) {
	run(t, "a simulated link holds nothing", func(t *testing.T, s Store) {
		openSimulatedRun(t, s, "run-sim", "developer")
		if err := claim(t, s, "run-sim", "developer", otherCard); err != nil {
			t.Fatalf("simulated claim: %v", err)
		}
		if err := linkWith(t, s, "run-sim", "developer", linked(linkAlias, otherCard, 2)); !errors.Is(err, domain.ErrWorkItemLinkUnproven) {
			t.Fatalf("simulated link without proof = %v, want ErrWorkItemLinkUnproven", err)
		}
		proof := returnPullRequest(t, s, "run-sim", "developer", linkAlias)
		if err := linkWith(t, s, "run-sim", "developer", linked(linkAlias, otherCard, proof)); err != nil {
			t.Fatalf("simulated link = %v, want the rehearsal sealed", err)
		}

		openLinked(t, s, "run-a", "developer", linkAlias, card)
	})
}

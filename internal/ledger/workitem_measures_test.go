package ledger_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
The ledger's weekly measures (#201, PR 1), against both stores.

Per ISO week (Monday 00:00 UTC) and agent: the runs that opened that week,
the cards they claimed, what they asked people and how people answered, and
the cost split three ways that adds up to the runs' own. A run belongs to the
week it opened in, with everything it asked. Narrowed by the caller's own
filter, never a simulation.
*/

// measuresWindow is the two weeks ending with trailStart's: trailStart is a
// Thursday, so [2026-09-21, 2026-10-05).
func measuresWindow(t *testing.T) domain.WorkItemWindow {
	t.Helper()
	w, err := domain.MeasureWindow(trailStart, 2)
	if err != nil {
		t.Fatalf("window: %v", err)
	}
	return w
}

// trailWeek is the Monday that starts trailStart's week.
var trailWeek = time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)

func measuresOf(t *testing.T, s Store, filter domain.RunFilter, cost *domain.RunFilter) []domain.WorkItemWeek {
	t.Helper()
	got, err := s.WorkItemMeasures(context.Background(), filter, cost, measuresWindow(t))
	if err != nil {
		t.Fatalf("WorkItemMeasures: %v", err)
	}
	return got
}

// everyCost is cost:read everywhere.
var everyCost = &domain.RunFilter{}

func weekRow(t *testing.T, rows []domain.WorkItemWeek, week time.Time, agent domain.AgentID) domain.WorkItemWeek {
	t.Helper()
	for _, r := range rows {
		if r.Week.Equal(week) && r.AgentID == agent {
			return r
		}
	}
	t.Fatalf("no row for %s in the week of %s: %+v", agent, week.Format(time.DateOnly), rows)
	return domain.WorkItemWeek{}
}

// openAt opens a run of agent in scope at an exact instant.
func openAt(t *testing.T, s Store, id domain.RunID, agent domain.AgentID, scope domain.Scope, at time.Time) {
	t.Helper()
	started := step(id, domain.StepRunStarted)
	started.AgentID, started.Scope, started.At = agent, scope, at
	mustAppend(t, s, started)
}

// stepOf is a step of a run in its own agent and scope.
func stepOf(id domain.RunID, agent domain.AgentID, scope domain.Scope, kind domain.StepKind) domain.Step {
	st := step(id, kind)
	st.AgentID, st.Scope = agent, scope
	return st
}

// propose asks a person about a call of tool with arguments of digest, and
// answers the request's seq.
func propose(t *testing.T, s Store, id domain.RunID, agent domain.AgentID, scope domain.Scope, tool domain.ToolID, digest string) int64 {
	t.Helper()
	req := stepOf(id, agent, scope, domain.StepApprovalRequested)
	req.Payload = mustJSON(t, domain.ApprovalRequestedPayload{
		Tool: tool, ArgsRef: "content:SECRET-ARGS-" + digest, ArgsDigest: digest, Reason: "SECRET-REASON",
	})
	return mustAppendSeq(t, s, req)
}

// decide answers the request at atSeq (0 is a legacy decision naming none).
func decide(t *testing.T, s Store, id domain.RunID, agent domain.AgentID, scope domain.Scope, d domain.ApprovalDecidedPayload) {
	t.Helper()
	dec := stepOf(id, agent, scope, domain.StepApprovalDecided)
	if d.By == "" {
		d.By = "ana"
	}
	dec.Payload = mustJSON(t, d)
	mustAppend(t, s, dec)
}

func refuse(t *testing.T, s Store, id domain.RunID, agent domain.AgentID, scope domain.Scope, atSeq int64) {
	t.Helper()
	decide(t, s, id, agent, scope, domain.ApprovalDecidedPayload{Approved: false, Note: "SECRET-NOTE", AtSeq: atSeq})
}

// pad seals n steps that ask nothing, pushing the run's next seq on.
func pad(t *testing.T, s Store, id domain.RunID, agent domain.AgentID, scope domain.Scope, n int) {
	t.Helper()
	for range n {
		mustAppend(t, s, stepOf(id, agent, scope, domain.StepPlanned))
	}
}

func TestWorkItemMeasures_aRunOpenedBeforeTheWindow_isAbsentWithItsApprovals(t *testing.T) {
	run(t, "a run opened before the window", func(t *testing.T, s Store) {
		w := measuresWindow(t)
		openAt(t, s, "run-before", "developer", areaFactory, w.From.Add(-time.Microsecond))
		// Asked and answered inside the window, it still belongs to the week it opened in.
		refuse(t, s, "run-before", "developer", areaFactory,
			propose(t, s, "run-before", "developer", areaFactory, "github.issue_write", "d1"))
		openAt(t, s, "run-inside", "developer", areaFactory, w.From)
		propose(t, s, "run-inside", "developer", areaFactory, "github.issue_write", "d1")

		rows := measuresOf(t, s, domain.RunFilter{}, everyCost)

		if len(rows) != 1 {
			t.Fatalf("rows = %+v, want only the week of the run inside the window", rows)
		}
		row := weekRow(t, rows, w.From, "developer")
		if row.Runs != 1 || row.Proposals.Requested != 1 || row.Proposals.Refused != 0 {
			t.Fatalf("row = %+v, want the inside run alone, without the earlier run's approvals", row)
		}
	})
}

// followUps seeds one run per case, each its own agent, and answers what
// each agent's row counts after the refusal.
func TestWorkItemMeasures_afterARefusal_sameToolCountsRevisedOrRepeatedByDigest(t *testing.T) {
	type seeder func(t *testing.T, s Store, id domain.RunID, agent domain.AgentID)
	const tool, other = domain.ToolID("github.issue_write"), domain.ToolID("github.pr_write")
	cases := []struct {
		name               string
		seed               seeder
		revised, repeated  int
		refused, requested int
	}{
		{"other arguments of the same tool", func(t *testing.T, s Store, id domain.RunID, a domain.AgentID) {
			refuse(t, s, id, a, areaFactory, propose(t, s, id, a, areaFactory, tool, "d1"))
			propose(t, s, id, a, areaFactory, tool, "d2")
		}, 1, 0, 1, 2},
		{"the same arguments again", func(t *testing.T, s Store, id domain.RunID, a domain.AgentID) {
			refuse(t, s, id, a, areaFactory, propose(t, s, id, a, areaFactory, tool, "d1"))
			propose(t, s, id, a, areaFactory, tool, "d1")
		}, 0, 1, 1, 2},
		{"another tool", func(t *testing.T, s Store, id domain.RunID, a domain.AgentID) {
			refuse(t, s, id, a, areaFactory, propose(t, s, id, a, areaFactory, tool, "d1"))
			propose(t, s, id, a, areaFactory, other, "d2")
		}, 0, 0, 1, 2},
		{"a proposal before the refusal", func(t *testing.T, s Store, id domain.RunID, a domain.AgentID) {
			propose(t, s, id, a, areaFactory, tool, "d2")
			refuse(t, s, id, a, areaFactory, propose(t, s, id, a, areaFactory, tool, "d1"))
		}, 0, 0, 1, 2},
		{"each refusal once per category", func(t *testing.T, s Store, id domain.RunID, a domain.AgentID) {
			refuse(t, s, id, a, areaFactory, propose(t, s, id, a, areaFactory, tool, "d1"))
			propose(t, s, id, a, areaFactory, tool, "d2")
			propose(t, s, id, a, areaFactory, tool, "d3")
			propose(t, s, id, a, areaFactory, tool, "d1")
			propose(t, s, id, a, areaFactory, tool, "d1")
		}, 1, 1, 1, 5},
		{"a legacy refusal that names no request", func(t *testing.T, s Store, id domain.RunID, a domain.AgentID) {
			propose(t, s, id, a, areaFactory, tool, "d1")
			refuse(t, s, id, a, areaFactory, 0)
			propose(t, s, id, a, areaFactory, tool, "d2")
			propose(t, s, id, a, areaFactory, tool, "d1")
		}, 0, 0, 1, 3},
		{"an approval is no refusal", func(t *testing.T, s Store, id domain.RunID, a domain.AgentID) {
			at := propose(t, s, id, a, areaFactory, tool, "d1")
			decide(t, s, id, a, areaFactory, domain.ApprovalDecidedPayload{Approved: true, AtSeq: at})
			propose(t, s, id, a, areaFactory, tool, "d2")
		}, 0, 0, 0, 2},
	}
	for _, c := range cases {
		run(t, c.name, func(t *testing.T, s Store) {
			openAt(t, s, "run-case", "developer", areaFactory, trailStart)
			c.seed(t, s, "run-case", "developer")

			p := weekRow(t, measuresOf(t, s, domain.RunFilter{}, everyCost), trailWeek, "developer").Proposals

			if p.RevisedAfterRefusal != c.revised || p.RepeatedAfterRefusal != c.repeated ||
				p.Refused != c.refused || p.Requested != c.requested {
				t.Fatalf("proposals = %+v; want revised %d, repeated %d, refused %d, requested %d",
					p, c.revised, c.repeated, c.refused, c.requested)
			}
		})
	}
}

// A refusal in one run and a later proposal of the same tool in another
// run of the same agent and week are two runs' business: nothing was
// revised. The second run is padded so its proposal comes at a higher seq
// than the refusal, as a comparison across runs would need.
func TestWorkItemMeasures_aProposalInAnotherRun_revisesNothing(t *testing.T) {
	run(t, "revised is bounded to the run", func(t *testing.T, s Store) {
		const tool = domain.ToolID("github.issue_write")
		openAt(t, s, "run-refused", "developer", areaFactory, trailStart)
		refuse(t, s, "run-refused", "developer", areaFactory, propose(t, s, "run-refused", "developer", areaFactory, tool, "d1"))
		openAt(t, s, "run-later", "developer", areaFactory, trailStart.Add(time.Minute))
		pad(t, s, "run-later", "developer", areaFactory, 5)
		propose(t, s, "run-later", "developer", areaFactory, tool, "d2")
		propose(t, s, "run-later", "developer", areaFactory, tool, "d1")

		p := weekRow(t, measuresOf(t, s, domain.RunFilter{}, everyCost), trailWeek, "developer").Proposals

		if p.RevisedAfterRefusal != 0 || p.RepeatedAfterRefusal != 0 || p.Requested != 3 {
			t.Fatalf("proposals = %+v; a proposal in another run counted as a follow-up", p)
		}
	})
}

func TestWorkItemMeasures_aStandingApproval_countsAsStandingAndApproved(t *testing.T) {
	run(t, "standing approvals", func(t *testing.T, s Store) {
		openAt(t, s, "run-1", "developer", areaFactory, trailStart)
		decide(t, s, "run-1", "developer", areaFactory, domain.ApprovalDecidedPayload{
			Approved: true, By: "ana", Note: domain.StandingApprovalNotePrefix + "sa_1: nightly",
			AtSeq: propose(t, s, "run-1", "developer", areaFactory, "github.issue_write", "d1"),
		})
		decide(t, s, "run-1", "developer", areaFactory, domain.ApprovalDecidedPayload{
			Approved: true, AtSeq: propose(t, s, "run-1", "developer", areaFactory, "github.issue_write", "d2"),
		})

		p := weekRow(t, measuresOf(t, s, domain.RunFilter{}, everyCost), trailWeek, "developer").Proposals

		if p.Standing != 1 || p.Approved != 2 {
			t.Fatalf("proposals = %+v; want standing 1 counted in approved 2", p)
		}
	})
}

func TestWorkItemMeasures_openRunsPendingAndAbandoned_areCounted(t *testing.T) {
	run(t, "open, pending and abandoned", func(t *testing.T, s Store) {
		openAt(t, s, "run-waiting", "developer", areaFactory, trailStart)
		propose(t, s, "run-waiting", "developer", areaFactory, "github.issue_write", "d1")
		openAt(t, s, "run-done", "developer", areaFactory, trailStart.Add(time.Minute))
		endRun(t, s, "run-done", "developer", areaFactory)
		openAt(t, s, "run-dropped", "developer", areaFactory, trailStart.Add(2*time.Minute))
		abandoned := stepOf("run-dropped", "developer", areaFactory, domain.StepAbandoned)
		abandoned.Payload = mustJSON(t, domain.AbandonedPayload{By: "ana", Reason: "SECRET-ABANDON"})
		mustAppend(t, s, abandoned)

		row := weekRow(t, measuresOf(t, s, domain.RunFilter{}, everyCost), trailWeek, "developer")

		if row.Runs != 3 || row.OpenRuns != 1 || row.Proposals.Pending != 1 || row.Proposals.Abandoned != 1 {
			t.Fatalf("row = %+v; want 3 runs, 1 open, 1 pending, 1 abandoned", row)
		}
	})
}

func TestWorkItemMeasures_anInheritedOnlyRun_countsItsWorkAndItsCostApart(t *testing.T) {
	run(t, "an inherited-only run", func(t *testing.T, s Store) {
		trailRunIn(t, s, "run-claim", "developer", areaFactory, 0, card)
		costed(t, s, "run-claim", "developer", areaFactory, 100)
		inheritIn(t, s, "run-inherited", "developer", areaFactory, 1, delivered(otherCard))
		costed(t, s, "run-inherited", "developer", areaFactory, 30)
		propose(t, s, "run-inherited", "developer", areaFactory, "github.issue_write", "d1")

		row := weekRow(t, measuresOf(t, s, domain.RunFilter{}, everyCost), trailWeek, "developer")

		if row.Runs != 2 || row.Proposals.Requested != 1 || row.Cards != 1 {
			t.Fatalf("row = %+v; want both runs and the inherited run's proposal, one card", row)
		}
		want := domain.WorkItemWeekCost{OnCardsMicros: 100, InheritedMicros: 30}
		if row.Cost == nil || *row.Cost != want {
			t.Fatalf("cost = %+v, want %+v", row.Cost, want)
		}
	})
}

func TestWorkItemMeasures_aRunOnNoCard_isOffCardCost(t *testing.T) {
	run(t, "a run on no card", func(t *testing.T, s Store) {
		openAt(t, s, "run-loose", "developer", areaFactory, trailStart)
		costed(t, s, "run-loose", "developer", areaFactory, 7)

		row := weekRow(t, measuresOf(t, s, domain.RunFilter{}, everyCost), trailWeek, "developer")

		if row.Cards != 0 || row.Cost == nil || *row.Cost != (domain.WorkItemWeekCost{OffCardMicros: 7}) {
			t.Fatalf("row = %+v cost %+v; want no card and 7 off-card", row, row.Cost)
		}
	})
}

// The developer claims the issue and links its pull request; a later run of
// the same agent claims the pull request; a third claims another card.
func TestWorkItemMeasures_aPullRequestAndItsIssue_areOneCard(t *testing.T) {
	run(t, "an alias resolves to its root", func(t *testing.T, s Store) {
		trailRunIn(t, s, "run-issue", "developer", areaFactory, 0, card)
		linkIn(t, s, "run-issue", "developer", areaFactory, linkAlias, card)
		endRun(t, s, "run-issue", "developer", areaFactory)
		trailRunIn(t, s, "run-pr", "developer", areaFactory, 10, linkAlias)
		endRun(t, s, "run-pr", "developer", areaFactory)
		trailRunIn(t, s, "run-other", "developer", areaFactory, 20, otherCard)

		row := weekRow(t, measuresOf(t, s, domain.RunFilter{}, everyCost), trailWeek, "developer")

		if row.Runs != 3 || row.Cards != 2 {
			t.Fatalf("row = %+v; want 3 runs on 2 cards: the pull request is its issue", row)
		}
	})
}

func TestWorkItemMeasures_simulatedAndOutOfScopeRuns_areAbsent(t *testing.T) {
	run(t, "simulations and other scopes", func(t *testing.T, s Store) {
		trailRunIn(t, s, "run-real", "developer", areaFactory, 0, card)
		sim := stepOf("run-sim", "developer", areaFactory, domain.StepRunStarted)
		sim.At = trailStart.Add(time.Minute)
		sim.Payload = mustJSON(t, domain.RunStartedPayload{Trigger: "simulation", Simulated: true, Simulation: "sim-1"})
		mustAppend(t, s, sim)
		propose(t, s, "run-sim", "developer", areaFactory, "github.issue_write", "d1")
		trailRunIn(t, s, "run-cx", "developer", areaCX, 2, otherCard)
		propose(t, s, "run-cx", "developer", areaCX, "github.issue_write", "d1")

		rows := measuresOf(t, s, domain.RunFilter{Scopes: []domain.Scope{areaFactory}}, everyCost)

		if len(rows) != 1 {
			t.Fatalf("rows = %+v, want the one real run in scope", rows)
		}
		if row := rows[0]; row.Runs != 1 || row.Cards != 1 || row.Proposals.Requested != 0 {
			t.Fatalf("row = %+v; a simulation or another scope was counted", row)
		}
	})
}

func TestWorkItemMeasures_filteredByAgent_readsThatAgentAlone(t *testing.T) {
	run(t, "agent filter", func(t *testing.T, s Store) {
		trailRunIn(t, s, "run-dev", "developer", areaFactory, 0, card)
		trailRunIn(t, s, "run-qa", "qa", areaFactory, 1, card)

		rows := measuresOf(t, s, domain.RunFilter{AgentID: "qa"}, everyCost)

		if len(rows) != 1 || rows[0].AgentID != "qa" {
			t.Fatalf("rows = %+v, want qa alone", rows)
		}
	})
}

// The three costs add up to what the runs themselves cost, per row, as the
// runs projection holds it — read here by ListRuns, not by the measures.
func TestWorkItemMeasures_theThreeCostsReconcileWithTheRunsCost(t *testing.T) {
	run(t, "cost reconciliation", func(t *testing.T, s Store) {
		trailRunIn(t, s, "run-claim", "developer", areaFactory, 0, card)
		costed(t, s, "run-claim", "developer", areaFactory, 1_000)
		endRun(t, s, "run-claim", "developer", areaFactory)
		trailRunIn(t, s, "run-alias", "developer", areaFactory, 1, linkAlias)
		costed(t, s, "run-alias", "developer", areaFactory, 200)
		inheritIn(t, s, "run-event", "developer", areaFactory, 2, fromEvent(card, "run-claim"))
		costed(t, s, "run-event", "developer", areaFactory, 30)
		openAt(t, s, "run-loose", "developer", areaCX, trailStart.Add(3*time.Minute))
		costed(t, s, "run-loose", "developer", areaCX, 4)
		openAt(t, s, "run-last-week", "developer", areaFactory, trailWeek.Add(-time.Hour))
		costed(t, s, "run-last-week", "developer", areaFactory, 50_000)

		rows := measuresOf(t, s, domain.RunFilter{}, everyCost)

		for _, row := range rows {
			if row.Cost == nil {
				t.Fatalf("row %+v has no cost", row)
			}
			got := row.Cost.OnCardsMicros + row.Cost.InheritedMicros + row.Cost.OffCardMicros
			if want := runsCost(t, s, row.AgentID, row.Week); got != want {
				t.Errorf("week %s: %d on cards + %d inherited + %d off card = %d, the runs cost %d",
					row.Week.Format(time.DateOnly), row.Cost.OnCardsMicros, row.Cost.InheritedMicros,
					row.Cost.OffCardMicros, got, want)
			}
		}
		this := weekRow(t, rows, trailWeek, "developer")
		if want := (domain.WorkItemWeekCost{OnCardsMicros: 1_200, InheritedMicros: 30, OffCardMicros: 4}); *this.Cost != want {
			t.Errorf("this week = %+v, want %+v", *this.Cost, want)
		}
	})
}

func runsCost(t *testing.T, s Store, agent domain.AgentID, week time.Time) int64 {
	t.Helper()
	runs, err := s.ListRuns(context.Background(), domain.RunFilter{
		AgentID: agent, Since: week, Until: week.AddDate(0, 0, 7),
	}, "", 200)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	var sum int64
	for _, r := range runs {
		sum += r.Cost.Micros
	}
	return sum
}

func TestWorkItemMeasures_costReadInOneScopeOfTwo_leavesTheRowsCostOut(t *testing.T) {
	run(t, "partial cost:read", func(t *testing.T, s Store) {
		trailRunIn(t, s, "run-factory", "developer", areaFactory, 0, card)
		costed(t, s, "run-factory", "developer", areaFactory, 10)
		trailRunIn(t, s, "run-cx", "developer", areaCX, 1, otherCard)
		costed(t, s, "run-cx", "developer", areaCX, 20)

		partial := measuresOf(t, s, domain.RunFilter{}, &domain.RunFilter{Scopes: []domain.Scope{areaFactory}})
		none := measuresOf(t, s, domain.RunFilter{}, nil)

		for name, rows := range map[string][]domain.WorkItemWeek{"partial": partial, "none": none} {
			row := weekRow(t, rows, trailWeek, "developer")
			if row.Cost != nil || row.Runs != 2 {
				t.Errorf("%s cost:read: row = %+v cost %+v; want both runs and no cost", name, row, row.Cost)
			}
		}
	})
}

// Sunday's last microsecond and Monday's first belong to two weeks, in both
// stores, whatever zone the instant was written in.
func TestWorkItemMeasures_aRunAtTheSundayBoundary_fallsInItsUTCWeek(t *testing.T) {
	run(t, "the Sunday boundary", func(t *testing.T, s Store) {
		openAt(t, s, "run-sunday", "developer", areaFactory, trailWeek.Add(-time.Microsecond))
		openAt(t, s, "run-monday", "developer", areaFactory, trailWeek.In(time.FixedZone("BRT", -3*60*60)))

		rows := measuresOf(t, s, domain.RunFilter{}, everyCost)

		if got := weekRow(t, rows, trailWeek.AddDate(0, 0, -7), "developer"); got.Runs != 1 {
			t.Errorf("the week before = %+v, want Sunday's run", got)
		}
		if got := weekRow(t, rows, trailWeek, "developer"); got.Runs != 1 {
			t.Errorf("the week = %+v, want Monday's run", got)
		}
	})
}

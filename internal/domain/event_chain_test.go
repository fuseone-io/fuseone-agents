package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestChainPositionNext_aRunNoEventOpened_startsTheChainAtHopOne(t *testing.T) {
	t.Parallel()

	for _, trigger := range []string{"manual", "cron", "webhook", ""} {
		next, withheld := domain.ChainPosition{Trigger: trigger}.Next("run-root")
		if withheld != nil {
			t.Fatalf("%q: withheld %+v; a run no event opened may always publish", trigger, withheld)
		}
		if next != (domain.EventChain{Root: "run-root", Hop: 1}) {
			t.Fatalf("%q: next = %+v, want the source as root at hop 1", trigger, next)
		}
	}
}

func TestChainPositionNext_carriesTheRootAndCountsOneHop(t *testing.T) {
	t.Parallel()

	position := domain.ChainPosition{
		Trigger: domain.TriggerEvent, Chain: &domain.EventChain{Root: "run-root", Hop: 3},
	}
	next, withheld := position.Next("run-middle")

	if withheld != nil || next != (domain.EventChain{Root: "run-root", Hop: 4}) {
		t.Fatalf("next = %+v, withheld %+v; want the same root at hop 4", next, withheld)
	}
}

func TestChainPositionNext_atTheLimit_withholdsWithTheHopAndTheLimit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		hop      int
		withhold bool
	}{
		{domain.MaxEventHops - 1, false},
		{domain.MaxEventHops, true},
		{domain.MaxEventHops + 3, true},
	}
	for _, c := range cases {
		position := domain.ChainPosition{
			Trigger: domain.TriggerEvent, Chain: &domain.EventChain{Root: "run-root", Hop: c.hop},
		}
		_, withheld := position.Next("run-x")
		if (withheld != nil) != c.withhold {
			t.Fatalf("hop %d: withheld %+v, want withheld = %v", c.hop, withheld, c.withhold)
		}
		want := domain.EventsWithheld{
			Reason: domain.ReasonEventHopLimit, Root: "run-root", Hop: c.hop, Limit: domain.MaxEventHops,
		}
		if c.withhold && *withheld != want {
			t.Fatalf("hop %d: withheld %+v, want %+v", c.hop, *withheld, want)
		}
	}
}

/*
A run an event opened that carries no chain was opened by a dispatcher that
did not count — an older binary during a rolling update. Read as a root it
would restart the count, so it publishes nothing and says why.
*/
func TestChainPositionNext_anEventOpenedRunWithNoChain_isNotARoot(t *testing.T) {
	t.Parallel()

	_, withheld := domain.ChainPosition{Trigger: domain.TriggerEvent}.Next("run-x")

	want := domain.EventsWithheld{Reason: domain.ReasonEventChainUnknown, Limit: domain.MaxEventHops}
	if withheld == nil || *withheld != want {
		t.Fatalf("withheld = %+v, want %+v", withheld, want)
	}
}

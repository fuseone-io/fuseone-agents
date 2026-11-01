package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestEventChainNext_aRunNoEventOpened_startsTheChainAtHopOne(t *testing.T) {
	t.Parallel()

	var source *domain.EventChain
	next, ok := source.Next("run-root")

	if !ok {
		t.Fatal("a run no event opened may always publish")
	}
	if next != (domain.EventChain{Root: "run-root", Hop: 1}) {
		t.Fatalf("next = %+v, want the source as root at hop 1", next)
	}
}

func TestEventChainNext_carriesTheRootAndCountsOneHop(t *testing.T) {
	t.Parallel()

	source := &domain.EventChain{Root: "run-root", Hop: 3}
	next, ok := source.Next("run-middle")

	if !ok || next != (domain.EventChain{Root: "run-root", Hop: 4}) {
		t.Fatalf("next = %+v, %v; want the same root at hop 4", next, ok)
	}
}

func TestEventChainNext_atTheLimit_publishesNothingFurther(t *testing.T) {
	t.Parallel()

	cases := []struct {
		hop  int
		want bool
	}{
		{domain.MaxEventHops - 1, true},
		{domain.MaxEventHops, false},
		{domain.MaxEventHops + 3, false},
	}
	for _, c := range cases {
		source := &domain.EventChain{Root: "run-root", Hop: c.hop}
		if _, ok := source.Next("run-x"); ok != c.want {
			t.Errorf("hop %d: may publish = %v, want %v", c.hop, ok, c.want)
		}
		if source.Exhausted() == c.want {
			t.Errorf("hop %d: exhausted = %v, want %v", c.hop, source.Exhausted(), !c.want)
		}
	}
}

func TestEventChainWithheld_namesTheReasonTheHopAndTheLimit(t *testing.T) {
	t.Parallel()

	source := &domain.EventChain{Root: "run-root", Hop: domain.MaxEventHops}
	got := source.Withheld()

	want := domain.EventsWithheld{
		Reason: domain.ReasonEventHopLimit, Root: "run-root",
		Hop: domain.MaxEventHops, Limit: domain.MaxEventHops,
	}
	if got == nil || *got != want {
		t.Fatalf("withheld = %+v, want %+v", got, want)
	}
	var root *domain.EventChain
	if root.Withheld() != nil {
		t.Fatal("a run no event opened withholds nothing")
	}
}

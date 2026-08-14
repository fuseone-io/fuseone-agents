package drift_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/drift"
	"github.com/fuseone/agents/internal/simulate"
)

/*
Nobody changed anything and it stopped working.

A provider ships a new model under a name that did not change and behaviour
moves. Inside somebody else's network nobody finds out until it is an
incident, and the whole point of a corpus on a clock is that the platform
says so first.

Read from two batteries of the *same* version: the definition is a digest of
its own bytes, so if it did not change then nothing about the agent did, and
a correction that stopped holding is the world moving underneath it.
*/

func TestSweep_theSameVersionStoppedHolding_isAnnounced(t *testing.T) {
	t.Parallel()

	said := &spy{}
	sweep := drift.New(corpusOf("triage"), batteries{
		"triage@v2": {"sim-new", "sim-old"},
	}, reports{
		"sim-old": held("estorno"),
		"sim-new": broke("estorno"),
	}, said, nil)

	found, err := sweep.Sweep(context.Background(), at())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if found != 1 {
		t.Fatalf("drifted = %d, want one", found)
	}
	if len(said.posted) != 1 {
		t.Fatalf("messages = %d, want one", len(said.posted))
	}
	// Named, so somebody can go and read the case rather than the whole
	// corpus. And marked as drift, which is a different thing from a person
	// having broken it.
	if said.posted[0].Event != channel.EventDrifted {
		t.Errorf("event = %q, want drift", said.posted[0].Event)
	}
	if said.posted[0].Reason == "" {
		t.Error("the notice says nothing about which correction moved")
	}
}

func TestSweep_stillHolding_saysNothing(t *testing.T) {
	t.Parallel()

	said := &spy{}
	sweep := drift.New(corpusOf("triage"), batteries{
		"triage@v2": {"sim-new", "sim-old"},
	}, reports{"sim-old": held("estorno"), "sim-new": held("estorno")}, said, nil)

	if _, err := sweep.Sweep(context.Background(), at()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	// A channel that hears everything is a channel people mute, and the
	// muted one always turns out to be where the real notice went.
	if len(said.posted) != 0 {
		t.Errorf("messages = %+v, want silence", said.posted)
	}
}

// A correction that started holding again is the world moving too, and it is
// not something to wake anybody up about.
func TestSweep_aCorrectionThatCameBack_isNotAnnounced(t *testing.T) {
	t.Parallel()

	said := &spy{}
	sweep := drift.New(corpusOf("triage"), batteries{
		"triage@v2": {"sim-new", "sim-old"},
	}, reports{"sim-old": broke("estorno"), "sim-new": held("estorno")}, said, nil)

	if _, err := sweep.Sweep(context.Background(), at()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(said.posted) != 0 {
		t.Errorf("messages = %+v, want silence", said.posted)
	}
}

// One battery is a reading, not a comparison. Announcing on it would fire
// once for every agent the first time this ever ran.
func TestSweep_onlyOneBatteryEverRun_saysNothing(t *testing.T) {
	t.Parallel()

	said := &spy{}
	sweep := drift.New(corpusOf("triage"), batteries{"triage@v2": {"sim-new"}},
		reports{"sim-new": broke("estorno")}, said, nil)

	if _, err := sweep.Sweep(context.Background(), at()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(said.posted) != 0 {
		t.Errorf("messages = %+v, want silence", said.posted)
	}
}

// A battery still being advanced is half a fold. Comparing against it would
// report every case that has not run yet as having stopped holding.
func TestSweep_aBatteryStillRunning_isNotComparedYet(t *testing.T) {
	t.Parallel()

	said := &spy{}
	running := broke("estorno")
	running.Running = true

	sweep := drift.New(corpusOf("triage"), batteries{"triage@v2": {"sim-new", "sim-old"}},
		reports{"sim-old": held("estorno"), "sim-new": running}, said, nil)

	if _, err := sweep.Sweep(context.Background(), at()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(said.posted) != 0 {
		t.Errorf("messages = %+v, want silence until it settles", said.posted)
	}
}

// Said once. A sweep that announced the same drift every pass is a sweep
// somebody silences, and then the next one is not heard either.
func TestSweep_twice_announcesOnce(t *testing.T) {
	t.Parallel()

	said := &spy{}
	sweep := drift.New(corpusOf("triage"), batteries{"triage@v2": {"sim-new", "sim-old"}},
		reports{"sim-old": held("estorno"), "sim-new": broke("estorno")}, said, nil)

	for range 3 {
		if _, err := sweep.Sweep(context.Background(), at()); err != nil {
			t.Fatalf("Sweep: %v", err)
		}
	}
	if len(said.posted) != 1 {
		t.Errorf("messages = %d, want one", len(said.posted))
	}
}

// --- fixtures ---------------------------------------------------------------

func at() time.Time { return time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC) }

func held(id string) simulate.Report {
	return simulate.Report{Cases: []simulate.Case{{ID: id}}}
}

func broke(id string) simulate.Report {
	return simulate.Report{Cases: []simulate.Case{{
		ID: id, Unmet: []domain.Expectation{{Kind: domain.ExpectCalls, Value: "crm.lookup"}},
	}}}
}

type watched map[domain.AgentID]domain.WatchedCorpus

func corpusOf(agent domain.AgentID) watched {
	return watched{agent: {
		Agent: agent, Version: "v2",
		Scope: domain.Scope{Company: "acme", Area: "cx"},
	}}
}

func (w watched) Watching(context.Context) ([]domain.WatchedCorpus, error) {
	out := make([]domain.WatchedCorpus, 0, len(w))
	for _, one := range w {
		out = append(out, one)
	}
	return out, nil
}

type batteries map[string][]string

func (b batteries) Batteries(
	_ context.Context, agent domain.AgentID, version domain.VersionID, limit int,
) ([]string, error) {
	found := b[string(agent)+"@"+string(version)]
	if len(found) > limit {
		found = found[:limit]
	}
	return found, nil
}

type reports map[string]simulate.Report

func (r reports) Fold(_ context.Context, simulation string) (simulate.Report, error) {
	return r[simulation], nil
}

type spy struct{ posted []channel.Message }

func (s *spy) Announce(_ context.Context, _ domain.Scope, m channel.Message) error {
	s.posted = append(s.posted, m)
	return nil
}

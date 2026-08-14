package httpapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/simulate"
)

/*
Starting an agent whose corpus is broken.

The gate is here and not on publishing, which is where I first put it and where
it cannot work: a version's identifier is the digest of its own bytes, so it
does not exist until it is published and nothing can have been simulated
against it. Publishing writes a definition. Starting is what makes it act, and
it is the act worth stopping.

What it refuses is narrow and deliberate. Only a case that ran against this
exact version and broke: an agent nobody has simulated is not refused here —
FU-10 covers a first publication and demanding a fresh battery for every
correction to an instruction would make the corpus something people route
around. A corpus that says nothing about this version says nothing, and this
gate answers only what it was told.

And it is binary, per NT-006 §2.1. These are structural assertions — this tool,
that ending, never this call — and an agent that reached a financial tool in
one case out of twenty is not ninety-five per cent good. Non-determinism is an
argument for running the case again, not for softening the criterion.
*/

// LastBattery finds the most recent battery run against one version, declared
// here by the consumer.
type LastBattery interface {
	Latest(ctx context.Context, agent domain.AgentID, version domain.VersionID) (string, bool, error)
}

// WithBatteries wires where a version's last battery is looked up.
func (s *Server) WithBatteries(batteries LastBattery) *Server {
	s.batteries = batteries
	return s
}

// brokenCases answers which corrections this version stopped holding, or
// nothing at all when there is nothing to say.
func (s *Server) brokenCases(
	ctx context.Context, agent domain.AgentID, version domain.VersionID,
) ([]string, error) {
	if s.regressions == nil || s.batteries == nil || s.store == nil {
		return nil, nil
	}

	simulation, found, err := s.batteries.Latest(ctx, agent, version)
	if err != nil || !found {
		// Nothing has been run against this version. That is not a failure and
		// is not treated as one: see the note above.
		return nil, err
	}

	corpus, err := s.regressions.List(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("read the corpus of %s: %w", agent, err)
	}
	if len(corpus) == 0 {
		return nil, nil
	}

	report, err := simulate.Gather(ctx, s.store, simulation)
	if err != nil {
		return nil, fmt.Errorf("read simulation %s: %w", simulation, err)
	}

	var broken []string
	for _, c := range simulate.Battery(report, corpus).Cases {
		if len(c.Unmet) > 0 {
			broken = append(broken, c.ID)
		}
	}
	return broken, nil
}

// refusedForCorpus renders the refusal, naming the cases.
//
// Named rather than counted: "three corrections stopped holding" sends
// somebody to go and find out which, and the whole reason a correction is a
// case with an identifier is so it can be pointed at.
func refusedForCorpus(agent domain.AgentID, broken []string) string {
	return fmt.Sprintf(
		"%s cannot be started: %d correction(s) it used to hold no longer hold — %s. "+
			"Correct the agent or the expectation, run the battery again, then start it.",
		agent, len(broken), strings.Join(broken, ", "))
}

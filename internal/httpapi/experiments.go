package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/simulate"
)

/*
StartExperiment opens one simulated run per case of a dataset version, once per
repetition, on one agent version under one configuration (NT-015 § Experiments).

The same authority as a simulation, because it spends the same real money. The same
fence an erasure takes around reading the corpus and opening its runs. And never a
battery: every run names the experiment, which every door that reads batteries leaves
out.
*/
func (s *Server) StartExperiment(
	ctx context.Context, req openapi.StartExperimentRequestObject,
) (openapi.StartExperimentResponseObject, error) {
	if s.regressions == nil {
		return experimentAbsent(req.AgentId), nil
	}
	agent := domain.AgentID(req.AgentId)
	id := experimentID(agent, req.Params.IdempotencyKey)
	ask, refused := askOf(req.Body)
	if errors.Is(refused, errVersionAndSeal) {
		return openapi.StartExperiment400ApplicationProblemPlusJSONResponse(invalid(refused.Error())), nil
	}
	target, refusal, err := s.experimentTarget(ctx, agent, id, ask)
	if err != nil || refusal != nil {
		return refusal, err
	}
	if refused != nil {
		return openapi.StartExperiment400ApplicationProblemPlusJSONResponse(invalid(refused.Error())), nil
	}
	out, err := s.openExperiment(ctx, target, id, ask)
	switch {
	case err != nil:
		return nil, fmt.Errorf("open experiment %s: %w", id, err)
	case out.unreadable:
		return candidateUnreadable(id), nil
	case out.hidden:
		return experimentAbsent(id), nil
	case out.conflict != nil:
		return openapi.StartExperiment409ApplicationProblemPlusJSONResponse(conflicted(out.conflict.Error())), nil
	case out.refused != nil:
		return openapi.StartExperiment400ApplicationProblemPlusJSONResponse(invalid(out.refused.Error())), nil
	case len(out.opened.Runs) == 0:
		return openapi.StartExperiment409ApplicationProblemPlusJSONResponse(
			conflicted(firstOr(out.opened.Failed, "no case opened a run"))), nil
	}
	return openapi.StartExperiment202JSONResponse{
		Id: id, Cases: out.cases, Repetitions: out.intent.Repetitions,
		Runs: len(out.opened.Runs), DatasetVersion: out.intent.DatasetVersion,
	}, nil
}

// experimentOpened is what opening an experiment came to, refusals included.
type experimentOpened struct {
	intent simulate.Intention
	cases  int
	opened simulate.Opened
	// conflict is a retry stating another intention than its key's; refused is an
	// intention nobody could run; hidden is one whose runs live where the caller may not
	// publish, answered as if it did not exist.
	conflict, refused error
	hidden            bool
	// unreadable is a first attempt on a candidate whose seal cannot be read.
	unreadable bool
}

/*
openExperiment settles the intention and opens its runs, inside the corpus fence taken
alone.

Alone, and not shared as a battery takes it, because the intention is read and then acted
on: two first attempts under one key, with different bodies, would otherwise both find
nothing recorded and open their runs into one experiment. Taken alone, the second reads
what the first recorded. The fence still keeps an erasure out while the corpus is copied.
*/
func (s *Server) openExperiment(
	ctx context.Context, target experimentTarget, id string, ask experimentAsk,
) (out experimentOpened, err error) {
	err = s.regressions.Exclusive(ctx, []domain.AgentID{target.summary.ID}, func(ctx context.Context) error {
		target, err := s.withSealedDefinition(ctx, target)
		if err != nil {
			return err
		}
		published := target.summary
		before, found, err := simulate.Recorded(ctx, s.store, id)
		if err != nil {
			return err
		}
		// Where it was recorded first, and only then what it asked for: a conflict answered
		// to a caller with no claim there would confirm the experiment, and name its fields.
		if found {
			if out.hidden, err = s.hiddenFrom(ctx, published, before.Intention, &before); err != nil || out.hidden {
				return err
			}
		}
		if out.intent, out.conflict = settle(ask, before.Intention, found, published.VersionID); out.conflict != nil {
			return nil
		}
		if !found {
			if out.hidden, err = s.hiddenFrom(ctx, published, out.intent, nil); err != nil || out.hidden {
				return err
			}
		}
		return s.openSettled(ctx, target, experimentBatch{published, id, &out}, found)
	})
	return out, err
}

// openSettled opens what the settled intention still lacks, and answers from the record.
func (s *Server) openSettled(ctx context.Context, target experimentTarget, b experimentBatch, found bool) error {
	// A candidate that cannot be read opens nothing: a first attempt is refused, and a
	// retry is answered from the runs it holds, never moved to another seal.
	if target.unreadable {
		if !found {
			b.out.unreadable = true
			return nil
		}
		return s.answerFromRecord(ctx, b.id, b.out)
	}
	refused, err := s.addMissing(ctx, b)
	if err != nil {
		return err
	}
	// Refused only before anything was opened. After, it decides whether a retry can
	// still add what the first attempt left unopened, never whether it happened.
	if !found && refused != nil {
		b.out.refused = refused
		return nil
	}
	return s.answerFromRecord(ctx, b.id, b.out)
}

// experimentBatch is one experiment being opened: its agent, its id and its result so far.
type experimentBatch struct {
	published domain.AgentSummary
	id        string
	out       *experimentOpened
}

// addMissing opens the runs of the settled intention that are not open yet, unless its
// pair cannot be asked or its cases cannot be read, which is the refusal it returns.
func (s *Server) addMissing(ctx context.Context, b experimentBatch) (refused error, err error) {
	if refused, err = s.unservedPair(ctx, b.published, b.out.intent); err != nil || refused != nil {
		return refused, err
	}
	corpus, readErr := s.corpusOccurrences(ctx, b.published.ID, b.out.intent.DatasetVersion)
	if readErr != nil {
		return readErr, nil
	}
	b.out.opened, err = simulate.Open(ctx, s.opener(), simulate.Batch{
		ID: b.id, Agent: b.published.ID, By: callerOf(ctx),
		DatasetVersion: corpus.Version, Cases: corpus.Cases, Experiment: &b.out.intent.Experiment,
	})
	return nil, err
}

// answerFromRecord reports the experiment as the ledger holds it once this attempt is done,
// so the answer does not shrink with a corpus erased or a provider removed since.
func (s *Server) answerFromRecord(ctx context.Context, id string, out *experimentOpened) error {
	after, found, err := simulate.Recorded(ctx, s.store, id)
	if err != nil || !found {
		return err
	}
	out.intent, out.opened.Runs, out.cases = after.Intention, after.Runs, after.Cases
	return nil
}

// experimentID names an experiment after its agent and the intention alone, so asking
// again with the same key reaches the runs it already opened.
func experimentID(agent domain.AgentID, key string) string {
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("exp_%s_%s", agent, hex.EncodeToString(sum[:8]))
}

package channel_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

/*
Telling a decline apart from a failure, against the real sentinels.

The consumer's own tests use a fake that returns `ErrWontStart` directly, which
proves the consumer handles it and proves nothing about whether anything ever
produces it. This is the half that was missing: the four states the trigger
actually refuses with, and one that is not a state at all.
*/

func TestFromTrigger_theAgentDeclined_isARefusal(t *testing.T) {
	t.Parallel()

	for _, declined := range []error{
		trigger.ErrPaused, trigger.ErrStopped, trigger.ErrDraft, trigger.ErrUnknownAgent,
	} {
		opener := channel.FromTrigger(openerRefusing(t, declined))

		_, err := opener.Open(t.Context(), channel.Request{
			Agent: "triage", IdemKey: "k", Trigger: "channel",
		})
		if !errors.Is(err, channel.ErrWontStart) {
			t.Errorf("%v -> %v, want a refusal", declined, err)
		}
		// The sentence the trigger wrote is what the person is told, and it
		// already says which of the four it was.
		if !errors.Is(err, declined) {
			t.Errorf("%v was replaced rather than wrapped: %v", declined, err)
		}
	}
}

// A ledger that was away is not a decline, and an ask closed because of it
// would answer a good question with a refusal that was never about it.
func TestFromTrigger_theLedgerFailed_isNotARefusal(t *testing.T) {
	t.Parallel()

	opener := channel.FromTrigger(trigger.NewOpener(
		brokenLedger{}, publishedAgent{}, atNoon{}))

	_, err := opener.Open(t.Context(), channel.Request{
		Agent: "triage", IdemKey: "k", Trigger: "channel",
	})
	if err == nil {
		t.Fatal("a broken ledger opened a run")
	}
	if errors.Is(err, channel.ErrWontStart) {
		t.Errorf("a failure was reported as a refusal: %v", err)
	}
}

// --- the harness ------------------------------------------------------------

// openerRefusing builds an opener that will decline for the reason given.
func openerRefusing(t *testing.T, why error) *trigger.Opener {
	t.Helper()
	store := ledger.NewMemory()

	switch {
	case errors.Is(why, trigger.ErrUnknownAgent):
		return trigger.NewOpener(store, noAgents{}, atNoon{})
	case errors.Is(why, trigger.ErrPaused):
		return trigger.NewOpener(store, publishedAgent{}, atNoon{}).WithPauses(pausedAlways{})
	case errors.Is(why, trigger.ErrStopped):
		return trigger.NewOpener(store, publishedAgent{}, atNoon{}).WithStops(stoppedAlways{})
	default:
		return trigger.NewOpener(store, publishedAgent{}, atNoon{}).WithStages(draftAlways{})
	}
}

type atNoon struct{}

func (atNoon) Now() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) }

type publishedAgent struct{}

func (publishedAgent) Versions(context.Context, domain.AgentID) ([]domain.AgentSummary, error) {
	return []domain.AgentSummary{{
		ID: "triage", VersionID: "v1", Latest: true,
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}}, nil
}

type noAgents struct{}

func (noAgents) Versions(context.Context, domain.AgentID) ([]domain.AgentSummary, error) {
	return nil, nil
}

type pausedAlways struct{}

func (pausedAlways) IsPaused(context.Context, domain.AgentID) (bool, error) { return true, nil }

type stoppedAlways struct{}

func (stoppedAlways) InForce(context.Context) ([]domain.Stop, error) {
	return []domain.Stop{{
		Level: domain.StopInstallation, Reason: "incident",
	}}, nil
}

type draftAlways struct{}

func (draftAlways) StageOf(context.Context, domain.AgentID) (domain.Stage, error) {
	return domain.StageDraft, nil
}

type brokenLedger struct{ *ledger.Memory }

func (brokenLedger) Append(context.Context, domain.Step) (domain.Step, error) {
	return domain.Step{}, errors.New("the ledger is away")
}

func (brokenLedger) Read(context.Context, domain.RunID, int64) ([]domain.Step, error) {
	return nil, nil
}

// A nil error means the key names a run, so "nothing found" has to be an
// error. A stub that answered otherwise would make the opener return an empty
// run and no failure, which is how this test passed while proving nothing.
func (brokenLedger) RunByIdemKey(context.Context, string) (domain.RunID, error) {
	return "", errors.New("no run by that key")
}

package domain_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
The three steps of a live rehearsal (NT-018 phase 4), known to every reader one release
before anything writes them: the run waiting for the author to answer for the world, the
author's answer, and the author's decision where the Gate held an action. None of them
holds free text: the answer lives in the content store, named by reference and digest.
*/
func TestStepKinds_listTheRehearsalSteps(t *testing.T) {
	t.Parallel()
	for _, k := range []domain.StepKind{domain.StepWorldAwaited, domain.StepWorldSupplied, domain.StepRehearsalDecided} {
		if !k.Valid() || !slices.Contains(domain.StepKinds(), k) {
			t.Errorf("%s is not a kind the ledger knows", k)
		}
	}
}

func TestRehearsalPayloads_refuseWhatTheyMustNameAndDoNot(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		payload interface{ Validate() error }
		ok      bool
	}{
		"awaited":                {domain.WorldAwaitedPayload{Tool: "crm.lookup", CallSeq: 4}, true},
		"awaited, no tool":       {domain.WorldAwaitedPayload{CallSeq: 4}, false},
		"awaited, no call":       {domain.WorldAwaitedPayload{Tool: "crm.lookup"}, false},
		"supplied":               {domain.WorldSuppliedPayload{AtSeq: 5, Ref: "run://r/6/abc", Digest: "abc", By: "usr_ana"}, true},
		"supplied, no request":   {domain.WorldSuppliedPayload{Ref: "run://r/6/abc", Digest: "abc", By: "usr_ana"}, false},
		"supplied, no reference": {domain.WorldSuppliedPayload{AtSeq: 5, Digest: "abc", By: "usr_ana"}, false},
		"supplied, no digest":    {domain.WorldSuppliedPayload{AtSeq: 5, Ref: "run://r/6/abc", By: "usr_ana"}, false},
		"supplied, nobody":       {domain.WorldSuppliedPayload{AtSeq: 5, Ref: "run://r/6/abc", Digest: "abc"}, false},
		"decided":                {domain.RehearsalDecidedPayload{AtSeq: 7, Approved: true, By: "usr_ana"}, true},
		"decided, no request":    {domain.RehearsalDecidedPayload{Approved: true, By: "usr_ana"}, false},
		"decided, nobody":        {domain.RehearsalDecidedPayload{AtSeq: 7}, false},
	} {
		err := tc.payload.Validate()
		if tc.ok != (err == nil) {
			t.Errorf("%s: Validate = %v, want ok %v", name, err, tc.ok)
		}
		if err != nil && !errors.Is(err, domain.ErrRehearsalStepInvalid) {
			t.Errorf("%s: Validate = %v, want ErrRehearsalStepInvalid", name, err)
		}
	}
}

package domain_test

import (
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
How an agent's owner asked to be told is part of the agent.

The Gate decides whether a person must answer; this decides where they are
asked. Only the second is the owner's, which is why it is a specification field
and not a grant.
*/
func TestApprovalPolicy_namesWithoutAsking_isRefused(t *testing.T) {
	t.Parallel()

	// Configured and doing nothing is the shape worth refusing: whoever wrote
	// it believed they had asked for something.
	err := domain.ApprovalPolicy{Notify: []domain.UserID{"usr_ana"}}.Validate()
	if !errors.Is(err, domain.ErrApprovalNotifyWithoutDirect) {
		t.Fatalf("err = %v, want ErrApprovalNotifyWithoutDirect", err)
	}

	if err := (domain.ApprovalPolicy{Direct: true, Notify: []domain.UserID{"usr_ana"}}).Validate(); err != nil {
		t.Errorf("naming somebody while asking for a message: %v", err)
	}
	// The ordinary shape: tell whoever may decide.
	if err := (domain.ApprovalPolicy{Direct: true}).Validate(); err != nil {
		t.Errorf("asking for a message and naming nobody: %v", err)
	}
}

func TestApprovalPolicy_aListLongerThanAnybodyReads_isRefused(t *testing.T) {
	t.Parallel()

	long := domain.ApprovalPolicy{Direct: true}
	for i := range domain.MaxApprovalNotify + 1 {
		long.Notify = append(long.Notify, domain.UserID(string(rune('a'+i))))
	}
	if err := long.Validate(); !errors.Is(err, domain.ErrApprovalNotifyTooMany) {
		t.Fatalf("err = %v, want ErrApprovalNotifyTooMany", err)
	}
}

// A list written with stray spaces or the same person twice means what it looks
// like. Kept in the author's order: the file is read by people.
func TestApprovalPolicy_normalize_keepsOneOfEachInTheOrderWritten(t *testing.T) {
	t.Parallel()

	got := domain.ApprovalPolicy{
		Direct: true,
		Notify: []domain.UserID{" usr_ana ", "", "usr_bob", "usr_ana"},
	}.Normalize()

	want := []domain.UserID{"usr_ana", "usr_bob"}
	if len(got.Notify) != len(want) {
		t.Fatalf("notify = %v, want %v", got.Notify, want)
	}
	for i := range want {
		if got.Notify[i] != want[i] {
			t.Fatalf("notify = %v, want %v", got.Notify, want)
		}
	}
}

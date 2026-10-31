package guard

import (
	"context"
	"errors"
	"testing"
)

// tokenFailsAtMerge hands out the Merger's token for every read and fails the
// moment a token is asked for after the guard's last read: the merge call's.
type tokenFailsAtMerge struct{ h *harness }

func (s tokenFailsAtMerge) Token(context.Context) (string, error) {
	s.h.fake.mu.Lock()
	defer s.h.fake.mu.Unlock()
	if s.h.fake.compared {
		return "", errors.New("token refresh failed")
	}
	return "merger-token", nil
}

// A merge that never left the process is known not to have happened. It is a
// retryable refusal, not an outcome somebody has to reconcile by hand.
func TestMergeIfReady_failureBeforeTheMergeIsSent_notSentAndNotReconciled(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	gh := NewClient(ClientOptions{
		APIURL: h.cfg.APIURL, Repo: h.cfg.Repo, HTTP: h.srv.Client(), Tokens: tokenFailsAtMerge{h: h},
	})
	res, err := NewMerger(h.cfg, gh).MergeIfReady(t.Context(), MergeInput{PR: testPR, HeadSHA: headA})
	if err != nil {
		t.Fatalf("merge_if_ready returned a bare error: %v", err)
	}
	if !h.fake.compared {
		t.Fatal("the token failed before the reads finished: the scenario did not reach the merge")
	}
	if len(h.fake.attempts) != 0 {
		t.Fatalf("merge endpoint called %d times, want none", len(h.fake.attempts))
	}
	if res.Merged || res.Reason != ReasonMergeNotSent {
		t.Fatalf("result = %+v, want merge_not_sent (not outcome_unknown)", res)
	}
}

package ledger_test

import (
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
One approval proves one link at most (#239, review round 1, P2).

The decision approved a request whose arguments name one pull request, and the
store binds the proof to it: the same link citing it again is the idempotent
same link, and a link of any other pull request citing it is refused.
*/
func TestWorkItemLink_oneApproval_provesItsOwnAliasOnly(t *testing.T) {
	run(t, "an approval is bound to its alias", func(t *testing.T, s Store) {
		openClaimed(t, s, "run-a", "developer", card)
		approved := approvalProofOf(t, s, domain.ToolWorkItemLink, linkAlias, true)

		if err := linkWith(t, s, "run-a", "developer", approved); err != nil {
			t.Fatalf("the approved link = %v, want it sealed", err)
		}
		if err := linkWith(t, s, "run-a", "developer", approved); err != nil {
			t.Fatalf("the same approved link again = %v, want the idempotent success", err)
		}
		reused := approved
		reused.Alias = otherAlias
		if err := linkWith(t, s, "run-a", "developer", reused); !errors.Is(err, domain.ErrWorkItemLinkUnproven) {
			t.Fatalf("another pull request citing the same decision = %v, want ErrWorkItemLinkUnproven", err)
		}
	})
}

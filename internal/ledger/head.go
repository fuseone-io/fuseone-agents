package ledger

import (
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// headIs checks a caller's precondition against the head it actually found.
//
// One comparison, written once, so the fake and the store cannot answer the
// question differently — which is the whole reason the fake exists.
func headIs(expect *domain.StepRef, head *domain.Step) error {
	if expect == nil {
		return nil
	}
	if head == nil {
		return fmt.Errorf("%w: the run has no steps", domain.ErrHeadMoved)
	}
	if head.Seq != expect.Seq || head.Kind != expect.Kind {
		return fmt.Errorf("%w: head is %s at %d, expected %s at %d",
			domain.ErrHeadMoved, head.Kind, head.Seq, expect.Kind, expect.Seq)
	}
	return nil
}

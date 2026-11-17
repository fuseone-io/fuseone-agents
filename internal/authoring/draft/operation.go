package draft

import (
	"errors"
	"fmt"
)

// Operation is one typed change to one field of a draft. The model proposes
// them in the conversation; the controls apply the same ones by hand.
type Operation interface {
	// Kind is the operation's name on the wire and in the trail.
	Kind() string
	// Class is who may apply it, the class of the fields it edits.
	Class() Class
	// Fields are the AgentDefinition fields it edits, and no others.
	Fields() []string
	apply(d *Draft) error
}

// Origin is where an operation comes from.
type Origin int

const (
	// FromControl is a person applying it by hand, through the public route.
	FromControl Origin = iota
	// FromProposal is a proposal of the model that a person accepted.
	FromProposal
)

var (
	// ErrHumanOnly is a proposal of the model carrying an operation only a
	// person may apply: it is shown as invalid and cannot be accepted.
	ErrHumanOnly = errors.New("draft: only a person may apply this operation")
	// ErrConversationOnly is a block operation applied by hand: the
	// instructions change only through an accepted proposal (contract 3).
	ErrConversationOnly = errors.New("draft: the instructions change only through an accepted proposal")
	// ErrInvalidOperation is an operation on something the draft does not
	// have, or that it already has.
	ErrInvalidOperation = errors.New("draft: invalid operation")
)

// Operations is one of each operation, for the parity tests and the wire's
// list of kinds.
func Operations() []Operation {
	return []Operation{
		SetIdentity{}, SetStandingApprovals{},
		SetModel{}, SetFallback{}, SetCachePrefixTTL{}, SetReplyLocale{},
		AddTool{}, RemoveTool{}, SetBudget{}, SetTriggers{},
		SetMemoryLearning{}, SetApprovalDelivery{}, SetEmits{},
		AddStep{}, EditStep{}, RemoveStep{},
		InsertBlock{}, ReplaceBlock{}, RemoveBlock{}, MoveBlock{},
	}
}

// admitted answers whether an operation may come from where it came.
func admitted(op Operation, origin Origin) error {
	switch {
	case origin == FromProposal && op.Class() == Human:
		return fmt.Errorf("%w: %s", ErrHumanOnly, op.Kind())
	case origin == FromControl && op.Class() == Conversation:
		return fmt.Errorf("%w: %s", ErrConversationOnly, op.Kind())
	}
	return nil
}

func invalidOp(kind, format string, args ...any) error {
	return fmt.Errorf("%w: %s: %s", ErrInvalidOperation, kind, fmt.Sprintf(format, args...))
}

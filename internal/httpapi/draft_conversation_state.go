package httpapi

import (
	"context"

	"github.com/fuseone/agents/internal/httpapi/openapi"
)

type conversationState = openapi.ListDraftTurns200JSONResponseBodyConversation

/*
conversationState is whether a turn could be sent now, judged as a turn is:
the installation's assistant on, its model priced, and something left of the
day's ceiling. Open is not a promise, since a turn's reservation may still not
fit what is left; closed is certain, and the console closes the conversation
before the person writes into it.
*/
func (s *Server) conversationState(ctx context.Context) (conversationState, error) {
	if s.turnModels == nil || s.authoring == nil {
		return openapi.ListDraftTurns200JSONResponseBodyConversationOff, nil
	}
	choice, err := s.authoring.Current(ctx)
	if err != nil {
		return "", err
	}
	if !choice.Enabled || choice.DailyMicros <= 0 {
		return openapi.ListDraftTurns200JSONResponseBodyConversationOff, nil
	}
	// A provider no longer registered cannot be asked: off, as the send would
	// find it.
	if _, priced, err := s.turnModels.PriceFor(choice.Provider, choice.Model); err != nil {
		return openapi.ListDraftTurns200JSONResponseBodyConversationOff, nil
	} else if !priced {
		return openapi.ListDraftTurns200JSONResponseBodyConversationUnpriced, nil
	}
	if s.spend == nil {
		return openapi.ListDraftTurns200JSONResponseBodyConversationOpen, nil
	}
	spent, err := s.spend.SpentToday(ctx)
	if err != nil {
		return "", err
	}
	if spent >= choice.DailyMicros {
		return openapi.ListDraftTurns200JSONResponseBodyConversationSpent, nil
	}
	return openapi.ListDraftTurns200JSONResponseBodyConversationOpen, nil
}

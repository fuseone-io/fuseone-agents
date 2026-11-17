package httpapi

import (
	"context"

	"github.com/fuseone/agents/internal/httpapi/openapi"
)

type conversationState = openapi.ListDraftTurns200JSONResponseBodyConversation

// conversationStatus is whether a turn could be sent now and, while the assistant
// is on, what the day holds against its ceiling.
type conversationStatus struct {
	state        conversationState
	spent, daily *int64
}

/*
conversationNow is whether a turn could be sent now, judged as a turn is: the
installation's assistant on, its model priced, and something left of the
day's ceiling. Open is not a promise, since a turn's reservation may still not
fit what is left; closed is certain, and the console closes the conversation
before the person writes into it. The day's figures come with it while the
assistant is on, from the same reading a turn's reservation is judged by.
*/
func (s *Server) conversationNow(ctx context.Context) (conversationStatus, error) {
	off := conversationStatus{state: openapi.ListDraftTurns200JSONResponseBodyConversationOff}
	if s.turnModels == nil || s.authoring == nil {
		return off, nil
	}
	choice, err := s.authoring.Current(ctx)
	if err != nil {
		return conversationStatus{}, err
	}
	if !choice.Enabled || choice.DailyMicros <= 0 {
		return off, nil
	}
	out := conversationStatus{state: openapi.ListDraftTurns200JSONResponseBodyConversationOpen, daily: ptr(choice.DailyMicros)}
	if s.spend != nil {
		spent, err := s.spend.SpentToday(ctx)
		if err != nil {
			return conversationStatus{}, err
		}
		out.spent = ptr(spent)
		if spent >= choice.DailyMicros {
			out.state = openapi.ListDraftTurns200JSONResponseBodyConversationSpent
		}
	}
	// A provider no longer registered cannot be asked: off, as the send would
	// find it.
	if _, priced, err := s.turnModels.PriceFor(choice.Provider, choice.Model); err != nil {
		return off, nil
	} else if !priced {
		out.state = openapi.ListDraftTurns200JSONResponseBodyConversationUnpriced
	}
	return out, nil
}

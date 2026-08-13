package httpapi

import (
	"context"
	"errors"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/authoring"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/model"
)

// Assistants builds a completer for the provider Administração points at.
// Declared here by the consumer, so the httpapi does not depend on how a
// provider is reached.
type Assistants interface {
	Completer(provider string, cfg model.Config) (model.Completer, error)
}

// Spend is the authoring ledger: what the day has cost, and appending to it.
type Spend interface {
	SpentToday(ctx context.Context) (int64, error)
	RecordSpend(ctx context.Context, cost domain.Cost, by domain.UserID) error
}

// WithAssistants wires the interview.
func (s *Server) WithAssistants(a Assistants, spend Spend) *Server {
	s.assistants = a
	s.spend = spend
	return s
}

// InterviewAgent turns the interview's answers into the half of a draft that
// needs translating.
func (s *Server) InterviewAgent(
	ctx context.Context, req openapi.InterviewAgentRequestObject,
) (openapi.InterviewAgentResponseObject, error) {
	if len(auth.VisibleScopes(ctx, domain.PermAgentPublish)) == 0 {
		return openapi.InterviewAgent403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish, domain.Scope{}),
		}, nil
	}
	if s.assistants == nil || s.spend == nil || s.authoring == nil || req.Body == nil {
		return nil, errNoAdministration
	}

	choice, err := s.authoring.Current(ctx)
	if err != nil {
		return nil, err
	}
	// Asked before a provider is looked up. A fresh installation has no
	// assistant, and answering that with "no provider named \"\"" describes
	// the plumbing instead of the state somebody is actually in.
	if !choice.Enabled {
		return assistantUnavailable(authoring.ErrDisabled), nil
	}

	spentToday, err := s.spend.SpentToday(ctx)
	if err != nil {
		return nil, err
	}
	completer, err := s.assistants.Completer(choice.Provider, model.Config{
		Model: choice.Model, Effort: choice.Effort,
	})
	if err != nil {
		return assistantUnavailable(err), nil
	}

	result, err := authoring.Translate(ctx, authoring.Job{
		Completer:  completer,
		Choice:     choice,
		SpentToday: spentToday,
		Answers:    answersFrom(req.Body),
		Catalogue:  s.catalogue(ctx),
	})

	// Recorded before the error is answered. The money left the installation
	// whether or not the answer was usable, and a spend that only reaches the
	// trail on success is a ceiling that drifts in the direction that costs.
	if result.Cost.Micros > 0 {
		if err := s.spend.RecordSpend(ctx, result.Cost, callerOf(ctx)); err != nil {
			return nil, err
		}
	}
	if err != nil {
		return assistantUnavailable(err), nil
	}

	return openapi.InterviewAgent200JSONResponse(draftFrom(result)), nil
}

// assistantUnavailable answers the two configuration states and the provider's
// own failures. Being switched off or out of budget is not a fault: the form
// remains the way to publish an agent without the assistant.
func assistantUnavailable(err error) openapi.InterviewAgentResponseObject {
	if errors.Is(err, authoring.ErrDisabled) || errors.Is(err, authoring.ErrOverCeiling) {
		return openapi.InterviewAgent409ApplicationProblemPlusJSONResponse(
			conflicted(err.Error()))
	}
	return openapi.InterviewAgent400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			upstreamRefused(err.Error())),
	}
}

// catalogue is what the assistant may name. Read here rather than passed in,
// so the boundary is the tools this installation actually connected.
func (s *Server) catalogue(ctx context.Context) []domain.ToolID {
	if s.tools == nil {
		return nil
	}
	known, err := s.tools.Tools(ctx)
	if err != nil {
		return nil
	}
	out := make([]domain.ToolID, 0, len(known))
	for _, t := range known {
		out = append(out, t.ID)
	}
	return out
}

func answersFrom(in *openapi.InterviewAgentJSONRequestBody) authoring.Answers {
	return authoring.Answers{
		MustKnow:  valueOr(in.MustKnow),
		Steps:     valueOr(in.Steps),
		GoesWrong: valueOr(in.GoesWrong),
		NotDecide: valueOr(in.NotDecide),
	}
}

func draftFrom(r authoring.Result) openapi.InterviewDraft {
	tools := make([]string, 0, len(r.Translated.Tools))
	for _, t := range r.Translated.Tools {
		tools = append(tools, string(t))
	}
	steps := make([]openapi.AgentStep, 0, len(r.Translated.Steps))
	for _, step := range r.Translated.Steps {
		reaches := make([]string, 0, len(step.Reaches))
		for _, t := range step.Reaches {
			reaches = append(reaches, string(t))
		}
		steps = append(steps, openapi.AgentStep{
			Name: step.Name, Reaches: &reaches, StopsWhen: ptr(step.StopsWhen),
		})
	}
	return openapi.InterviewDraft{Tools: tools, Steps: steps, Micros: ptr(r.Cost.Micros)}
}

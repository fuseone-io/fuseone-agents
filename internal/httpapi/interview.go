package httpapi

import (
	"context"
	"errors"
	"strings"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/authoring"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/model"
)

const maxInterviewCaptureBytes = 12_000

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

	assistant, err := s.authoringAssistant(ctx, req.Params.Locale)
	if err != nil {
		if unavailable, ok := asAuthoringUnavailable(err); ok {
			return assistantUnavailable(unavailable), nil
		}
		return nil, err
	}

	result, err := authoring.Translate(ctx, authoring.Job{
		Locale:     assistant.locale,
		Completer:  assistant.completer,
		Choice:     assistant.choice,
		SpentToday: assistant.spentToday,
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

// SuggestInterviewAnswers turns one free description into the seven fixed
// interview fields. It does not generate a draft; the author still reads and
// edits the fields before the translation endpoint may run.
func (s *Server) SuggestInterviewAnswers(
	ctx context.Context, req openapi.SuggestInterviewAnswersRequestObject,
) (openapi.SuggestInterviewAnswersResponseObject, error) {
	if len(auth.VisibleScopes(ctx, domain.PermAgentPublish)) == 0 {
		return openapi.SuggestInterviewAnswers403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish, domain.Scope{}),
		}, nil
	}
	if s.assistants == nil || s.spend == nil || s.authoring == nil || req.Body == nil {
		return nil, errNoAdministration
	}
	text := strings.TrimSpace(req.Body.Text)
	switch {
	case text == "":
		return openapi.SuggestInterviewAnswers400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid("authoring: describe the work before asking for suggestions")),
		}, nil
	case len(req.Body.Text) > maxInterviewCaptureBytes:
		return openapi.SuggestInterviewAnswers400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid("authoring: the description is too large")),
		}, nil
	}

	assistant, err := s.authoringAssistant(ctx, req.Params.Locale)
	if err != nil {
		if unavailable, ok := asAuthoringUnavailable(err); ok {
			return suggestionUnavailable(unavailable), nil
		}
		return nil, err
	}
	result, err := authoring.SuggestAnswers(ctx, authoring.SuggestionJob{
		Locale:     assistant.locale,
		Completer:  assistant.completer,
		Choice:     assistant.choice,
		SpentToday: assistant.spentToday,
		Text:       text,
	})
	if result.Cost.Micros > 0 {
		if err := s.spend.RecordSpend(ctx, result.Cost, callerOf(ctx)); err != nil {
			return nil, err
		}
	}
	if err != nil {
		return suggestionUnavailable(err), nil
	}
	return openapi.SuggestInterviewAnswers200JSONResponse(suggestionsFrom(result)), nil
}

type authoringAssistant struct {
	choice     authoring.Choice
	spentToday int64
	completer  model.Completer
	locale     string
}

type authoringUnavailable struct{ err error }

func (e authoringUnavailable) Error() string { return e.err.Error() }
func (e authoringUnavailable) Unwrap() error { return e.err }

func asAuthoringUnavailable(err error) (error, bool) {
	var unavailable authoringUnavailable
	if errors.As(err, &unavailable) {
		return unavailable.err, true
	}
	return nil, false
}

func (s *Server) authoringAssistant(ctx context.Context, locale *string) (authoringAssistant, error) {
	choice, err := s.authoring.Current(ctx)
	if err != nil {
		return authoringAssistant{}, err
	}
	// Asked before a provider is looked up. A fresh installation has no
	// assistant, and answering that with "no provider named \"\"" describes
	// the plumbing instead of the state somebody is actually in.
	if !choice.Enabled {
		return authoringAssistant{}, authoringUnavailable{authoring.ErrDisabled}
	}
	spentToday, err := s.spend.SpentToday(ctx)
	if err != nil {
		return authoringAssistant{}, err
	}
	completer, err := s.assistants.Completer(choice.Provider, model.Config{
		Model: choice.Model, Effort: authoring.EffortFor(choice.Effort),
	})
	if err != nil {
		return authoringAssistant{}, authoringUnavailable{err}
	}
	out := authoringAssistant{choice: choice, spentToday: spentToday, completer: completer}
	if locale != nil {
		out.locale = *locale
	}
	return out, nil
}

// assistantUnavailable answers the two configuration states and the provider's
// own failures. Being switched off or out of budget is not a fault: the form
// remains the way to publish an agent without the assistant.
func assistantUnavailable(err error) openapi.InterviewAgentResponseObject {
	if errors.Is(err, authoring.ErrDisabled) || errors.Is(err, authoring.ErrOverCeiling) {
		return openapi.InterviewAgent409ApplicationProblemPlusJSONResponse(
			conflicted(err.Error()))
	}
	if failure, ok := model.FailureSummaryOf(err); ok && failure.Retryable {
		return openapi.InterviewAgent503ApplicationProblemPlusJSONResponse(
			upstreamRefusedLater(err.Error()))
	}
	return openapi.InterviewAgent400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			upstreamRefused(err.Error())),
	}
}

func suggestionUnavailable(err error) openapi.SuggestInterviewAnswersResponseObject {
	if errors.Is(err, authoring.ErrDisabled) || errors.Is(err, authoring.ErrOverCeiling) {
		return openapi.SuggestInterviewAnswers409ApplicationProblemPlusJSONResponse(
			conflicted(err.Error()))
	}
	if failure, ok := model.FailureSummaryOf(err); ok && failure.Retryable {
		return openapi.SuggestInterviewAnswers503ApplicationProblemPlusJSONResponse(
			upstreamRefusedLater(err.Error()))
	}
	return openapi.SuggestInterviewAnswers400ApplicationProblemPlusJSONResponse{
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
		// Collected by the screen and never sent, though it changes what the
		// assistant should answer: a tool the author has just forbidden is a
		// tool they will have to take away again.
		NeverDo: valueOr(in.NeverDo),
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

func suggestionsFrom(r authoring.SuggestionResult) openapi.InterviewSuggestions {
	return openapi.InterviewSuggestions{
		Answers: openapi.InterviewSuggestedAnswers{
			Trigger:   r.Answers.Trigger,
			MustKnow:  r.Answers.MustKnow,
			Steps:     r.Answers.Steps,
			GoesWrong: r.Answers.GoesWrong,
			NotDecide: r.Answers.NotDecide,
			Closing:   r.Answers.Closing,
			NeverDo:   r.Answers.NeverDo,
		},
		Micros: ptr(r.Cost.Micros),
	}
}

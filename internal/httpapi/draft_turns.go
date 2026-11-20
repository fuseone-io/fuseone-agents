package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/authoring"
	"github.com/fuseone/agents/internal/authoring/conversation"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/model"
)

/*
The workbench's conversation, over HTTP (NT-018 phase 3, contracts 7 and 8).

A turn is run by conversation.Engine against the draft store. The route picks
the installation's authoring model, its configured price and the day's
ceiling, and what the draft's scope offers; it answers a turn that was
recorded, whatever the model said, and refuses before anything is reserved
what cannot be run.
*/

// TurnModels is the model registry as a turn needs it. Declared here.
type TurnModels interface {
	Completer(provider string, cfg model.Config) (model.Completer, error)
	PriceFor(provider, model string) (model.Prices, bool, error)
	Names() []string
	Configured(name string, at domain.Scope) bool
	// Models is what a provider serves, as configured, for a run at scope (#414).
	Models(name string, at domain.Scope) []string
}

// WithTurnModels wires the conversation's models.
func (s *Server) WithTurnModels(m TurnModels) *Server {
	s.turnModels = m
	return s
}

// SendDraftTurn runs one turn of the caller's conversation.
func (s *Server) SendDraftTurn(
	ctx context.Context, req openapi.SendDraftTurnRequestObject,
) (openapi.SendDraftTurnResponseObject, error) {
	if s.drafts == nil || req.Body == nil {
		return openapi.SendDraftTurn404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId)}, nil
	}
	agent := domain.AgentID(req.AgentId)
	turn := conversation.Request{
		Agent: agent, Key: req.Body.Key, Message: req.Body.Message, Pasted: valueOr(req.Body.Pasted),
		Locale: valueOr(req.Body.Locale), Caller: draftCaller(ctx), Now: clockOr(s.clock).Now(),
	}
	// A stored key is answered before the assistant, its price or the
	// catalogue are asked anything: what was stored does not depend on them.
	if replayed, found, err := (conversation.Engine{Store: s.drafts}).Replay(ctx, turn); err != nil || found {
		if found {
			return openapi.SendDraftTurn200JSONResponse(turnView(replayed)), nil
		}
		return turnRefused(req.AgentId, err)
	}
	engine, err := s.turnEngine(ctx, agent)
	if err != nil {
		return turnRefused(req.AgentId, err)
	}
	result, err := engine.Run(ctx, turn)
	// A turn that was recorded is answered, failed or not: the money left,
	// and the person reads what happened to their message.
	if err == nil || result.Turn.State == drafts.TurnFailed {
		return openapi.SendDraftTurn200JSONResponse(turnView(result)), nil
	}
	return turnRefused(req.AgentId, err)
}

// turnEngine is the engine for one turn on the agent's draft.
func (s *Server) turnEngine(ctx context.Context, agent domain.AgentID) (conversation.Engine, error) {
	if s.turnModels == nil || s.authoring == nil {
		return conversation.Engine{}, authoring.ErrDisabled
	}
	choice, err := s.authoring.Current(ctx)
	if err != nil {
		return conversation.Engine{}, err
	}
	if !choice.Enabled {
		return conversation.Engine{}, authoring.ErrDisabled
	}
	opened, err := s.drafts.Get(ctx, agent, draftCaller(ctx))
	if err != nil {
		return conversation.Engine{}, err
	}
	completer, err := s.turnModels.Completer(choice.Provider, model.Config{
		Model: choice.Model, Effort: authoring.EffortFor(choice.Effort), MaxTokens: conversation.MaxOutputTokens,
	})
	if err != nil {
		return conversation.Engine{}, errors.Join(authoring.ErrDisabled, err)
	}
	price, priced, err := s.turnModels.PriceFor(choice.Provider, choice.Model)
	if err != nil {
		return conversation.Engine{}, errors.Join(authoring.ErrDisabled, err)
	}
	offer, err := s.offerFor(ctx, domain.Scope{Company: opened.Draft.Spec.Company, Area: opened.Draft.Spec.Area})
	if err != nil {
		return conversation.Engine{}, err
	}
	return conversation.Engine{
		Store: s.drafts, Completer: completer, Price: price, Priced: priced, DailyMicros: choice.DailyMicros,
		Offer: offer, Validator: draft.Validator{Checker: s.checker()},
	}, nil
}

// offerFor is what a scope offers a turn: the tools on the surface there, and
// the providers configured there with the models each serves there.
func (s *Server) offerFor(ctx context.Context, scope domain.Scope) (conversation.Offer, error) {
	rulings, err := s.checker().Rulings(ctx, scope)
	if err != nil {
		return conversation.Offer{}, err
	}
	var offer conversation.Offer
	for _, entry := range rulings {
		if entry.OnSurface {
			offer.Tools = append(offer.Tools, entry)
		}
	}
	// The catalogue is a map: sorted, the same draft renders the same prompt.
	slices.SortFunc(offer.Tools, func(a, b domain.ToolEntry) int { return strings.Compare(string(a.ID), string(b.ID)) })
	for _, name := range s.turnModels.Names() {
		if s.turnModels.Configured(name, scope) {
			offer.Providers = append(offer.Providers, conversation.ProviderOffer{Name: name, Models: s.turnModels.Models(name, scope)})
		}
	}
	return offer, nil
}

// turnRefusals are the refusals a turn meets before its call, each with the
// status and the code the console chooses its words by.
var turnRefusals = []struct {
	err    error
	status int
	code   Code
}{
	{conversation.ErrEmptyMessage, http.StatusBadRequest, CodeInvalidInput},
	{conversation.ErrTurnTooLarge, http.StatusBadRequest, CodeTurnTooLarge},
	{authoring.ErrDisabled, http.StatusConflict, CodeConversationOff},
	{drafts.ErrNoCeiling, http.StatusConflict, CodeConversationOff},
	{conversation.ErrNoPrice, http.StatusConflict, CodeConversationUnpriced},
	{drafts.ErrOverCeiling, http.StatusConflict, CodeConversationSpent},
	{drafts.ErrTurnInFlight, http.StatusConflict, CodeTurnInFlight},
}

func turnRefused(agent string, err error) (openapi.SendDraftTurnResponseObject, error) {
	if stale, ok := staleFrom(err); ok {
		return openapi.SendDraftTurn412ApplicationProblemPlusJSONResponse{PreconditionFailedApplicationProblemPlusJSONResponse: stale}, nil
	}
	if errors.Is(err, drafts.ErrNotFound) {
		return openapi.SendDraftTurn404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(agent)}, nil
	}
	for _, r := range turnRefusals {
		if !errors.Is(err, r.err) {
			continue
		}
		problem := refusal(r.status, r.code, "Turn refused", err.Error())
		if r.status == http.StatusBadRequest {
			return openapi.SendDraftTurn400ApplicationProblemPlusJSONResponse{
				BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(problem),
			}, nil
		}
		return openapi.SendDraftTurn409ApplicationProblemPlusJSONResponse(problem), nil
	}
	return nil, err
}

// ListDraftTurns is the caller's own conversation.
func (s *Server) ListDraftTurns(
	ctx context.Context, req openapi.ListDraftTurnsRequestObject,
) (openapi.ListDraftTurnsResponseObject, error) {
	absent := openapi.ListDraftTurns404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId)}
	if s.drafts == nil {
		return absent, nil
	}
	turns, err := s.drafts.Turns(ctx, domain.AgentID(req.AgentId), draftCaller(ctx))
	if errors.Is(err, drafts.ErrNotFound) {
		return absent, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]openapi.DraftTurn, 0, len(turns))
	for _, t := range turns {
		items = append(items, turnView(conversation.Result{Turn: t}))
	}
	now, err := s.conversationNow(ctx)
	if err != nil {
		return nil, err
	}
	return openapi.ListDraftTurns200JSONResponse{
		Items: items, Conversation: now.state, SpentMicros: now.spent, DailyMicros: now.daily,
	}, nil
}

// EraseDraftThread erases what was said in the caller's own conversation.
func (s *Server) EraseDraftThread(
	ctx context.Context, req openapi.EraseDraftThreadRequestObject,
) (openapi.EraseDraftThreadResponseObject, error) {
	absent := openapi.EraseDraftThread404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId)}
	if s.drafts == nil {
		return absent, nil
	}
	err := s.drafts.EraseThread(ctx, domain.AgentID(req.AgentId), draftCaller(ctx))
	switch {
	case err == nil:
		return openapi.EraseDraftThread204Response{}, nil
	case errors.Is(err, drafts.ErrNotFound):
		return absent, nil
	}
	return nil, err
}

func turnView(r conversation.Result) openapi.DraftTurn {
	t := r.Turn
	out := openapi.DraftTurn{
		TurnId: t.ID, Key: t.Key, State: openapi.DraftTurnState(t.State), Erased: t.Erased, Pasted: t.Pasted,
		ReadRevision: string(t.Read), ReservedMicros: t.Reserved, CostMicros: t.Cost, CreatedAt: t.At,
	}
	if t.Message != "" {
		out.Message = ptr(t.Message)
	}
	if t.Reply != "" {
		out.Reply = ptr(t.Reply)
	}
	if t.State == drafts.TurnUnknown {
		out.KeptMicros = ptr(t.Reserved)
	}
	if len(r.Proposals) > 0 {
		ids := make([]string, 0, len(r.Proposals))
		for _, p := range r.Proposals {
			ids = append(ids, p.ID)
		}
		out.Proposals = &ids
	}
	if r.Dropped > 0 {
		out.Dropped = ptr(r.Dropped)
	}
	if r.Stale > 0 {
		out.Stale = ptr(r.Stale)
	}
	return out
}

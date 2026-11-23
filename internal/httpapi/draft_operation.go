package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

// ApplyDraftOperation changes one field of the agent's draft, by hand.
func (s *Server) ApplyDraftOperation(
	ctx context.Context, req openapi.ApplyDraftOperationRequestObject,
) (openapi.ApplyDraftOperationResponseObject, error) {
	bad := func(detail string) openapi.ApplyDraftOperationResponseObject {
		return openapi.ApplyDraftOperation400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(detail)),
		}
	}
	expect, problem := requiredIfMatch(req.Params.IfMatch)
	if problem != "" {
		return bad(problem), nil
	}
	if req.Body == nil {
		return bad("an operation is required"), nil
	}
	op, err := operationOf(*req.Body)
	if err != nil {
		return bad(err.Error()), nil
	}
	absent := openapi.ApplyDraftOperation404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId)}
	if s.drafts == nil {
		return absent, nil
	}
	opened, err := s.drafts.Apply(ctx, drafts.Application{
		Agent: domain.AgentID(req.AgentId), Op: op, Origin: draft.FromControl, Expect: expect,
		Caller: draftCaller(ctx), Validator: draft.Validator{Checker: s.checker()}, Now: clockOr(s.clock).Now(),
	})
	var denied *drafts.ForbiddenError
	if stale, ok := staleFrom(err); ok {
		return openapi.ApplyDraftOperation412ApplicationProblemPlusJSONResponse{PreconditionFailedApplicationProblemPlusJSONResponse: stale}, nil
	}
	switch {
	case err == nil:
		return openapi.ApplyDraftOperation200JSONResponse{
			Body: draftView(opened), Headers: openapi.ApplyDraftOperation200ResponseHeaders{ETag: string(opened.Revision)},
		}, nil
	case errors.Is(err, drafts.ErrNotFound):
		return absent, nil
	case errors.As(err, &denied):
		return openapi.ApplyDraftOperation403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish, denied.Scope),
		}, nil
	case invalidDraftChange(err):
		return bad(err.Error()), nil
	}
	return nil, err
}

// errBlockByHand is contract 3 on the public route.
var errBlockByHand = errors.New("the instructions change only through an accepted proposal of the conversation; " +
	"block operations are not applied here")

/*
operationOf is the operation a request names, reading only the fields its
kind reads. A block kind is refused here, before anything stored is asked:
the public route never applies one (contract 3).
*/
func operationOf(in openapi.DraftOperation) (draft.Operation, error) {
	if draft.ConversationOnly(string(in.Kind)) {
		return nil, errBlockByHand
	}
	if build, ok := fieldOperations[in.Kind]; ok {
		return build(in), nil
	}
	return nil, fmt.Errorf("%q is not an operation on a draft", in.Kind)
}

var fieldOperations = map[openapi.DraftOperationKind]func(openapi.DraftOperation) draft.Operation{
	openapi.DraftOperationKindSetIdentity: func(in openapi.DraftOperation) draft.Operation {
		return draft.SetIdentity{Name: valueOr(in.Name), Scope: domain.Scope{
			Company: domain.CompanyID(valueOr(in.Company)), Area: domain.AreaID(valueOr(in.Area)),
		}}
	},
	openapi.DraftOperationKindSetStandingApprovals: func(in openapi.DraftOperation) draft.Operation {
		return draft.SetStandingApprovals{Policy: standingOf(in.StandingApprovals)}
	},
	openapi.DraftOperationKindSetModel: func(in openapi.DraftOperation) draft.Operation {
		return draft.SetModel{Provider: valueOr(in.Provider), Model: valueOr(in.Model), Effort: valueOr(in.Effort)}
	},
	openapi.DraftOperationKindSetFallback: func(in openapi.DraftOperation) draft.Operation {
		return draft.SetFallback{Pair: pairOf(in.Fallback)}
	},
	openapi.DraftOperationKindSetCachePrefixTtl: func(in openapi.DraftOperation) draft.Operation {
		return draft.SetCachePrefixTTL{TTL: valueOr(in.CachePrefixTtl)}
	},
	openapi.DraftOperationKindSetReplyLocale: func(in openapi.DraftOperation) draft.Operation {
		return draft.SetReplyLocale{Locale: valueOr(in.ReplyLocale)}
	},
	openapi.DraftOperationKindAddTool: func(in openapi.DraftOperation) draft.Operation {
		return draft.AddTool{Tool: domain.ToolID(valueOr(in.Tool))}
	},
	openapi.DraftOperationKindRemoveTool: func(in openapi.DraftOperation) draft.Operation {
		return draft.RemoveTool{Tool: domain.ToolID(valueOr(in.Tool))}
	},
	openapi.DraftOperationKindSetBudget: func(in openapi.DraftOperation) draft.Operation {
		return draft.SetBudget{Budget: budgetOf(in.Budget)}
	},
	openapi.DraftOperationKindSetTriggers: func(in openapi.DraftOperation) draft.Operation {
		var triggers []spec.Trigger
		for _, t := range valueOr(in.Triggers) {
			triggers = append(triggers, specTrigger(t))
		}
		return draft.SetTriggers{Triggers: triggers}
	},
	openapi.DraftOperationKindSetMemoryLearning: func(in openapi.DraftOperation) draft.Operation {
		return draft.SetMemoryLearning{Policy: memoryLearningOf(in.MemoryLearning)}
	},
	openapi.DraftOperationKindSetApprovalDelivery: func(in openapi.DraftOperation) draft.Operation {
		return draft.SetApprovalDelivery{Policy: approvalsOf(in.Approvals)}
	},
	openapi.DraftOperationKindSetEmits: func(in openapi.DraftOperation) draft.Operation {
		return draft.SetEmits{Emits: emitsOf(in.Emits)}
	},
	openapi.DraftOperationKindAddStep: func(in openapi.DraftOperation) draft.Operation {
		return draft.AddStep{Step: stepOf(in.Step)}
	},
	openapi.DraftOperationKindEditStep: func(in openapi.DraftOperation) draft.Operation {
		return draft.EditStep{Name: valueOr(in.StepName), Step: stepOf(in.Step)}
	},
	openapi.DraftOperationKindRemoveStep: func(in openapi.DraftOperation) draft.Operation {
		return draft.RemoveStep{Name: valueOr(in.StepName)}
	},
}

func budgetOf(in *openapi.Budget) domain.Budget {
	if in == nil {
		return domain.Budget{}
	}
	return domain.Budget{
		Micros: valueOr(in.Micros), Tokens: valueOr(in.Tokens), ToolCalls: valueOr(in.ToolCalls),
		Steps: valueOr(in.Steps), WallClockMS: valueOr(in.WallClockMs),
	}
}

func stepOf(in *openapi.AgentStep) spec.Step {
	if in == nil {
		return spec.Step{}
	}
	steps := stepsOf(&[]openapi.AgentStep{*in})
	return steps[0]
}

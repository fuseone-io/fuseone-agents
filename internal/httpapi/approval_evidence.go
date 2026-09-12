package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

var errApprovalEvidenceUnavailable = errors.New("approval evidence is unavailable")

func (s *Server) GetApprovalEvidence(
	ctx context.Context, req openapi.GetApprovalEvidenceRequestObject,
) (openapi.GetApprovalEvidenceResponseObject, error) {
	absent := openapi.GetApprovalEvidence404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId),
	}
	steps, err := s.store.Read(ctx, domain.RunID(req.RunId), domain.FirstSeq)
	if err != nil {
		if isNotFound(err) {
			return absent, nil
		}
		return nil, fmt.Errorf("read approval evidence: %w", err)
	}
	state, err := engine.Fold(steps)
	if err != nil {
		return nil, err
	}
	if !mayRead(ctx, domain.PermApprovalAct, state.Scope) {
		return openapi.GetApprovalEvidence403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermApprovalAct, state.Scope),
		}, nil
	}
	if state.PendingApproval == nil || state.PendingApproval.AtSeq != req.AtSeq {
		return absent, nil
	}
	detail, err := s.approvalEvidenceDetail(ctx, state, steps, req.AtSeq)
	if err != nil {
		return openapi.GetApprovalEvidence409ApplicationProblemPlusJSONResponse(
			conflicted(errApprovalEvidenceUnavailable.Error())), nil
	}
	return openapi.GetApprovalEvidence200JSONResponse(detail), nil
}

func (s *Server) approvalEvidenceDetail(
	ctx context.Context, state engine.State, steps []domain.Step, atSeq int64,
) (openapi.ApprovalEvidenceDetail, error) {
	evidence, found, err := approvalEvidenceAt(steps, atSeq)
	if err != nil {
		return openapi.ApprovalEvidenceDetail{}, err
	}
	if !found {
		if state.PendingApproval != nil &&
			connectortools.RequiresApprovalEvidence(state.PendingApproval.Tool) {
			return openapi.ApprovalEvidenceDetail{}, errApprovalEvidenceUnavailable
		}
		return openapi.ApprovalEvidenceDetail{Kind: openapi.ApprovalEvidenceDetailKindNone}, nil
	}
	if s.content == nil || state.Ticket.Ref != evidence.Ticket {
		return openapi.ApprovalEvidenceDetail{}, errApprovalEvidenceUnavailable
	}
	raw, err := s.content.Get(ctx, evidence.Ref)
	if err != nil {
		return openapi.ApprovalEvidenceDetail{}, errApprovalEvidenceUnavailable
	}
	snapshot, err := connectortools.DecodeGraviteeApprovalEvidence(raw, evidence)
	if err != nil {
		return openapi.ApprovalEvidenceDetail{}, errApprovalEvidenceUnavailable
	}
	return graviteeApprovalEvidence(snapshot)
}

func approvalEvidenceAt(
	steps []domain.Step, atSeq int64,
) (domain.ApprovalEvidence, bool, error) {
	for _, step := range steps {
		if step.Seq != atSeq || step.Kind != domain.StepApprovalRequested {
			continue
		}
		var payload domain.ApprovalRequestedPayload
		if err := json.Unmarshal(step.Payload, &payload); err != nil {
			return domain.ApprovalEvidence{}, false, errApprovalEvidenceUnavailable
		}
		if payload.Evidence == nil {
			return domain.ApprovalEvidence{}, false, nil
		}
		if !payload.Evidence.Valid() {
			return domain.ApprovalEvidence{}, false, errApprovalEvidenceUnavailable
		}
		return *payload.Evidence, true, nil
	}
	return domain.ApprovalEvidence{}, false, errApprovalEvidenceUnavailable
}

func graviteeApprovalEvidence(
	snapshot connectortools.GraviteeSnapshot,
) (openapi.ApprovalEvidenceDetail, error) {
	updated, err := time.Parse(time.RFC3339Nano, snapshot.RemoteUpdatedAt)
	if err != nil {
		return openapi.ApprovalEvidenceDetail{}, errApprovalEvidenceUnavailable
	}
	var expires *time.Time
	if snapshot.RequestedExpiration != nil {
		parsed, err := time.Parse(time.RFC3339Nano, *snapshot.RequestedExpiration)
		if err != nil {
			return openapi.ApprovalEvidenceDetail{}, errApprovalEvidenceUnavailable
		}
		expires = &parsed
	}
	return openapi.ApprovalEvidenceDetail{
		Kind: openapi.ApprovalEvidenceDetailKindGraviteeSubscription,
		Gravitee: &openapi.GraviteeApprovalEvidence{
			SubscriptionId: snapshot.SubscriptionID, Status: snapshot.Status,
			Application: openapi.GraviteeApprovalApplication{
				Id: snapshot.Application.ID, Name: snapshot.Application.Name,
				PrimaryOwnerEmail: openapi_types.Email(snapshot.Application.PrimaryOwnerEmail),
			},
			Api:          openapi.GraviteeApprovalResource{Id: snapshot.API.ID, Name: snapshot.API.Name},
			Plan:         openapi.GraviteeApprovalResource{Id: snapshot.Plan.ID, Name: snapshot.Plan.Name},
			PlanSecurity: snapshot.PlanSecurity, RequestedExpiration: expires,
			RemoteUpdatedAt: updated,
		},
	}, nil
}

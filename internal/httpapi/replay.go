package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/replay"
)

// Replays re-derive a run's decisions, declared here by the consumer.
type Replays interface {
	Snapshot(ctx context.Context, hash string) ([]domain.Policy, error)
	Pack(ctx context.Context, agent domain.AgentID, version domain.VersionID) (gate.Pack, error)
}

/*
ReplayRun asks whether the recorded decisions would be made the same way again.

Read as an audit, because that is what it is: it changes nothing, calls
nothing, and answers a question about the record rather than about the world.
The authority to read the trail is the authority to check it.
*/
func (s *Server) ReplayRun(ctx context.Context, req openapi.ReplayRunRequestObject) (openapi.ReplayRunResponseObject, error) {
	steps, state, err := s.trail(ctx, domain.RunID(req.RunId))
	if err != nil {
		return nil, err
	}
	if steps == nil || s.replays == nil {
		return openapi.ReplayRun404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId),
		}, nil
	}
	if err := auth.Require(ctx, domain.PermAuditRead, state.Scope); err != nil {
		return openapi.ReplayRun403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAuditRead, state.Scope),
		}, nil
	}

	report, err := replay.Run(ctx, steps, s.replays, s.replays)
	if err != nil {
		return nil, fmt.Errorf("replay %s: %w", req.RunId, err)
	}

	out := openapi.ReplayReport{
		RunId: string(report.RunID), Decisions: report.Decisions,
		Reproduced: report.Reproduced, Faithful: report.Faithful(),
		Divergences: make([]openapi.Divergence, 0, len(report.Divergences)),
	}
	for _, d := range report.Divergences {
		out.Divergences = append(out.Divergences, divergenceFrom(d))
	}
	return openapi.ReplayRun200JSONResponse(out), nil
}

func divergenceFrom(d replay.Divergence) openapi.Divergence {
	out := openapi.Divergence{Seq: d.Seq}
	if d.Tool != "" {
		out.Tool = ptr(string(d.Tool))
	}
	if d.Was != domain.VerdictUnknown {
		out.Was = ptr(openapi.Verdict(d.Was.String()))
	}
	if d.Now != domain.VerdictUnknown {
		out.Now = ptr(openapi.Verdict(d.Now.String()))
	}
	if d.WasRule != "" {
		out.WasRule = ptr(d.WasRule)
	}
	if d.NowRule != "" {
		out.NowRule = ptr(d.NowRule)
	}
	if d.Why != "" {
		out.Why = ptr(d.Why)
	}
	return out
}

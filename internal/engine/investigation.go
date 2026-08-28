package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/fuseone/agents/internal/domain"
)

const maxRepeatedReadResults = 3

type investigationCall struct {
	tool    domain.ToolID
	effect  domain.Effect
	idemKey string
}

type investigationStreak struct {
	tool         domain.ToolID
	resultDigest string
	resultBytes  int64
	calls        int
	cachedCalls  int
	lastIdemKey  string
}

// ResultDigest identifies complete tool-result bytes without recording them
// in the ledger. SHA-256 is used as an equality key, not as a content preview.
func ResultDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (s *State) recordInvestigationCall(p domain.ToolCalledPayload, idemKey string) {
	s.pendingInvestigation = &investigationCall{tool: p.Tool, effect: p.Effect, idemKey: idemKey}
	if p.Effect != domain.EffectRead {
		s.resetInvestigation()
	}
}

func (s *State) recordInvestigationResult(p domain.ToolReturnedPayload) {
	call := s.pendingInvestigation
	s.pendingInvestigation = nil
	if call == nil || call.tool != p.Tool || call.effect != domain.EffectRead ||
		p.Failed || p.ResultDigest == "" {
		s.resetInvestigation()
		return
	}
	if s.sameInvestigationResult(call, p) {
		s.investigation.calls++
		if p.Cached {
			s.investigation.cachedCalls++
		}
		s.investigation.lastIdemKey = call.idemKey
		return
	}
	s.investigation = investigationStreak{
		tool: call.tool, resultDigest: p.ResultDigest, resultBytes: p.ResultBytes,
		calls: 1, lastIdemKey: call.idemKey,
	}
	if p.Cached {
		s.investigation.cachedCalls = 1
	}
}

func (s *State) sameInvestigationResult(call *investigationCall, p domain.ToolReturnedPayload) bool {
	return s.investigation.calls > 0 && s.investigation.tool == call.tool &&
		s.investigation.resultDigest == p.ResultDigest && s.investigation.lastIdemKey != call.idemKey
}

func (s *State) resetInvestigation() {
	s.pendingInvestigation = nil
	s.investigation = investigationStreak{}
}

func (s State) stalledInvestigation() (*domain.InvestigationSummary, bool) {
	if s.investigation.calls < maxRepeatedReadResults {
		return nil, false
	}
	return &domain.InvestigationSummary{
		Tool: s.investigation.tool, Calls: s.investigation.calls,
		ResultBytes: s.investigation.resultBytes, CachedCalls: s.investigation.cachedCalls,
		ResultDigest: s.investigation.resultDigest,
	}, true
}

func (r *Runner) parkInvestigation(
	ctx context.Context, state State, start Start, summary *domain.InvestigationSummary,
) (Status, error) {
	state, err := r.append(ctx, state, start, domain.Step{
		Kind: domain.StepParked,
		Payload: mustJSON(domain.ParkedPayload{
			Reason: "investigation_stalled", Investigation: summary,
		}),
	})
	if err == nil && r.deps.Metrics != nil {
		r.deps.Metrics.InvestigationStalled()
	}
	return status(state), err
}

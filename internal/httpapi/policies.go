package httpapi

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/policy"
)

// hitWindow is how far back a policy's hit count reaches when nobody says.
// A week: long enough that a rule fired at all, short enough that the number
// describes now rather than the year.
const hitWindow = 7 * 24 * time.Hour

// Policies is the authored set, declared here by the consumer.
type Policies interface {
	Active(ctx context.Context) (policy.Set, error)
	Put(ctx context.Context, p domain.Policy, by domain.UserID) (policy.Set, error)
	Delete(ctx context.Context, code string) (policy.Set, error)
}

// WithPolicies wires the authored set.
func (s *Server) WithPolicies(policies Policies) *Server {
	s.policies = policies
	return s
}

// ListPolicies answers with the rules in force and how often each decided.
func (s *Server) ListPolicies(
	ctx context.Context, req openapi.ListPoliciesRequestObject,
) (openapi.ListPoliciesResponseObject, error) {
	// Held anywhere, not in the administrative scope. A policy constrains
	// people who hold nothing there — an author in one area has to be able to
	// read the rule that stopped their agent, or being stopped is unactionable.
	if len(auth.VisibleScopes(ctx, domain.PermPolicyRead)) == 0 {
		return openapi.ListPolicies403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermPolicyRead, domain.Scope{}),
		}, nil
	}
	if s.policies == nil {
		return openapi.ListPolicies200JSONResponse{
			Items: []openapi.Policy{}, PolicyHash: "",
		}, nil
	}

	set, err := s.policies.Active(ctx)
	if err != nil {
		return nil, fmt.Errorf("read policies: %w", err)
	}

	since := clockOr(s.clock).Now().Add(-hitWindow)
	if req.Params.Since != nil {
		since = *req.Params.Since
	}
	hits, err := s.policyHits(ctx, since)
	if err != nil {
		return nil, err
	}

	items := make([]openapi.Policy, 0, len(set.Policies))
	for _, p := range set.Policies {
		item := policyFrom(p)
		if count, fired := hits[p.Code]; fired {
			item.Hits = ptr(count)
		}
		items = append(items, item)
	}
	return openapi.ListPolicies200JSONResponse{Items: items, PolicyHash: set.Hash}, nil
}

// PutPolicy writes one rule and re-snapshots the set.
func (s *Server) PutPolicy(
	ctx context.Context, req openapi.PutPolicyRequestObject,
) (openapi.PutPolicyResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermPolicyWrite); resp != nil {
		return openapi.PutPolicy403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.policies == nil {
		return nil, errNoAdministration
	}

	written, err := policyInto(req.Code, *req.Body)
	if err != nil {
		return openapi.PutPolicy400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(400, "A política não pode ser gravada", err.Error())),
		}, nil
	}

	set, err := s.policies.Put(ctx, written, callerOf(ctx))
	if err != nil {
		return nil, fmt.Errorf("write policy %s: %w", req.Code, err)
	}
	return openapi.PutPolicy200JSONResponse{
		Policy: policyFrom(written), PolicyHash: set.Hash,
	}, nil
}

// DeletePolicy stops a rule being evaluated.
//
// Decisions it already made keep pointing at the snapshot that held it, so
// removing a policy never rewrites what it did.
func (s *Server) DeletePolicy(
	ctx context.Context, req openapi.DeletePolicyRequestObject,
) (openapi.DeletePolicyResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermPolicyWrite); resp != nil {
		return openapi.DeletePolicy403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.policies == nil {
		return nil, errNoAdministration
	}
	if _, err := s.policies.Delete(ctx, req.Code); err != nil {
		return nil, fmt.Errorf("delete policy %s: %w", req.Code, err)
	}
	return openapi.DeletePolicy204Response{}, nil
}

// policyHits counts what each rule decided, from the ledger rather than from a
// counter. A counter drifts; the trail is what actually happened.
func (s *Server) policyHits(ctx context.Context, since time.Time) (map[string]int64, error) {
	decisions, err := s.store.Decisions(ctx, domain.RunFilter{Since: since}, maxHitScan)
	if err != nil {
		return nil, fmt.Errorf("count policy hits: %w", err)
	}
	hits := map[string]int64{}
	for _, d := range decisions {
		if d.PolicyCode != "" {
			hits[d.PolicyCode]++
		}
	}
	return hits, nil
}

// maxHitScan bounds the count. Stated rather than silent: past this the
// figure is "at least", and a screen claiming an exact count over an unbounded
// ledger would be claiming something nobody measured.
const maxHitScan = 5000

// SimulatePolicy replays a draft rule against decisions already recorded.
//
// Reading, not writing: nothing is stored and nothing changes, so it needs the
// permission to read policies rather than the one to author them. Somebody
// asking "what would this do" before proposing it is behaving well.
func (s *Server) SimulatePolicy(
	ctx context.Context, req openapi.SimulatePolicyRequestObject,
) (openapi.SimulatePolicyResponseObject, error) {
	if len(auth.VisibleScopes(ctx, domain.PermPolicyRead)) == 0 {
		return openapi.SimulatePolicy403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermPolicyRead, domain.Scope{}),
		}, nil
	}

	// A draft is simulated before it is named: what a rule does has nothing to
	// do with what it is called, and refusing to answer until somebody names
	// it would make the safety check the last thing anybody runs.
	draft, err := draftInto("POL-DRAFT", *req.Body)
	if err != nil {
		return openapi.SimulatePolicy400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(400, "A regra não pode ser simulada", err.Error())),
		}, nil
	}

	since := clockOr(s.clock).Now().Add(-hitWindow)
	if req.Params.Since != nil {
		since = *req.Params.Since
	}

	// Scoped like every other read of the trail: a simulation that counted
	// decisions from an area the caller cannot see would report their volume.
	filter := domain.RunFilter{Since: since, Scopes: auth.VisibleScopes(ctx, domain.PermRunRead)}
	decisions, err := s.store.Decisions(ctx, filter, limitOf(req.Params.Limit))
	if err != nil {
		return nil, fmt.Errorf("simulate policy: %w", err)
	}

	return openapi.SimulatePolicy200JSONResponse(simulationFrom(policy.Simulate(draft, decisions))), nil
}

func simulationFrom(sim policy.Simulation) openapi.Simulation {
	samples := make([]openapi.SimulationSample, 0, len(sim.Samples))
	for _, s := range sim.Samples {
		samples = append(samples, openapi.SimulationSample{
			RunId: string(s.RunID), Seq: s.Seq, Tool: string(s.Tool),
			Was: openapi.Verdict(s.Was.String()), WouldBe: openapi.Verdict(s.WouldBe.String()),
		})
	}
	return openapi.Simulation{
		Considered:    sim.Considered,
		Matched:       sim.Matched,
		Unknown:       sim.Unknown,
		WouldDeny:     ptr(sim.ByVerdict[domain.VerdictBlock]),
		WouldEscalate: ptr(sim.ByVerdict[domain.VerdictRequireApproval]),
		Samples:       samples,
	}
}

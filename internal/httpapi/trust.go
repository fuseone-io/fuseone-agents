package httpapi

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/simulate"
)

const trustEvidenceWindow = 30 * 24 * time.Hour

type trustWindow struct {
	from  time.Time
	until time.Time
}

func (s *Server) GetAgentTrust(
	ctx context.Context, req openapi.GetAgentTrustRequestObject,
) (openapi.GetAgentTrustResponseObject, error) {
	absent := openapi.GetAgentTrust404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
	}
	wanted, previous, hasPrevious, ok, err := s.trustVersions(ctx, req.AgentId, req.Params.Version)
	if err != nil || !ok {
		return absent, err
	}
	if err := s.applyTrustState(ctx, &wanted); err != nil {
		return nil, err
	}

	window := currentTrustWindow(time.Now())
	evidence, err := s.trustEvidence(ctx, wanted, previous, hasPrevious, window)
	if err != nil {
		return nil, err
	}
	status := trustStatus(evidence)
	out := openapi.AgentTrust{
		VersionId: string(wanted.VersionID),
		Status:    status, Summary: trustSummary(status),
		Recommendation: trustRecommendation(wanted.Stage, status),
		Window: openapi.AgentTrustWindow{
			From:  window.from,
			Until: window.until,
		},
		Evidence: evidence,
	}
	if hasPrevious {
		out.PreviousVersionId = ptr(string(previous.VersionID))
	}
	return openapi.GetAgentTrust200JSONResponse(out), nil
}

func currentTrustWindow(now time.Time) trustWindow {
	until := now.UTC()
	return trustWindow{from: until.Add(-trustEvidenceWindow), until: until}
}

func (s *Server) trustVersions(
	ctx context.Context, agent string, named *string,
) (domain.AgentSummary, domain.AgentSummary, bool, bool, error) {
	if s.agents == nil {
		return domain.AgentSummary{}, domain.AgentSummary{}, false, false, nil
	}
	versions, err := s.agents.Versions(ctx, domain.AgentID(agent))
	if err != nil {
		return domain.AgentSummary{}, domain.AgentSummary{}, false, false,
			fmt.Errorf("agent versions: %w", err)
	}
	visible := visibleAgentScopes(ctx)
	if len(versions) == 0 || !readable(versions[0].Scope, visible) {
		return domain.AgentSummary{}, domain.AgentSummary{}, false, false, nil
	}
	at := trustVersionIndex(versions, named)
	if at < 0 {
		return domain.AgentSummary{}, domain.AgentSummary{}, false, false, nil
	}
	wanted := versions[at]
	if !readable(wanted.Scope, visible) {
		return domain.AgentSummary{}, domain.AgentSummary{}, false, false, nil
	}

	var previous domain.AgentSummary
	hasPrevious := at+1 < len(versions)
	if hasPrevious && readable(versions[at+1].Scope, visible) {
		previous = versions[at+1]
	} else {
		hasPrevious = false
	}
	return wanted, previous, hasPrevious, true, nil
}

func trustVersionIndex(versions []domain.AgentSummary, named *string) int {
	if named == nil || *named == "" {
		return 0
	}
	for i, v := range versions {
		if string(v.VersionID) == *named {
			return i
		}
	}
	return -1
}

func (s *Server) applyTrustState(ctx context.Context, a *domain.AgentSummary) error {
	if s.pauses != nil {
		paused, err := s.pauses.IsPaused(ctx, a.ID)
		if err != nil {
			return fmt.Errorf("agent state: %w", err)
		}
		a.Started = !paused
	}
	if s.promotions != nil {
		stage, err := s.promotions.StageOf(ctx, a.ID)
		if err != nil {
			return fmt.Errorf("agent stage: %w", err)
		}
		a.Stage = stage
	}
	return nil
}

func visibleAgentScopes(ctx context.Context) []domain.Scope {
	return auth.VisibleScopes(ctx, domain.PermAgentRead)
}

func (s *Server) trustEvidence(
	ctx context.Context, current, previous domain.AgentSummary, hasPrevious bool,
	window trustWindow,
) ([]openapi.AgentTrustEvidence, error) {
	currentActivity, ran, err := s.trustActivity(ctx, current, window)
	if err != nil {
		return nil, err
	}
	previousActivity, previousRan, err := s.trustActivity(ctx, previous, window)
	if err != nil && hasPrevious {
		return nil, err
	}
	currentAgreement, err := s.trustAgreement(ctx, current, window)
	if err != nil {
		return nil, err
	}
	currentBlocks, err := s.trustGateBlocks(ctx, current, window)
	if err != nil {
		return nil, err
	}
	previousBlocks, err := s.trustGateBlocks(ctx, previous, window)
	if err != nil && hasPrevious {
		return nil, err
	}
	corpus, err := s.trustCorpus(ctx, current.ID)
	if err != nil {
		return nil, err
	}
	currentBattery, batteryRan, err := s.trustBattery(ctx, current, corpus)
	if err != nil {
		return nil, err
	}
	previousBattery, previousBatteryRan, err := s.trustBattery(ctx, previous, corpus)
	if err != nil && hasPrevious {
		return nil, err
	}
	comparison, compared := trustComparison(previousBattery, currentBattery, hasPrevious && previousBatteryRan && batteryRan)

	return []openapi.AgentTrustEvidence{
		runsTrust(currentActivity, ran),
		simulationTrust(corpus, currentBattery, batteryRan, comparison, compared),
		versionTrust(hasPrevious, compared, comparison),
		costTrust(currentActivity, ran, previousActivity, hasPrevious && previousRan),
		policyTrust(currentBlocks, ran, previousBlocks, hasPrevious && previousRan),
		decisionTrust(currentAgreement, currentActivity),
		capabilityTrust(current.Tools, previous.Tools, hasPrevious),
		launchTrust(current),
	}, nil
}

func (s *Server) trustActivity(
	ctx context.Context, a domain.AgentSummary, window trustWindow,
) (domain.AgentActivity, bool, error) {
	if a.ID == "" || a.VersionID == "" {
		return domain.AgentActivity{}, false, nil
	}
	seen, err := s.store.AgentActivity(ctx, trustRunFilter(a, window))
	if err != nil {
		return domain.AgentActivity{}, false, fmt.Errorf("agent version activity: %w", err)
	}
	if len(seen) == 0 {
		return domain.AgentActivity{}, false, nil
	}
	return seen[0], true, nil
}

func (s *Server) trustAgreement(
	ctx context.Context, a domain.AgentSummary, window trustWindow,
) (domain.VersionAgreement, error) {
	if a.ID == "" || a.VersionID == "" {
		return domain.VersionAgreement{}, nil
	}
	seen, err := s.store.VersionAgreement(ctx, trustRunFilter(a, window))
	if err != nil {
		return domain.VersionAgreement{}, fmt.Errorf("agent version agreement: %w", err)
	}
	if len(seen) == 0 {
		return domain.VersionAgreement{Agent: a.ID, Version: a.VersionID}, nil
	}
	return seen[0], nil
}

func (s *Server) trustGateBlocks(
	ctx context.Context, a domain.AgentSummary, window trustWindow,
) (domain.VersionGateBlocks, error) {
	if a.ID == "" || a.VersionID == "" {
		return domain.VersionGateBlocks{}, nil
	}
	seen, err := s.store.VersionGateBlocks(ctx, trustRunFilter(a, window))
	if err != nil {
		return domain.VersionGateBlocks{}, fmt.Errorf("agent version gate blocks: %w", err)
	}
	if len(seen) == 0 {
		return domain.VersionGateBlocks{Agent: a.ID, Version: a.VersionID}, nil
	}
	return seen[0], nil
}

func trustRunFilter(a domain.AgentSummary, window trustWindow) domain.RunFilter {
	return domain.RunFilter{
		Scope: a.Scope, AgentID: a.ID, VersionID: a.VersionID,
		Since: window.from, Until: window.until,
	}
}

func (s *Server) trustCorpus(ctx context.Context, agent domain.AgentID) ([]domain.RegressionCase, error) {
	if s.regressions == nil {
		return nil, nil
	}
	corpus, err := s.regressions.List(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("read regression corpus: %w", err)
	}
	return corpus, nil
}

func (s *Server) trustBattery(
	ctx context.Context, a domain.AgentSummary, corpus []domain.RegressionCase,
) (simulate.Report, bool, error) {
	if a.ID == "" || a.VersionID == "" || s.batteries == nil {
		return simulate.Report{}, false, nil
	}
	simulation, found, err := s.batteries.Latest(ctx, a.ID, a.VersionID)
	if err != nil || !found {
		return simulate.Report{}, false, err
	}
	report, err := simulate.Gather(ctx, s.store, simulation)
	if err != nil {
		return simulate.Report{}, false, fmt.Errorf("simulation %s: %w", simulation, err)
	}
	report.Agent, report.Version = a.ID, a.VersionID
	if len(corpus) > 0 {
		report = simulate.Battery(report, corpus)
	}
	return report, true, nil
}

func trustComparison(
	previous, current simulate.Report, ok bool,
) (simulate.Comparison, bool) {
	if !ok {
		return simulate.Comparison{}, false
	}
	return simulate.Compare(previous, current), true
}

func runsTrust(a domain.AgentActivity, ran bool) openapi.AgentTrustEvidence {
	if !ran || a.Runs == 0 {
		return trustEvidence(openapi.AgentTrustEvidenceIdRuns,
			openapi.AgentTrustEvidenceStatusMissing, openapi.RunsMissing, nil)
	}
	if a.Finished == a.Runs {
		return trustEvidence(openapi.AgentTrustEvidenceIdRuns,
			openapi.AgentTrustEvidenceStatusGood, openapi.RunsFinished,
			values("finished", a.Finished, "runs", a.Runs))
	}
	unfinished := max64(a.Runs-a.Finished, 0)
	waiting := max64(a.Waiting, 0)
	open := int64(0)
	if runStillOpenTrust(a.LastPhase) {
		open = 1
	}
	unexplained := max64(unfinished-waiting-open, 0)
	if unexplained > 0 {
		return trustEvidence(openapi.AgentTrustEvidenceIdRuns,
			openapi.AgentTrustEvidenceStatusBad, openapi.RunsUnfinished,
			values("unfinished", unexplained, "runs", a.Runs))
	}
	if waiting > 0 {
		return trustEvidence(openapi.AgentTrustEvidenceIdRuns,
			openapi.AgentTrustEvidenceStatusGood, openapi.RunsWaiting,
			values("waiting", waiting, "runs", a.Runs))
	}
	return trustEvidence(openapi.AgentTrustEvidenceIdRuns,
		openapi.AgentTrustEvidenceStatusUnknown, openapi.RunsInProgress,
		values("unfinished", unfinished, "runs", a.Runs))
}

func simulationTrust(
	corpus []domain.RegressionCase, report simulate.Report, ran bool,
	cmp simulate.Comparison, compared bool,
) openapi.AgentTrustEvidence {
	if len(corpus) == 0 {
		return trustEvidence(openapi.AgentTrustEvidenceIdSimulation,
			openapi.AgentTrustEvidenceStatusMissing, openapi.SimulationMissingCorpus, nil)
	}
	if !ran {
		return trustEvidence(openapi.AgentTrustEvidenceIdSimulation,
			openapi.AgentTrustEvidenceStatusMissing, openapi.SimulationNotRun,
			values("cases", len(corpus)))
	}
	tally := report.Tally()
	vals := simulationValues(report, tally, cmp, compared)
	switch {
	case compared && cmp.Regressed > 0:
		return trustEvidence(openapi.AgentTrustEvidenceIdSimulation,
			openapi.AgentTrustEvidenceStatusBad, openapi.SimulationRegressed, vals)
	case report.Broken > 0 || tally.Stopped > 0:
		return trustEvidence(openapi.AgentTrustEvidenceIdSimulation,
			openapi.AgentTrustEvidenceStatusBad, openapi.SimulationBroken, vals)
	case tally.Parked > 0 || tally.Unsettled > 0 || tally.NotRun > 0:
		return trustEvidence(openapi.AgentTrustEvidenceIdSimulation,
			openapi.AgentTrustEvidenceStatusBad, openapi.SimulationIncomplete, vals)
	case tally.Waiting > 0:
		return trustEvidence(openapi.AgentTrustEvidenceIdSimulation,
			openapi.AgentTrustEvidenceStatusUnknown, openapi.SimulationIncomplete, vals)
	default:
		return trustEvidence(openapi.AgentTrustEvidenceIdSimulation,
			openapi.AgentTrustEvidenceStatusGood, openapi.SimulationReady, vals)
	}
}

func versionTrust(
	hasPrevious, compared bool, cmp simulate.Comparison,
) openapi.AgentTrustEvidence {
	if !hasPrevious {
		return trustEvidence(openapi.AgentTrustEvidenceIdVersion,
			openapi.AgentTrustEvidenceStatusUnknown, openapi.VersionNoPrevious, nil)
	}
	if !compared {
		return trustEvidence(openapi.AgentTrustEvidenceIdVersion,
			openapi.AgentTrustEvidenceStatusMissing, openapi.VersionMissingBaseline, nil)
	}
	vals := values("from", string(cmp.From), "to", string(cmp.To),
		"regressed", cmp.Regressed, "fixed", cmp.Fixed, "costMicros", cmp.CostMicros)
	if cmp.Regressed > 0 {
		return trustEvidence(openapi.AgentTrustEvidenceIdVersion,
			openapi.AgentTrustEvidenceStatusBad, openapi.VersionRegressed, vals)
	}
	return trustEvidence(openapi.AgentTrustEvidenceIdVersion,
		openapi.AgentTrustEvidenceStatusGood, openapi.VersionCompared, vals)
}

func costTrust(
	current domain.AgentActivity, ran bool,
	previous domain.AgentActivity, previousRan bool,
) openapi.AgentTrustEvidence {
	if !ran || current.Runs == 0 {
		return trustEvidence(openapi.AgentTrustEvidenceIdCost,
			openapi.AgentTrustEvidenceStatusMissing, openapi.CostMissing, nil)
	}
	currentAvg := current.CostMicros / current.Runs
	if !previousRan || previous.Runs == 0 {
		return trustEvidence(openapi.AgentTrustEvidenceIdCost,
			openapi.AgentTrustEvidenceStatusUnknown, openapi.CostMissing,
			values("currentAverageMicros", currentAvg, "runs", current.Runs))
	}
	previousAvg := previous.CostMicros / previous.Runs
	vals := values("currentAverageMicros", currentAvg,
		"previousAverageMicros", previousAvg, "runs", current.Runs)
	switch {
	case currentAvg > previousAvg:
		return trustEvidence(openapi.AgentTrustEvidenceIdCost,
			openapi.AgentTrustEvidenceStatusBad, openapi.CostIncreased, vals)
	case currentAvg < previousAvg:
		return trustEvidence(openapi.AgentTrustEvidenceIdCost,
			openapi.AgentTrustEvidenceStatusGood, openapi.CostDecreased, vals)
	default:
		return trustEvidence(openapi.AgentTrustEvidenceIdCost,
			openapi.AgentTrustEvidenceStatusGood, openapi.CostStable, vals)
	}
}

func decisionTrust(
	agreement domain.VersionAgreement, activity domain.AgentActivity,
) openapi.AgentTrustEvidence {
	vals := values("approved", agreement.Approved, "refused", agreement.Refused,
		"waiting", activity.Waiting)
	if activity.Waiting > 0 {
		return trustEvidence(openapi.AgentTrustEvidenceIdDecisions,
			openapi.AgentTrustEvidenceStatusUnknown, openapi.DecisionsWaiting, vals)
	}
	if agreement.Decided() == 0 {
		return trustEvidence(openapi.AgentTrustEvidenceIdDecisions,
			openapi.AgentTrustEvidenceStatusMissing, openapi.DecisionsMissing, vals)
	}
	if agreement.Refused > 0 && versionWarrantsDemotion(agreement) {
		return trustEvidence(openapi.AgentTrustEvidenceIdDecisions,
			openapi.AgentTrustEvidenceStatusBad, openapi.DecisionsRefused, vals)
	}
	if agreement.Refused > 0 {
		return trustEvidence(openapi.AgentTrustEvidenceIdDecisions,
			openapi.AgentTrustEvidenceStatusUnknown, openapi.DecisionsRefused, vals)
	}
	return trustEvidence(openapi.AgentTrustEvidenceIdDecisions,
		openapi.AgentTrustEvidenceStatusGood, openapi.DecisionsApproved, vals)
}

func policyTrust(
	current domain.VersionGateBlocks, ran bool,
	previous domain.VersionGateBlocks, previousRan bool,
) openapi.AgentTrustEvidence {
	if !ran {
		return trustEvidence(openapi.AgentTrustEvidenceIdPolicy,
			openapi.AgentTrustEvidenceStatusMissing, openapi.PolicyMissing, nil)
	}
	vals := values("blocks", current.Blocks, "runs", current.Runs)
	if current.Blocks == 0 {
		return trustEvidence(openapi.AgentTrustEvidenceIdPolicy,
			openapi.AgentTrustEvidenceStatusGood, openapi.PolicyNoBlocks, vals)
	}
	if !previousRan {
		return trustEvidence(openapi.AgentTrustEvidenceIdPolicy,
			openapi.AgentTrustEvidenceStatusBad, openapi.PolicyBlocks, vals)
	}
	vals["previousBlocks"] = previous.Blocks
	if current.Blocks > previous.Blocks {
		return trustEvidence(openapi.AgentTrustEvidenceIdPolicy,
			openapi.AgentTrustEvidenceStatusBad, openapi.PolicyBlocksIncreased, vals)
	}
	if current.Blocks < previous.Blocks {
		return trustEvidence(openapi.AgentTrustEvidenceIdPolicy,
			openapi.AgentTrustEvidenceStatusUnknown, openapi.PolicyBlocksDecreased, vals)
	}
	return trustEvidence(openapi.AgentTrustEvidenceIdPolicy,
		openapi.AgentTrustEvidenceStatusBad, openapi.PolicyBlocks, vals)
}

func capabilityTrust(
	current, previous []domain.ToolID, hasPrevious bool,
) openapi.AgentTrustEvidence {
	if !hasPrevious {
		return trustEvidence(openapi.AgentTrustEvidenceIdCapabilities,
			openapi.AgentTrustEvidenceStatusUnknown, openapi.CapabilitiesNoPrevious, nil)
	}
	added, removed := toolDelta(current, previous)
	vals := values("added", added, "removed", removed, "total", len(current))
	if added > 0 {
		return trustEvidence(openapi.AgentTrustEvidenceIdCapabilities,
			openapi.AgentTrustEvidenceStatusBad, openapi.CapabilitiesAdded, vals)
	}
	if removed > 0 {
		return trustEvidence(openapi.AgentTrustEvidenceIdCapabilities,
			openapi.AgentTrustEvidenceStatusGood, openapi.CapabilitiesRemoved, vals)
	}
	return trustEvidence(openapi.AgentTrustEvidenceIdCapabilities,
		openapi.AgentTrustEvidenceStatusGood, openapi.CapabilitiesUnchanged, vals)
}

func launchTrust(agent domain.AgentSummary) openapi.AgentTrustEvidence {
	if agent.Started && !agent.Retired {
		return trustEvidence(openapi.AgentTrustEvidenceIdLaunch,
			openapi.AgentTrustEvidenceStatusGood, openapi.LaunchRunning, nil)
	}
	return trustEvidence(openapi.AgentTrustEvidenceIdLaunch,
		openapi.AgentTrustEvidenceStatusMissing, openapi.LaunchPaused, nil)
}

func simulationValues(
	report simulate.Report, tally simulate.Tally, cmp simulate.Comparison, compared bool,
) map[string]interface{} {
	vals := values("cases", tally.Cases, "held", report.Held,
		"broken", report.Broken, "finished", tally.Finished,
		"parked", tally.Parked, "waiting", tally.Waiting,
		"unsettled", tally.Unsettled, "stopped", tally.Stopped,
		"notRun", tally.NotRun, "steps", reportSteps(report),
		"costMicros", tally.Cost.Micros)
	if compared {
		vals["regressed"] = cmp.Regressed
		vals["fixed"] = cmp.Fixed
		vals["costDeltaMicros"] = cmp.CostMicros
	}
	return vals
}

func reportSteps(report simulate.Report) int64 {
	var total int64
	for _, result := range report.Cases {
		total += int64(result.Steps)
	}
	return total
}

func versionWarrantsDemotion(a domain.VersionAgreement) bool {
	return a.Decided() >= domain.DemoteAfter && a.Rate() < domain.DemoteBelow
}

func toolDelta(current, previous []domain.ToolID) (int, int) {
	cur, prev := map[domain.ToolID]bool{}, map[domain.ToolID]bool{}
	for _, tool := range current {
		cur[tool] = true
	}
	for _, tool := range previous {
		prev[tool] = true
	}
	added, removed := 0, 0
	for tool := range cur {
		if !prev[tool] {
			added++
		}
	}
	for tool := range prev {
		if !cur[tool] {
			removed++
		}
	}
	return added, removed
}

func trustEvidence(
	id openapi.AgentTrustEvidenceId, status openapi.AgentTrustEvidenceStatus,
	code openapi.AgentTrustEvidenceCode, vals map[string]interface{},
) openapi.AgentTrustEvidence {
	if vals == nil {
		vals = map[string]interface{}{}
	}
	return openapi.AgentTrustEvidence{
		Id: id, Status: status, Code: code, Values: vals,
	}
}

func trustStatus(evidence []openapi.AgentTrustEvidence) openapi.AgentTrustStatus {
	for _, item := range evidence {
		if item.Status == openapi.AgentTrustEvidenceStatusBad {
			return openapi.AgentTrustStatusNeedsReview
		}
	}
	for _, item := range evidence {
		if item.Status != openapi.AgentTrustEvidenceStatusGood {
			return openapi.AgentTrustStatusNeedsEvidence
		}
	}
	return openapi.AgentTrustStatusReady
}

func trustSummary(status openapi.AgentTrustStatus) openapi.AgentTrustSummary {
	switch status {
	case openapi.AgentTrustStatusReady:
		return openapi.AgentTrustSummaryReady
	case openapi.AgentTrustStatusNeedsReview:
		return openapi.AgentTrustSummaryReview
	default:
		return openapi.AgentTrustSummaryEvidence
	}
}

func trustRecommendation(
	stage domain.Stage, status openapi.AgentTrustStatus,
) openapi.AgentTrustRecommendation {
	switch status {
	case openapi.AgentTrustStatusNeedsReview:
		if stage == domain.StageAutonomous {
			return openapi.AgentTrustRecommendationDemote
		}
		return openapi.AgentTrustRecommendationReview
	case openapi.AgentTrustStatusNeedsEvidence:
		return openapi.AgentTrustRecommendationCollect
	}
	switch domain.StageOf(string(stage)) {
	case domain.StageDraft:
		return openapi.AgentTrustRecommendationCopilot
	case domain.StageCopilot:
		return openapi.AgentTrustRecommendationAutonomous
	default:
		return openapi.AgentTrustRecommendationKeep
	}
}

func values(items ...interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for i := 0; i+1 < len(items); i += 2 {
		key, ok := items[i].(string)
		if ok && key != "" {
			out[key] = items[i+1]
		}
	}
	return out
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func runStillOpenTrust(phase string) bool {
	return phase == "running" || phase == "awaiting_tool"
}

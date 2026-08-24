import type { Agent } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

type RegressionCase = components["schemas"]["RegressionCase"];

export type TrustStatus = "ready" | "needs_evidence" | "needs_review";
export type TrustEvidenceStatus = "good" | "bad" | "missing" | "unknown";

export interface TrustEvidence {
  id: "runs" | "regressions" | "decisions" | "launch";
  status: TrustEvidenceStatus;
  titleKey: string;
  bodyKey: string;
  bodyValues?: Record<string, unknown>;
  to: string;
}

export interface AgentTrustModel {
  status: TrustStatus;
  recommendationKey: string;
  summaryKey: string;
  evidence: TrustEvidence[];
}

export interface AgentTrustInput {
  agent: Agent;
  regressions?: RegressionCase[];
  regressionsLoading?: boolean;
  regressionsError?: unknown;
}

export function agentTrustModel(input: AgentTrustInput): AgentTrustModel {
  const evidence = [
    runsEvidence(input.agent),
    regressionEvidence(input),
    decisionEvidence(input.agent),
    launchEvidence(input.agent),
  ];
  const status = trustStatus(evidence);
  return {
    status,
    recommendationKey: recommendationKey(input.agent, evidence),
    summaryKey: summaryKey(status),
    evidence,
  };
}

function runsEvidence(agent: Agent): TrustEvidence {
  const activity = agent.activity;
  if (!activity || activity.runs === 0) {
    return evidence("runs", "missing", "agents.trustRunsMissing", {}, "/runs");
  }
  if (activity.finished === activity.runs) {
    return evidence(
      "runs",
      "good",
      "agents.trustRunsFinished",
      { finished: activity.finished, runs: activity.runs },
      "/runs",
    );
  }
  const unfinished = Math.max(activity.runs - activity.finished, 0);
  const waiting = Math.max(activity.waiting, 0);
  const open = runStillOpen(activity.lastPhase) ? 1 : 0;
  const unexplained = Math.max(unfinished - waiting - open, 0);
  if (unexplained > 0) {
    return evidence(
      "runs",
      "bad",
      "agents.trustRunsUnfinished",
      { unfinished: unexplained, runs: activity.runs },
      "/runs",
    );
  }
  if (waiting > 0) {
    return evidence(
      "runs",
      "good",
      "agents.trustRunsNoExecutionFailures",
      { runs: activity.runs },
      "/runs",
    );
  }
  if (open > 0) {
    return evidence(
      "runs",
      "unknown",
      "agents.trustRunsInProgress",
      { unfinished, runs: activity.runs },
      "/runs",
    );
  }
  return evidence("runs", "unknown", "agents.trustRunsUnknown", {}, "/runs");
}

function regressionEvidence(input: AgentTrustInput): TrustEvidence {
  const to = `/agents/${input.agent.agentId}/simulate`;
  if (input.regressionsLoading) {
    return evidence("regressions", "unknown", "agents.trustRegressionLoading", {}, to);
  }
  if (input.regressionsError) {
    return evidence("regressions", "unknown", "agents.trustRegressionError", {}, to);
  }
  const count = input.regressions?.length ?? 0;
  if (count === 0) {
    return evidence("regressions", "missing", "agents.trustRegressionMissing", {}, to);
  }
  return evidence(
    "regressions",
    "good",
    "agents.trustRegressionReady",
    { count },
    to,
  );
}

function decisionEvidence(agent: Agent): TrustEvidence {
  const waiting = agent.activity?.waiting ?? 0;
  if (waiting > 0) {
    return evidence(
      "decisions",
      "unknown",
      "agents.trustDecisionsWaiting",
      { waiting },
      "/approvals",
    );
  }
  return evidence("decisions", "good", "agents.trustDecisionsQuiet", {}, "/approvals");
}

function launchEvidence(agent: Agent): TrustEvidence {
  const running = agent.paused === false && agent.retired !== true;
  if (running) {
    return evidence("launch", "good", "agents.trustLaunchRunning", {}, "/runtime");
  }
  return evidence(
    "launch",
    "missing",
    "agents.trustLaunchPaused",
    {},
    `/agents/${agent.agentId}/edit`,
  );
}

function evidence(
  id: TrustEvidence["id"],
  status: TrustEvidenceStatus,
  bodyKey: string,
  bodyValues: Record<string, unknown>,
  to: string,
): TrustEvidence {
  return {
    id,
    status,
    titleKey: `agents.trustEvidence${titleID(id)}`,
    bodyKey,
    bodyValues,
    to,
  };
}

function trustStatus(evidence: TrustEvidence[]): TrustStatus {
  if (evidence.some((item) => item.status === "bad")) return "needs_review";
  if (evidence.some((item) => item.status !== "good")) return "needs_evidence";
  return "ready";
}

function recommendationKey(agent: Agent, evidence: TrustEvidence[]) {
  const status = trustStatus(evidence);
  const stage = agent.stage ?? "draft";
  if (status === "needs_review") {
    return stage === "autonomous"
      ? "agents.trustRecommendDemote"
      : "agents.trustRecommendReview";
  }
  if (status === "needs_evidence") return "agents.trustRecommendCollect";
  if (stage === "draft") return "agents.trustRecommendCopilot";
  if (stage === "copilot") return "agents.trustRecommendAutonomous";
  return "agents.trustRecommendKeep";
}

function summaryKey(status: TrustStatus) {
  if (status === "ready") return "agents.trustSummaryReady";
  if (status === "needs_review") return "agents.trustSummaryReview";
  return "agents.trustSummaryEvidence";
}

function titleID(id: TrustEvidence["id"]) {
  return `${id.slice(0, 1).toUpperCase()}${id.slice(1)}`;
}

function runStillOpen(phase?: string) {
  return phase === "running" || phase === "awaiting_tool";
}

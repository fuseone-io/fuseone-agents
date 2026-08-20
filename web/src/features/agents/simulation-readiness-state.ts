import type { Agent } from "@/lib/api/client";

export interface SimulationReadiness {
  title: string;
  body: string;
  blocksStart: boolean;
  canRetry?: boolean;
  canOpenAgent?: boolean;
}

type Translate = (key: string) => string;

export function simulationReadiness({
  agent,
  agentLoading,
  agentError,
  t,
}: {
  agent?: Agent;
  agentLoading?: boolean;
  agentError?: Error | null;
  t: Translate;
}): SimulationReadiness | null {
  if (agentLoading) {
    return {
      title: t("simulation.checkingAgent"),
      body: t("simulation.checkingAgentHint"),
      blocksStart: true,
    };
  }
  if (agentError || !agent) {
    return {
      title: t("simulation.agentReadFailed"),
      body: t("simulation.agentReadFailedHint"),
      blocksStart: true,
      canRetry: true,
    };
  }
  if (agent.retired) {
    return {
      title: t("simulation.agentRetired"),
      body: t("simulation.agentRetiredHint"),
      blocksStart: true,
      canOpenAgent: true,
    };
  }
  if (agent.paused !== false) {
    return {
      title: t("simulation.agentStopped"),
      body: t("simulation.agentStoppedHint"),
      blocksStart: true,
      canOpenAgent: true,
    };
  }
  return null;
}

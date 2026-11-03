import { useMemo } from "react";
import { useAgents } from "@/features/agents/api";
import type { MeasureFilters } from "@/features/work-items/api";

const unique = (values: string[]) => [...new Set(values)].sort();

/**
 * What the weekly panel may narrow to, read from the agents the reader can see
 * in the active scope, and the reader's filters cut down to those choices.
 *
 * The list is the scope's own, so a filter chosen in another scope — or one
 * that names an area or role this scope does not have — is dropped here
 * rather than sent: the local filters narrow within the active scope, never
 * beside it. While the list is loading there are no choices, and nothing is
 * narrowed.
 */
export function useMeasureChoices(filters: MeasureFilters) {
  const { data } = useAgents();
  return useMemo(() => {
    const items = data?.items ?? [];
    const areas = unique(items.map((a) => a.scope.area));
    const area =
      filters.area && areas.includes(filters.area) ? filters.area : undefined;
    const agents = unique(
      items.filter((a) => !area || a.scope.area === area).map((a) => a.agentId),
    );
    const agentId =
      filters.agentId && agents.includes(filters.agentId)
        ? filters.agentId
        : undefined;
    // The policy the agent's published spec declares. The list holds the
    // current version, so this is what the agent forbids now; an agent the
    // list does not name, or one that never said, is not warned about.
    const forbidsStanding = new Set(
      items
        .filter((a) => a.standingApprovals === "forbidden")
        .map((a) => a.agentId),
    );
    return { areas, agents, filters: { area, agentId }, forbidsStanding };
  }, [data, filters.area, filters.agentId]);
}

export type MeasureChoices = ReturnType<typeof useMeasureChoices>;

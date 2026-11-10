import { useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { agentKeys } from "@/features/agents/api";
import type { components } from "@/lib/api/schema.gen";

export type ExperimentListing = components["schemas"]["ExperimentListing"];
export type ExperimentConfig = components["schemas"]["ExperimentConfig"];
export type ExperimentReport = components["schemas"]["ExperimentReport"];
export type ExperimentSummary = components["schemas"]["ExperimentSummary"];
export type ExperimentCase = components["schemas"]["ExperimentCase"];
export type ExperimentComparison = components["schemas"]["ExperimentComparison"];
export type ExperimentCaseChange = components["schemas"]["ExperimentCaseChange"];
export type ExperimentRequest = components["schemas"]["ExperimentRequest"];

export const experimentKeys = {
  all: (agentId: string) => [...agentKeys.all, "experiments", agentId] as const,
  one: (agentId: string, id: string) =>
    [...experimentKeys.all(agentId), "one", id] as const,
  comparison: (agentId: string, from: string, to: string) =>
    [...experimentKeys.all(agentId), "comparison", from, to] as const,
};

/** How often a running experiment is re-read. */
const WHILE_RUNNING_MS = 2_000;

/** An agent's newest experiments, by what each pinned. */
export function useExperiments(agentId: string) {
  return useQuery({
    enabled: agentId !== "",
    queryKey: experimentKeys.all(agentId),
    queryFn: async () =>
      unwrap(
        await api.GET("/agents/{agentId}/experiments", {
          params: { path: { agentId } },
        }),
      ),
  });
}

/**
 * One experiment's report, re-read while any of its runs is still going.
 *
 * A fold of the runs, like a simulation's: a partial report is the cases that
 * settled, complete, and the rest saying they have not.
 */
export function useExperiment(agentId: string, id: string) {
  return useQuery({
    enabled: agentId !== "" && id !== "",
    queryKey: experimentKeys.one(agentId, id),
    queryFn: async () =>
      unwrap(
        await api.GET("/agents/{agentId}/experiments/{experimentId}", {
          params: { path: { agentId, experimentId: id } },
        }),
      ),
    refetchInterval: (query) =>
      query.state.data?.summary.running ? WHILE_RUNNING_MS : false,
  });
}

/**
 * Starts an experiment.
 *
 * The key is minted once per intention and retired once the experiment exists.
 * It names the experiment: clicking again after a request that never answered
 * reaches the runs already opened, and a key minted per request would open a
 * second experiment and pay for every case again.
 */
export function useStartExperiment(agentId: string) {
  const queryClient = useQueryClient();
  const intention = useRef<string | undefined>(undefined);

  return useMutation({
    mutationFn: async (body: ExperimentRequest) => {
      intention.current ??= `exp-${crypto.randomUUID()}`;
      return unwrap(
        await api.POST("/agents/{agentId}/experiments", {
          params: {
            path: { agentId },
            header: { "Idempotency-Key": intention.current },
          },
          body,
        }),
      );
    },
    onSuccess: () => {
      intention.current = undefined;
      void queryClient.invalidateQueries({ queryKey: experimentKeys.all(agentId) });
    },
  });
}

/** Two experiments of one agent, case by case. */
export function useExperimentComparison(agentId: string, from: string, to: string) {
  return useQuery({
    enabled: agentId !== "" && from !== "" && to !== "",
    queryKey: experimentKeys.comparison(agentId, from, to),
    retry: false,
    queryFn: async () =>
      unwrap(
        await api.GET("/agents/{agentId}/experiment-comparison", {
          params: { path: { agentId }, query: { from, to } },
        }),
      ),
  });
}

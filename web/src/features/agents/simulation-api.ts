import { useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { agentKeys } from "@/features/agents/api";
import type { components } from "@/lib/api/schema.gen";

export type SimulationReport = components["schemas"]["SimulationReport"];
export type SimulationCase = components["schemas"]["SimulationCase"];
export type SimulationAct = components["schemas"]["SimulationAct"];

export const simulationKeys = {
  one: (agentId: string, id: string) =>
    [...agentKeys.all, "simulation", agentId, id] as const,
};

/** How often a running simulation is re-read. */
const WHILE_RUNNING_MS = 2_000;

/**
 * One simulation, re-read while it is still going.
 *
 * The report is a fold of the runs, so a partial one is not a partial answer:
 * the cases that have settled are complete rows and the rest say they have
 * not. Polling stops the moment nothing is left running.
 */
export function useSimulation(agentId: string, id: string) {
  return useQuery({
    enabled: agentId !== "" && id !== "",
    queryKey: simulationKeys.one(agentId, id),
    queryFn: async () =>
      unwrap(
        await api.GET("/agents/{agentId}/simulations/{simulationId}", {
          params: { path: { agentId, simulationId: id } },
        }),
      ),
    refetchInterval: (query) =>
      query.state.data?.running ? WHILE_RUNNING_MS : false,
  });
}

/**
 * Starts a simulation of an agent against a set of occurrences.
 *
 * The idempotency key is minted once per intention and retired once the
 * simulation exists, exactly as opening a run does. A key minted per request
 * would make the header decorative, and somebody clicking again after a
 * request that never answered would pay for the whole set twice.
 */
export function useStartSimulation(agentId: string) {
  const queryClient = useQueryClient();
  const intention = useRef<string | undefined>(undefined);

  return useMutation({
    mutationFn: async (input: { cases?: string; corpus?: boolean }) => {
      intention.current ??= `sim-${crypto.randomUUID()}`;
      return unwrap(
        await api.POST("/agents/{agentId}/simulations", {
          params: {
            path: { agentId },
            header: { "Idempotency-Key": intention.current },
          },
          body: input,
        }),
      );
    },
    onSuccess: () => {
      intention.current = undefined;
      void queryClient.invalidateQueries({ queryKey: agentKeys.all });
    },
  });
}

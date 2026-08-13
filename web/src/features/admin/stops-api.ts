import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type Stop = components["schemas"]["Stop"];
export type StopLevel = Stop["level"];

export const stopKeys = { all: ["stops"] as const };

/**
 * What is currently stopped.
 *
 * Read everywhere rather than on one screen: an operator who stopped the
 * installation should see that from any page, and somebody wondering why
 * nothing is running should not have to know where the switch lives.
 */
export function useStops() {
  return useQuery({
    queryKey: stopKeys.all,
    queryFn: async () => unwrap(await api.GET("/admin/stops")).items,
    // The one query the console polls: a stop thrown from another tab, or by
    // somebody else during an incident, has to show up without a reload.
    refetchInterval: 20_000,
  });
}

export function useSetStop() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      level: StopLevel;
      stopped: boolean;
      reason?: string;
      scope?: { company: string; area: string };
      agentId?: string;
    }) => unwrap(await api.PUT("/admin/stops", { body: input })),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: stopKeys.all });
    },
  });
}

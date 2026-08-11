import { useRef } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { runKeys } from "@/features/runs/api";
import { agentKeys } from "@/features/agents/api";

/**
 * Opens a run of an agent.
 *
 * The idempotency key is minted once per intention and reused by every attempt
 * at it, then retired once a run exists. A key minted per request would make
 * the header decorative: somebody clicking again after a request that never
 * answered would open a second run, and a run is real tools against real
 * systems.
 */
export function useStartRun(agentId: string) {
  const queryClient = useQueryClient();
  const intention = useRef<string | undefined>(undefined);

  return useMutation({
    mutationFn: async (input?: string) => {
      intention.current ??= newIntentionKey();
      return unwrap(
        await api.POST("/agents/{agentId}/runs", {
          params: {
            path: { agentId },
            header: { "Idempotency-Key": intention.current },
          },
          body: input ? { input } : {},
        }),
      );
    },
    onSuccess: () => {
      // The intention was fulfilled. The next click is a different one, and
      // reusing this key would answer it with the run that already exists.
      intention.current = undefined;
      void queryClient.invalidateQueries({ queryKey: runKeys.all });
      void queryClient.invalidateQueries({ queryKey: agentKeys.all });
    },
  });
}

/**
 * A key for one intention.
 *
 * randomUUID rather than a timestamp: two people pressing the button in the
 * same millisecond is unlikely, and "unlikely" is not the standard for
 * something that decides whether an effect happens once or twice.
 */
function newIntentionKey(): string {
  return `manual-${crypto.randomUUID()}`;
}

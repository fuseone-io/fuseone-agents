import { useRef } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { runKeys } from "@/features/runs/api";

/**
 * Opens a new run from a recorded run's input (#16).
 *
 * The body names no content and, for now, no version: the server reads the
 * input off the recorded run and pins the new one to the version published
 * now. The key is one per intention, kept across retries and retired once a
 * run exists — the same rule as the Run button, for the same reason: a retry
 * after a lost answer must reach the run it already opened.
 */
export function useRunFromInput(runId: string) {
  const queryClient = useQueryClient();
  const intention = useRef<string | undefined>(undefined);

  return useMutation({
    mutationFn: async () => {
      intention.current ??= `rerun-${crypto.randomUUID()}`;
      return unwrap(
        await api.POST("/runs/{runId}/rerun", {
          params: {
            path: { runId },
            header: { "Idempotency-Key": intention.current },
          },
          body: {},
        }),
      );
    },
    onSuccess: () => {
      intention.current = undefined;
      void queryClient.invalidateQueries({ queryKey: runKeys.all });
    },
  });
}

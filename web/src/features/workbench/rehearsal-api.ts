import { useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { Phase, Run, Step } from "@/lib/api/client";
import { workbenchKeys } from "@/features/workbench/api";

/*
A live rehearsal in the workbench (NT-018 phase 4, release 4c). The run is read every
second while it moves, and not at all while it rests: waiting for an author to answer for
the world or to decide a held action, parked, or over. Its trail is read again only when
the run's head moved, so a resting rehearsal costs nothing.
*/

export const rehearsalKeys = {
  run: (runId: string) => [...workbenchKeys.all, "rehearsal", runId] as const,
  steps: (runId: string, seq: number) => [...rehearsalKeys.run(runId), "steps", seq] as const,
};

const RESTING: readonly Phase[] = ["awaiting_world", "awaiting_approval", "parked", "finished", "failed"];

/** Whether the rehearsal waits for a person, or for nothing: polling it would change nothing. */
export const rests = (phase: Phase | undefined) => phase !== undefined && RESTING.includes(phase);

/** The rehearsal and its trail, read while it moves. */
export function useRehearsal(runId: string) {
  const run = useQuery({
    queryKey: rehearsalKeys.run(runId),
    queryFn: async (): Promise<Run> => unwrap(await api.GET("/runs/{runId}", { params: { path: { runId } } })),
    enabled: runId !== "",
    refetchInterval: (query) => (rests(query.state.data?.phase) ? false : 1000),
  });
  const seq = run.data?.seq ?? 0;
  const steps = useQuery({
    queryKey: rehearsalKeys.steps(runId, seq),
    queryFn: () => allSteps(runId),
    enabled: run.data !== undefined,
    placeholderData: (previous) => previous,
  });
  return { run, steps };
}

/** Every step of the run, a page at a time: a rehearsal's trail is short, and read whole. */
async function allSteps(runId: string): Promise<Step[]> {
  const out: Step[] = [];
  let fromSeq: number | undefined;
  for (;;) {
    const page = unwrap(await api.GET("/runs/{runId}/steps", { params: { path: { runId }, query: { limit: 200, fromSeq } } }));
    out.push(...page.items);
    if (!page.nextSeq) return out;
    fromSeq = page.nextSeq;
  }
}

/**
 * Opens a rehearsal of a candidate on a case. The key names one intention, so it is minted
 * per body and kept until that body's rehearsal exists: asking again after an answer that
 * was lost reaches the rehearsal already opened, never a second one. Another seal or case is
 * another intention, with a key of its own.
 */
export function useOpenRehearsal(agentId: string) {
  const intentions = useRef(new Map<string, string>());
  return useMutation({
    mutationFn: async (body: { sealId: string; case: string }): Promise<Run> => {
      const intention = JSON.stringify(body);
      const key = intentions.current.get(intention) ?? `reh-${crypto.randomUUID()}`;
      intentions.current.set(intention, key);
      return unwrap(await api.POST("/agents/{agentId}/rehearsals", {
        params: { path: { agentId }, header: { "Idempotency-Key": key } },
        body,
      }));
    },
    onSuccess: (_, body) => void intentions.current.delete(JSON.stringify(body)),
  });
}

function useRehearsalAct<T>(runId: string, act: (body: T) => Promise<Run>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: act,
    onSettled: () => void queryClient.invalidateQueries({ queryKey: rehearsalKeys.run(runId) }),
  });
}

/** Answers the call the rehearsal waits on, as the world would. */
export function useAnswerWorld(runId: string) {
  return useRehearsalAct(runId, async (body: { atSeq: number; answer: string }) =>
    unwrap(await api.POST("/runs/{runId}/world", { params: { path: { runId } }, body })));
}

/** Decides the action the Gate held: a rehearsal's decision, never a real approval. */
export function useDecideInRehearsal(runId: string) {
  return useRehearsalAct(runId, async (body: { atSeq: number; approved: boolean }) =>
    unwrap(await api.POST("/runs/{runId}/rehearsal-decisions", { params: { path: { runId } }, body })));
}

/** Ends the rehearsal wherever it stands. */
export function useAbandonRehearsal(runId: string) {
  return useRehearsalAct(runId, async () =>
    unwrap(await api.POST("/runs/{runId}/rehearsal-abandon", { params: { path: { runId } } })));
}

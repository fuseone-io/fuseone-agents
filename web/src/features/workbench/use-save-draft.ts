import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { applyOperation, isAbsent, workbenchKeys, type RevisedDraft } from "@/features/workbench/api";
import { isStale } from "@/lib/api/revision";
import type { DraftOperation } from "@/lib/api/client";

/** Where a save stands: refused for an older ETag, or for a draft gone. */
export type SaveState = "idle" | "saving" | "stale" | "gone";

/**
 * Sends a save's operations one at a time, each under the ETag the one before
 * answered, and keeps that ETag: the next write always names the draft as it
 * now is. A refusal stops the rest and is reported; what landed before it
 * stays landed, in the cache and on the screen.
 */
export function useSaveDraft(agentId: string) {
  const queryClient = useQueryClient();
  const [state, setState] = useState<SaveState>("idle");

  async function save(
    start: RevisedDraft,
    operations: DraftOperation[],
    on: { draft: (d: RevisedDraft, last: boolean) => void; error: (e: unknown) => void },
  ) {
    setState("saving");
    let current = start;
    for (const [at, operation] of operations.entries()) {
      try {
        current = await applyOperation(agentId, current.etag, operation);
      } catch (error) {
        const refused = isStale(error) ? "stale" : isAbsent(error) ? "gone" : "idle";
        setState(refused);
        if (refused === "idle") on.error(error);
        return;
      }
      queryClient.setQueryData(workbenchKeys.draft(agentId), current);
      on.draft(current, at === operations.length - 1);
    }
    setState("idle");
  }

  return { state, save, settle: () => setState("idle") };
}

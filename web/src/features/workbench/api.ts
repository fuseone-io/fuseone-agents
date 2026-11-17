import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, api, unwrap } from "@/lib/api/client";
import type { AgentDraft, DraftIdentity, DraftOperation } from "@/lib/api/client";
import { revised, revisionReads, type Revised } from "@/lib/api/revision";

/*
The workbench's draft (NT-018 1b). Every write names the ETag it read, and
every answer carries the next one, which is what the panel keeps: publishing
is not here, only the draft.
*/

export const workbenchKeys = {
  all: ["workbench"] as const,
  draft: (agentId: string) => [...workbenchKeys.all, "draft", agentId] as const,
};

export type RevisedDraft = Revised<AgentDraft>;

export async function readDraft(agentId: string): Promise<RevisedDraft> {
  return revised(
    await api.GET("/agents/{agentId}/draft", { params: { path: { agentId } } }),
  );
}

/**
 * The agent's draft, read again whenever the panel opens and at no other
 * moment: a read on focus would bring a newer draft under edits not yet
 * saved. A 404 is an answer, not a failure to retry.
 */
export function useDraft(agentId: string) {
  return useQuery({
    queryKey: workbenchKeys.draft(agentId),
    queryFn: () => readDraft(agentId),
    enabled: agentId !== "",
    // One more try, as every read here gets, but none for a draft that is not
    // there: that is the answer, and the panel says what to do about it.
    retry: (count, error) => !isAbsent(error) && count < 1,
    ...revisionReads,
  });
}

/** The draft is not there, or not for this person: they answer the same. */
export function isAbsent(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404;
}

/** Opens a draft: from an identity for a new agent, or from the published one. */
export function useCreateDraft() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ agentId, identity }: { agentId: string; identity?: DraftIdentity }) =>
      revised(
        await api.POST("/agents/{agentId}/draft", {
          params: { path: { agentId }, header: { "If-None-Match": "*" } },
          body: identity,
        }),
      ),
    onSuccess: (draft, { agentId }) =>
      queryClient.setQueryData(workbenchKeys.draft(agentId), draft),
  });
}

/** One operation, against the ETag the panel holds; answers the next draft. */
export async function applyOperation(
  agentId: string,
  etag: string,
  operation: DraftOperation,
): Promise<RevisedDraft> {
  return revised(
    await api.POST("/agents/{agentId}/draft/operations", {
      params: { path: { agentId }, header: { "If-Match": etag } },
      body: operation,
    }),
  );
}

export function useDiscardDraft() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ agentId, etag }: { agentId: string; etag: string }) =>
      unwrap(
        await api.DELETE("/agents/{agentId}/draft", {
          params: { path: { agentId }, header: { "If-Match": etag } },
        }),
      ),
    onSuccess: (_, { agentId }) =>
      queryClient.removeQueries({ queryKey: workbenchKeys.draft(agentId) }),
  });
}

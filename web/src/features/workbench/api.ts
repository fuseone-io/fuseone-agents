import { useIsMutating, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { agentKeys } from "@/features/agents/api";
import { ApiError, api, unwrap } from "@/lib/api/client";
import type { AgentDraft, DraftIdentity, DraftOperation } from "@/lib/api/client";
import { revised, revisionReads, type Revised } from "@/lib/api/revision";
import type { EventCycleWarning } from "@/features/agents/event-cycle-warning";

/*
The workbench's draft (NT-018 1b). Every write names the ETag it read, and
every answer carries the next one, which is what the panel keeps: publishing
is not here, only the draft.
*/

export const workbenchKeys = {
  all: ["workbench"] as const,
  draft: (agentId: string) => [...workbenchKeys.all, "draft", agentId] as const,
  publish: (agentId: string) => [...workbenchKeys.all, "publish", agentId] as const,
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

/** What publishing the draft answered: the version, the draft based on it, and any event cycle the version closes. */
export type DraftPublished = {
  versionId: string;
  created: boolean;
  paused: boolean;
  draft: RevisedDraft;
  eventCycleWarning?: EventCycleWarning;
};

/** Whether a publication's snapshot is still the draft on screen: nothing was written over it since it was sent. */
export function isCurrentSnapshot(cached: RevisedDraft | undefined, published: DraftPublished): boolean {
  return cached?.etag === published.draft.etag;
}

/**
 * Publishes the saved draft under the ETag the panel holds (NT-018, contract
 * 4). The draft stays, based on the version it became.
 */
export function usePublishDraft(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: workbenchKeys.publish(agentId),
    mutationFn: async (etag: string): Promise<DraftPublished> => {
      const answer = revised(
        await api.POST("/agents/{agentId}/draft/publish", {
          params: { path: { agentId }, header: { "If-Match": etag } },
        }),
      );
      const { versionId, created, paused, draft, eventCycleWarning } = answer.body;
      return { versionId, created, paused, eventCycleWarning, draft: { body: draft, etag: answer.etag } };
    },
    onSuccess: (published, sent) => {
      // The snapshot is the draft at the ETag sent. A save or an accepted
      // proposal that landed while the publication was answering is newer:
      // it stays, and only its base moves, which leaves its ETag as it is.
      queryClient.setQueryData<RevisedDraft>(workbenchKeys.draft(agentId), (cached) =>
        cached && cached.etag !== sent
          ? { ...cached, body: { ...cached.body, baseVersionId: published.versionId } }
          : published.draft,
      );
      void queryClient.invalidateQueries({ queryKey: agentKeys.all });
    },
  });
}

/** Whether the agent's draft is being published right now. */
export function useIsPublishing(agentId: string): boolean {
  return useIsMutating({ mutationKey: workbenchKeys.publish(agentId) }) > 0;
}

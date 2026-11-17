import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { DraftProposal, DraftTurn } from "@/lib/api/client";
import { revised } from "@/lib/api/revision";
import { workbenchKeys, type RevisedDraft } from "@/features/workbench/api";

/*
The workbench's conversation (NT-018 phase 3). A turn is sent once under the
key the client chose, and its answer is the turn; what the model proposed is
read from the caller's own thread, and only accepting one changes the draft.
Polled while a turn is on its way, never over SSE: a turn has one answer.
*/

export const conversationKeys = {
  turns: (agentId: string) => [...workbenchKeys.all, "turns", agentId] as const,
  proposals: (agentId: string) => [...workbenchKeys.all, "proposals", agentId] as const,
};

/** Whether a turn could be sent now, or why not. */
export type Conversation = "open" | "off" | "unpriced" | "spent";

export type Transcript = { items: DraftTurn[]; conversation: Conversation };

/** How often a turn on its way is read again, until it settles. */
const SETTLING_MS = 2_000;

export function useTurns(agentId: string) {
  return useQuery({
    queryKey: conversationKeys.turns(agentId),
    queryFn: async (): Promise<Transcript> => {
      const body = unwrap(await api.GET("/agents/{agentId}/draft/turns", { params: { path: { agentId } } }));
      // An answer that does not say is read as closed: a conversation is
      // offered only when the server says it is open.
      return { items: body.items, conversation: body.conversation ?? "off" };
    },
    refetchInterval: (query) =>
      query.state.data?.items.some((t) => t.state === "in_progress") ? SETTLING_MS : false,
  });
}

/**
 * The turns that have settled, as one value: it changes whenever a turn
 * settles, including an unknown one settling late. Undefined before the
 * turns are read.
 */
export function settledOf(turns: DraftTurn[] | undefined): string | undefined {
  return turns?.filter((t) => t.state !== "in_progress").map((t) => `${t.turnId}:${t.state}`).join(",");
}

/**
 * The thread's proposals, read only after its turns and again whenever one
 * settles. A turn's proposals are stored with its settlement, in the same
 * transaction, so a read made after seeing the turn settled holds them all:
 * neither a reload nor a lost answer leaves a settled turn without its cards.
 */
export function useProposals(agentId: string, settled: string | undefined) {
  return useQuery({
    queryKey: [...conversationKeys.proposals(agentId), settled ?? ""] as const,
    queryFn: async (): Promise<DraftProposal[]> =>
      unwrap(await api.GET("/agents/{agentId}/draft/proposals", { params: { path: { agentId } } })).items,
    enabled: settled !== undefined,
    // The cards already shown stay while the next read is on its way.
    placeholderData: keepPreviousData,
  });
}

export type Sending = { key: string; message: string; pasted: boolean; locale: string };

/** One turn; whatever it answered, the thread and its proposals are read again. */
export function useSendTurn(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (turn: Sending): Promise<DraftTurn> =>
      unwrap(await api.POST("/agents/{agentId}/draft/turns", { params: { path: { agentId } }, body: turn })),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: conversationKeys.turns(agentId) });
      void queryClient.invalidateQueries({ queryKey: conversationKeys.proposals(agentId) });
    },
  });
}

/** Accepts a proposal by its id alone, and answers the draft it made. */
export function useAcceptProposal(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (proposalId: string): Promise<RevisedDraft> =>
      revised(await api.POST("/agents/{agentId}/draft/proposals/{proposalId}/accept", {
        params: { path: { agentId, proposalId } },
      })),
    onSuccess: (draft) => queryClient.setQueryData(workbenchKeys.draft(agentId), draft),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: conversationKeys.proposals(agentId) }),
  });
}

export function useRejectProposal(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (proposalId: string) =>
      unwrap(await api.POST("/agents/{agentId}/draft/proposals/{proposalId}/reject", {
        params: { path: { agentId, proposalId } },
      })),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: conversationKeys.proposals(agentId) }),
  });
}

/** Erases what was said in the caller's thread; the figures stay. */
export function useEraseThread(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () =>
      unwrap(await api.DELETE("/agents/{agentId}/draft/thread", { params: { path: { agentId } } })),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: conversationKeys.turns(agentId) });
      void queryClient.invalidateQueries({ queryKey: conversationKeys.proposals(agentId) });
    },
  });
}

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { agentKeys } from "@/features/agents/api";
import { api, unwrap } from "@/lib/api/client";
import type { AgentActivated, Candidate, CandidateSeal, DraftSeal } from "@/lib/api/client";
import { revised, revisionReads } from "@/lib/api/revision";
import { isAbsent, workbenchKeys } from "@/features/workbench/api";

/*
The workbench's candidates (NT-018 phase 2, release 2d): versions sealed from the draft,
outside the agent's published versions. Each door answers 404 for what the caller may not
see, which the list reads as "none": it says nothing about whether one exists elsewhere.
*/

export const candidateKeys = {
  all: (agentId: string) => [...workbenchKeys.all, "candidates", agentId] as const,
  one: (agentId: string, version: string) => [...candidateKeys.all(agentId), version] as const,
};

/** The candidates the caller may see, newest first; none is an empty list, not a failure. */
export function useCandidates(agentId: string) {
  return useQuery({
    queryKey: candidateKeys.all(agentId),
    queryFn: async (): Promise<CandidateSeal[]> => {
      try {
        const page = unwrap(await api.GET("/agents/{agentId}/candidates", { params: { path: { agentId } } }));
        return page.items;
      } catch (error) {
        if (isAbsent(error)) return [];
        throw error;
      }
    },
    enabled: agentId !== "",
  });
}

/** One candidate's sealed text, read from the candidate itself, never from the draft. */
export function useCandidate(agentId: string, version: string) {
  return useQuery({
    queryKey: candidateKeys.one(agentId, version),
    queryFn: async (): Promise<Candidate> =>
      unwrap(await api.GET("/agents/{agentId}/candidates/{version}", { params: { path: { agentId, version } } })),
    enabled: agentId !== "" && version !== "",
    // Not found and unreadable are answers about this candidate, not a network to retry.
    retry: false,
  });
}

/**
 * One candidate seal, by its id: what an experiment on it will run, found whatever page of
 * the listing it would fall on. Null when it is not a candidate the person may see; a
 * failed read is an error, never taken for an absent seal.
 */
export function useSeal(agentId: string, sealId: string) {
  return useQuery({
    queryKey: [...candidateKeys.all(agentId), "seal", sealId] as const,
    queryFn: async (): Promise<CandidateSeal | null> => {
      try {
        return unwrap(await api.GET("/agents/{agentId}/seals/{sealId}", { params: { path: { agentId, sealId } } }));
      } catch (error) {
        if (isAbsent(error)) return null;
        throw error;
      }
    },
    enabled: agentId !== "" && sealId !== "",
    retry: false,
  });
}

/** Seals the saved draft under the ETag the panel holds. */
export function useSealDraft(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (etag: string): Promise<DraftSeal> =>
      unwrap(await api.POST("/agents/{agentId}/draft/seals", { params: { path: { agentId }, header: { "If-Match": etag } } })),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: candidateKeys.all(agentId) }),
  });
}

/**
 * The agent as an activation names it: its current version and ETag, or null when it has
 * no version yet. Read when the dialog opens, so what is sent is what the person saw.
 */
export function useAgentHead(agentId: string, enabled: boolean) {
  return useQuery({
    queryKey: [...candidateKeys.all(agentId), "head"] as const,
    queryFn: async (): Promise<{ versionId: string; etag: string } | null> => {
      try {
        const head = revised(await api.GET("/agents/{agentId}", { params: { path: { agentId } } }));
        return { versionId: head.body.agent.versionId, etag: head.etag };
      } catch (error) {
        if (isAbsent(error)) return null;
        throw error;
      }
    },
    enabled: enabled && agentId !== "",
    ...revisionReads,
  });
}

/** Makes a candidate the agent's current version, under the agent as the dialog read it. */
export function useActivateCandidate(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ version, etag }: { version: string; etag: string | null }): Promise<AgentActivated> => {
      const header = etag === null ? { "If-None-Match": "*" as const } : { "If-Match": etag };
      return unwrap(await api.POST("/agents/{agentId}/candidates/{version}/activate", {
        params: { path: { agentId, version }, header },
      }));
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: candidateKeys.all(agentId) });
      void queryClient.invalidateQueries({ queryKey: agentKeys.all });
    },
  });
}

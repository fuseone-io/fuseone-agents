import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type AgentTemplate = components["schemas"]["AgentTemplate"];

/**
 * What an author can start from (PRD FU-16).
 *
 * Shipped inside the binary and identical in every installation, so it is held
 * for the session rather than refetched: nothing about it can change while
 * somebody is reading it.
 */
export function useTemplates() {
  return useQuery({
    queryKey: ["agent-templates"],
    queryFn: async () => unwrap(await api.GET("/agents/templates")).items,
    staleTime: Infinity,
  });
}

import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type AgentTemplate = components["schemas"]["AgentTemplate"];

/**
 * What an author can start from (PRD FU-16).
 *
 * The language goes with the request. A template's instructions are the
 * agent's prompt rather than interface text, so they are not translated by a
 * catalogue here — the server ships one set per language and the console says
 * which one it is rendering. Guessing from a header is how a server gets it
 * wrong on the one request that matters.
 *
 * Held for the session and keyed by language: nothing about a template can
 * change while somebody is reading it, but switching language changes all of
 * them at once.
 */
export function useTemplates() {
  const { i18n } = useTranslation();
  const locale = i18n.language;

  return useQuery({
    queryKey: ["agent-templates", locale],
    queryFn: async () =>
      unwrap(await api.GET("/agents/templates", { params: { query: { locale } } }))
        .items,
    staleTime: Infinity,
  });
}

import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type ProviderChoice = components["schemas"]["ProviderChoice"];

export const providerKeys = {
  all: ["providers"] as const,
  choices: (scope?: string) => [...providerKeys.all, scope ?? ""] as const,
};

/**
 * The providers an author picks from, asked for the agent's target scope
 * (NT-019, rule 5).
 *
 * The reduced answer — name, kind, models, usable — rather than the
 * integrations listing, which is the platform's configuration and which an
 * area's author cannot read. Without a target, the platform's reader gets
 * every provider and anyone else the installation's.
 */
export function useProviderChoices(scope?: string, enabled = true) {
  return useQuery({
    queryKey: providerKeys.choices(scope),
    enabled,
    queryFn: async () =>
      unwrap(
        await api.GET("/providers", {
          params: { query: scope ? { scope } : {} },
        }),
      ),
  });
}

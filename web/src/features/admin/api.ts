import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { usePagedQuery } from "@/features/runs/use-paged";
import type { components } from "@/lib/api/schema.gen";

export type Tool = components["schemas"]["Tool"];
export type Effect = components["schemas"]["Effect"];
/**
 * What a tool does, as a reader sees it — the same values and `unknown`
 * besides. `Effect` is what a caller may assert and never carries it: a tool
 * nobody has ruled on is a state to show, never one to claim.
 */
export type ToolEffect = components["schemas"]["ToolEffect"];
export type AdminEvent = components["schemas"]["AdminEvent"];
export type ScopeBudget = components["schemas"]["ScopeBudget"];

export const adminKeys = {
  all: ["admin"] as const,
  tools: () => [...adminKeys.all, "tools"] as const,
  events: (target?: string) =>
    [...adminKeys.all, "events", target ?? ""] as const,
  budgets: () => [...adminKeys.all, "budgets"] as const,
};

export function useTools(enabled = true) {
  return useQuery({
    queryKey: adminKeys.tools(),
    enabled,
    queryFn: async () => unwrap(await api.GET("/admin/tools")),
  });
}

export function useAdminEvents(target?: string) {
  return usePagedQuery(adminKeys.events(target), async (cursor) =>
    unwrap(
      await api.GET("/admin/events", {
        params: { query: { target, limit: 50, cursor } },
      }),
    ),
  );
}

/**
 * Classifying is the single point where write access enters the platform, so
 * the trail is invalidated alongside the catalogue: the change and the record
 * of it are shown together or the screen is lying about one of them.
 */
export function useClassifyTool() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      toolId: string;
      effect: Effect;
      untrusted: boolean;
      reason?: string;
      compensatedBy?: string;
      /**
       * The definition being judged, as this screen showed it.
       *
       * Sent rather than left to the server for the reason an approval carries
       * the step it approved: what was read is what is being ruled on, and a
       * server free to change the tool while the dialog is open would have the
       * ruling recorded against something nobody saw. The API answers 409 when
       * it no longer matches.
       */
      digest?: string;
    }) =>
      unwrap(
        await api.PUT("/admin/tools/{toolId}/classification", {
          params: { path: { toolId: input.toolId } },
          body: {
            effect: input.effect,
            untrusted: input.untrusted,
            reason: input.reason,
            compensatedBy: input.compensatedBy,
            digest: input.digest,
          },
        }),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: adminKeys.all });
    },
  });
}

export function useBudgets() {
  return useQuery({
    queryKey: adminKeys.budgets(),
    queryFn: async () => unwrap(await api.GET("/admin/budgets")),
  });
}

/**
 * The scope is a path segment with three shapes: `installation`, a company, or
 * `company/area`. Encoding it as one string keeps the hierarchy visible in the
 * URL, which is where an operator reading an audit trail will see it.
 */
export function usePutBudget() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      scope: string;
      period: "daily" | "monthly";
      micros?: number;
      steps?: number;
      toolCalls?: number;
      enabled: boolean;
    }) =>
      unwrap(
        await api.PUT("/admin/budgets/{scope}", {
          params: { path: { scope: input.scope } },
          body: {
            period: input.period,
            micros: input.micros,
            steps: input.steps,
            toolCalls: input.toolCalls,
            enabled: input.enabled,
          },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: adminKeys.all }),
  });
}

export function useDeleteBudget() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (scope: string) =>
      unwrap(
        await api.DELETE("/admin/budgets/{scope}", {
          params: { path: { scope } },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: adminKeys.all }),
  });
}

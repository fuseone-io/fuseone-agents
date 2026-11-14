import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { usePagedQuery } from "@/features/runs/use-paged";
import type { components } from "@/lib/api/schema.gen";

export type Tool = components["schemas"]["Tool"];
export type Effect = components["schemas"]["Effect"];
export type ToolDedupe = components["schemas"]["ToolDedupe"];
/**
 * What a tool does, as a reader sees it — the same values and `unknown`
 * besides. `Effect` is what a caller may assert and never carries it: a tool
 * nobody has ruled on is a state to show, never one to claim.
 */
export type ToolEffect = components["schemas"]["ToolEffect"];
/** Whose word a ruling is: the platform's ceiling, or the server's owner. */
export type RulingLevel = components["schemas"]["RulingLevel"];
export type Scope = components["schemas"]["Scope"];
export type ToolRuling = components["schemas"]["ToolRuling"];
export type AdminEvent = components["schemas"]["AdminEvent"];
export type ScopeBudget = components["schemas"]["ScopeBudget"];

export const adminKeys = {
  all: ["admin"] as const,
  tools: (scope?: string) => [...adminKeys.all, "tools", scope ?? ""] as const,
  classifiable: () => [...adminKeys.all, "tools", "classifiable"] as const,
  events: (target?: string) =>
    [...adminKeys.all, "events", target ?? ""] as const,
  budgets: () => [...adminKeys.all, "budgets"] as const,
};

/**
 * The tool catalogue, asked for one target scope when there is one (NT-019).
 *
 * With `scope` (`company` or `company/area`), the tools an agent stored there
 * can run, which is what an author picks from. Without one, the platform's
 * reader gets every tool and anyone else the installation's alone.
 */
export function useTools(enabled = true, scope?: string) {
  return useQuery({ ...toolsQuery(scope), enabled });
}

/**
 * The tools the caller may rule on as their owner, every tool for the
 * platform's curator (NT-019): the curator's queue, in one request. A target
 * answer runs the other way and would never hold a company's areas' tools.
 */
export function useClassifiableTools(enabled = true) {
  return useQuery({
    queryKey: adminKeys.classifiable(),
    enabled,
    queryFn: async () =>
      unwrap(
        await api.GET("/admin/tools", {
          params: { query: { classifiable: true } },
        }),
      ),
  });
}

/** The catalogue's query for one target, shared by every reader of it. */
export function toolsQuery(scope?: string) {
  return {
    queryKey: adminKeys.tools(scope),
    queryFn: async () =>
      unwrap(
        await api.GET("/admin/tools", {
          params: { query: scope ? { scope } : {} },
        }),
      ),
  };
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
      dedupe?: ToolDedupe;
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
      /** Absent is the platform's, which is what the server takes it as. */
      level?: RulingLevel;
    }) =>
      unwrap(
        await api.PUT("/admin/tools/{toolId}/classification", {
          params: { path: { toolId: input.toolId } },
          body: {
            effect: input.effect,
            untrusted: input.untrusted,
            reason: input.reason,
            compensatedBy: input.compensatedBy,
            dedupe: input.dedupe,
            digest: input.digest,
            level: input.level,
          },
        }),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: adminKeys.all });
    },
  });
}

/**
 * Withdraws the platform's ruling on a tool, the ceiling (NT-019). The owner's
 * ruling, if any, is then in force; with none the tool never executes.
 */
export function useWithdrawPlatformRuling() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (toolId: string) =>
      unwrap(
        await api.DELETE("/admin/tools/{toolId}/classification", {
          params: { path: { toolId }, query: { level: "platform" } },
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

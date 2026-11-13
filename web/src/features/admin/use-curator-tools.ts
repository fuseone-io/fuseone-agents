import { useQueries } from "@tanstack/react-query";
import { toolsQuery, type Tool } from "@/features/admin/api";
import { useMe, type Me } from "@/features/session/api";

/**
 * The tools a curator's queue is drawn from (NT-019).
 *
 * The platform's curator asks once, without a target, and is answered every
 * tool. Anyone else is answered only the installation's without one, so they
 * ask for each scope they classify at and the answers are merged: a tool
 * reached from two of those scopes is listed once.
 */
export function useCuratorTools() {
  const me = useMe();
  const targets = curatorTargets(me.data);
  const answers = useQueries({
    queries: (targets ?? []).map((scope) => toolsQuery(scope)),
  });
  const items = new Map<string, Tool>();
  for (const answer of answers) {
    for (const tool of answer.data?.items ?? []) items.set(tool.toolId, tool);
  }
  return {
    me: me.data,
    data: targets ? { items: [...items.values()] } : undefined,
    isLoading: me.isLoading || answers.some((answer) => answer.isLoading),
    error: me.error ?? answers.find((answer) => answer.error)?.error ?? null,
    refetch: () => answers.forEach((answer) => void answer.refetch()),
  };
}

/** Undefined until the session answers; `[undefined]` is one unscoped ask. */
function curatorTargets(me: Me | null | undefined): (string | undefined)[] | undefined {
  if (me === undefined) return undefined;
  if (me === null || (me.platform ?? me.can).includes("tool:classify")) {
    return [undefined];
  }
  return (me.classify ?? [])
    .filter((scope) => scope.company !== "" && scope.company !== "*")
    .map((scope) => (scope.area ? `${scope.company}/${scope.area}` : scope.company));
}

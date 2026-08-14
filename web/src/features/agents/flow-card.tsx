import { useTranslation } from "react-i18next";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ArrowRight, CircleCheck, ShieldAlert } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import { api, unwrap, type AgentDefinition } from "@/lib/api/client";

/**
 * Where data from outside reaches an act, answered before publishing (SE-07).
 *
 * From the server rather than from the tool list on screen, because the answer
 * depends on the order of the steps: a write that happens before the read
 * cannot carry its taint, and a check that ignored that would tell every
 * author their agent is dangerous.
 *
 * It reports and does not block. The path it finds is usually the point of the
 * agent; what this buys is that nobody is surprised by the approval queue on
 * Monday.
 */
export function FlowCard({ draft }: { draft: AgentDefinition }) {
  const { t } = useTranslation();
  const tools = draft.tools ?? [];

  const flow = useQuery({
    // Keyed on what the answer depends on: the area decides the catalogue,
    // and the tools decide the paths.
    queryKey: ["flow", draft.area, tools.join(",")],
    enabled: tools.length > 0 && Boolean(draft.area),
    // The rail should not blank between keystrokes: the previous answer is
    // right until the new one disagrees.
    placeholderData: keepPreviousData,
    queryFn: async () =>
      unwrap(
        await api.POST("/agents/{agentId}/flow", {
          params: { path: { agentId: draft.name || "draft" } },
          body: draft,
        }),
      ),
  });

  if (!flow.data) return null;
  const { paths, unclassified } = flow.data;

  return (
    <div className="rounded-xl border bg-card p-3 shadow-sm">
      <h3 className="mb-2 text-xs font-medium">{t("agents.flowTitle")}</h3>

      {paths.length === 0 && unclassified.length === 0 ? (
        <p className="flex items-start gap-2 text-xs text-muted-foreground">
          <CircleCheck
            className="mt-px size-3.5 shrink-0 text-success"
            aria-hidden
          />
          {t("agents.flowClean")}
        </p>
      ) : (
        <ul className="flex flex-col gap-1.5">
          {paths.map((path) => (
            <li
              key={`${path.from}-${path.to}-${path.toStep ?? ""}`}
              className="flex items-start gap-2 text-xs text-muted-foreground"
            >
              <ShieldAlert
                className="mt-px size-3.5 shrink-0 text-warning"
                aria-hidden
              />
              <span className="flex flex-wrap items-center gap-1">
                <Mono className="text-2xs">{path.from}</Mono>
                <ArrowRight aria-hidden className="size-3" />
                <Mono className="text-2xs">{path.to}</Mono>
                <span>({t(`effect.${path.effect}`)})</span>
              </span>
            </li>
          ))}
          {unclassified.length > 0 && (
            <li className="flex items-start gap-2 text-xs text-muted-foreground">
              <ShieldAlert className="mt-px size-3.5 shrink-0" aria-hidden />
              {t("agents.flowUnclassified", { tools: unclassified.join(", ") })}
            </li>
          )}
        </ul>
      )}

      <p className="mt-2 text-2xs text-muted-foreground">
        {t("agents.flowHint")}
      </p>
    </div>
  );
}

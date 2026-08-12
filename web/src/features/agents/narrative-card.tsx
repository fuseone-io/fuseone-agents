import { useTranslation } from "react-i18next";
import { BookOpenText } from "lucide-react";
import { narrate } from "@/features/agents/narrative";
import { usePolicies } from "@/features/policies/api";
import { useTools } from "@/features/admin/api";
import { formatMicros } from "@/lib/format";
import type { AgentDefinition } from "@/lib/api/client";

/**
 * The read-back, before anything is published (FU-08).
 *
 * The author approves prose, never the form: a form can be filled correctly
 * and still describe an agent nobody meant to create. Every sentence here is
 * derived — from the specification and from what the Gate decides today — so
 * approving it approves something the platform has actually promised.
 */
export function NarrativeCard({ draft }: { draft: AgentDefinition }) {
  const { t } = useTranslation();
  const tools = useTools().data?.items ?? [];
  const policies = usePolicies().data?.items ?? [];

  // Micros are the wire's unit, never a reader's. A sentence about money has
  // to read as money, or the ceiling it describes is not something anybody can
  // agree to.
  const lines = narrate(draft, tools, policies).map((line) =>
    line.values?.micros === undefined
      ? line
      : {
          ...line,
          values: {
            ...line.values,
            micros: formatMicros(Number(line.values.micros)),
          },
        },
  );

  return (
    <section className="flex flex-col gap-2.5 rounded-xl border border-border bg-card p-4 shadow-sm">
      <div className="flex items-center gap-2">
        <BookOpenText className="size-4 text-muted-foreground" aria-hidden />
        <h2 className="text-sm font-medium">{t("narrative.title")}</h2>
      </div>

      <ul className="flex flex-col gap-1.5">
        {lines.map((line) => (
          <li key={line.key} className="text-sm leading-relaxed">
            {t(line.key, line.values)}
          </li>
        ))}
      </ul>

      <p className="text-xs text-muted-foreground">{t("narrative.hint")}</p>
    </section>
  );
}

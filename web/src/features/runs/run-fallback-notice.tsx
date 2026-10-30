import { Shuffle } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { failureLabel } from "@/features/runtime/failure-labels";
import type { Run } from "@/lib/api/client";

/**
 * Says that this run finished its reasoning on the declared fallback pair.
 *
 * Its evidence is about a pair the agent was not promoted on, so the page says
 * so up front rather than leaving it to somebody reading every planned step.
 */
export function RunFallbackNotice({ run }: { run: Run }) {
  const { t } = useTranslation();
  const fallback = run.fallback;
  if (!fallback) return null;

  return (
    <section className="rounded-xl border border-warning bg-warning-surface p-4 text-warning">
      <div className="flex min-w-0 items-start gap-3">
        <Shuffle className="mt-0.5 size-5 shrink-0" aria-hidden />
        <div className="min-w-0 flex-1">
          <h2 className="font-medium">{t("runs.fellBackTitle")}</h2>
          <p className="mt-1 text-sm">
            <Mono>{`${fallback.from.provider}/${fallback.from.model}`}</Mono>
            {" → "}
            <Mono>{`${fallback.to.provider}/${fallback.to.model}`}</Mono>
          </p>
          <p className="mt-1 text-sm">
            {t("runs.fellBackDetail", {
              cause: t(failureLabel(fallback.code)),
              seq: fallback.atSeq,
            })}
          </p>
        </div>
      </div>
    </section>
  );
}

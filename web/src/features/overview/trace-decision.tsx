import { Trans, useTranslation } from "react-i18next";
import { Hand } from "lucide-react";
import { Link } from "react-router-dom";
import { Mono } from "@/components/shared/mono";
import { explainRule } from "@/lib/gate-rules";
import { formatRelative } from "@/lib/format";
import type { PendingApproval } from "@/lib/api/client";

/**
 * What the run is waiting on, at the width of a rail.
 *
 * The full decision card shows the proposed arguments, their provenance, the
 * rule, the effect and the estimated cost side by side, and it needs the width
 * to do it. Stacked into 340px those become a column somebody scrolls past to
 * reach the buttons — so this states the ask and links to the card that shows
 * the rest.
 *
 * The rule that governs the full card still governs here: nobody is asked to
 * approve without being told what will run. The difference is that "what will
 * run" is one click away rather than on screen, which is a real reduction and
 * the reason the link is not optional.
 */
export function TraceDecision({
  runId,
  approval,
}: {
  runId: string;
  approval: PendingApproval;
}) {
  const { t } = useTranslation();
  return (
    <section className="flex flex-col gap-1.5 rounded-lg border border-warning bg-warning-surface p-3">
      <div className="flex items-center gap-2">
        <Hand className="size-3.5 shrink-0 text-warning" aria-hidden />
        <span className="text-xs font-medium text-warning">
          {t("overview.awaitingDecision", { seq: approval.atSeq })}
        </span>
        <span className="ml-auto shrink-0 text-2xs text-warning">
          {formatRelative(approval.requestedAt)}
        </span>
      </div>

      <p className="text-xs">
        <Trans
          i18nKey="overview.wantsToRunTool"
          values={{
            tool: approval.tool,
            why: explainRule(approval.rule)
              ? ` — ${explainRule(approval.rule)}`
              : ".",
          }}
          components={{ tool: <Mono /> }}
        />
      </p>

      <Link
        to={`/runs/${runId}`}
        className="text-xs text-primary hover:underline"
      >
        {t("overview.seeArguments")}
      </Link>
    </section>
  );
}

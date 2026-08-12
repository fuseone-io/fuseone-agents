import { Trans, useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { Mono } from "@/components/shared/mono";
import { Badge } from "@/components/ui/badge";
import { formatRelative } from "@/lib/format";
import { explainRule } from "@/lib/gate-rules";
import { RISK_DOT, RISK_LABEL, riskOf } from "@/features/approvals/risk";
import type { PendingApproval } from "@/lib/api/client";

/**
 * One suspended action, as the queue lists it.
 *
 * The card leads with what is at stake and how long it has waited, because
 * those are what decide which one to open first.
 */
export function DecisionCard({
  item,
  selected,
  onSelect,
}: {
  item: PendingApproval;
  selected: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  const risk = riskOf(item.effect);

  return (
    <button
      type="button"
      onClick={onSelect}
      aria-current={selected}
      className={cn(
        "flex w-full flex-col gap-2 rounded-xl border bg-card p-4 text-left transition-shadow",
        selected ? "border-primary shadow-md" : "shadow-sm hover:shadow-md",
      )}
    >
      <div className="flex items-center gap-2">
        <span
          aria-hidden
          className={cn("size-[7px] shrink-0 rounded-pill", RISK_DOT[risk])}
        />
        <span className="min-w-0 flex-1 truncate font-medium">
          <Trans
            i18nKey="approvals.wantsToUse"
            values={{ agent: item.agentId ?? item.runId, tool: item.tool }}
            components={{ tool: <Mono /> }}
          />
        </span>
        <Mono dim>{formatRelative(item.requestedAt)}</Mono>
      </div>

      {/* The trail never says "denied by policy": it names the rule and
          explains it, so the approver knows what they are deciding about. */}
      <p className="text-sm text-text-secondary">
        {explainRule(item.rule) || item.reason || "Aguardando decisão humana."}
      </p>

      <div className="flex flex-wrap items-center gap-1.5">
        <Badge variant="secondary" className="font-normal">
          {t("approvals.risk", { level: t(RISK_LABEL[risk]).toLowerCase() })}
        </Badge>
        {item.scope?.area && (
          <Badge
            variant="outline"
            className="font-normal text-muted-foreground"
          >
            {item.scope.area}
          </Badge>
        )}
        <Badge
          variant="outline"
          className="font-mono text-2xs font-normal text-muted-foreground"
        >
          {item.runId}
        </Badge>
      </div>
    </button>
  );
}

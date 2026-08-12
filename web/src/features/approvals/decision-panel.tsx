import { useTranslation } from "react-i18next";
import { useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Mono } from "@/components/shared/mono";
import { formatInstant, formatRelative } from "@/lib/format";
import { explainRule } from "@/lib/gate-rules";
import { useDecideApproval } from "@/features/runs/api";
import { EFFECT_LABEL, RISK_LABEL, riskOf } from "@/features/approvals/risk";
import type { PendingApproval } from "@/lib/api/client";

/**
 * What the approver decides on.
 *
 * Everything needed to decide is here — which agent, which run, what the
 * action does, how long it has waited — because a decision made by opening
 * three screens is a decision made by guessing on two of them.
 */
export function DecisionPanel({
  item,
  onDecided,
}: {
  item: PendingApproval;
  onDecided: () => void;
}) {
  const { t } = useTranslation();
  const [note, setNote] = useState("");
  const decide = useDecideApproval(item.runId);

  async function submit(approved: boolean) {
    try {
      await decide.mutateAsync({
        approved,
        atSeq: item.atSeq,
        note: note.trim() || undefined,
      });
      toast.success(approved ? t("runs.actionApproved") : "Ação negada", {
        description: `${item.tool} · ${item.runId}`,
      });
      setNote("");
      onDecided();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t("runs.decisionFailed"),
      );
    }
  }

  return (
    <aside className="flex w-[320px] shrink-0 flex-col gap-4 rounded-xl border bg-card p-4 shadow-sm">
      <header>
        <h2 className="font-medium">
          <Mono>{item.tool}</Mono>
        </h2>
        <p className="text-xs text-muted-foreground">
          {t("approvals.requestedAt", {
            relative: formatRelative(item.requestedAt),
            instant: formatInstant(item.requestedAt),
          })}
        </p>
      </header>

      <dl className="flex flex-col gap-2 border-y border-border-subtle py-3 text-sm">
        <Row label={t("cost.agent")} value={item.agentId ?? "—"} />
        <Row label={t("admin.area")} value={item.scope?.area ?? "—"} />
        <Row label="Risco" value={RISK_LABEL[riskOf(item.effect)]} />
        <Row
          label={t("policies.effect")}
          value={item.effect ? EFFECT_LABEL[item.effect] : "não classificado"}
        />
        <Row label={t("policies.rule")} value={item.rule ?? "—"} mono />
        <Row label="Passo" value={`#${item.atSeq}`} mono />
      </dl>

      <div>
        <h3 className="text-2xs uppercase tracking-label text-muted-foreground">
          {t("approvals.context")}
        </h3>
        <p className="mt-1 text-sm text-text-secondary">
          {explainRule(item.rule) || item.reason || t("approvals.noDetail")}
        </p>
        <Link
          to={`/runs/${item.runId}`}
          className="mt-2 inline-block text-sm text-text-accent underline-offset-4 hover:underline"
        >
          {t("approvals.seeWholeRun")}
        </Link>
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="note">{t("approvals.note")}</Label>
        <Input
          id="note"
          value={note}
          onChange={(event) => setNote(event.target.value)}
          placeholder={t("approvals.notePlaceholder")}
        />
      </div>

      <footer className="flex gap-2">
        <Button
          variant="outline"
          className="flex-1"
          disabled={decide.isPending}
          onClick={() => void submit(false)}
        >
          {t("approvals.deny")}
        </Button>
        <Button
          className="flex-1"
          disabled={decide.isPending}
          onClick={() => void submit(true)}
        >
          {t("approvals.approve")}
        </Button>
      </footer>
    </aside>
  );
}

function Row({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="flex items-baseline gap-3">
      <dt className="w-20 shrink-0 text-xs text-muted-foreground">{label}</dt>
      <dd
        className={
          mono
            ? "min-w-0 flex-1 truncate font-mono text-xs"
            : "min-w-0 flex-1 truncate"
        }
      >
        {value}
      </dd>
    </div>
  );
}

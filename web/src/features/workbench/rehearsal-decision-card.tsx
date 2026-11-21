import { useId } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { ShieldAlert } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { StepContent } from "@/features/runs/step-content";
import { useDecideInRehearsal } from "@/features/workbench/rehearsal-api";
import { problemMessage } from "@/lib/api/problem-message";
import type { PendingApproval } from "@/lib/api/client";

/**
 * The action the Gate held in a rehearsal, and the author's decision on it. The decision
 * is the rehearsal's own: it is no real approval, and an approved call still reaches no
 * tool, it waits for the world's answer like any other.
 */
export function RehearsalDecisionCard({ runId, pending }: { runId: string; pending: PendingApproval }) {
  const { t } = useTranslation();
  const titleId = useId();
  const decide = useDecideInRehearsal(runId);

  const answer = (approved: boolean) =>
    decide.mutate({ atSeq: pending.atSeq, approved }, { onError: (error) => toast.error(problemMessage(error, t)) });

  return (
    <Card role="region" aria-labelledby={titleId}>
      <CardHeader>
        <CardTitle id={titleId} className="flex items-center gap-2 text-sm">
          <ShieldAlert className="size-4" aria-hidden />
          {t("workbench.rehearsal.heldTitle")}
        </CardTitle>
        <CardDescription>{t("workbench.rehearsal.heldBody")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div>
          <p className="text-xs text-muted-foreground">{t("workbench.rehearsal.tool")}</p>
          <Mono>{pending.tool}</Mono>
          {pending.reason && <p className="mt-2 text-sm">{pending.reason}</p>}
          <p className="mt-3 text-xs text-muted-foreground">{t("workbench.rehearsal.arguments")}</p>
          <StepContent runId={runId} seq={pending.atSeq} open />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => answer(true)} disabled={decide.isPending}>{t("workbench.rehearsal.approve")}</Button>
          <Button variant="outline" onClick={() => answer(false)} disabled={decide.isPending}>{t("workbench.rehearsal.reject")}</Button>
        </div>
      </CardContent>
    </Card>
  );
}

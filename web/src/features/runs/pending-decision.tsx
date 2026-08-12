import { Trans, useTranslation } from "react-i18next";
import { Hand } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Mono } from "@/components/shared/mono";
import { ConfirmAction } from "@/features/runs/confirm-action";
import {
  DecisionArguments,
  DecisionProvenance,
} from "@/features/runs/decision-arguments";
import { DecisionFacts } from "@/features/runs/decision-facts";
import { useDecideApproval, useStepContent } from "@/features/runs/api";
import { formatRelative } from "@/lib/format";
import { explainRule } from "@/lib/gate-rules";
import type { PendingApproval, Step } from "@/lib/api/client";

/**
 * The taint the arguments carry, as recorded with the request.
 *
 * Not the step's own labels: those are what the step produced, and an approval
 * request produces nothing. What matters here is what the arguments inherited.
 */
function labelsOf(step?: Step): string[] | undefined {
  const labels = (step?.payload as Record<string, unknown> | undefined)?.labels;
  return Array.isArray(labels)
    ? labels.filter((l): l is string => typeof l === "string")
    : undefined;
}

/**
 * What the run is waiting on, and what it will do if you say yes.
 *
 * Never ask for an approval without showing what will run: the arguments,
 * where they came from, the rule that stopped them and what it will cost, all
 * on screen at the moment of the decision. An approver who has to open another
 * screen to find out what they are approving will stop opening it.
 */
export function PendingDecision({
  runId,
  approval,
  step,
}: {
  runId: string;
  approval: PendingApproval;
  step?: Step;
}) {
  const { t } = useTranslation();
  const decide = useDecideApproval(runId);
  const content = useStepContent(runId, approval.atSeq);

  const submit = (approved: boolean) => {
    decide.mutate(
      { approved, atSeq: approval.atSeq },
      {
        onSuccess: () =>
          toast.success(approved ? "Ação aprovada" : "Ação recusada"),
        onError: (error) =>
          toast.error("Não foi possível registrar a decisão", {
            description: error instanceof Error ? error.message : undefined,
          }),
      },
    );
  };

  return (
    <section
      aria-labelledby="decision-heading"
      className="overflow-hidden rounded-xl border border-warning bg-card shadow-sm"
    >
      <div className="flex gap-3.5 bg-warning-surface p-4">
        <span className="flex size-[30px] shrink-0 items-center justify-center rounded-lg border border-warning bg-card text-warning">
          <Hand className="size-4" aria-hidden />
        </span>
        <div className="min-w-0 flex-1">
          <h2
            id="decision-heading"
            className="text-sm font-medium text-warning"
          >
            {t("runs.awaitingYou", { seq: approval.atSeq })}
          </h2>
          <p className="mt-0.5 text-sm">
            <Trans
              i18nKey="runs.agentWantsToRun"
              values={{
                tool: approval.tool,
                why: explainRule(approval.rule)
                  ? ` — ${explainRule(approval.rule)}`
                  : ".",
              }}
              components={{
                tool: (
                  <Mono className="rounded-md border border-border bg-card px-1.5 py-px" />
                ),
              }}
            />
          </p>
        </div>
        <div className="ml-auto shrink-0 text-right">
          <div className="text-2xs text-warning">{t("runs.requested")}</div>
          <div className="font-mono text-sm tabular-nums text-warning">
            {formatRelative(approval.requestedAt)}
          </div>
        </div>
      </div>

      <div className="grid border-t border-border md:grid-cols-[1fr_288px]">
        <div className="border-border p-4 md:border-r">
          <h3 className="mb-2 text-2xs uppercase tracking-label text-muted-foreground">
            {t("runs.proposedArguments")}
          </h3>
          {content.isLoading ? (
            <Skeleton className="h-20 w-full rounded-lg" />
          ) : (
            <DecisionArguments body={content.data?.content} />
          )}
          <DecisionProvenance labels={labelsOf(step)} />
        </div>

        <div className="flex flex-col gap-2.5 p-4">
          <DecisionFacts approval={approval} step={step} />
          <div className="mt-auto flex gap-2 pt-2">
            <ConfirmAction
              label="Aprovar"
              title="Aprovar esta ação?"
              description={`A ferramenta ${approval.tool} será executada e o efeito ficará registrado na trilha em seu nome.`}
              disabled={decide.isPending}
              onConfirm={() => submit(true)}
            />
            <Button
              variant="outline"
              className="h-[34px] flex-1"
              disabled={decide.isPending}
              onClick={() => submit(false)}
            >
              {t("runs.refuse")}
            </Button>
          </div>
        </div>
      </div>
    </section>
  );
}

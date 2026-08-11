import { useParams } from "react-router-dom";
import { ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { PhaseBadge } from "./phase-badge";
import { StepRow } from "./step-row";
import { ApprovalPanel } from "./approval-panel";
import { useRun, useRunSteps, useVerifyRun } from "./api";
import { formatCost, formatInstant, formatTokens } from "@/lib/format";

export function RunDetailPage() {
  const { runId = "" } = useParams();
  const run = useRun(runId);
  const steps = useRunSteps(runId);

  if (run.isLoading || steps.isLoading) return <LoadingRows rows={8} />;
  if (run.error) return <ErrorState error={run.error} onRetry={() => void run.refetch()} />;
  if (steps.error) return <ErrorState error={steps.error} onRetry={() => void steps.refetch()} />;
  if (!run.data) return null;

  const { data } = run;

  return (
    <div className="flex flex-col gap-4">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{data.agentId}</h1>
          <p className="font-mono text-xs text-muted-foreground">{data.runId}</p>
        </div>
        <div className="flex items-center gap-2">
          <PhaseBadge phase={data.phase} />
          <VerifyButton runId={runId} />
        </div>
      </header>

      {data.pendingApproval && (
        <ApprovalPanel runId={runId} approval={data.pendingApproval} />
      )}

      <dl className="grid gap-3 sm:grid-cols-4">
        <Stat label="Custo" value={formatCost(data.cost)} />
        <Stat label="Tokens" value={formatTokens(data.cost.inputTokens)} />
        <Stat label="Passos" value={String(data.seq)} />
        <Stat label="Início" value={formatInstant(data.startedAt)} />
      </dl>

      <section aria-labelledby="trail-heading">
        <h2 id="trail-heading" className="mb-2 text-sm font-medium">
          Trilha
        </h2>
        <ul className="rounded-lg border">
          {(steps.data?.items ?? []).map((step) => (
            <StepRow key={step.seq} step={step} />
          ))}
        </ul>
      </section>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <CardContent className="px-4 py-3">
        <dt className="text-xs text-muted-foreground">{label}</dt>
        <dd className="mt-0.5 font-medium tabular-nums">{value}</dd>
      </CardContent>
    </Card>
  );
}

/**
 * Verification walks every step, so it runs on request rather than on page
 * load. The result is stated in words, not only in colour.
 */
function VerifyButton({ runId }: { runId: string }) {
  const verify = useVerifyRun(runId);

  return (
    <div className="flex items-center gap-2">
      <Button
        variant="outline"
        size="sm"
        onClick={() => verify.mutate()}
        disabled={verify.isPending}
      >
        <ShieldCheck className="size-4" />
        Verificar trilha
      </Button>
      {verify.data && (
        <span
          role="status"
          className={
            verify.data.valid
              ? "text-sm text-emerald-700 dark:text-emerald-400"
              : "text-sm text-destructive"
          }
        >
          {verify.data.valid
            ? `Íntegra — ${verify.data.stepsChecked} passos`
            : `Rompida no passo ${verify.data.brokenAtSeq}`}
        </span>
      )}
    </div>
  );
}

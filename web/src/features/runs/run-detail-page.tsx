import { useParams } from "react-router-dom";
import { PageHeader } from "@/components/shared/page-header";
import { Panel } from "@/components/shared/panel";
import { Mono } from "@/components/shared/mono";
import { StateDot } from "@/components/shared/state-dot";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { stateOfPhase } from "@/lib/agent-state";
import { PHASE_LABELS } from "@/features/runs/phase-badge";
import { StepRow } from "@/features/runs/step-row";
import { ApprovalPanel } from "@/features/runs/approval-panel";
import { useRun, useRunSteps } from "@/features/runs/api";
import { VerifyButton } from "@/features/runs/verify-button";
import { formatCost, formatDuration, formatInstant, formatTokens } from "@/lib/format";

export function RunDetailPage() {
  const { runId = "" } = useParams();
  const run = useRun(runId);
  const steps = useRunSteps(runId);

  if (run.isLoading || steps.isLoading) return <LoadingRows rows={8} />;
  if (run.error) return <ErrorState error={run.error} onRetry={() => void run.refetch()} />;
  if (steps.error) return <ErrorState error={steps.error} onRetry={() => void steps.refetch()} />;
  if (!run.data) return null;

  const { data } = run;
  const items = steps.data?.items ?? [];

  return (
    <>
      <PageHeader title={data.agentId} description={data.scope.area}>
        <VerifyButton runId={runId} />
      </PageHeader>

      {data.pendingApproval && <ApprovalPanel runId={runId} approval={data.pendingApproval} />}

      <Panel
        title={
          <span className="flex items-center gap-2">
            <StateDot state={stateOfPhase(data.phase)} />
            {PHASE_LABELS[data.phase]}
          </span>
        }
        action={
          // Everything the platform produced, in one mono line: the design's
          // "id · N steps · cost".
          <Mono dim>
            {data.runId} · {data.seq} passos · {formatCost(data.cost)} ·{" "}
            {formatDuration(data.startedAt, data.endedAt)}
          </Mono>
        }
      >
        <dl className="grid gap-3 sm:grid-cols-4">
          <Stat label="Início" value={formatInstant(data.startedAt)} />
          <Stat label="Tokens" value={formatTokens((data.cost.inputTokens ?? 0) + (data.cost.outputTokens ?? 0))} />
          <Stat label="Versão" value={data.versionId.slice(0, 9)} />
          <Stat label="Em nome de" value={data.onBehalfOf ?? "—"} />
        </dl>
      </Panel>

      <Panel title="Trilha" action={<span className="text-xs text-muted-foreground">append-only</span>}>
        <ol className="flex flex-col">
          {items.map((step, i) => (
            <StepRow key={step.seq} step={step} last={i === items.length - 1} />
          ))}
        </ol>
      </Panel>
    </>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-2xs uppercase tracking-label text-muted-foreground">{label}</dt>
      <dd className="mt-0.5 font-mono text-sm tabular-nums">{value}</dd>
    </div>
  );
}

import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { RotateCcw } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { LoadFailed } from "@/features/experiments/load-failed";
import { PhaseBadge } from "@/features/runs/phase-badge";
import { TrailPanel } from "@/features/runs/trail-panel";
import { RehearsalAbandon } from "@/features/workbench/rehearsal-abandon";
import { useRehearsal } from "@/features/workbench/rehearsal-api";
import { RehearsalKeep } from "@/features/workbench/rehearsal-keep";
import { RehearsalDecisionCard } from "@/features/workbench/rehearsal-decision-card";
import { RehearsalWorldCard, type WorldWait } from "@/features/workbench/rehearsal-world-card";
import { problemMessage } from "@/lib/api/problem-message";
import type { Run, Step } from "@/lib/api/client";

const OVER: Run["phase"][] = ["finished", "failed"];

/**
 * The wait the rehearsal stands at, read from the head of its trail. A trail that does not
 * reach the run's head yet is an earlier one still shown while the new one is read: a wait
 * read from it is one the run has already moved past.
 */
function waitAt(run: Run, steps: Step[]): WorldWait | undefined {
  const head = steps[steps.length - 1];
  if (run.phase !== "awaiting_world" || head?.kind !== "world_awaited" || head.seq !== run.seq) return undefined;
  const payload = head.payload as { tool?: string; call_seq?: number } | undefined;
  return { atSeq: head.seq, tool: payload?.tool ?? "", callSeq: payload?.call_seq ?? 0 };
}

/** One rehearsal as it runs: where it stands, what it waits for, and its trail. */
export function RehearsalRun({ agentId, runId, onNew }: { agentId: string; runId: string; onNew: () => void }) {
  const { t } = useTranslation();
  const { run, steps } = useRehearsal(runId);

  if (run.isLoading) {
    return (
      <div role="status" aria-label={t("workbench.rehearsal.reading")} className="flex flex-col gap-2">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }
  if (run.error || !run.data) {
    return <LoadFailed message={problemMessage(run.error, t)} onRetry={() => void run.refetch()} />;
  }
  const current = run.data;
  const trail = steps.data ?? [];
  const wait = waitAt(current, trail);
  const over = OVER.includes(current.phase);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <PhaseBadge phase={current.phase} />
        <Button asChild size="sm" variant="link" className="h-auto p-0">
          <Link to={`/runs/${runId}`}><Mono>{runId}</Mono></Link>
        </Button>
        <div className="ml-auto flex gap-2">
          {!over && <RehearsalAbandon runId={runId} />}
          {over && <RehearsalKeep agentId={agentId} run={current} steps={trail} />}
          {over && (
            <Button size="sm" variant="outline" onClick={onNew}>
              <RotateCcw className="size-4" aria-hidden />
              {t("workbench.rehearsal.again")}
            </Button>
          )}
        </div>
      </div>
      {wait && <RehearsalWorldCard key={wait.atSeq} runId={runId} wait={wait} />}
      {current.phase === "awaiting_approval" && current.pendingApproval && (
        <RehearsalDecisionCard key={current.pendingApproval.atSeq} runId={runId} pending={current.pendingApproval} />
      )}
      {steps.error && <LoadFailed message={problemMessage(steps.error, t)} onRetry={() => void steps.refetch()} />}
      {trail.length > 0 && <TrailPanel runId={runId} steps={trail} />}
    </div>
  );
}

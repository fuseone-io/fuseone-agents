import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";
import { Skeleton } from "@/components/ui/skeleton";
import { LoadFailed } from "@/features/experiments/load-failed";
import { useCandidates } from "@/features/workbench/candidates-api";
import { CaseRunResult } from "@/features/workbench/case-run-result";
import { CaseRunStart } from "@/features/workbench/case-run-start";
import type { CaseRun } from "@/features/workbench/case-run-api";
import { problemMessage } from "@/lib/api/problem-message";

const PARAMS = ["experiment", "baseline"] as const;

/**
 * The experiment tab (NT-018 phase 5c): a candidate run against the agent's cases and
 * compared with the published version. The two experiments are named in the address, so a
 * reload or a shared link shows the same comparison, never one against a newer baseline.
 */
export function WorkbenchExperiment({ agentId, onTrace }: { agentId: string; onTrace: (runId: string) => void }) {
  const { t } = useTranslation();
  const [params, setParams] = useSearchParams();
  const experiment = params.get("experiment") ?? "";

  const follow = (run?: CaseRun) =>
    setParams((current) => {
      const next = new URLSearchParams(current);
      for (const p of PARAMS) next.delete(p);
      if (run) next.set("experiment", run.experiment);
      if (run?.baseline) next.set("baseline", run.baseline);
      return next;
    });

  return (
    <div className="flex flex-col gap-4">
      <p className="max-w-prose text-sm text-muted-foreground">{t("workbench.experiment.intro")}</p>
      {experiment ? (
        <CaseRunResult agentId={agentId} opened={{ experiment, baseline: params.get("baseline") ?? "" }} onTrace={onTrace} onNew={() => follow()} />
      ) : (
        <Candidates agentId={agentId} onOpened={follow} />
      )}
    </div>
  );
}

function Candidates({ agentId, onOpened }: { agentId: string; onOpened: (run: CaseRun) => void }) {
  const { t } = useTranslation();
  const candidates = useCandidates(agentId);
  if (candidates.isLoading) return <Skeleton aria-label={t("workbench.experiment.reading")} className="h-32 w-full" />;
  if (candidates.error) return <LoadFailed message={problemMessage(candidates.error, t)} onRetry={() => void candidates.refetch()} />;
  if (!candidates.data?.length) {
    return (
      <div className="flex flex-col gap-1 rounded-lg border border-dashed p-4">
        <p className="text-sm font-medium">{t("workbench.rehearsal.noCandidate")}</p>
        <p className="text-sm text-muted-foreground">{t("workbench.experiment.noCandidateBody")}</p>
      </div>
    );
  }
  return <CaseRunStart agentId={agentId} candidates={candidates.data} onOpened={onOpened} />;
}

import { useTranslation } from "react-i18next";
import { Skeleton } from "@/components/ui/skeleton";
import { LoadFailed } from "@/features/experiments/load-failed";
import { CaseRow } from "@/features/workbench/case-row";
import { useCases } from "@/features/workbench/cases-api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * The cases tab (NT-018 phase 5b): the corpus every future version of the agent is checked
 * against, oldest first, each to be removed. A case is made from an ended rehearsal.
 */
export function WorkbenchCases({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  const cases = useCases(agentId);

  if (cases.isLoading) {
    return (
      <div role="status" aria-label={t("workbench.cases.reading")} className="flex flex-col gap-2">
        <Skeleton className="h-16 w-full" />
        <Skeleton className="h-16 w-full" />
      </div>
    );
  }
  if (cases.error) {
    return <LoadFailed message={problemMessage(cases.error, t)} onRetry={() => void cases.refetch()} />;
  }
  return (
    <div className="flex flex-col gap-4">
      <p className="max-w-prose text-sm text-muted-foreground">{t("workbench.cases.intro")}</p>
      {cases.data?.length ? (
        <ul className="flex flex-col gap-2">
          {cases.data.map((entry) => <CaseRow key={entry.id} agentId={agentId} entry={entry} />)}
        </ul>
      ) : (
        <div className="flex flex-col gap-1 rounded-lg border border-dashed p-4">
          <p className="text-sm font-medium">{t("workbench.cases.empty")}</p>
          <p className="text-sm text-muted-foreground">{t("workbench.cases.emptyBody")}</p>
        </div>
      )}
    </div>
  );
}

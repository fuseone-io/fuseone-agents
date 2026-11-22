import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import type { ExperimentCaseChange } from "@/features/experiments/experiments-api";
import type { CaseGroup } from "@/features/workbench/case-run-groups";

/** Written out rather than built from the value, so the guard that checks every key exists can see them. */
const TITLE: Record<CaseGroup, string> = {
  regressed: "workbench.experiment.groups.regressed",
  fixed: "workbench.experiment.groups.fixed",
  kept: "workbench.experiment.groups.kept",
  apart: "workbench.experiment.groups.apart",
};
const STANDING: Record<ExperimentCaseChange["was"], string> = {
  held: "comparison.standing.held",
  broke: "comparison.standing.broke",
  absent: "comparison.standing.absent",
  not_comparable: "comparison.standing.not_comparable",
};

/**
 * One group of the comparison. A regression offers its run, the worst of the candidate's
 * repetitions, to read in the rehearsal tab; no other group does, since what a reader opens
 * a trace for is what got worse.
 */
export function CaseRunGroup({
  group,
  changes,
  runOf,
  onTrace,
}: {
  group: CaseGroup;
  changes: ExperimentCaseChange[];
  runOf: (caseId: string) => string | undefined;
  onTrace: (runId: string) => void;
}) {
  const { t } = useTranslation();
  const heading = useId();

  return (
    <section aria-labelledby={heading} className="flex min-w-0 flex-col gap-2 rounded-xl border bg-card p-4 shadow-sm">
      <h3 id={heading} className="text-sm font-medium">
        {t(TITLE[group])} <span className="tabular-nums text-muted-foreground">({changes.length})</span>
      </h3>
      {changes.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t("workbench.experiment.groupEmpty")}</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {changes.map((change) => (
            <li key={change.id} className="flex min-w-0 flex-wrap items-center gap-2 wrap-anywhere">
              <Mono className="min-w-0 text-xs">{change.id}</Mono>
              <Badge variant={group === "regressed" ? "destructive" : "outline"}>
                {t(STANDING[change.was])} → {t(STANDING[change.now])}
              </Badge>
              {group === "regressed" && runOf(change.id) && (
                <Button
                  size="sm"
                  variant="link"
                  className="h-auto p-0 text-xs"
                  aria-label={t("workbench.experiment.traceOf", { case: change.id })}
                  onClick={() => onTrace(runOf(change.id) ?? "")}
                >
                  {t("workbench.experiment.trace")}
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

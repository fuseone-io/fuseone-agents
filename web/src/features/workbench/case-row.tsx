import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Mono } from "@/components/shared/mono";
import { Button } from "@/components/ui/button";
import { expectationLabel } from "@/features/agents/correction-options";
import type { RegressionCase } from "@/features/agents/regressions-api";
import { CaseRemove } from "@/features/workbench/case-remove";
import { formatInstant } from "@/lib/format";

/** One kept case: what it checks, in the words it was ticked with, and where it came from. */
export function CaseRow({ agentId, entry }: { agentId: string; entry: RegressionCase }) {
  const { t } = useTranslation();

  return (
    <li aria-label={entry.id} className="flex items-start gap-3 rounded-lg border p-3">
      {/* Nothing here has a bound in the contract: a note or a tool without a space must
          break where it reaches the edge, not widen the panel. */}
      <div className="flex min-w-0 flex-1 flex-col gap-1.5 wrap-anywhere">
        <Mono>{entry.id}</Mono>
        <ul className="flex flex-col gap-0.5 text-sm">
          {entry.expectations.map((e, i) => {
            const said = expectationLabel(e);
            return (
              <li key={i}>
                {t(said.label)}
                {said.tool && <Mono className="ml-1.5 text-xs">{said.tool}</Mono>}
                {e.step && <span className="ml-1.5 text-xs text-muted-foreground">{t("correction.atStep", { step: e.step })}</span>}
              </li>
            );
          })}
        </ul>
        {entry.note && <p className="text-sm text-muted-foreground">{entry.note}</p>}
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          {entry.fromRun && (
            <Button asChild size="sm" variant="link" className="h-auto min-w-0 shrink whitespace-normal p-0 text-xs">
              <Link to={`/runs/${entry.fromRun}`}><Mono>{entry.fromRun}</Mono></Link>
            </Button>
          )}
          {entry.createdBy && <span>{entry.createdBy}</span>}
          {entry.createdAt && <Mono dim>{formatInstant(entry.createdAt)}</Mono>}
        </div>
      </div>
      <CaseRemove agentId={agentId} caseId={entry.id} />
    </li>
  );
}

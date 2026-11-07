import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/shared/mono";
import type { SimulationReport } from "@/features/agents/simulation-api";

/**
 * What a battery was read against, and what has joined the corpus since.
 *
 * A result stays true after the corpus grows, and without this it reads as if it
 * covered today's corpus. An uploaded set says none of this, and renders nothing.
 */
export function SimulationCorpusNote({ report }: { report: SimulationReport }) {
  const { t } = useTranslation();
  const unchecked = report.notChecked ?? [];

  return (
    <>
      {report.unpinned && (
        <p className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          <Badge variant="outline">{t("simulation.corpusUnpinned")}</Badge>
          {t("simulation.corpusUnpinnedHint")}
        </p>
      )}
      {report.datasetVersion !== undefined && (
        <p className="text-sm text-muted-foreground">
          {t("simulation.corpusVersion", { version: report.datasetVersion })}
        </p>
      )}
      {unchecked.length > 0 && (
        <div className="flex flex-col gap-1 text-sm">
          <p className="font-medium">{t("simulation.corpusNotChecked")}</p>
          <p className="text-muted-foreground">
            {t("simulation.corpusNotCheckedHint")}
          </p>
          <ul className="flex flex-wrap gap-2">
            {unchecked.map((id) => (
              <li key={id}>
                <Mono>{id}</Mono>
              </li>
            ))}
          </ul>
        </div>
      )}
    </>
  );
}

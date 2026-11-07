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
      <CaseList
        title={t("simulation.corpusNotChecked")}
        hint={t("simulation.corpusNotCheckedHint")}
        ids={report.notChecked}
      />
      <CaseList
        title={t("simulation.corpusErasedSince")}
        hint={t("simulation.corpusErasedSinceHint")}
        ids={report.erasedSince}
      />
    </>
  );
}

function CaseList({
  title,
  hint,
  ids = [],
}: {
  title: string;
  hint: string;
  ids?: string[];
}) {
  if (ids.length === 0) {
    return null;
  }
  return (
    <div className="flex flex-col gap-1 text-sm">
      <p className="font-medium">{title}</p>
      <p className="text-muted-foreground">{hint}</p>
      <ul className="flex flex-wrap gap-2">
        {ids.map((id) => (
          <li key={id}>
            <Mono>{id}</Mono>
          </li>
        ))}
      </ul>
    </div>
  );
}

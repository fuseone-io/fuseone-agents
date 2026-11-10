import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { TableCell, TableRow } from "@/components/ui/table";
import { Mono } from "@/components/shared/mono";
import { formatMicros } from "@/lib/format";
import type { ExperimentCaseChange } from "@/features/experiments/experiments-api";

/**
 * Written out rather than built from the value, so the guard that checks every
 * key exists can see them.
 */
const STANDING: Record<ExperimentCaseChange["was"], string> = {
  held: "comparison.standing.held",
  broke: "comparison.standing.broke",
  absent: "comparison.standing.absent",
  not_comparable: "comparison.standing.not_comparable",
};

/** One case on both sides, with in how many repetitions it broke on each. */
export function ExperimentChangeRow({ change }: { change: ExperimentCaseChange }) {
  const { t } = useTranslation();
  const regressed = change.was === "held" && change.now === "broke";

  return (
    <TableRow>
      <TableCell>
        <Mono className="text-xs">{change.id}</Mono>
      </TableCell>
      <TableCell>
        {/* The words, never the colour alone. */}
        <Badge variant={regressed ? "destructive" : "outline"}>
          {t(STANDING[change.was])} → {t(STANDING[change.now])}
        </Badge>
      </TableCell>
      <TableCell className="tabular-nums">
        {t("experiments.brokenIn", { broken: change.wasBrokenIn, count: change.wasRan })}
        {" → "}
        {t("experiments.brokenIn", { broken: change.nowBrokenIn, count: change.nowRan })}
      </TableCell>
      <TableCell className="text-right">
        {change.costMicros !== 0 && (
          <Mono dim className="text-xs">
            {change.costMicros > 0 ? "+" : "−"}
            {formatMicros(Math.abs(change.costMicros))}
          </Mono>
        )}
      </TableCell>
    </TableRow>
  );
}

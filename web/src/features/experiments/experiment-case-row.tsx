import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { TableCell, TableRow } from "@/components/ui/table";
import { Mono } from "@/components/shared/mono";
import { DivergedNote } from "@/features/runs/diverged-note";
import { formatMicros } from "@/lib/format";
import type { ExperimentCase } from "@/features/experiments/experiments-api";

type Standing = "held" | "broken" | "erased" | "notRun" | "notComparable";

/**
 * Written out rather than built from the value, so the guard that checks every
 * key exists can see them.
 */
const STANDING: Record<Standing, string> = {
  held: "experiments.standingHeld",
  broken: "experiments.standingBroken",
  erased: "experiments.standingErased",
  notRun: "experiments.standingNotRun",
  notComparable: "experiments.standingNotComparable",
};

/**
 * How a case stood across every repetition.
 *
 * The case is broken if any repetition broke — never an average — and the row
 * says in how many, out of how many that ran: a case broken in one of five is a
 * flaky model, in five of five a wrong one, and "broken" alone tells them apart
 * for nobody.
 */
function standingOf(row: ExperimentCase): Standing {
  if (row.case.settled === "erased") return "erased";
  if (row.ran === 0) return "notRun";
  // A replayed case that left its world answered a situation nobody corrected (NT-018 phase 5).
  if (row.case.diverged) return "notComparable";
  return (row.case.unmet?.length ?? 0) > 0 ? "broken" : "held";
}

export function ExperimentCaseRow({
  row,
  repetitions,
}: {
  row: ExperimentCase;
  repetitions: number;
}) {
  const { t } = useTranslation();
  const standing = standingOf(row);

  return (
    <TableRow>
      <TableCell>
        <Mono className="text-xs">{row.case.id ?? "—"}</Mono>
      </TableCell>
      <TableCell>
        {/* The word, never the colour alone. */}
        <Badge variant={standing === "broken" || standing === "notRun" ? "destructive" : "outline"}>
          {t(STANDING[standing])}
        </Badge>
        {row.case.diverged && <DivergedNote divergence={row.case.diverged} />}
      </TableCell>
      <TableCell className="tabular-nums">
        {standing === "erased"
          ? "—"
          : t("experiments.brokenIn", { broken: row.brokenIn, count: row.ran })}
        {row.divergedIn > 0 && (
          <span className="ml-2 text-2xs text-muted-foreground">
            {t("experiments.divergedIn", { count: row.divergedIn })}
          </span>
        )}
        {row.ran > 0 && row.ran < repetitions && (
          <span className="ml-2 text-2xs text-muted-foreground">
            {t("experiments.ranFewer", { ran: row.ran, count: repetitions })}
          </span>
        )}
      </TableCell>
      <TableCell className="text-right">
        <Mono dim className="text-xs">
          {formatMicros(row.case.cost.micros)}
        </Mono>
      </TableCell>
    </TableRow>
  );
}

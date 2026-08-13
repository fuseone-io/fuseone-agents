import { useTranslation } from "react-i18next";
import { CircleCheck, CircleHelp, TriangleAlert } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import type { components } from "@/lib/api/schema.gen";

type Report = components["schemas"]["ReplayReport"];
type Divergence = components["schemas"]["Divergence"];

/**
 * What the replay found.
 *
 * Three outcomes, not two. A decision that could not be re-derived at all —
 * a policy set nobody kept, a trail older than the field it needs — is
 * neither a match nor a mismatch, and folding it into either would be the
 * report lying in the direction that suits it.
 */
export function ReplayReport({ report }: { report: Report }) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-3">
      <p className="flex items-center gap-2 text-sm">
        {report.faithful ? (
          <CircleCheck aria-hidden className="size-4 text-success" />
        ) : (
          <TriangleAlert aria-hidden className="size-4 text-warning" />
        )}
        {t("runs.replayCount", {
          reproduced: report.reproduced,
          decisions: report.decisions,
        })}
      </p>

      {report.divergences.length > 0 && (
        <ul className="flex flex-col gap-1.5">
          {report.divergences.map((d) => (
            <DivergenceRow key={d.seq} divergence={d} />
          ))}
        </ul>
      )}
    </div>
  );
}

function DivergenceRow({ divergence }: { divergence: Divergence }) {
  const { t } = useTranslation();
  const undecidable = Boolean(divergence.why);

  return (
    <li className="flex items-start gap-2 rounded-lg border px-3 py-2 text-xs">
      {undecidable ? (
        <CircleHelp aria-hidden className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />
      ) : (
        <TriangleAlert aria-hidden className="mt-0.5 size-3.5 shrink-0 text-warning" />
      )}
      <div className="flex flex-col gap-0.5">
        <span className="flex items-center gap-2">
          <Mono className="text-2xs text-muted-foreground">
            {t("runs.replayAtStep", { seq: divergence.seq })}
          </Mono>
          {divergence.tool && <Mono className="text-xs">{divergence.tool}</Mono>}
        </span>
        <span className="text-muted-foreground">
          {undecidable
            ? divergence.why
            : t("runs.replayChanged", {
                was: t(`verdict.${divergence.was}`),
                now: t(`verdict.${divergence.now}`),
              })}
        </span>
      </div>
    </li>
  );
}

import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import type { Finding } from "@/features/agents/instruction-lint";
import type { Summary } from "@/features/agents/instructions-summary";

/**
 * What the card adds up to, along its bottom edge.
 *
 * The observation on the left names the rule rather than the count: "one
 * observation" sends somebody scrolling, and the identifier is what they take
 * to the policy screen. Everything on the right is read out of the text.
 */
export function InstructionsStrip({
  summary,
  findings,
}: {
  summary: Summary;
  findings: Finding[];
}) {
  const { t } = useTranslation();
  const first = findings[0];

  return (
    <div className="-mx-4 -mb-4 mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-border bg-muted px-4 py-2.5">
      {first && (
        <span className="flex items-center gap-1.5 text-2xs text-warning">
          <TriangleAlert className="size-3.5 shrink-0" aria-hidden />
          {t("agents.observations", { count: findings.length })}
        </span>
      )}
      {first && (
        <span className="min-w-0 flex-1 truncate text-2xs text-muted-foreground">
          {first.because ? (
            <>
              <Mono className="text-2xs">{first.because}</Mono>{" "}
              {t("agents.deniesWhatTheTextPromises")}{" "}
              <Mono className="text-2xs">{first.tool}</Mono>
            </>
          ) : (
            <>
              {t("agents.ladderDeniesInStrip")}{" "}
              <Mono className="text-2xs">{first.tool}</Mono>
            </>
          )}
        </span>
      )}

      <span className="ml-auto flex items-center gap-2 text-2xs text-muted-foreground">
        <span>{t("agents.toolsCited", { count: summary.tools })}</span>
        <span aria-hidden>·</span>
        <span>{t("agents.stopsCounted", { count: summary.stops })}</span>
        <span aria-hidden>·</span>
        <Mono dim className="text-2xs">
          {t("agents.charactersCounted", { count: summary.characters })}
        </Mono>
      </span>
    </div>
  );
}

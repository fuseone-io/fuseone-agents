import { useTranslation } from "react-i18next";
import { ShieldX } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import type { Finding } from "@/features/agents/instruction-lint";

/**
 * A sentence that promises what the platform already refuses.
 *
 * Inside the block that produced it, never as a banner on the card: a warning
 * at the top leaves somebody hunting for the sentence it is about, and by the
 * third visit they stop reading it at all.
 *
 * Two exits, both real. Removing the sentence is one answer; keeping it
 * because it explains the rule to whoever reads the definition next is an
 * equally good one, and the author is the person who knows which. There is no
 * dismissal without a decision and no third "learn more" — a warning nobody
 * has to answer is a warning that is always there.
 */
export function InstructionFinding({
  finding,
  onRemove,
  onKeep,
}: {
  finding: Finding;
  onRemove: () => void;
  onKeep: () => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="mt-2 flex flex-col gap-2 rounded-lg border border-border bg-card p-3">
      <p className="flex items-start gap-2 text-xs">
        <ShieldX className="mt-px size-4 shrink-0 text-danger" aria-hidden />
        <span>
          {finding.because ? (
            <>
              {t("agents.policyAlreadyDenies")}{" "}
              <Mono className="text-2xs">{finding.because}</Mono>{" "}
              <Mono className="text-2xs">{finding.tool}</Mono>
            </>
          ) : (
            <>
              {t("agents.ladderAlreadyDenies")}{" "}
              <Mono className="text-2xs">{finding.tool}</Mono>
            </>
          )}
        </span>
      </p>

      <p className="text-2xs text-muted-foreground">
        {t("agents.forbiddingTwiceAges")}
      </p>

      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-6 text-2xs"
          onClick={onRemove}
        >
          {t("agents.removeTheSentence")}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-6 text-2xs"
          onClick={onKeep}
        >
          {t("agents.keepItExplains")}
        </Button>
      </div>
    </div>
  );
}

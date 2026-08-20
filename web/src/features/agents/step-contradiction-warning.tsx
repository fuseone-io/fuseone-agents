import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import type { StepContradiction } from "@/features/agents/steps-drift";

/**
 * A disagreement between "Never" and the process stages.
 *
 * It is shown in both places somebody can fix it: beside the instructions
 * where the prohibition was written, and beside the stages that still do the
 * thing. The run trail is too late to be the first place this becomes visible.
 */
export function StepContradictionWarning({
  conflicts,
  onOpen,
}: {
  conflicts: StepContradiction[];
  onOpen?: () => void;
}) {
  const { t } = useTranslation();
  const first = conflicts[0];
  if (!first) return null;

  return (
    <div className="flex items-start gap-2 rounded-md bg-warning-surface px-3 py-2 text-2xs text-warning">
      <TriangleAlert className="mt-px size-3.5 shrink-0" aria-hidden />
      <p className="min-w-0 flex-1">
        <span className="font-medium">
          {t("agents.stepContradictions", { count: conflicts.length })}
        </span>{" "}
        <ConflictSentence conflict={first} />
      </p>
      {onOpen && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-6 shrink-0 px-2 text-2xs text-warning hover:text-warning"
          onClick={onOpen}
        >
          {t("agents.reviewSteps")}
        </Button>
      )}
    </div>
  );
}

function ConflictSentence({ conflict }: { conflict: StepContradiction }) {
  const { t } = useTranslation();

  if (conflict.why === "forbiddenReach") {
    return (
      <>
        {conflict.at === undefined
          ? t("agents.agentStillReaches")
          : t("agents.stepStillReaches", { step: conflict.at + 1 })}{" "}
        <Mono className="text-2xs">{conflict.tool ?? conflict.term}</Mono>.
      </>
    );
  }

  return (
    <>
      {t("agents.stepStillStopsOn", { step: (conflict.at ?? 0) + 1 })}{" "}
      <Mono className="text-2xs">{conflict.term}</Mono>.
    </>
  );
}

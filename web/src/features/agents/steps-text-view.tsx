import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { StepCard } from "@/features/agents/step-card";
import { useListReorder } from "@/features/agents/use-list-reorder";
import { gateStops } from "@/features/agents/step-gating";
import type { Policy, Tool } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

/**
 * The sequence as sentences.
 *
 * The same steps the strip draws, and never beside it: two editors of one
 * thing on screen means each gets half the width it needs and the reader has
 * to work out which is authoritative. They alternate.
 *
 * Read, with editing on the pencil. A list of open inputs is a form, and a
 * process somebody wants to read back before publishing should read like a
 * process.
 */
export function StepsTextView({
  steps,
  catalogue,
  policies,
  onEdit,
  onAdd,
  onMove,
}: {
  steps: AgentStep[];
  catalogue: Tool[];
  policies: Policy[];
  onEdit: (at: number) => void;
  onAdd: () => void;
  onMove: (from: number, to: number) => void;
}) {
  const { t } = useTranslation();
  const drag = useListReorder(onMove);

  return (
    <div className="flex flex-col gap-2">
      {steps.map((step, at) => (
        <StepCard
          key={at}
          step={step}
          index={at}
          stops={gateStops(step, catalogue, policies) ? t("agents.mayStopHere") : undefined}
          onEdit={() => onEdit(at)}
          drag={drag}
        />
      ))}

      <Button
        type="button"
        variant="outline"
        onClick={onAdd}
        className="h-10 justify-start border-dashed text-muted-foreground"
      >
        {t("agents.writeTheNextStep")}
      </Button>

      {/* The contract between the two views, said once and where it applies. */}
      <p className="text-2xs text-muted-foreground">{t("agents.sameThing")}</p>
    </div>
  );
}

import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";
import { EmptyCanvas } from "@/features/agents/empty-canvas";
import { ReadAgainButton } from "@/features/agents/read-again-button";
import { AgentCanvas } from "@/features/agents/agent-canvas";
import { StepInspector } from "@/features/agents/step-inspector";
import { StepRail } from "@/features/agents/step-rail";
import { undescribed } from "@/features/agents/steps-drift";
import { useStepDrawing } from "@/features/agents/use-step-drawing";
import type { AgentDefinition, Policy, Tool } from "@/lib/api/client";

/**
 * The process, drawn and edited in one panel.
 *
 * Three columns and a fixed height, which is the point: a form per stage
 * underneath the canvas made the page as long as the process, and twelve
 * stages meant twelve open forms nobody was reading. What is edited is
 * whichever card is selected.
 *
 * The rail holds this agent's own capability pack and nothing else. A
 * component library looks like a catalogue of things you may have, so the
 * only things in here are things this agent was granted — dragging cannot
 * widen it, because there is nothing wider to drag.
 */
export function AgentFlowEditor({
  draft,
  patch,
  catalogue,
  policies,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
  catalogue: Tool[];
  policies: Policy[];
}) {
  const { t } = useTranslation();
  const drawing = useStepDrawing(draft, patch);
  const { steps, pack, selected } = drawing;

  // What the drawing allows and the words never mention. The direction that
  // matters: prose may say more than the permissions do, and permissions
  // saying more means the agent is allowed something nobody wrote down.
  const silent = undescribed(steps, draft.instructions);

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2">
        <p className="text-2xs text-muted-foreground">
          {steps.length === 0
            ? t("agents.noStepsMeans")
            : t("agents.dragToReorder")}
        </p>
        {/* Only once there is something to re-read. While the canvas is
            empty the empty state carries the action, and two buttons saying
            the same thing is how somebody stops reading either.

            Never on open, either, and that is a cost decision rather than a
            preference: every reading spends real money at the provider and
            counts against the assistant's daily ceiling, so calling it on
            mount would charge somebody for arriving to fix a typo. */}
        {steps.length > 0 && (
          <ReadAgainButton
            steps={steps.length}
            disabled={drawing.reading || draft.instructions.trim() === ""}
            onConfirm={drawing.read}
          />
        )}
      </div>

      {/* Said where the drawing is, not in a toast: it is a fact about the
          definition somebody is about to publish, and it stays true until one
          of the two halves changes. */}
      {silent.length > 0 && (
        <p className="flex items-start gap-1.5 rounded-md bg-warning-surface px-2.5 py-2 text-2xs text-warning">
          <TriangleAlert className="mt-px size-3.5 shrink-0" aria-hidden />
          {t("agents.undescribed", {
            count: silent.length,
            tools: silent.join(", "),
          })}
        </p>
      )}

      <div className="flex h-[420px] overflow-hidden rounded-lg border border-border">
        <StepRail catalogue={catalogue} pack={pack} />
        {steps.length === 0 ? (
          <EmptyCanvas
            reading={drawing.reading}
            canRead={draft.instructions.trim() !== ""}
            onRead={drawing.read}
          />
        ) : (
        <AgentCanvas
          steps={steps}
          selected={selected}
          onReorder={drawing.reorder}
          onSelect={drawing.setSelected}
          onDropTool={drawing.insert}
        />
        )}
        <StepInspector
          step={selected === undefined ? undefined : steps[selected]}
          at={selected}
          tools={{ pack, catalogue, policies }}
          onChange={drawing.change}
          onRemove={drawing.remove}
        />
      </div>
    </div>
  );
}


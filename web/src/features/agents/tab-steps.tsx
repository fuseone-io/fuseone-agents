import { useState } from "react";
import { useTranslation } from "react-i18next";
import { List, Plus, Workflow } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AgentCanvas } from "@/features/agents/agent-canvas";
import { EmptyCanvas } from "@/features/agents/empty-canvas";
import { ReadAgainButton } from "@/features/agents/read-again-button";
import { StepInspector } from "@/features/agents/step-inspector";
import { StepsTextView } from "@/features/agents/steps-text-view";
import { undescribed } from "@/features/agents/steps-drift";
import { useStepDrawing } from "@/features/agents/use-step-drawing";
import { DriftWarning } from "@/features/agents/drift-warning";
import type { AgentDefinition, Policy, Tool } from "@/lib/api/client";

/**
 * One editor, two views, never both at once.
 *
 * Sentences or a canvas — the same sequence either way, and an edit in one
 * shows in the other because there is only one sequence. Tiling them gave each
 * half the width it needed and left the reader working out which was
 * authoritative.
 */
export function TabSteps({
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
  const [view, setView] = useState<"text" | "flow">("text");
  const drawing = useStepDrawing(draft, patch);
  const { steps, pack, selected } = drawing;

  const silent = undescribed(steps, draft.instructions);

  return (
    <div className="flex flex-col gap-3">
      <div className="mx-auto flex w-full max-w-[820px] flex-wrap items-center gap-3">
        <Tabs
          value={view}
          onValueChange={(next) => setView(next as typeof view)}
        >
          <TabsList className="h-8">
            <TabsTrigger value="text">
              <List className="size-3.5" aria-hidden />
              {t("agents.asText")}
            </TabsTrigger>
            <TabsTrigger value="flow">
              <Workflow className="size-3.5" aria-hidden />
              {t("agents.asFlowView")}
            </TabsTrigger>
          </TabsList>
        </Tabs>
        <p className="hidden text-2xs text-muted-foreground sm:block">
          {t(view === "text" ? "agents.textHint" : "agents.flowHint")}
        </p>

        {steps.length > 0 ? (
          <ReadAgainButton
            steps={steps.length}
            disabled={drawing.reading || draft.instructions.trim() === ""}
            onConfirm={drawing.read}
          />
        ) : (
          <span className="ml-auto" />
        )}
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-8"
          onClick={() => drawing.insert("", steps.length)}
        >
          <Plus className="size-3.5" aria-hidden />
          {t("agents.step")}
        </Button>
      </div>

      <div className="mx-auto w-full max-w-[820px]">
        <DriftWarning tools={silent} />
      </div>

      {steps.length === 0 ? (
        <div className="flex h-[320px] overflow-hidden rounded-lg border border-border">
          <EmptyCanvas
            reading={drawing.reading}
            canRead={draft.instructions.trim() !== ""}
            onRead={drawing.read}
          />
        </div>
      ) : view === "text" ? (
        // Sentences are read left to right and stop at a measure somebody can
        // follow; the canvas is looked at and takes what it is given.
        <div className="mx-auto w-full max-w-[820px]">
          <StepsTextView
            steps={steps}
            catalogue={catalogue}
            policies={policies}
            onChange={(at, over) => drawing.changeAt(at, over)}
            onAdd={() => drawing.insert("", steps.length)}
          />
        </div>
      ) : (
        <div className="flex h-[440px] overflow-hidden rounded-lg border border-border">
          <AgentCanvas
            steps={steps}
            selected={selected}
            onReorder={drawing.reorder}
            onSelect={drawing.setSelected}
            onDropTool={drawing.insert}
          />
          <StepInspector
            step={selected === undefined ? undefined : steps[selected]}
            at={selected}
            total={steps.length}
            onChange={drawing.change}
            onRemove={drawing.remove}
            tools={{ pack, catalogue, policies }}
          />
        </div>
      )}
    </div>
  );
}

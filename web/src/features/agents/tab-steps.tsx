import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { List, Plus, Workflow } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { StepStrip } from "@/features/agents/step-strip";
import { gateStops } from "@/features/agents/step-gating";
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

  /*
  Read once on arriving here, when there is prose and no stages yet.

  On opening the tab and never on opening the screen: this spends real money
  at the provider and counts against the assistant's daily ceiling, so it must
  follow an intention rather than a page load — somebody who came to change a
  cost ceiling must not pay for a reading they did not ask for. Opening the
  tab is asking to see the process, and deriving it is what showing it means
  when none was written down.

  Once, tracked in a ref: a second attempt on every render would charge for
  the same answer repeatedly, and an assistant that is switched off would be
  asked for ever.
  */
  const asked = useRef(false);
  useEffect(() => {
    if (asked.current || steps.length > 0) return;
    if (draft.instructions.trim() === "") return;
    asked.current = true;
    drawing.read();
  }, [draft.instructions, steps.length, drawing]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* The tab's own bar: full width, with a rule under it, so the choice
          of view and the way to add a step sit where every tab's controls sit
          rather than floating above the content as one more card. */}
      <div className="flex flex-wrap items-center gap-3 border-b border-border px-5 py-3">
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

      <div className="mx-auto w-full max-w-[820px] px-5 pt-4">
        <DriftWarning tools={silent} />
      </div>

      {steps.length === 0 ? (
        <div className="flex min-h-0 flex-1">
          <EmptyCanvas
            reading={drawing.reading}
            canRead={draft.instructions.trim() !== ""}
            onRead={drawing.read}
          />
        </div>
      ) : view === "text" ? (
        // Sentences are read left to right and stop at a measure somebody can
        // follow; the canvas is looked at and takes what it is given.
        <div className="mx-auto w-full max-w-[820px] overflow-y-auto px-5 pt-4 pb-10">
          <StepsTextView
            steps={steps}
            catalogue={catalogue}
            policies={policies}
            onEdit={(at) => {
              // Editing a stage is the inspector's job, and the inspector
              // lives in the strip: the pencil takes you there rather than
              // opening a second editor beside the first.
              drawing.setSelected(at);
              setView("flow");
            }}
            onAdd={() => drawing.insert("", steps.length)}
            onMove={drawing.reorder}
          />
        </div>
      ) : (
        <div className="flex min-h-0 flex-1">
          <StepStrip
            steps={steps}
            selected={selected}
            stops={(at) => {
              const step = steps[at];
              return step ? gateStops(step, catalogue, policies) : false;
            }}
            onSelect={drawing.setSelected}
            onAdd={() => drawing.insert("", steps.length)}
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

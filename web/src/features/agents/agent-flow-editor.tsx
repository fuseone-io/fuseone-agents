import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Sparkles } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { AgentCanvas } from "@/features/agents/agent-canvas";
import { StepInspector } from "@/features/agents/step-inspector";
import { StepRail } from "@/features/agents/step-rail";
import { useInterview } from "@/features/agents/interview-api";
import { problemMessage } from "@/lib/api/problem-message";
import type { AgentDefinition } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

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
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
}) {
  const { t } = useTranslation();
  const propose = useInterview();
  const [selected, setSelected] = useState<number | undefined>(undefined);

  const steps = draft.steps ?? [];
  const pack = draft.tools ?? [];

  const write = (next: AgentStep[]) => patch({ steps: next });

  const insert = (tool: string, at: number) => {
    const step: AgentStep = { name: "", reaches: tool ? [tool] : [] };
    const next = [...steps];
    next.splice(Math.min(at, next.length), 0, step);
    write(next);
    setSelected(Math.min(at, next.length - 1));
  };

  const reorder = (from: number, to: number) => {
    const next = [...steps];
    const [moved] = next.splice(from, 1);
    if (moved) next.splice(to, 0, moved);
    write(next);
    setSelected(to);
  };

  const read = () =>
    propose.mutate(
      { steps: draft.instructions },
      {
        onSuccess: (drafted) => {
          write(drafted.steps);
          setSelected(undefined);
          toast.success(t("agents.stepsProposed"), {
            description: t("agents.stepsProposedHint"),
          });
        },
        onError: (error) => toast.error(problemMessage(error, t)),
      },
    );

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2">
        <p className="text-2xs text-muted-foreground">
          {steps.length === 0
            ? t("agents.noStepsMeans")
            : t("agents.dragToReorder")}
        </p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="ml-auto h-8"
          disabled={propose.isPending || draft.instructions.trim() === ""}
          onClick={read}
        >
          <Sparkles className="size-3.5" aria-hidden />
          {steps.length === 0
            ? t("agents.readTheInstructions")
            : t("agents.readAgain")}
        </Button>
      </div>

      <div className="flex h-[420px] overflow-hidden rounded-lg border border-border">
        <StepRail pack={pack} />
        <AgentCanvas
          steps={steps}
          selected={selected}
          onReorder={reorder}
          onSelect={setSelected}
          onDropTool={insert}
        />
        <StepInspector
          step={selected === undefined ? undefined : steps[selected]}
          at={selected}
          pack={pack}
          onChange={(over) =>
            write(steps.map((s, i) => (i === selected ? { ...s, ...over } : s)))
          }
          onRemove={() => {
            write(steps.filter((_, i) => i !== selected));
            setSelected(undefined);
          }}
        />
      </div>
    </div>
  );
}

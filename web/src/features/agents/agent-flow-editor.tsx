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
import type { AgentDefinition, Policy, Tool } from "@/lib/api/client";
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
  catalogue,
  policies,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
  catalogue: Tool[];
  policies: Policy[];
}) {
  const { t } = useTranslation();
  const propose = useInterview();
  const [selected, setSelected] = useState<number | undefined>(undefined);

  const steps = draft.steps ?? [];
  const pack = draft.tools ?? [];

  const write = (next: AgentStep[]) => patch({ steps: next });

  /*
  Dropping a tool creates a stage that reaches it, and grants it if the agent
  did not hold it.

  The same authority the tools section of this form already carries, in the
  place somebody is actually thinking about the process. What it must not be
  is quiet: the toast says the pack grew, and the rail shows what every tool
  does, so `erp.transfer` never arrives as invisibly as `crm.lookup`.
  */
  const insert = (tool: string, at: number) => {
    const step: AgentStep = { name: "", reaches: tool ? [tool] : [] };
    const next = [...steps];
    next.splice(Math.min(at, next.length), 0, step);

    const granting = tool !== "" && !pack.includes(tool);
    patch({
      steps: next,
      ...(granting ? { tools: [...pack, tool] } : {}),
    });
    if (granting) {
      toast.info(t("agents.alsoGranted", { tool }));
    }
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
        {/* Only once there is something to re-read. While the canvas is
            empty the empty state carries the action, and two buttons saying
            the same thing is how somebody stops reading either.

            Never on open, either, and that is a cost decision rather than a
            preference: every reading spends real money at the provider and
            counts against the assistant's daily ceiling, so calling it on
            mount would charge somebody for arriving to fix a typo. */}
        {steps.length > 0 && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="ml-auto h-8"
            disabled={propose.isPending || draft.instructions.trim() === ""}
            onClick={read}
          >
            <Sparkles className="size-3.5" aria-hidden />
            {t("agents.readAgain")}
          </Button>
        )}
      </div>

      <div className="flex h-[420px] overflow-hidden rounded-lg border border-border">
        <StepRail catalogue={catalogue} pack={pack} />
        {steps.length === 0 ? (
          <EmptyCanvas
            reading={propose.isPending}
            canRead={draft.instructions.trim() !== ""}
            onRead={read}
          />
        ) : (
        <AgentCanvas
          steps={steps}
          selected={selected}
          onReorder={reorder}
          onSelect={setSelected}
          onDropTool={insert}
        />
        )}
        <StepInspector
          step={selected === undefined ? undefined : steps[selected]}
          at={selected}
          tools={{ pack, catalogue, policies }}
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

/**
 * An agent with no stages yet.
 *
 * The empty state carries the action rather than describing it, because the
 * canvas is where somebody is already looking — a line of prose here and a
 * button in the header above is how an empty screen stays empty.
 */
function EmptyCanvas({
  reading,
  canRead,
  onRead,
}: {
  reading: boolean;
  canRead: boolean;
  onRead: () => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 bg-background p-6 text-center">
      <p className="max-w-sm text-xs text-muted-foreground">
        {canRead ? t("agents.emptyCanvas") : t("agents.writeFirst")}
      </p>
      {canRead && (
        <Button type="button" size="sm" disabled={reading} onClick={onRead}>
          <Sparkles className="size-3.5" aria-hidden />
          {t("agents.readTheInstructions")}
        </Button>
      )}
    </div>
  );
}

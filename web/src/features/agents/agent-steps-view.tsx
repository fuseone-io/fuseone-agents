import { Plus, Sparkles } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { AgentCanvas } from "@/features/agents/agent-canvas";
import { StepRow } from "@/features/agents/step-row";
import { useInterview } from "@/features/agents/interview-api";
import { problemMessage } from "@/lib/api/problem-message";
import type { AgentDefinition } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

/**
 * The same instructions, read as the stages they describe.
 *
 * A view of the field above rather than a section of its own, which is what it
 * was and why it read as a second thing to fill in: an author who has written
 * their process in prose should not find an empty form underneath asking for
 * it again.
 *
 * Written by hand this was two descriptions of one process, and they drift:
 * the prose says one thing, the fields say another, and nobody can tell which
 * is true. So the instructions stay the single account and this is a reading
 * of them that a person corrects and keeps.
 *
 * Not derived silently, either. The prose is instruction to a model and a step
 * is a permission — `reaches` is what the Gate is meant to allow at that
 * moment — and a model's guess at where one ends is not a boundary anybody
 * should inherit without looking. What comes back is a proposal on screen,
 * and what the author leaves there is what gets published.
 *
 * The proposal cannot widen anything: the server reads every tool it names
 * against the catalogue and drops the ones that are not in it, however
 * confidently they were proposed.
 */
export function AgentStepsView({
  draft,
  patch,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
}) {
  const { t } = useTranslation();
  const propose = useInterview();
  const steps = draft.steps ?? [];
  const pack = draft.tools ?? [];

  const reorder = (from: number, to: number) => {
    const next = [...steps];
    const [moved] = next.splice(from, 1);
    if (moved) next.splice(to, 0, moved);
    patch({ steps: next });
  };

  const replace = (at: number, over: Partial<AgentStep>) =>
    patch({
      steps: steps.map((step, i) => (i === at ? { ...step, ...over } : step)),
    });

  const read = () =>
    propose.mutate(
      { steps: draft.instructions },
      {
        onSuccess: (drafted) => {
          patch({ steps: drafted.steps });
          toast.success(t("agents.stepsProposed"), {
            description: t("agents.stepsProposedHint"),
          });
        },
        onError: (error) => toast.error(problemMessage(error, t)),
      },
    );

  return (
    <div className="flex flex-col gap-3">
      {steps.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t("agents.noStepsMeans")}
        </p>
      ) : (
        <div className="flex flex-col gap-3">
          {/* Dragging reorders: there is nowhere to keep "the author put this
              box here", so what a card carries is its place in the sequence
              and the grid re-derives from it (NT-007 §2.1). */}
          <AgentCanvas steps={steps} onReorder={reorder} />

          {steps.map((step, at) => (
            <StepRow
              key={at}
              step={step}
              pack={pack}
              onChange={(over) => replace(at, over)}
              onRemove={() =>
                patch({ steps: steps.filter((_, i) => i !== at) })
              }
            />
          ))}
        </div>
      )}

      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={propose.isPending || draft.instructions.trim() === ""}
          onClick={read}
        >
          <Sparkles className="size-3.5" aria-hidden />
          {steps.length === 0
            ? t("agents.readTheInstructions")
            : t("agents.readAgain")}
        </Button>
        {/* Correcting a reading has to be possible, and adding one by hand is
            the same act. It is not the way in, though: an empty form invites
            somebody to describe the process a second time. */}
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => patch({ steps: [...steps, { name: "" }] })}
        >
          <Plus className="size-3.5" aria-hidden />
          {t("agents.addStep")}
        </Button>
      </div>
    </div>
  );
}

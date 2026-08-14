import { ListOrdered, Plus, Sparkles } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Section } from "@/features/policies/section";
import { StepRow } from "@/features/agents/step-row";
import { useInterview } from "@/features/agents/interview-api";
import { problemMessage } from "@/lib/api/problem-message";
import type { AgentDefinition } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

/**
 * The stages of the process, read out of the instructions rather than typed
 * beside them.
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
export function AgentStepsSection({
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
    <Section
      icon={ListOrdered}
      title={t("agents.steps")}
      hint={t("agents.stepsHint")}
    >
      {steps.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t("agents.noStepsMeans")}
        </p>
      ) : (
        <div className="flex flex-col gap-2">
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
    </Section>
  );
}

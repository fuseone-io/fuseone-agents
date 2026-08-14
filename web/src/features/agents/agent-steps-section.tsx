import { ListOrdered, Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Section } from "@/features/policies/section";
import { StepRow } from "@/features/agents/step-row";
import type { AgentDefinition } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

/**
 * The stages of the process, which are what the Gate is meant to obey.
 *
 * The capability pack is the ceiling and a step is the actual permission:
 * `reaches` is what a run may call while it sits at one. Declaring none is a
 * real answer and the common one — a single envelope holding the whole pack —
 * so this section starts empty and says so.
 *
 * A step is not a tool call. Summarising reaches nothing at all, and a step
 * that calls nothing is the agent thinking, which is why an empty one is
 * allowed to be saved.
 */
export function AgentStepsSection({
  draft,
  patch,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
}) {
  const { t } = useTranslation();
  const steps = draft.steps ?? [];
  const pack = draft.tools ?? [];

  const replace = (at: number, over: Partial<AgentStep>) =>
    patch({
      steps: steps.map((step, i) => (i === at ? { ...step, ...over } : step)),
    });

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

      <div>
        <Button
          type="button"
          variant="outline"
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

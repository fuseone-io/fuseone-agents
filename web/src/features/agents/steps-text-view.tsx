import { useTranslation } from "react-i18next";
import { GripVertical, UserRoundCheck } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Mono } from "@/components/shared/mono";
import { ruleFor } from "@/features/agents/tool-rule";
import type { Policy, Tool } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

/**
 * The sequence as sentences.
 *
 * The same steps the canvas draws, and never beside it: two editors of one
 * thing on screen at once means each gets half the width it needs and the
 * reader has to work out which is authoritative. They alternate.
 *
 * The metadata row under each sentence is read, not set — what the policy in
 * force will do with this stage's tools. Membership is edited here; the
 * verdict belongs to the policy that produced it.
 */
export function StepsTextView({
  steps,
  catalogue,
  policies,
  onChange,
  onAdd,
}: {
  steps: AgentStep[];
  catalogue: Tool[];
  policies: Policy[];
  onChange: (at: number, over: Partial<AgentStep>) => void;
  onAdd: () => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-2">
      {steps.map((step, at) => (
        <div
          key={at}
          className="flex items-start gap-3 rounded-lg border border-border bg-card px-3.5 py-3 shadow-xs"
        >
          <GripVertical
            className="mt-1.5 size-4 shrink-0 text-text-disabled"
            aria-hidden
          />
          <span className="mt-0.5 grid size-[22px] shrink-0 place-items-center rounded-md bg-muted text-2xs tabular-nums text-muted-foreground">
            {at + 1}
          </span>

          <div className="flex min-w-0 flex-1 flex-col gap-1.5">
            <Input
              value={step.name}
              onChange={(e) => onChange(at, { name: e.target.value })}
              placeholder={t("agents.stepName")}
              aria-label={t("agents.stepNumber", { number: at + 1 })}
              className="h-8 border-0 px-0 shadow-none focus-visible:ring-0"
            />
            <StepMeta step={step} catalogue={catalogue} policies={policies} />
          </div>
        </div>
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

/** What this stage reaches, and what the Gate will do about it. */
function StepMeta({
  step,
  catalogue,
  policies,
}: {
  step: AgentStep;
  catalogue: Tool[];
  policies: Policy[];
}) {
  const { t } = useTranslation();
  const reaches = step.reaches ?? [];

  if (reaches.length === 0) {
    return (
      <p className="text-2xs text-muted-foreground">{t("agents.thinksOnly")}</p>
    );
  }

  const asking = reaches.some((tool) => {
    const effect = catalogue.find((one) => one.toolId === tool)?.effect ?? "write";
    return ruleFor(tool, effect, policies).kind !== "allowed";
  });

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {reaches.map((tool) => (
        <Badge key={tool} variant="outline">
          <Mono className="text-2xs">{tool}</Mono>
        </Badge>
      ))}
      {asking && (
        <span className="flex items-center gap-1 text-2xs text-warning">
          <UserRoundCheck className="size-3" aria-hidden />
          {t("agents.mayStopHere")}
        </span>
      )}
    </div>
  );
}

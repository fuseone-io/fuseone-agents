import { useTranslation } from "react-i18next";
import { Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Mono } from "@/components/shared/mono";
import { StepGuardrails } from "@/features/agents/step-guardrails";
import type { Policy, Tool } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

/**
 * The selected stage, and the only place one is edited.
 *
 * A list of every step under the canvas said the same thing twice and made the
 * page as long as the process — twelve stages meant twelve open forms nobody
 * was looking at. One panel, whichever card is selected.
 */
export function StepInspector({
  step,
  at,
  onChange,
  onRemove,
  tools,
}: {
  step?: AgentStep;
  at?: number;
  onChange: (over: Partial<AgentStep>) => void;
  onRemove: () => void;
  /** The pack, the catalogue it came from, and the policies over it. */
  tools: { pack: string[]; catalogue: Tool[]; policies: Policy[] };
}) {
  const { t } = useTranslation();

  if (!step || at === undefined) {
    return (
      <div className="flex w-[280px] shrink-0 items-center border-l border-border p-4">
        <p className="text-xs text-muted-foreground">{t("agents.pickAStep")}</p>
      </div>
    );
  }

  const { pack, catalogue, policies } = tools;
  const reaches = step.reaches ?? [];
  const toggle = (tool: string) =>
    onChange({
      reaches: reaches.includes(tool)
        ? reaches.filter((one) => one !== tool)
        : [...reaches, tool],
    });

  return (
    <div className="flex w-[280px] shrink-0 flex-col gap-3 overflow-y-auto border-l border-border p-3">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="step-name" className="text-2xs uppercase tracking-label">
          {t("agents.stepNumber", { number: at + 1 })}
        </Label>
        <Input
          id="step-name"
          value={step.name}
          onChange={(e) => onChange({ name: e.target.value })}
          placeholder={t("agents.stepName")}
        />
      </div>

      <div className="flex flex-col gap-1.5">
        <span className="text-2xs uppercase tracking-label text-muted-foreground">
          {t("agents.mayReach")}
        </span>
        {/* From the pack, never typed: a step naming a tool the agent does not
            hold is a permission that reads as granted and is refused. */}
        <div className="flex flex-wrap gap-1.5">
          {pack.length === 0 ? (
            <span className="text-2xs text-muted-foreground">
              {t("agents.packEmptyForStep")}
            </span>
          ) : (
            pack.map((tool) => (
              <Button
                key={tool}
                type="button"
                variant="ghost"
                size="sm"
                className="h-6 px-1"
                onClick={() => toggle(tool)}
                aria-pressed={reaches.includes(tool)}
              >
                <Badge variant={reaches.includes(tool) ? "default" : "outline"}>
                  <Mono className="text-2xs">{tool}</Mono>
                </Badge>
              </Button>
            ))
          )}
        </div>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="step-stops" className="text-2xs uppercase tracking-label">
          {t("agents.stopsWhenLabel")}
        </Label>
        <Input
          id="step-stops"
          value={step.stopsWhen ?? ""}
          onChange={(e) => onChange({ stopsWhen: e.target.value })}
          placeholder={t("agents.stopsWhenPlaceholder")}
        />
      </div>

      <div className="flex flex-col gap-1.5">
        <span className="text-2xs uppercase tracking-label text-muted-foreground">
          {t("agents.whatTheGateDoes")}
        </span>
        <StepGuardrails
          reaches={reaches}
          catalogue={catalogue}
          policies={policies}
        />
      </div>

      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="mt-auto justify-start text-destructive hover:text-destructive"
        onClick={onRemove}
      >
        <Trash2 className="size-3.5" aria-hidden />
        {t("agents.removeStep")}
      </Button>
    </div>
  );
}

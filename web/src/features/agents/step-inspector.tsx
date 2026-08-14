import { useTranslation } from "react-i18next";
import { Trash2, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Mono } from "@/components/shared/mono";
import { AddToolPopover } from "@/features/agents/add-tool-popover";
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
  total,
  onChange,
  onRemove,
  tools,
}: {
  step?: AgentStep;
  at?: number;
  total: number;
  onChange: (over: Partial<AgentStep>) => void;
  onRemove: () => void;
  /** The pack, the catalogue it came from, and the policies over it. */
  tools: { pack: string[]; catalogue: Tool[]; policies: Policy[] };
}) {
  const { t } = useTranslation();

  if (!step || at === undefined) {
    return (
      <div className="flex w-[300px] shrink-0 items-center border-l border-border p-4">
        <p className="text-xs text-muted-foreground">{t("agents.pickAStep")}</p>
      </div>
    );
  }

  const { pack, catalogue, policies } = tools;
  const reaches = step.reaches ?? [];
  return (
    <div className="flex w-[300px] shrink-0 flex-col gap-3 overflow-y-auto border-l border-border p-4">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="step-name" className="text-2xs uppercase tracking-label">
          {t("agents.stepOf", { number: at + 1, total })}
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
        {/* Chips and a popover, never a parked catalogue: browsing eighty
            tools is a task with its own tab, and it was competing for width
            with the stage being edited. */}
        <div className="flex flex-wrap items-center gap-1.5">
          {reaches.map((tool) => (
            <Badge key={tool} variant="outline" className="gap-1 pr-1">
              <Mono className="text-2xs">{tool}</Mono>
              <button
                type="button"
                onClick={() =>
                  onChange({ reaches: reaches.filter((one) => one !== tool) })
                }
                aria-label={t("agents.stopReaching", { tool })}
                className="text-muted-foreground hover:text-foreground"
              >
                <X className="size-3" aria-hidden />
              </button>
            </Badge>
          ))}
          <AddToolPopover
            catalogue={catalogue}
            pack={pack}
            reaches={reaches}
            onPick={(tool) => onChange({ reaches: [...reaches, tool] })}
          />
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

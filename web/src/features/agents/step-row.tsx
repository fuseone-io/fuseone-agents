import { Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/shared/mono";
import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

/**
 * One stage: its name, what it may reach, and when it gives up.
 *
 * The tools are chosen from the agent's own pack rather than typed, because a
 * step can only ever narrow it — naming a tool the agent does not hold would
 * be a permission that reads as granted and is refused at the Gate.
 */
export function StepRow({
  step,
  pack,
  onChange,
  onRemove,
}: {
  step: AgentStep;
  pack: string[];
  onChange: (over: Partial<AgentStep>) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const reaches = step.reaches ?? [];

  const toggle = (tool: string) =>
    onChange({
      reaches: reaches.includes(tool)
        ? reaches.filter((one) => one !== tool)
        : [...reaches, tool],
    });

  return (
    <div className="flex flex-col gap-2 rounded-lg border border-border p-3">
      <div className="flex items-center gap-2">
        <Input
          value={step.name}
          onChange={(e) => onChange({ name: e.target.value })}
          placeholder={t("agents.stepName")}
          aria-label={t("agents.stepName")}
        />
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={onRemove}
          aria-label={t("agents.removeStep")}
        >
          <Trash2 className="size-4" aria-hidden />
        </Button>
      </div>

      <div className="flex flex-wrap items-center gap-1.5">
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

      <Input
        value={step.stopsWhen ?? ""}
        onChange={(e) => onChange({ stopsWhen: e.target.value })}
        placeholder={t("agents.stopsWhenPlaceholder")}
        aria-label={t("agents.stopsWhenLabel")}
      />
    </div>
  );
}

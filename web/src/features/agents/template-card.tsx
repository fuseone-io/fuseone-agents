import {
  AlertTriangle,
  FileSearch,
  LayoutTemplate,
  Scale,
  Ticket,
  type LucideIcon,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { AgentTemplate } from "@/features/agents/templates-api";

/**
 * The icon each template is recognised by.
 *
 * Mapped here rather than carried in the template, the way PAGE_ICONS is:
 * a template is a document an installation could write for itself, and putting
 * a lucide identifier inside one would make an agent definition depend on the
 * console's icon set. An id nobody mapped falls back rather than breaking.
 */
const ICONS: Record<string, LucideIcon> = {
  "alert-response": AlertTriangle,
  "lead-qualification": FileSearch,
  reconciliation: Scale,
  "ticket-triage": Ticket,
};

/**
 * One template, as a card that is itself the button.
 *
 * The whole card, not a button inside it: the card carries a name, a sentence
 * and two facts, and every one of them is a reason to pick this one — a target
 * the size of the words in the corner makes the reader aim.
 */
export function TemplateCard({
  template,
  chosen,
  onChoose,
}: {
  template: AgentTemplate;
  chosen: boolean;
  onChoose: () => void;
}) {
  const { t } = useTranslation();
  const Icon = ICONS[template.id] ?? LayoutTemplate;

  return (
    <Button
      type="button"
      variant="outline"
      onClick={onChoose}
      aria-pressed={chosen}
      className={cn(
        "h-auto min-w-0 flex-col items-start gap-2 rounded-xl bg-card p-3 text-left shadow-sm",
        "hover:border-border-strong hover:bg-muted",
        chosen && "border-primary bg-accent hover:bg-accent",
      )}
    >
      <div className="flex w-full items-center gap-2">
        <span className="flex size-6 shrink-0 items-center justify-center rounded-[7px] border bg-muted text-muted-foreground">
          <Icon className="size-3.5" aria-hidden />
        </span>
        <span className="min-w-0 truncate text-sm font-medium">
          {template.name}
        </span>
      </div>

      {/* A minimum height rather than a clamp: four cards side by side whose
          footers do not line up read as four different things. */}
      <span className="min-h-8 text-xs text-muted-foreground">
        {template.summary}
      </span>

      <span className="flex w-full items-center gap-1.5 border-t border-border-subtle pt-2">
        <span className="font-mono text-2xs tabular-nums text-muted-foreground">
          {t("agents.templateNeeds", { count: template.needs.length })}
        </span>
        {template.area && (
          <span className="ml-auto min-w-0 truncate text-2xs text-muted-foreground">
            {template.area}
          </span>
        )}
      </span>
    </Button>
  );
}

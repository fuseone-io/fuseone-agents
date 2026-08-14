import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Brain, Check, GripVertical } from "lucide-react";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Mono } from "@/components/shared/mono";
import { EffectBadge } from "@/features/agents/effect-badge";
import type { Tool } from "@/lib/api/client";

/**
 * What can be dragged onto the canvas.
 *
 * The organisation's catalogue, with this agent's pack marked. Dragging one it
 * does not hold grants it — the same authority the tools section of this form
 * already carries, in the place somebody is actually thinking about the
 * process, rather than a second trip up the page.
 *
 * Which is why the effect is on every row and not only on the ones already
 * granted. Widening an agent by dragging is fine; widening it *without seeing
 * what the tool does* is not, and `erp.transfer` arriving as quietly as
 * `crm.lookup` is exactly the kind of screen this platform argues against.
 *
 * A stage that calls nothing sits at the top because it is a real answer and
 * an easy one to miss: a rail of tools alone teaches that every step is a tool
 * call, and the simplest agent here has one that is not.
 */
export function StepRail({
  catalogue,
  pack,
}: {
  catalogue: Tool[];
  pack: string[];
}) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");

  const term = query.trim().toLowerCase();
  const matching = catalogue.filter(
    (tool) =>
      tool.toolId.toLowerCase().includes(term) ||
      (tool.description ?? "").toLowerCase().includes(term),
  );

  return (
    <div className="flex w-[210px] shrink-0 flex-col gap-2 border-r border-border p-2">
      <Input
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder={t("agents.searchTheCatalogue")}
        className="h-8"
        aria-label={t("agents.searchTheCatalogue")}
      />

      <ScrollArea className="flex-1">
        <div className="flex flex-col gap-0.5 pr-2">
          <Row label={t("agents.aThinkingStep")} icon={<Brain className="size-3.5" />} />
          {matching.map((tool) => (
            <Row
              key={tool.toolId}
              tool={tool.toolId}
              label={tool.toolId}
              effect={tool.effect}
              held={pack.includes(tool.toolId)}
              mono
            />
          ))}
        </div>
      </ScrollArea>
    </div>
  );
}

/**
 * One draggable row.
 *
 * The tool travels in the drag payload rather than in component state: a drop
 * lands after a re-render, and state read on the other side would be whatever
 * was hovered last.
 */
function Row({
  tool,
  label,
  icon,
  effect,
  held,
  mono,
}: {
  tool?: string;
  label: string;
  icon?: React.ReactNode;
  effect?: Tool["effect"];
  held?: boolean;
  mono?: boolean;
}) {
  const { t } = useTranslation();

  return (
    <div
      draggable
      onDragStart={(e) => {
        e.dataTransfer.setData("application/fuseone-step", tool ?? "");
        e.dataTransfer.effectAllowed = "copy";
      }}
      className="group flex cursor-grab items-center gap-1.5 rounded-md px-1.5 py-1.5 hover:bg-muted"
    >
      <span className="grid size-[22px] shrink-0 place-items-center rounded-md border border-border bg-muted text-muted-foreground">
        {icon ?? <Check className={held ? "size-3" : "size-3 opacity-0"} aria-hidden />}
      </span>
      {mono ? (
        <Mono className="min-w-0 flex-1 truncate text-2xs">{label}</Mono>
      ) : (
        <span className="min-w-0 flex-1 truncate text-xs">{label}</span>
      )}
      {effect && <EffectBadge effect={effect} />}
      {tool !== undefined && !held && (
        <span className="sr-only">{t("agents.notInThePackYet")}</span>
      )}
      <GripVertical
        className="size-3.5 shrink-0 text-muted-foreground opacity-0 group-hover:opacity-100"
        aria-hidden
      />
    </div>
  );
}

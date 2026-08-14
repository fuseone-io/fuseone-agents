import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Brain, GripVertical, Wrench } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Mono } from "@/components/shared/mono";
import { ScrollArea } from "@/components/ui/scroll-area";

/**
 * What can be dragged onto the canvas.
 *
 * The agent's own capability pack, and nothing else. A rail of components
 * looks like a catalogue of things you may have, so this one holds exactly
 * what this agent was granted — dragging cannot widen it, because there is
 * nothing wider in here to drag.
 *
 * A stage that calls nothing sits at the top because it is a real answer and
 * an easy one to miss: the agent reading, deciding, summarising. A rail of
 * tools alone would teach that every step is a tool call.
 */
export function StepRail({ pack }: { pack: string[] }) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");

  const matching = pack.filter((tool) =>
    tool.toLowerCase().includes(query.trim().toLowerCase()),
  );

  return (
    <div className="flex w-[196px] shrink-0 flex-col gap-2 border-r border-border p-2">
      <Input
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder={t("agents.searchThePack")}
        className="h-8"
        aria-label={t("agents.searchThePack")}
      />

      <ScrollArea className="flex-1">
        <div className="flex flex-col gap-0.5 pr-2">
          <RailItem icon={<Brain className="size-3.5" />} label={t("agents.aThinkingStep")} />
          {matching.map((tool) => (
            <RailItem
              key={tool}
              tool={tool}
              icon={<Wrench className="size-3.5" />}
              label={tool}
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
 * The tool travels in the drag payload rather than in component state,
 * because a drop can land after a re-render and state read on the other side
 * would be whatever was last hovered.
 */
function RailItem({
  tool,
  icon,
  label,
  mono,
}: {
  tool?: string;
  icon: React.ReactNode;
  label: string;
  mono?: boolean;
}) {
  return (
    <div
      draggable
      onDragStart={(e) => {
        e.dataTransfer.setData("application/fuseone-step", tool ?? "");
        e.dataTransfer.effectAllowed = "copy";
      }}
      className="group flex cursor-grab items-center gap-2 rounded-md px-1.5 py-1.5 hover:bg-muted"
    >
      <span className="grid size-[22px] shrink-0 place-items-center rounded-md border border-border bg-muted text-muted-foreground">
        {icon}
      </span>
      {mono ? (
        <Mono className="min-w-0 flex-1 truncate text-2xs">{label}</Mono>
      ) : (
        <span className="min-w-0 flex-1 truncate text-xs">{label}</span>
      )}
      <GripVertical
        className="size-3.5 shrink-0 text-muted-foreground opacity-0 group-hover:opacity-100"
        aria-hidden
      />
    </div>
  );
}

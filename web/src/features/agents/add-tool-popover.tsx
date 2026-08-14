import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Mono } from "@/components/shared/mono";
import { EffectBadge } from "@/features/agents/effect-badge";
import type { Tool } from "@/lib/api/client";

/**
 * Adding a tool to the stage being edited.
 *
 * Summoned rather than parked. A catalogue permanently beside the canvas was a
 * browse-and-search task competing for width with the thing being edited, and
 * browsing eighty tools is a job with its own tab.
 *
 * It offers the whole catalogue, and picking one the agent does not hold
 * grants it — the same authority the tools tab carries. Which is why the
 * effect is on every row: `erp.transfer` must not arrive as quietly as
 * `crm.lookup`.
 */
export function AddToolPopover({
  catalogue,
  pack,
  reaches,
  onPick,
}: {
  catalogue: Tool[];
  pack: string[];
  reaches: string[];
  onPick: (tool: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  const offered = catalogue.filter((tool) => !reaches.includes(tool.toolId));

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-6 border-dashed px-2 text-2xs"
        >
          <Plus className="size-3" aria-hidden />
          {t("common.add")}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-[300px] p-0">
        <Command>
          <CommandInput placeholder={t("agents.searchTheCatalogue")} />
          <CommandList>
            <CommandEmpty>{t("agents.noToolMatches")}</CommandEmpty>
            {offered.map((tool) => (
              <CommandItem
                key={tool.toolId}
                value={`${tool.toolId} ${tool.description ?? ""}`}
                onSelect={() => {
                  onPick(tool.toolId);
                  setOpen(false);
                }}
                className="gap-2"
              >
                <Mono className="min-w-0 flex-1 truncate text-2xs">
                  {tool.toolId}
                </Mono>
                {!pack.includes(tool.toolId) && (
                  <span className="text-2xs text-muted-foreground">
                    {t("agents.willGrant")}
                  </span>
                )}
                <EffectBadge effect={tool.effect} />
              </CommandItem>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

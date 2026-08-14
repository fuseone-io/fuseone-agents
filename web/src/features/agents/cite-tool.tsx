import { useTranslation } from "react-i18next";
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from "@/components/ui/popover";
import { Mono } from "@/components/shared/mono";
import { EffectBadge } from "@/features/agents/effect-badge";
import type { Tool } from "@/lib/api/client";

/**
 * Citing a tool while writing, opened by typing `@`.
 *
 * What it inserts is the bare identifier — the same characters somebody could
 * have typed — because that is what the model receives. The chip around it is
 * a rendering, and an editor that inserted anything else would be writing
 * something the payload does not contain.
 *
 * It offers the whole catalogue rather than this agent's pack: prose is
 * allowed to mention a tool the agent does not hold, and that sentence is
 * exactly the one worth marking rather than preventing.
 */
export function CiteTool({
  open,
  catalogue,
  onPick,
  onClose,
  children,
}: {
  open: boolean;
  catalogue: Tool[];
  onPick: (tool: string) => void;
  onClose: () => void;
  children: React.ReactNode;
}) {
  const { t } = useTranslation();

  return (
    <Popover open={open} onOpenChange={(next) => !next && onClose()}>
      <PopoverAnchor asChild>{children}</PopoverAnchor>
      <PopoverContent
        align="start"
        className="w-[300px] p-0"
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <Command>
          <CommandInput placeholder={t("agents.citeATool")} autoFocus />
          <CommandList>
            <CommandEmpty>{t("agents.noToolMatches")}</CommandEmpty>
            {catalogue.map((tool) => (
              <CommandItem
                key={tool.toolId}
                value={`${tool.toolId} ${tool.description ?? ""}`}
                onSelect={() => onPick(tool.toolId)}
                className="gap-2"
              >
                <Mono className="min-w-0 flex-1 truncate text-2xs">
                  {tool.toolId}
                </Mono>
                <EffectBadge effect={tool.effect} />
              </CommandItem>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

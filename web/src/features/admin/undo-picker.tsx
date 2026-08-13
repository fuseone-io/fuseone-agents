import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, ChevronsUpDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { cn } from "@/lib/utils";
import type { Tool } from "@/features/admin/api";

/**
 * Which tool takes this one back.
 *
 * Searchable because an installation has as many tools as its servers offer,
 * and the one that undoes a tool is rarely next to it alphabetically. A tool
 * cannot undo itself, and nothing here invents an answer: leaving it empty is
 * the honest ruling for an act that cannot be taken back by machine.
 */
export function UndoPicker({
  tools,
  value,
  onChange,
  self,
}: {
  tools: Tool[];
  value: string;
  onChange: (value: string) => void;
  /** The tool being ruled on, which is never its own undo. */
  self: string;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const options = tools.filter((tool) => tool.toolId !== self);

  function pick(toolId: string) {
    onChange(toolId === value ? "" : toolId);
    setOpen(false);
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id="compensatedBy"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className="justify-between font-normal"
        >
          <span className={cn(!value && "text-muted-foreground")}>
            {value || t("admin.noUndo")}
          </span>
          <ChevronsUpDown aria-hidden className="size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>

      <PopoverContent className="w-(--radix-popover-trigger-width) p-0">
        <Command>
          <CommandInput placeholder={t("admin.searchTools")} />
          <CommandList>
            <CommandEmpty>{t("admin.noToolsFound")}</CommandEmpty>
            <CommandGroup>
              {options.map((tool) => (
                <CommandItem
                  key={tool.toolId}
                  value={tool.toolId}
                  onSelect={pick}
                >
                  <Check
                    aria-hidden
                    className={cn(
                      "size-4",
                      value === tool.toolId ? "opacity-100" : "opacity-0",
                    )}
                  />
                  <span className="font-mono text-xs">{tool.toolId}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

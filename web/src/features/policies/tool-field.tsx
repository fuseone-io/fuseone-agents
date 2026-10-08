import { Check, ChevronsUpDown } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { useTools } from "@/features/admin/api";
import { Labelled } from "@/features/policies/section";
import { cn } from "@/lib/utils";

/**
 * The tool a rule covers, offered from what the platform already governs.
 *
 * The suggestions are the governed tool ids; picking fills the field, and
 * typing stays just as valid — a glob (`cloudflare.*`) is a legitimate rule
 * and no listing can offer every id a policy may legitimately name. The
 * listing is asked for lazily, when the suggestions are opened, and a
 * refusal leaves a plain input: the page is visible to roles the tool
 * listing refuses.
 */
export function ToolField({
  value,
  onChange,
}: {
  value: string;
  onChange: (resource: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState("");
  const tools = useTools(open);
  const suggestions = (tools.data?.items ?? []).map((tool) => ({
    value: tool.toolId,
    hint: tool.effect ?? "",
  }));

  return (
    <Labelled label={t("admin.tool")} htmlFor="resource">
      <div className="flex min-w-0 items-center gap-1">
        <Input
          id="resource"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={t("policies.resourcePlaceholder")}
          className="min-w-0 flex-1 font-mono"
        />
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="icon"
              role="combobox"
              aria-expanded={open}
              aria-label={t("policies.suggestTool")}
              className="size-9 shrink-0"
            >
              <ChevronsUpDown className="size-3.5 opacity-60" aria-hidden />
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" className="w-96 p-0">
            <Command>
              <CommandInput
                value={typed}
                onValueChange={setTyped}
                placeholder={t("policies.suggestFilter")}
                className="font-mono"
              />
              <CommandList>
                <CommandEmpty className="p-2">
                  <p className="text-xs text-muted-foreground">
                    {t("policies.suggestTypeFreely")}
                  </p>
                </CommandEmpty>
                <CommandGroup>
                  {suggestions.map((tool) => (
                    <CommandItem
                      key={tool.value}
                      value={tool.value}
                      className="font-mono text-xs"
                      onSelect={() => {
                        onChange(tool.value);
                        setOpen(false);
                      }}
                    >
                      <Check
                        className={cn(
                          "size-3.5",
                          tool.value === value ? "opacity-100" : "opacity-0",
                        )}
                      />
                      <span className="truncate">{tool.value}</span>
                      {tool.hint && (
                        <span className="ml-auto pl-2 text-muted-foreground">
                          {tool.hint}
                        </span>
                      )}
                    </CommandItem>
                  ))}
                </CommandGroup>
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>
      </div>
    </Labelled>
  );
}

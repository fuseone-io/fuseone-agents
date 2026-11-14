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
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { cn } from "@/lib/utils";

/** One value a field can offer, with a hint shown beside it. */
export type SuggestOption = { value: string; hint?: string };

/**
 * The suggestions beside a free-text input: a combobox button that opens a
 * filterable list, and picking fills the input. It never replaces the input,
 * so a value nothing lists stays just as valid — the caller lays out its own
 * `Input` and puts this beside it.
 *
 * `onOpen` fires when the list opens, for a listing asked for lazily.
 */
export function SuggestPopover({
  label,
  options,
  selected,
  onPick,
  onOpen,
}: {
  label: string;
  options: SuggestOption[];
  selected: string;
  onPick: (value: string) => void;
  onOpen?: () => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        if (next) onOpen?.();
        setOpen(next);
      }}
    >
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="icon"
          role="combobox"
          aria-expanded={open}
          aria-label={label}
          className="shrink-0"
        >
          <ChevronsUpDown className="size-3.5 opacity-60" aria-hidden />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 p-0">
        <Command>
          <CommandInput
            placeholder={t("common.suggestFilter")}
            className="font-mono"
          />
          <CommandList>
            <CommandEmpty className="p-2">
              <p className="text-xs text-muted-foreground">
                {t("common.suggestTypeFreely")}
              </p>
            </CommandEmpty>
            <CommandGroup>
              {options.map((option) => (
                <CommandItem
                  key={option.value}
                  value={option.value}
                  className="font-mono text-xs"
                  onSelect={() => {
                    onPick(option.value);
                    setOpen(false);
                  }}
                >
                  <Check
                    className={cn(
                      "size-3.5",
                      option.value === selected ? "opacity-100" : "opacity-0",
                    )}
                  />
                  <span className="truncate">{option.value}</span>
                  {option.hint && (
                    <span className="ml-auto truncate pl-2 text-muted-foreground">
                      {option.hint}
                    </span>
                  )}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

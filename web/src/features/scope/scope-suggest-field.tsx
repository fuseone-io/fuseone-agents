import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronsUpDown } from "lucide-react";
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
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import type { ScopeSelectField } from "@/features/scope/scope-select";

/** What a typed scope field suggests: the value it writes, and how it reads. */
export type ScopeSuggestion = { value: string; label: string; hint?: string };

export type ScopeSuggestFieldConfig = ScopeSelectField & {
  /** The accessible name of the button that opens the suggestions. */
  suggest: string;
  placeholder?: string;
};

/**
 * A company or an area typed, with what is known suggested beside it. For
 * when the full company list cannot be read: nothing then proves a typed
 * value wrong, and the server is the one that decides.
 */
export function ScopeSuggestField({
  field,
  value,
  suggestions,
  disabled,
  onChange,
}: {
  field: ScopeSuggestFieldConfig;
  value: string;
  suggestions: ScopeSuggestion[];
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const errorId = `${field.id}-error`;
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <Label htmlFor={field.id} className={field.labelClass}>
        {field.label}
      </Label>
      <div className="flex min-w-0 items-center gap-1">
        <Input
          id={field.id}
          value={value}
          disabled={disabled}
          placeholder={field.placeholder}
          autoComplete="off"
          aria-invalid={field.error ? true : undefined}
          aria-describedby={field.error ? errorId : undefined}
          className="h-9 min-w-0 flex-1 font-mono"
          onChange={(event) => onChange(event.target.value)}
        />
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="icon"
              role="combobox"
              aria-expanded={open}
              aria-label={field.suggest}
              disabled={disabled}
              className="size-9 shrink-0"
            >
              <ChevronsUpDown className="size-3.5 opacity-60" aria-hidden />
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" className="w-80 p-0">
            <Command>
              <CommandInput className="font-mono" />
              <CommandList>
                <CommandEmpty className="p-2 text-xs text-muted-foreground">
                  {t("scope.noSuggestions")}
                </CommandEmpty>
                <CommandGroup>
                  {suggestions.map((one) => (
                    <CommandItem
                      key={one.value}
                      value={`${one.label} ${one.hint ?? ""}`}
                      className="text-xs"
                      onSelect={() => {
                        onChange(one.value);
                        setOpen(false);
                      }}
                    >
                      <span className="truncate font-mono">{one.label}</span>
                      {one.hint && (
                        <span className="ml-auto truncate pl-2 text-muted-foreground">
                          {one.hint}
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
      {field.error && (
        <p id={errorId} className="text-sm text-destructive">
          {t(field.error)}
        </p>
      )}
    </div>
  );
}

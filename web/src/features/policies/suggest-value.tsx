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
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import type { Suggestion } from "@/features/policies/suggestion";
import type { PolicyCondition } from "@/lib/api/client";
import { cn } from "@/lib/utils";

/**
 * A combobox whose suggestions help and never refuse.
 *
 * Picking fills the value; typing anything else is just as valid, which is
 * what keeps a policy able to name an agent that is not deployed yet. Under
 * the `in` operator the value is a comma-separated set, so picking appends to
 * the set instead of replacing it — and picking a member again removes it,
 * which is how a set control is expected to behave.
 */
export function SuggestValue({
  condition,
  label,
  suggestions,
  onChange,
}: {
  condition: PolicyCondition;
  label: string;
  suggestions: Suggestion[];
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState("");
  const set = condition.op === "in";
  const members = condition.value
    .split(",")
    .map((one) => one.trim())
    .filter((one) => one !== "");

  const pick = (value: string) => {
    if (!set) {
      onChange(value);
      setOpen(false);
      return;
    }
    const next = members.includes(value)
      ? members.filter((one) => one !== value)
      : [...members, value];
    onChange(next.join(","));
  };

  return (
    <div className="flex min-w-0 items-center gap-1">
      <div className="min-w-0 flex-1">
        <Label htmlFor={`value-${label}`} className="sr-only">
          {label}
        </Label>
        <Input
          id={`value-${label}`}
          value={condition.value}
          onChange={(e) => onChange(e.target.value)}
          className="h-[34px] font-mono text-xs"
        />
      </div>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size="icon"
            role="combobox"
            aria-expanded={open}
            aria-label={t("policies.suggestValue")}
            className="size-[34px] shrink-0"
          >
            <ChevronsUpDown className="size-3.5 opacity-60" aria-hidden />
          </Button>
        </PopoverTrigger>
        <PopoverContent align="end" className="w-72 p-0">
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
                {suggestions.map((suggestion) => {
                  const chosen = set
                    ? members.includes(suggestion.value)
                    : condition.value === suggestion.value;
                  return (
                    <CommandItem
                      key={suggestion.value}
                      value={suggestion.value}
                      className="font-mono text-xs"
                      onSelect={() => pick(suggestion.value)}
                    >
                      <Check
                        className={cn(
                          "size-3.5",
                          chosen ? "opacity-100" : "opacity-0",
                        )}
                      />
                      <span className="truncate">{suggestion.value}</span>
                      {suggestion.hint && (
                        <span className="ml-auto truncate pl-2 text-muted-foreground">
                          {suggestion.hint}
                        </span>
                      )}
                    </CommandItem>
                  );
                })}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  );
}

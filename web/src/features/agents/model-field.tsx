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

/**
 * The model, offered rather than dictated.
 *
 * A combobox and not a select, because the contract says so about the list it
 * serves: the models a provider is known to serve are *suggestions*, since a
 * list shipped in a binary ages between releases and one that refused a model
 * released last week would be worse than no list at all.
 *
 * So the known ones are one keystroke away and anything can still be typed.
 * Before this the field was a bare text input and every author had to know, and
 * spell, an identifier nothing on the screen offered.
 */
export function ModelField({
  value,
  options,
  onChange,
  id,
}: {
  value: string;
  options: string[];
  onChange: (model: string) => void;
  id: string;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  // Controlled, so what was typed is a value this component holds rather than
  // something it reads back out of the DOM at render time.
  const [typed, setTyped] = useState("");

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className="justify-between font-mono font-normal"
        >
          <span className={cn(!value && "text-muted-foreground")}>
            {value || t("agents.pickModel")}
          </span>
          <ChevronsUpDown className="size-3.5 opacity-50" />
        </Button>
      </PopoverTrigger>

      <PopoverContent className="w-[--radix-popover-trigger-width] p-0">
        <Command>
          {/* Typing filters the known ones and is itself a valid answer, which
              is why the empty state offers what was typed rather than saying
              nothing was found. */}
          <CommandInput
            value={typed}
            onValueChange={setTyped}
            placeholder={t("agents.modelPlaceholder")}
            className="font-mono"
          />
          <CommandList>
            <CommandEmpty className="p-2">
              {typed.trim() ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="w-full justify-start font-mono"
                  onClick={() => {
                    onChange(typed.trim());
                    setOpen(false);
                  }}
                >
                  {t("agents.useTyped", { model: typed.trim() })}
                </Button>
              ) : (
                <p className="text-xs text-muted-foreground">
                  {t("agents.typeAModel")}
                </p>
              )}
            </CommandEmpty>
            <CommandGroup>
              {options.map((model) => (
                <CommandItem
                  key={model}
                  value={model}
                  className="font-mono"
                  onSelect={() => {
                    onChange(model);
                    setOpen(false);
                  }}
                >
                  <Check
                    className={cn(
                      "size-3.5",
                      model === value ? "opacity-100" : "opacity-0",
                    )}
                  />
                  {model}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

/**
 * Which models to offer for a provider.
 *
 * What the provider itself declares, and the preset's list as a fallback: a
 * provider configured before the platform knew about it has no models of its
 * own, and an empty list would send the author back to guessing an identifier.
 */
export function modelsFor(
  provider: string,
  configured: { name: string; models?: string[] }[],
  presets: { name: string; models?: string[] }[],
): string[] {
  const chosen = configured.find((p) => p.name === provider);
  if (chosen?.models?.length) return chosen.models;
  return presets.find((p) => p.name === provider)?.models ?? [];
}

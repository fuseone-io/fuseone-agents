import { Check, ChevronsUpDown } from "lucide-react";
import { useMemo, useState } from "react";
import { useController, type UseFormReturn } from "react-hook-form";
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
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import type { MemoryFormValues } from "@/features/memory/memory-form-schema";
import { useScopes, type RegisteredScope } from "@/features/scope/api";
import { cn } from "@/lib/utils";

/** A valid company/area pair the caller may reach, chosen as one unit. */
export function MemoryScopeField({
  form,
}: {
  form: UseFormReturn<MemoryFormValues>;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const company = useController({ control: form.control, name: "company" });
  const area = useController({ control: form.control, name: "area" });
  const scopes = useScopes();
  const groups = useMemo(
    () => scopesByCompany(scopes.data?.items ?? []),
    [scopes.data?.items],
  );
  const value =
    company.field.value && area.field.value
      ? `${company.field.value}/${area.field.value}`
      : "";

  function pick(scope: RegisteredScope) {
    company.field.onChange(scope.company);
    area.field.onChange(scope.area);
    setOpen(false);
  }

  return (
    <div className="grid gap-2">
      <Label htmlFor="memory-scope">{t("memory.scope")}</Label>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            id="memory-scope"
            type="button"
            variant="outline"
            role="combobox"
            aria-expanded={open}
            aria-invalid={!!(company.fieldState.error || area.fieldState.error)}
            className="w-full justify-between font-normal"
          >
            <span
              className={cn(
                "min-w-0 truncate font-mono",
                !value && "font-sans text-muted-foreground",
              )}
            >
              {value ||
                (scopes.isLoading
                  ? t("memory.loadingScopes")
                  : t("memory.chooseScope"))}
            </span>
            <ChevronsUpDown aria-hidden className="size-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent
          align="start"
          className="w-(--radix-popover-trigger-width) p-0"
        >
          <Command>
            <CommandInput placeholder={t("memory.searchScopes")} />
            <CommandList>
              <CommandEmpty>
                {scopes.isError
                  ? t("memory.scopesUnavailable")
                  : t("memory.noScopesAvailable")}
              </CommandEmpty>
              {groups.map(([groupCompany, options]) => (
                <CommandGroup key={groupCompany} heading={groupCompany}>
                  {options.map((scope) => {
                    const option = `${scope.company}/${scope.area}`;
                    return (
                      <CommandItem
                        key={option}
                        value={`${option} ${scope.label ?? ""}`}
                        onSelect={() => pick(scope)}
                      >
                        <Check
                          aria-hidden
                          className={cn(
                            "size-4",
                            value === option ? "opacity-100" : "opacity-0",
                          )}
                        />
                        <span className="min-w-0">
                          <span className="block truncate font-mono text-xs">
                            {scope.area}
                          </span>
                          {scope.label && scope.label !== scope.area && (
                            <span className="block truncate text-xs text-muted-foreground">
                              {scope.label}
                            </span>
                          )}
                        </span>
                      </CommandItem>
                    );
                  })}
                </CommandGroup>
              ))}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  );
}

function scopesByCompany(
  scopes: RegisteredScope[],
): [string, RegisteredScope[]][] {
  const grouped = new Map<string, RegisteredScope[]>();
  for (const scope of scopes) {
    const options = grouped.get(scope.company) ?? [];
    options.push(scope);
    grouped.set(scope.company, options);
  }
  return [...grouped.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([company, options]) => [
      company,
      [...options].sort((left, right) => left.area.localeCompare(right.area)),
    ]);
}

import { Check, ChevronsUpDown } from "lucide-react";
import { useEffect, useState } from "react";
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
import { usePeople } from "@/features/admin/people-api";
import { useMe } from "@/features/session/api";
import { Labelled } from "@/features/policies/section";
import { cn } from "@/lib/utils";

/**
 * Who answers for the rule.
 *
 * Owner is metadata the Gate never reads, so the field must not pretend to be
 * authority: free text always works. What it gains here is convenience — a new
 * policy starts owned by whoever is writing it, and the directory is offered
 * to whoever may read it.
 *
 * The directory query runs only after the suggestions are asked for, and a
 * refusal leaves a plain input with no error on screen: the page is visible to
 * roles that only read policies, and a 403 painted over a working field would
 * send them to debug a platform behaving exactly as intended.
 */
export function OwnerField({
  creating,
  value,
  onChange,
}: {
  creating: boolean;
  value: string;
  onChange: (owner: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState("");
  const me = useMe();
  const people = usePeople(open);

  // A new policy starts owned by its author. Once only, and never overwriting
  // something already typed: a default that fights the person loses.
  useEffect(() => {
    if (creating && value === "" && me.data) {
      onChange(me.data.display || me.data.id);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [creating, me.data]);

  const suggestions = (people.data?.items ?? []).map((person) => ({
    value: person.display || person.id,
    hint: person.display ? person.id : "",
  }));

  return (
    <Labelled label={t("policies.owner")} htmlFor="owner">
      <div className="flex min-w-0 items-center gap-1">
        <Input
          id="owner"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="min-w-0 flex-1"
        />
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="icon"
              role="combobox"
              aria-expanded={open}
              aria-label={t("policies.suggestOwner")}
              className="size-9 shrink-0"
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
              />
              <CommandList>
                <CommandEmpty className="p-2">
                  <p className="text-xs text-muted-foreground">
                    {t("policies.suggestTypeFreely")}
                  </p>
                </CommandEmpty>
                <CommandGroup>
                  {suggestions.map((person) => (
                    <CommandItem
                      key={person.value + person.hint}
                      value={person.value}
                      onSelect={() => {
                        onChange(person.value);
                        setOpen(false);
                      }}
                    >
                      <Check
                        className={cn(
                          "size-3.5",
                          person.value === value ? "opacity-100" : "opacity-0",
                        )}
                      />
                      <span className="truncate">{person.value}</span>
                      {person.hint && (
                        <span className="ml-auto truncate pl-2 font-mono text-xs text-muted-foreground">
                          {person.hint}
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

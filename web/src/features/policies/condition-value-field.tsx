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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAgents } from "@/features/agents/api";
import { useScopes } from "@/features/scope/api";
import type { PolicyCondition } from "@/lib/api/client";
import { cn } from "@/lib/utils";

/**
 * The value of a condition, offered rather than dictated.
 *
 * Which control appears is decided by the row's field: an agent id comes from
 * the agents the platform already lists, an area from the declared scopes, a
 * tool effect from the Gate's own enum. Everything the platform cannot know —
 * tool globs, labels, argument paths — stays the mono input it always was.
 *
 * Suggestions never become a closed set: a policy may legitimately name an
 * agent that does not exist yet, so the combobox always accepts what was
 * typed. The one exception is tool.effect, whose domain is the Gate's enum
 * and not a list that ages — but even there an undeclared stored value is
 * shown as its own item rather than silently dropped.
 */
export function ConditionValueField({
  condition,
  index,
  onChange,
}: {
  condition: PolicyCondition;
  index: number;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const label = t("policies.conditionValue", { n: index + 1 });

  switch (condition.field) {
    case "tool.effect":
      return (
        <EffectValue
          value={condition.value}
          label={label}
          onChange={onChange}
        />
      );
    case "agent.id":
      return (
        <AgentValue condition={condition} label={label} onChange={onChange} />
      );
    case "scope.area":
    case "scope.company":
      return (
        <ScopeValue condition={condition} label={label} onChange={onChange} />
      );
    default:
      return (
        <div className="min-w-0">
          <Label htmlFor={`value-${index}`} className="sr-only">
            {label}
          </Label>
          <Input
            id={`value-${index}`}
            value={condition.value}
            onChange={(e) => onChange(e.target.value)}
            className="h-[34px] font-mono text-xs"
          />
        </div>
      );
  }
}

const EFFECTS = ["read", "write", "destructive", "financial"] as const;

function EffectValue({
  value,
  label,
  onChange,
}: {
  value: string;
  label: string;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const known = (EFFECTS as readonly string[]).includes(value);
  return (
    <Select value={value || undefined} onValueChange={onChange}>
      <SelectTrigger
        className="!h-[34px] w-full min-w-0 font-mono text-xs"
        aria-label={label}
      >
        <SelectValue placeholder={t("policies.pickEffectValue")} />
      </SelectTrigger>
      <SelectContent>
        {value !== "" && !known && (
          // A stored value this version does not know survives the editor:
          // dropping it on open is how an unrelated save rewrites a policy.
          <SelectItem value={value} className="font-mono text-xs">
            {value}
          </SelectItem>
        )}
        {EFFECTS.map((effect) => (
          <SelectItem key={effect} value={effect} className="font-mono text-xs">
            {effect}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function AgentValue({
  condition,
  label,
  onChange,
}: {
  condition: PolicyCondition;
  label: string;
  onChange: (value: string) => void;
}) {
  const agents = useAgents();
  const suggestions = (agents.data?.items ?? []).map((agent) => ({
    value: agent.agentId,
    hint: agent.name,
  }));
  return (
    <SuggestValue
      condition={condition}
      label={label}
      suggestions={dedupe(suggestions)}
      onChange={onChange}
    />
  );
}

function ScopeValue({
  condition,
  label,
  onChange,
}: {
  condition: PolicyCondition;
  label: string;
  onChange: (value: string) => void;
}) {
  const scopes = useScopes();
  const declared = scopes.data?.items ?? [];
  const suggestions =
    condition.field === "scope.company"
      ? declared.map((s) => ({ value: s.company }))
      : declared.map((s) => ({ value: s.area, hint: s.company }));
  return (
    <SuggestValue
      condition={condition}
      label={label}
      suggestions={dedupe(suggestions)}
      onChange={onChange}
    />
  );
}

type Suggestion = { value: string; hint?: string };

function dedupe(suggestions: Suggestion[]): Suggestion[] {
  const seen = new Set<string>();
  return suggestions.filter((s) => {
    if (s.value === "" || seen.has(s.value)) return false;
    seen.add(s.value);
    return true;
  });
}

/**
 * A combobox whose suggestions help and never refuse.
 *
 * Picking fills the value; typing anything else is just as valid, which is
 * what keeps a policy able to name an agent that is not deployed yet. Under
 * the `in` operator the value is a comma-separated set, so picking appends to
 * the set instead of replacing it — and picking a member again removes it,
 * which is how a set control is expected to behave.
 */
function SuggestValue({
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

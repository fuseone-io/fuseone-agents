import { Check, ChevronsUpDown, X } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
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
import { useAgents } from "@/features/agents/api";
import { Section } from "@/features/policies/section";
import { useScopes } from "@/features/scope/api";
import type { PolicyInput } from "@/lib/api/client";
import { cn } from "@/lib/utils";

const REACHES = [
  {
    value: "installation",
    label: "policies.reachInstallation",
    note: "policies.reachInstallationNote",
  },
  {
    value: "scopes",
    label: "policies.reachScopes",
    note: "policies.reachScopesNote",
  },
  {
    value: "agents",
    label: "policies.reachAgents",
    note: "policies.reachAgentsNote",
  },
] as const;

/**
 * Who the rule reaches, before any condition is read.
 *
 * This section writes the structural half the contract always had and the
 * console never offered: `reach` with its `agents` or `scopes`. The Gate
 * matches these before conditions, they are visible in the listing, and the
 * server refuses an empty set — a policy that says "these agents" and names
 * none governs nothing and must not pretend to.
 *
 * The agent set is suggestions plus free text, like every list here: a policy
 * may name an agent that is not deployed yet.
 */
export function ReachSection({
  draft,
  patch,
}: {
  draft: PolicyInput;
  patch: (over: Partial<PolicyInput>) => void;
}) {
  const { t } = useTranslation();
  const reach = draft.reach ?? "installation";

  return (
    <Section title={t("policies.reach")} hint={t("policies.reachHint")}>
      <fieldset>
        <legend className="sr-only">{t("policies.reach")}</legend>
        <div className="grid min-w-0 gap-2 sm:grid-cols-[repeat(3,minmax(0,1fr))]">
          {REACHES.map((choice) => (
            <button
              key={choice.value}
              type="button"
              role="radio"
              aria-checked={reach === choice.value}
              onClick={() => patch({ reach: choice.value })}
              className={cn(
                "flex min-w-0 flex-col gap-0.5 rounded-lg border p-3 text-left",
                reach === choice.value
                  ? "border-primary bg-surface-accent"
                  : "border-border",
              )}
            >
              <span className="break-words text-sm font-medium">
                {t(choice.label)}
              </span>
              <span className="break-words text-xs text-muted-foreground">
                {t(choice.note)}
              </span>
            </button>
          ))}
        </div>
      </fieldset>

      {reach === "agents" && (
        <AgentSet
          chosen={draft.agents ?? []}
          onChange={(agents) => patch({ agents })}
        />
      )}
      {reach === "scopes" && (
        <ScopeSet
          chosen={draft.scopes ?? []}
          onChange={(scopes) => patch({ scopes })}
        />
      )}
    </Section>
  );
}

function AgentSet({
  chosen,
  onChange,
}: {
  chosen: string[];
  onChange: (agents: string[]) => void;
}) {
  const { t } = useTranslation();
  const agents = useAgents();
  const suggestions = (agents.data?.items ?? []).map((agent) => ({
    value: agent.agentId,
    hint: agent.name,
  }));
  return (
    <SetPicker
      label={t("policies.reachAgentsPick")}
      empty={t("policies.reachAgentsEmpty")}
      chosen={chosen}
      suggestions={suggestions}
      onChange={onChange}
    />
  );
}

function ScopeSet({
  chosen,
  onChange,
}: {
  chosen: { company: string; area: string }[];
  onChange: (scopes: { company: string; area: string }[]) => void;
}) {
  const { t } = useTranslation();
  const scopes = useScopes();
  const suggestions = (scopes.data?.items ?? []).map((s) => ({
    value: `${s.company}/${s.area}`,
    hint: "",
  }));
  const values = chosen.map((s) => `${s.company}/${s.area}`);
  return (
    <SetPicker
      label={t("policies.reachScopesPick")}
      empty={t("policies.reachScopesEmpty")}
      chosen={values}
      suggestions={suggestions}
      onChange={(next) =>
        onChange(
          next
            .map((one) => {
              const [company = "", ...rest] = one.split("/");
              return { company, area: rest.join("/") };
            })
            .filter((s) => s.company !== "" && s.area !== ""),
        )
      }
    />
  );
}

/**
 * A set built from suggestions and typing, shown as removable chips.
 *
 * A member outside the suggestions is kept and rendered like any other: a
 * policy loaded from the store may name agents this listing cannot see, and a
 * save must not shrink the set behind the author's back.
 */
function SetPicker({
  label,
  empty,
  chosen,
  suggestions,
  onChange,
}: {
  label: string;
  empty: string;
  chosen: string[];
  suggestions: { value: string; hint: string }[];
  onChange: (next: string[]) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState("");

  const toggle = (value: string) =>
    onChange(
      chosen.includes(value)
        ? chosen.filter((one) => one !== value)
        : [...chosen, value],
    );

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <div className="flex min-w-0 items-center gap-2">
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="sm"
              role="combobox"
              aria-expanded={open}
              aria-label={label}
              className="h-8"
            >
              {label}
              <ChevronsUpDown className="size-3.5 opacity-60" aria-hidden />
            </Button>
          </PopoverTrigger>
          <PopoverContent align="start" className="w-80 p-0">
            <Command>
              <CommandInput
                value={typed}
                onValueChange={setTyped}
                placeholder={t("policies.suggestFilter")}
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
                        toggle(typed.trim());
                        setTyped("");
                      }}
                    >
                      {t("policies.useTyped", { value: typed.trim() })}
                    </Button>
                  ) : (
                    <p className="text-xs text-muted-foreground">
                      {t("policies.suggestTypeFreely")}
                    </p>
                  )}
                </CommandEmpty>
                <CommandGroup>
                  {suggestions.map((suggestion) => (
                    <CommandItem
                      key={suggestion.value}
                      value={suggestion.value}
                      className="font-mono text-xs"
                      onSelect={() => toggle(suggestion.value)}
                    >
                      <Check
                        className={cn(
                          "size-3.5",
                          chosen.includes(suggestion.value)
                            ? "opacity-100"
                            : "opacity-0",
                        )}
                      />
                      <span className="truncate">{suggestion.value}</span>
                      {suggestion.hint && (
                        <span className="ml-auto truncate pl-2 text-muted-foreground">
                          {suggestion.hint}
                        </span>
                      )}
                    </CommandItem>
                  ))}
                </CommandGroup>
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>
        {chosen.length === 0 && <p className="text-xs text-warning">{empty}</p>}
      </div>
      {chosen.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {chosen.map((one) => (
            <Badge key={one} variant="outline" className="gap-1 font-mono">
              {one}
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="size-4"
                aria-label={t("policies.removeFromSet", { value: one })}
                onClick={() => toggle(one)}
              >
                <X className="size-3" aria-hidden />
              </Button>
            </Badge>
          ))}
        </div>
      )}
    </div>
  );
}

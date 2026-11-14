import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useTools } from "@/features/admin/api";
import { useAgents } from "@/features/agents/api";
import { SuggestValue } from "@/features/policies/suggest-value";
import { dedupe } from "@/features/policies/suggestion";
import { useScopes } from "@/features/scope/api";
import type { PolicyCondition } from "@/lib/api/client";

/**
 * The value of a condition, offered rather than dictated.
 *
 * Which control appears is decided by the row's field: an agent id comes from
 * the agents the platform already lists, an area from the declared scopes, a
 * tool effect from the Gate's own enum, a tool id from the governed catalogue.
 * Everything the platform cannot know — labels, argument paths — stays the
 * mono input it always was.
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
    case "tool.id":
      return (
        <ToolValue condition={condition} label={label} onChange={onChange} />
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

/**
 * The governed tool ids, as the policy's own Tool field offers them. Asked
 * for only once the suggestions open: the page is visible to roles the tool
 * listing refuses, and a refusal must leave the plain input it always was.
 * Typing stays valid, because a glob such as `cloudflare.*` is a rule.
 */
function ToolValue({
  condition,
  label,
  onChange,
}: {
  condition: PolicyCondition;
  label: string;
  onChange: (value: string) => void;
}) {
  const [asked, setAsked] = useState(false);
  const tools = useTools(asked);
  const suggestions = (tools.data?.items ?? []).map((tool) => ({
    value: tool.toolId,
    hint: tool.effect ?? "",
  }));
  return (
    <SuggestValue
      condition={condition}
      label={label}
      suggestions={dedupe(suggestions)}
      onChange={onChange}
      onOpen={() => setAsked(true)}
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

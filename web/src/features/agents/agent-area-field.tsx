import { useTranslation } from "react-i18next";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { agentRequirementMarked } from "@/features/agents/agent-required";
import { Labelled } from "@/features/policies/section";
import { useScopes } from "@/features/scope/api";

/**
 * The area an agent is filed under, chosen from the declared ones.
 *
 * Typed free, this field is how `financeiro` and `Financeiro` became two areas
 * that never meet. Choosing removes that, but it introduces a hazard of its
 * own: an agent already filed under an area nobody declared — or one since
 * withdrawn — must keep it. A select that silently dropped the value would
 * refile the agent somewhere else on the next publish, quietly moving it out
 * from under the ceiling and the policies that governed it.
 */
export function AgentAreaField({
  company,
  area,
  onChange,
}: {
  company: string;
  area: string;
  onChange: (scope: { company: string; area: string }) => void;
}) {
  const { t } = useTranslation();
  const { data } = useScopes();
  const declared = data?.items ?? [];
  const value = company !== "" && area !== "" ? scopeValue(company, area) : "";
  const known = declared.some((s) => s.company === company && s.area === area);

  return (
    <Labelled
      label={t("admin.area")}
      htmlFor="agent-area"
      required={agentRequirementMarked("area")}
    >
      <Select
        value={value || undefined}
        onValueChange={(next) => {
          onChange(splitScope(next));
        }}
      >
        <SelectTrigger
          id="agent-area"
          className="w-full font-mono"
          aria-required={agentRequirementMarked("area")}
        >
          <SelectValue placeholder={t("agents.choose")} />
        </SelectTrigger>
        <SelectContent>
          {company !== "" && area !== "" && !known && (
            <SelectItem value={value} className="font-mono">
              {t("agents.areaUndeclared", { area: value })}
            </SelectItem>
          )}
          {declared.map((s) => (
            <SelectItem
              key={`${s.company}/${s.area}`}
              value={scopeValue(s.company, s.area)}
              className="font-mono"
            >
              {s.company}/{s.area}
              {s.label && s.label !== s.area ? ` · ${s.label}` : ""}
            </SelectItem>
          ))}
          {declared.length === 0 && area === "" && (
            <p className="px-2 py-3 text-xs text-muted-foreground">
              {t("agents.noAreasDeclared")}
            </p>
          )}
        </SelectContent>
      </Select>
    </Labelled>
  );
}

function scopeValue(company: string, area: string): string {
  return `${company}/${area}`;
}

function splitScope(value: string): { company: string; area: string } {
  const parts = value.split("/", 2);
  return { company: parts[0] ?? "", area: parts[1] ?? "" };
}

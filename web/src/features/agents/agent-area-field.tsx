import { useTranslation } from "react-i18next";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
  area,
  onChange,
}: {
  area: string;
  onChange: (area: string) => void;
}) {
  const { t } = useTranslation();
  const { data } = useScopes();
  const declared = data?.items ?? [];
  const known = declared.some((s) => s.area === area);

  return (
    <Labelled label="Área" htmlFor="agent-area">
      <Select value={area || undefined} onValueChange={onChange}>
        <SelectTrigger id="agent-area" className="w-full font-mono">
          <SelectValue placeholder="Escolha" />
        </SelectTrigger>
        <SelectContent>
          {area !== "" && !known && (
            <SelectItem value={area} className="font-mono">
              {area} · não declarada
            </SelectItem>
          )}
          {declared.map((s) => (
            <SelectItem
              key={`${s.company}/${s.area}`}
              value={s.area}
              className="font-mono"
            >
              {s.area}
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

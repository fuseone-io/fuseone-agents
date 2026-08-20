import { useTranslation } from "react-i18next";
import { Plus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ConditionField } from "@/features/policies/condition-field";
import type { PolicyCondition } from "@/lib/api/client";

/** What a rule can read. Short on purpose: every field here has to be
 *  renderable in a sentence the author checks by eye. */
const FIELDS = [
  { value: "tool.id", label: "policies.fieldTool" },
  { value: "tool.effect", label: "policies.toolEffect" },
  { value: "data.taint", label: "policies.dataLabel" },
  { value: "agent.id", label: "policies.fieldAgent" },
  { value: "scope.area", label: "policies.fieldAreaScope" },
  { value: "args.rows", label: "policies.fieldRows" },
];

const OPERATORS = [
  { value: "eq", label: "policies.isEqual" },
  { value: "ne", label: "policies.isNot" },
  { value: "gt", label: "policies.greaterThan" },
  { value: "lt", label: "policies.lessThan" },
  { value: "contains", label: "policies.contains" },
  { value: "in", label: "policies.isIn" },
];

/**
 * The rule, one clause per row.
 *
 * Every clause must hold — there is no `or`, because a rule stops being
 * readable at the first one and two policies say the same thing with two
 * owners. The conjunction column says so on every row rather than in a note
 * somebody scrolls past.
 */
export function ConditionBuilder({
  conditions,
  onChange,
}: {
  conditions: PolicyCondition[];
  onChange: (conditions: PolicyCondition[]) => void;
}) {
  const { t } = useTranslation();
  const update = (index: number, patch: Partial<PolicyCondition>) =>
    onChange(conditions.map((c, i) => (i === index ? { ...c, ...patch } : c)));

  return (
    <div className="flex flex-col gap-2">
      {conditions.length === 0 && (
        <p className="text-xs text-muted-foreground">
          {t("policies.noCondition")}
        </p>
      )}

      {conditions.map((condition, index) => (
        <div
          key={index}
          className="grid min-w-0 grid-cols-1 gap-2 rounded-lg border border-border p-2 md:grid-cols-[42px_minmax(0,1fr)_120px_minmax(0,1fr)_32px] md:items-center md:border-0 md:p-0"
        >
          <span className="min-w-0 text-xs text-muted-foreground md:text-right">
            {index === 0 ? t("policies.when") : t("policies.and")}
          </span>

          <ConditionField
            label={t("policies.field")}
            value={condition.field}
            options={FIELDS}
            onChange={(field) => update(index, { field })}
          />
          <ConditionField
            label={t("policies.operator")}
            value={condition.op}
            options={OPERATORS}
            onChange={(op) =>
              update(index, { op: op as PolicyCondition["op"] })
            }
          />

          <div className="min-w-0">
            <Label htmlFor={`value-${index}`} className="sr-only">
              {t("policies.conditionValue", { n: index + 1 })}
            </Label>
            <Input
              id={`value-${index}`}
              value={condition.value}
              onChange={(e) => update(index, { value: e.target.value })}
              className="h-[34px] font-mono text-xs"
            />
          </div>

          <Button
            variant="ghost"
            size="icon"
            className="size-8 text-muted-foreground"
            onClick={() => onChange(conditions.filter((_, i) => i !== index))}
          >
            <X className="size-4" aria-hidden />
            <span className="sr-only">
              {t("policies.removeCondition", { n: index + 1 })}
            </span>
          </Button>
        </div>
      ))}

      <Button
        variant="outline"
        size="sm"
        className="h-8 self-start"
        onClick={() =>
          onChange([...conditions, { field: "tool.id", op: "eq", value: "" }])
        }
      >
        <Plus className="size-4" aria-hidden />
        {t("policies.addCondition")}
      </Button>
    </div>
  );
}

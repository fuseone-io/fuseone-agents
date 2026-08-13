import { useTranslation } from "react-i18next";
import { Plus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
          className="grid grid-cols-[42px_1fr_120px_1fr_32px] items-center gap-2"
        >
          <span className="text-right text-xs text-muted-foreground">
            {index === 0 ? "policies.when" : "e"}
          </span>

          <Field
            label={t("policies.field")}
            value={condition.field}
            options={FIELDS}
            onChange={(field) => update(index, { field })}
          />
          <Field
            label={t("policies.operator")}
            value={condition.op}
            options={OPERATORS}
            onChange={(op) =>
              update(index, { op: op as PolicyCondition["op"] })
            }
          />

          <div>
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

function Field({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger
        className="!h-[34px] w-full font-mono text-xs"
        aria-label={label}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem
            key={option.value}
            value={option.value}
            className="font-mono text-xs"
          >
            {t(option.label)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

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
  { value: "tool.id", label: "ferramenta" },
  { value: "tool.effect", label: "efeito da ferramenta" },
  { value: "data.taint", label: "marcação do dado" },
  { value: "agent.id", label: "agente" },
  { value: "scope.area", label: "área" },
  { value: "args.rows", label: "args.rows" },
];

const OPERATORS = [
  { value: "eq", label: "é" },
  { value: "ne", label: "não é" },
  { value: "gt", label: "maior que" },
  { value: "lt", label: "menor que" },
  { value: "contains", label: "contém" },
  { value: "in", label: "está em" },
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
  const update = (index: number, patch: Partial<PolicyCondition>) =>
    onChange(conditions.map((c, i) => (i === index ? { ...c, ...patch } : c)));

  return (
    <div className="flex flex-col gap-2">
      {conditions.length === 0 && (
        <p className="text-xs text-muted-foreground">
          Sem condição, a regra vale para tudo que o escopo cobre. É assim que
          se escreve “negar toda escrita em crm”.
        </p>
      )}

      {conditions.map((condition, index) => (
        <div
          key={index}
          className="grid grid-cols-[42px_1fr_120px_1fr_32px] items-center gap-2"
        >
          <span className="text-right text-xs text-muted-foreground">
            {index === 0 ? "quando" : "e"}
          </span>

          <Field
            label="Campo"
            value={condition.field}
            options={FIELDS}
            onChange={(field) => update(index, { field })}
          />
          <Field
            label="Operador"
            value={condition.op}
            options={OPERATORS}
            onChange={(op) => update(index, { op: op as PolicyCondition["op"] })}
          />

          <div>
            <Label htmlFor={`value-${index}`} className="sr-only">
              Valor da condição {index + 1}
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
            <span className="sr-only">Remover a condição {index + 1}</span>
          </Button>
        </div>
      ))}

      <Button
        variant="outline"
        size="sm"
        className="h-8 self-start"
        onClick={() => onChange([...conditions, { field: "tool.id", op: "eq", value: "" }])}
      >
        <Plus className="size-4" aria-hidden />
        Adicionar condição
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
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className="!h-[34px] w-full font-mono text-xs" aria-label={label}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value} className="font-mono text-xs">
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

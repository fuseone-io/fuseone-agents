import { Plus, Trash2 } from "lucide-react";
import { useFieldArray, useFormContext } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import type { EvidenceInstanceValues } from "@/features/integrations/connectors/evidence-instance-model";
import { EvidenceTextField } from "@/features/integrations/connectors/evidence-text-field";

const kinds = {
  filters: {
    second: "value",
    title: "connectors.evidenceQueryFilters",
    hint: "connectors.evidenceQueryFiltersHint",
    add: "connectors.evidenceAddFilter",
    remove: "connectors.evidenceRemoveFilter",
    name: "connectors.evidenceFilterName",
    secondLabel: "connectors.evidenceFilterValue",
  },
  parameters: {
    second: "pattern",
    title: "connectors.evidenceQueryParameters",
    hint: "connectors.evidenceQueryParametersHint",
    add: "connectors.evidenceAddParameter",
    remove: "connectors.evidenceRemoveParameter",
    name: "connectors.evidenceParameterName",
    secondLabel: "connectors.evidenceParameterPattern",
  },
} as const;

/**
 * Rows rather than lines of `name = value`. The value of a filter and the
 * pattern of a parameter are compared byte for byte by the server, so spaces
 * and new lines in them mean something, and a format with a separator to parse
 * is a format that loses them.
 */
export function EvidencePairsEditor({
  query,
  kind,
}: {
  query: number;
  kind: keyof typeof kinds;
}) {
  // The two lists differ only in the name of their second column, which the
  // form's path types cannot follow through a variable. These casts state that
  // once, here, rather than duplicating the component per list.
  const { t } = useTranslation();
  const { control } = useFormContext<EvidenceInstanceValues>();
  const copy = kinds[kind];
  const rows = useFieldArray({
    control,
    name: `queries.${query}.${kind}` as `queries.${number}.filters`,
  });
  const secondOf = (row: number) =>
    `queries.${query}.${kind}.${row}.${copy.second}` as `queries.${number}.filters.${number}.value`;
  return (
    <div className="grid gap-2">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h5 className="text-sm font-medium">{t(copy.title)}</h5>
          <p className="text-xs text-muted-foreground">{t(copy.hint)}</p>
        </div>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() =>
            rows.append({ name: "", [copy.second]: "" } as {
              name: string;
              value: string;
            })
          }
        >
          <Plus className="size-4" aria-hidden />
          {t(copy.add)}
        </Button>
      </div>
      {rows.fields.map((row, index) => (
        <div
          key={row.id}
          className="grid items-start gap-2 sm:grid-cols-[1fr_2fr_auto]"
        >
          <EvidenceTextField
            field={{
              control,
              name: `queries.${query}.${kind}.${index}.name` as `queries.${number}.filters.${number}.name`,
              label: t(copy.name),
            }}
          />
          <EvidenceTextField
            field={{
              control,
              name: secondOf(index),
              label: t(copy.secondLabel),
              type: "lines",
            }}
          />
          <Button
            type="button"
            size="icon"
            variant="ghost"
            className="sm:mt-6"
            onClick={() => rows.remove(index)}
            aria-label={t(copy.remove, { number: index + 1 })}
          >
            <Trash2 className="size-4" aria-hidden />
          </Button>
        </div>
      ))}
    </div>
  );
}

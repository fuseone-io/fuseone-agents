import { Plus, Trash2 } from "lucide-react";
import { useFieldArray, useFormContext } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from "@/components/ui/form";
import { Switch } from "@/components/ui/switch";
import {
  emptyEvidenceQuery,
  type EvidenceInstanceValues,
} from "@/features/integrations/connectors/evidence-instance-model";
import { EvidenceTextField } from "@/features/integrations/connectors/evidence-text-field";

export function EvidenceQueryEditor() {
  const { t } = useTranslation();
  const { control } = useFormContext<EvidenceInstanceValues>();
  const queries = useFieldArray({ control, name: "queries" });
  return (
    <section className="grid gap-3">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-medium">
            {t("connectors.evidenceQueries")}
          </h3>
          <p className="text-xs text-muted-foreground">
            {t("connectors.evidenceQueriesHint")}
          </p>
        </div>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => queries.append(emptyEvidenceQuery())}
        >
          <Plus className="size-4" aria-hidden />
          {t("connectors.evidenceAddQuery")}
        </Button>
      </div>
      {queries.fields.map((query, index) => (
        <EvidenceQueryFields
          key={query.id}
          index={index}
          onRemove={() => queries.remove(index)}
        />
      ))}
    </section>
  );
}

function EvidenceQueryFields({
  index,
  onRemove,
}: {
  index: number;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const { control, watch } = useFormContext<EvidenceInstanceValues>();
  const id = watch(`queries.${index}.id`);
  const at = `queries.${index}` as const;
  return (
    <div className="grid gap-4 rounded-md border p-4">
      <div className="flex items-center gap-3">
        <h4 className="min-w-0 flex-1 truncate text-sm font-medium">
          {id || t("connectors.evidenceQueryUntitled", { number: index + 1 })}
        </h4>
        <Button
          type="button"
          size="icon"
          variant="ghost"
          onClick={onRemove}
          aria-label={t("connectors.evidenceRemoveQuery", {
            number: index + 1,
          })}
        >
          <Trash2 className="size-4" aria-hidden />
        </Button>
      </div>
      <div className="grid items-start gap-3 sm:grid-cols-2">
        <EvidenceTextField
          field={{
            control,
            name: `${at}.id`,
            label: t("connectors.evidenceQueryId"),
          }}
        />
        <EvidenceTextField
          field={{
            control,
            name: `${at}.path`,
            label: t("connectors.evidenceQueryPath"),
          }}
        />
      </div>
      <EvidenceTextField
        field={{
          control,
          name: `${at}.description`,
          label: t("connectors.evidenceQueryDescription"),
          description: t("connectors.evidenceQueryDescriptionHint"),
        }}
      />
      <EvidenceTextField
        field={{
          control,
          name: `${at}.fields`,
          label: t("connectors.evidenceQueryFields"),
          description: t("connectors.evidenceQueryFieldsHint"),
          type: "lines",
        }}
      />
      <div className="grid items-start gap-3 sm:grid-cols-2">
        <EvidenceTextField
          field={{
            control,
            name: `${at}.idField`,
            label: t("connectors.evidenceQueryIdField"),
          }}
        />
        <EvidenceTextField
          field={{
            control,
            name: `${at}.timeField`,
            label: t("connectors.evidenceQueryTimeField"),
          }}
        />
        <EvidenceTextField
          field={{
            control,
            name: `${at}.filters`,
            label: t("connectors.evidenceQueryFilters"),
            description: t("connectors.evidenceQueryFiltersHint"),
            type: "lines",
          }}
        />
        <EvidenceTextField
          field={{
            control,
            name: `${at}.parameters`,
            label: t("connectors.evidenceQueryParameters"),
            description: t("connectors.evidenceQueryParametersHint"),
            type: "lines",
          }}
        />
      </div>
      <FormField
        control={control}
        name={`${at}.windowless`}
        render={({ field }) => (
          <FormItem className="flex items-center justify-between rounded-lg border p-3">
            <FormLabel className="m-0">
              {t("connectors.evidenceQueryWindowless")}
            </FormLabel>
            <FormControl>
              <Switch checked={field.value} onCheckedChange={field.onChange} />
            </FormControl>
          </FormItem>
        )}
      />
    </div>
  );
}

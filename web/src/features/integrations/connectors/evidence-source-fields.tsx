import { useFormContext } from "react-hook-form";
import { useTranslation } from "react-i18next";
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  evidenceControlEligible,
  type EvidenceInstanceValues,
} from "@/features/integrations/connectors/evidence-instance-model";
import { EvidenceTextField } from "@/features/integrations/connectors/evidence-text-field";

export function EvidenceSourceFields() {
  const { t } = useTranslation();
  const { control, watch } = useFormContext<EvidenceInstanceValues>();
  const eligible = watch("queries").filter(evidenceControlEligible);
  const text = (
    name: keyof EvidenceInstanceValues,
    key: string,
    described = false,
  ) => (
    <EvidenceTextField
      field={{
        control,
        name,
        label: t(`connectors.${key}`),
        description: described ? t(`connectors.${key}Hint`) : undefined,
      }}
    />
  );
  return (
    <section className="grid gap-4">
      <h3 className="text-sm font-medium">
        {t("connectors.evidenceSourceSection")}
      </h3>
      {text("baseUrl", "evidenceBaseUrl")}
      <div className="grid items-start gap-3 sm:grid-cols-2">
        {text("authHeader", "evidenceAuthHeader")}
        {text("authPrefix", "evidenceAuthPrefix", true)}
        {text("rows", "evidenceRows", true)}
        <EvidenceTextField
          field={{
            control,
            name: "maxEvents",
            label: t("connectors.evidenceMaxEvents"),
            description: t("connectors.evidenceMaxEventsHint"),
            type: "number",
          }}
        />
        {text("refusalField", "evidenceRefusalField", true)}
        {text("refusalValue", "evidenceRefusalValue")}
        {text("gapField", "evidenceGapField", true)}
        {text("gapValue", "evidenceGapValue")}
      </div>
      <FormField
        control={control}
        name="control"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("connectors.evidenceControl")}</FormLabel>
            {/* Radix reports "" when the chosen item leaves the list, which
                happens for a moment whenever a declaration replaces the queries.
                Taking it would blank a control that is about to be valid. */}
            <Select
              value={field.value}
              onValueChange={(next) => next && field.onChange(next)}
            >
              <FormControl>
                <SelectTrigger className="w-full font-mono">
                  <SelectValue
                    placeholder={t("connectors.evidenceControlPlaceholder")}
                  />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                {eligible.map((query) => (
                  <SelectItem key={query.id} value={query.id}>
                    {query.id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FormDescription>
              {t("connectors.evidenceControlHint")}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </section>
  );
}

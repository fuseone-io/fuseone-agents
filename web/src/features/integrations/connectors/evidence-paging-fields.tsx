import { useFormContext, type FieldPath } from "react-hook-form";
import { useTranslation } from "react-i18next";
import {
  FormControl,
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
import type { EvidenceInstanceValues } from "@/features/integrations/connectors/evidence-instance-model";
import { EvidenceTextField } from "@/features/integrations/connectors/evidence-text-field";

export function EvidencePagingFields() {
  const { t } = useTranslation();
  const { control, watch } = useFormContext<EvidenceInstanceValues>();
  const paging = watch("pagingKind");
  const window = watch("windowKind");
  const text = (name: FieldPath<EvidenceInstanceValues>, key: string) => (
    <EvidenceTextField
      field={{ control, name, label: t(`connectors.${key}`) }}
    />
  );
  return (
    <section className="grid gap-4">
      <h3 className="text-sm font-medium">
        {t("connectors.evidencePagingSection")}
      </h3>
      <div className="grid items-start gap-3 sm:grid-cols-2">
        <Choice
          name="pagingKind"
          label={t("connectors.evidencePagingKind")}
          options={[
            ["offset", t("connectors.evidencePagingOffset")],
            ["cursor", t("connectors.evidencePagingCursor")],
          ]}
        />
        {text("limitParam", "evidenceLimitParam")}
        <EvidenceTextField
          field={{
            control,
            name: "pageSize",
            label: t("connectors.evidencePageSize"),
            type: "number",
          }}
        />
        {paging === "offset" && text("offsetParam", "evidenceOffsetParam")}
        {paging === "cursor" && text("cursorParam", "evidenceCursorParam")}
        {paging === "cursor" && text("nextCursor", "evidenceNextCursor")}
      </div>
      <div className="grid items-start gap-3 sm:grid-cols-2">
        <Choice
          name="windowKind"
          label={t("connectors.evidenceWindowKind")}
          options={[
            ["period", t("connectors.evidenceWindowPeriod")],
            ["range", t("connectors.evidenceWindowRange")],
          ]}
        />
        {window === "period" && text("periodParam", "evidencePeriodParam")}
        {window === "period" && (
          <EvidenceTextField
            field={{
              control,
              name: "periods",
              label: t("connectors.evidencePeriods"),
              description: t("connectors.evidencePeriodsHint"),
            }}
          />
        )}
        {window === "range" && text("startParam", "evidenceStartParam")}
        {window === "range" && text("endParam", "evidenceEndParam")}
        {window === "range" && (
          <Choice
            name="format"
            label={t("connectors.evidenceFormat")}
            options={[
              ["iso", t("connectors.evidenceFormatIso")],
              ["epoch", t("connectors.evidenceFormatEpoch")],
            ]}
          />
        )}
      </div>
    </section>
  );
}

function Choice({
  name,
  label,
  options,
}: {
  name: "pagingKind" | "windowKind" | "format";
  label: string;
  options: [string, string][];
}) {
  const { control } = useFormContext<EvidenceInstanceValues>();
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <Select value={field.value} onValueChange={field.onChange}>
            <FormControl>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
            </FormControl>
            <SelectContent>
              {options.map(([value, text]) => (
                <SelectItem key={value} value={value}>
                  {text}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

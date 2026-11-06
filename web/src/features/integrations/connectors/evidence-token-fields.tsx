import { useFormContext } from "react-hook-form";
import { useTranslation } from "react-i18next";
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from "@/components/ui/form";
import { Switch } from "@/components/ui/switch";
import type { EvidenceInstanceValues } from "@/features/integrations/connectors/evidence-instance-model";
import { EvidenceTextField } from "@/features/integrations/connectors/evidence-text-field";

export function EvidenceTokenFields({ hasToken }: { hasToken: boolean }) {
  const { t } = useTranslation();
  const { control } = useFormContext<EvidenceInstanceValues>();
  return (
    <>
      <EvidenceTextField
        field={{
          control,
          name: "token",
          label: t("connectors.evidenceToken"),
          description: hasToken
            ? t("connectors.tokenKept")
            : t("connectors.evidenceTokenHint"),
          type: "password",
        }}
      />
      {hasToken && (
        <FormField
          control={control}
          name="clearToken"
          render={({ field }) => (
            <FormItem className="flex items-center justify-between rounded-lg border p-3">
              <FormLabel className="m-0">
                {t("connectors.clearToken")}
              </FormLabel>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              </FormControl>
            </FormItem>
          )}
        />
      )}
    </>
  );
}
